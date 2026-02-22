package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
)

// EmbeddedServiceWrapper built-in service wrapper
type EmbeddedServiceWrapper struct {
	serviceName string
	config      ServiceConfig
	process     *exec.Cmd
	isRunning   bool
	logFile   	*os.File
}

// NewEmbeddedServiceWrapper creates a built-in service wrapper
func NewEmbeddedServiceWrapper(serviceName string, config ServiceConfig) *EmbeddedServiceWrapper {
	return &EmbeddedServiceWrapper{
		serviceName: serviceName,
		config:      config,
		isRunning:   false,
	}
}

// Execute implements the Windows service interface
func (esw *EmbeddedServiceWrapper) Execute(args []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	log.Printf("EmbeddedServiceWrapper starting service: %s", esw.serviceName)

	s <- svc.Status{State: svc.StartPending}

	err := esw.startTargetProcess()
	if err != nil {
		log.Printf("Failed to start target process: %v", err)
		s <- svc.Status{State: svc.Stopped}
		return false, 1
	}

	s <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	log.Printf("Service started, target process PID: %d", esw.process.Process.Pid)

	go esw.monitorTargetProcess()

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Stop, svc.Shutdown:
				log.Printf("Service received stop signal: %s", esw.serviceName)
				s <- svc.Status{State: svc.StopPending}
				esw.stopTargetProcess()
				s <- svc.Status{State: svc.Stopped}
				return false, 0
			case svc.Interrogate:
				s <- c.CurrentStatus
			default:
				log.Printf("Service received unknown command: %v", c.Cmd)
			}
		default:
			if !esw.isRunning {
				log.Printf("Target process exited, stopping service: %s", esw.serviceName)
				s <- svc.Status{State: svc.Stopped}
				return false, 0
			}
			time.Sleep(1 * time.Second)
		}
	}
}

// startTargetProcess starts the target program
func (esw *EmbeddedServiceWrapper) startTargetProcess() error {
	var args []string
	if esw.config.Args != "" {
		args = strings.Fields(esw.config.Args)
	}

	esw.process = exec.Command(esw.config.ExePath, args...)

	workingDir := esw.config.WorkingDir
	if workingDir == "" {
		workingDir = filepath.Dir(esw.config.ExePath)
	}
	esw.process.Dir = workingDir

	esw.process.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

    // ---- NEW: Set up log redirection ----
    if esw.config.LogPath != "" {
        // Ensure log directory exists
        logDir := filepath.Dir(esw.config.LogPath)
        if err := os.MkdirAll(logDir, 0755); err != nil {
            return fmt.Errorf("failed to create log directory: %w", err)
        }
        // Open log file (append, create if missing)
        logFile, err := os.OpenFile(esw.config.LogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
        if err != nil {
            return fmt.Errorf("failed to open log file: %w", err)
        }
        esw.process.Stdout = logFile
        esw.process.Stderr = logFile
        // Store the file so we can close it later
        esw.logFile = logFile
    } else {
        // Fallback: discard output (or log to Windows event log)
        esw.process.Stdout = nil
        esw.process.Stderr = nil
    }

	esw.process.SysProcAttr = &syscall.SysProcAttr{
        HideWindow: true, // still hide the target's window
    }

	err := esw.process.Start()
	if err != nil {
		return fmt.Errorf("failed to start target process: %v", err)
	}

	esw.isRunning = true
	log.Printf("Target process started: %s, PID: %d", esw.config.ExePath, esw.process.Process.Pid)
	return nil
}

// stopTargetProcess stops the target program
func (esw *EmbeddedServiceWrapper) stopTargetProcess() {
	if esw.process != nil && esw.isRunning {
		log.Printf("Stopping target process, PID: %d", esw.process.Process.Pid)

		esw.process.Process.Kill()

		esw.process.Wait()
		esw.isRunning = false
		log.Printf("Target process stopped")
	}
}

// monitorTargetProcess monitors the target process
func (esw *EmbeddedServiceWrapper) monitorTargetProcess() {
	if esw.process != nil {
		esw.process.Wait()
		esw.isRunning = false
		if esw.logFile != nil {
            esw.logFile.Close()
            esw.logFile = nil
        }
		log.Printf("Target process exited: %s", esw.config.ExePath)
	}
}

// RunAsWindowsService runs the program as a Windows service (built-in wrapper mode)
func RunAsWindowsService(serviceName string, config ServiceConfig) error {
	wrapper := NewEmbeddedServiceWrapper(serviceName, config)

	isService, err := svc.IsWindowsService()
	if err != nil {
		return fmt.Errorf("failed to check service status: %v", err)
	}

	if isService {
		log.Printf("Running as Windows service: %s", serviceName)
		err = svc.Run(serviceName, wrapper)
		if err != nil {
			return fmt.Errorf("service run failed: %v", err)
		}
	} else {
		log.Printf("Running in debug mode: %s", serviceName)
		err = debug.Run(serviceName, wrapper)
		if err != nil {
			return fmt.Errorf("debug run failed: %v", err)
		}
	}

	return nil
}

// IsServiceWrapperMode checks if running in service wrapper mode
func IsServiceWrapperMode() (bool, string) {
	args := os.Args
	if len(args) >= 3 && args[1] == "--service-wrapper" {
		return true, args[2] // return service name
	}
	return false, ""
}

// LoadServiceConfigFromRegistry loads service configuration from registry
func LoadServiceConfigFromRegistry(serviceName string) (*ServiceConfig, error) {
	keyPath := fmt.Sprintf(`SYSTEM\CurrentControlSet\Services\%s\Parameters`, serviceName)

	key, err := registry.OpenKey(registry.LOCAL_MACHINE, keyPath, registry.READ)
	if err != nil {
		return nil, fmt.Errorf("failed to open service configuration registry: %v", err)
	}
	defer key.Close()

	exePath, _, err := key.GetStringValue("ExePath")
	if err != nil {
		return nil, fmt.Errorf("failed to read ExePath: %v", err)
	}
	args, _, err := key.GetStringValue("Args")
	if err != nil {
		args = ""
	}
	workingDir, _, err := key.GetStringValue("WorkingDir")
	if err != nil {
		workingDir = ""
	}
	displayName, _, err := key.GetStringValue("DisplayName")
	if err != nil {
		displayName = serviceName
	}
	logPath, _, err := key.GetStringValue("StdoutLog")
	if err != nil {
		logPath = ""
	}

	return &ServiceConfig{
		Name:       displayName,
		ExePath:    exePath,
		Args:       args,
		WorkingDir: workingDir,
		LogPath:    logPath,
	}, nil
}
