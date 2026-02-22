package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Service represents a background service
type Service struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	ExePath    string    `json:"exePath"`
	Args       string    `json:"args"`
	WorkingDir string    `json:"workingDir"`
	Status     string    `json:"status"` // "running", "stopped", "error"
	PID        int       `json:"pid"`
	AutoStart  bool      `json:"autoStart"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// ServiceConfig is the configuration for creating a new service
type ServiceConfig struct {
	Name       string `json:"name"`
	ExePath    string `json:"exePath"`
	Args       string `json:"args"`
	WorkingDir string `json:"workingDir"`
	LogPath    string 
}

type ThemeData struct {
	Theme string `json:"theme"` // "light" or "dark"
}

type tailerInfo struct {
    cancel context.CancelFunc
    done   chan struct{}
}

type App struct {
	ctx                context.Context
	serviceManager     *WindowsServiceManager
	environmentManager *EnvironmentManager
	logTailers         map[string]*tailerInfo // serviceID -> tailer info
	logTailersLock     sync.Mutex
}

func NewApp() *App {
	return &App{
		serviceManager:     NewWindowsServiceManager(),
		environmentManager: NewEnvironmentManager(),
		logTailers:         make(map[string]*tailerInfo),
	}
}

// startup is called when the application starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.serviceManager.SetContext(ctx)
	a.serviceManager.loadServices()
}

// getThemeConfigPath returns the path to the theme config file
func (a *App) getThemeConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "Windows Service Manager.exe", "theme.json"), nil
}

// GetTheme returns the saved theme ("light" or "dark"), defaulting to "light"
func (a *App) GetTheme() string {
	path, err := a.getThemeConfigPath()
	if err != nil {
		return "light"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		// If file doesn't exist, return default
		return "light"
	}

	var themeData ThemeData
	if err := json.Unmarshal(data, &themeData); err != nil {
		return "light"
	}

	if themeData.Theme != "light" && themeData.Theme != "dark" {
		return "light"
	}
	return themeData.Theme
}

// SetTheme saves the theme preference
func (a *App) SetTheme(theme string) error {
	if theme != "light" && theme != "dark" {
		theme = "light" // fallback
	}

	path, err := a.getThemeConfigPath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	themeData := ThemeData{Theme: theme}
	data, err := json.MarshalIndent(themeData, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// GetServices returns the list of all services
func (a *App) GetServices() []*Service {
	services, err := a.serviceManager.GetServices()
	if err != nil {
		return []*Service{}
	}
	return services
}

// CreateService creates a new service
func (a *App) CreateService(config ServiceConfig) (*Service, error) {
	return a.serviceManager.CreateService(config)
}

// StartService starts a service
func (a *App) StartService(serviceID string) error {
	return a.serviceManager.StartService(serviceID)
}

// StopService stops a service
func (a *App) StopService(serviceID string) error {
	return a.serviceManager.StopService(serviceID)
}

// DeleteService deletes a service
func (a *App) DeleteService(serviceID string) error {
	// Stop any active log monitoring for this service
	a.StopMonitoringService(serviceID)
	return a.serviceManager.DeleteService(serviceID)
}

// StartMonitoringLog begins tailing the service's log file and emits lines to the frontend.
func (a *App) StartMonitoringService(serviceID string) error {
	a.logTailersLock.Lock()
	defer a.logTailersLock.Unlock()

	// If already monitoring, stop the previous tailer and start fresh.
	if info, exists := a.logTailers[serviceID]; exists {
		info.cancel()
		<-info.done // Wait for the old goroutine to exit
		delete(a.logTailers, serviceID)
	}

	logPath, _, err := a.serviceManager.GetServiceLogPath(serviceID)
	if err != nil {
		return fmt.Errorf("failed to get log path %s: %w", logPath, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	a.logTailers[serviceID] = &tailerInfo{
        cancel: cancel,
        done:   done,
    }

	go func() {
        defer close(done)
        a.tailLogFile(ctx, serviceID, logPath)
    }()
	return nil
}

// GetLogContent returns all current lines from the service's log file.
func (a *App) GetLogContent(serviceID string) ([]string, error) {
    logPath, _, err := a.serviceManager.GetServiceLogPath(serviceID)
    if err != nil {
        return nil, err
    }
    return a.readAllLines(logPath)
}

// readAllLines is a helper that reads a file and returns its lines.
func (a *App) readAllLines(path string) ([]string, error) {
    file, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer file.Close()

    var lines []string
    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        lines = append(lines, scanner.Text())
    }
    return lines, scanner.Err()
}

func (a *App) tailLogFile(ctx context.Context, serviceID, logPath string) {
    // Wait for file to exist (up to 10 seconds)
    for range 20 {
        if _, err := os.Stat(logPath); err == nil {
            break
        }
        time.Sleep(500 * time.Millisecond)
    }

    file, err := os.Open(logPath)
    if err != nil {
        runtime.LogErrorf(a.ctx, "Cannot open log file for %s: %v", serviceID, err)
        return
    }
    defer file.Close()

    // Seek to the end – we only want new lines from now on.
    if _, err := file.Seek(0, io.SeekEnd); err != nil {
        runtime.LogErrorf(a.ctx, "Seek error for %s: %v", serviceID, err)
        return
    }

    reader := bufio.NewReader(file)
    lineBuf := make([]byte, 0)

    for {
        select {
        case <-ctx.Done():
            return
        default:
            line, isPrefix, err := reader.ReadLine()
            if err != nil {
                if err != io.EOF {
                    runtime.LogErrorf(a.ctx, "Read error for %s: %v", serviceID, err)
                }
                time.Sleep(500 * time.Millisecond)
                continue
            }

            lineBuf = append(lineBuf, line...)
            if !isPrefix {
                runtime.EventsEmit(a.ctx, "service-log-line", map[string]interface{}{
                    "serviceId": serviceID,
                    "line":      string(lineBuf),
                })
                lineBuf = lineBuf[:0]
            }
        }
    }
}

// StopMonitoringLog stops tailing the service's log file.
func (a *App) StopMonitoringService(serviceID string) {
	a.logTailersLock.Lock()
	defer a.logTailersLock.Unlock()
    if info, exists := a.logTailers[serviceID]; exists {
        info.cancel()
        <-info.done // Wait for tailer to finish
        delete(a.logTailers, serviceID)
    }
}

// SelectFile opens a file selection dialog
func (a *App) SelectFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Executable File",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Executable Files (*.exe)",
				Pattern:     "*.exe",
			},
			{
				DisplayName: "All Files (*.*)",
				Pattern:     "*.*",
			},
		},
	})
}

// SelectDirectory opens a directory selection dialog
func (a *App) SelectDirectory() (string, error) {
	selection, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Working Directory",
	})

	if err != nil {
		return "", err
	}

	return selection, nil
}

// CheckAdminPrivileges checks if the application is running with administrator privileges
func (a *App) CheckAdminPrivileges() bool {
	return isUserAnAdmin()
}

func isUserAnAdmin() bool {
	if _, err := os.Open("\\\\.\\PHYSICALDRIVE0"); err == nil {
		return true
	}

	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid,
	)
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)

	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		token, err = openCurrentThreadTokenSafe()
		if err != nil {
			return false
		}
	}
	defer token.Close()

	member, err := token.IsMember(sid)
	if err != nil {
		return false
	}

	return member
}

// openCurrentThreadTokenSafe safely retrieves the access token of the current thread
func openCurrentThreadTokenSafe() (windows.Token, error) {
	if err := impersonateSelf(); err != nil {
		return 0, err
	}
	defer revertToSelf()

	thread, err := getCurrentThread()
	if err != nil {
		return 0, err
	}

	var token windows.Token
	err = openThreadToken(thread, windows.TOKEN_QUERY, true, &token)
	if err != nil {
		return 0, err
	}

	return token, nil
}

// Windows API function declarations
var (
	modadvapi32 = windows.NewLazySystemDLL("advapi32.dll")
	modkernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procGetCurrentThread = modkernel32.NewProc("GetCurrentThread")
	procOpenThreadToken  = modadvapi32.NewProc("OpenThreadToken")
	procImpersonateSelf  = modadvapi32.NewProc("ImpersonateSelf")
	procRevertToSelf     = modadvapi32.NewProc("RevertToSelf")
)

// getCurrentThread gets a pseudo handle for the current thread
func getCurrentThread() (windows.Handle, error) {
	r0, _, e1 := syscall.SyscallN(procGetCurrentThread.Addr(), 0, 0, 0, 0)
	handle := windows.Handle(r0)
	if handle == 0 {
		if e1 != 0 {
			return 0, error(e1)
		}
		return 0, syscall.EINVAL
	}
	return handle, nil
}

// openThreadToken opens the thread access token
func openThreadToken(h windows.Handle, access uint32, self bool, token *windows.Token) error {
	var _p0 uint32
	if self {
		_p0 = 1
	}
	r1, _, e1 := syscall.SyscallN(
		procOpenThreadToken.Addr(),
		4,
		uintptr(h),
		uintptr(access),
		uintptr(_p0),
		uintptr(unsafe.Pointer(token)),
		0, 0,
	)
	if r1 == 0 {
		if e1 != 0 {
			return error(e1)
		}
		return syscall.EINVAL
	}
	return nil
}

// impersonateSelf impersonates the calling thread
func impersonateSelf() error {
	r0, _, e1 := syscall.SyscallN(procImpersonateSelf.Addr(), 1, uintptr(2), 0, 0)
	if r0 == 0 {
		if e1 != 0 {
			return error(e1)
		}
		return syscall.EINVAL
	}
	return nil
}

// revertToSelf restores the original security context
func revertToSelf() error {
	r0, _, e1 := syscall.SyscallN(procRevertToSelf.Addr(), 0, 0, 0, 0)
	if r0 == 0 {
		if e1 != 0 {
			return error(e1)
		}
		return syscall.EINVAL
	}
	return nil
}

// SetAutoStart enables or disables automatic startup with Windows
func (a *App) SetAutoStart(enabled bool) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get program path: %v", err)
	}

	key, err := registry.OpenKey(registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("failed to open registry: %v", err)
	}
	defer key.Close()

	appName := "WindowsServiceManager"

	if enabled {
		err = key.SetStringValue(appName, execPath)
		if err != nil {
			return fmt.Errorf("failed to set startup entry: %v", err)
		}
	} else {
		err = key.DeleteValue(appName)
		if err != nil && err != registry.ErrNotExist {
			return fmt.Errorf("failed to delete startup entry: %v", err)
		}
	}

	return nil
}

// GetAutoStartStatus checks if automatic startup with Windows is enabled
func (a *App) GetAutoStartStatus() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()

	_, _, err = key.GetStringValue("WindowsServiceManager")
	return err == nil
}

// RestartAsAdmin restarts the application with administrator privileges
func (a *App) RestartAsAdmin() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	args := strings.Join(os.Args[1:], " ")

	verbPtr, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	exePtr, err := syscall.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	cwdPtr, err := syscall.UTF16PtrFromString(cwd)
	if err != nil {
		return err
	}
	argPtr, err := syscall.UTF16PtrFromString(args)
	if err != nil {
		return err
	}

	var showCmd int32 = 1

	err = windows.ShellExecute(0, verbPtr, exePtr, argPtr, cwdPtr, showCmd)
	if err != nil {
		return err
	}

	os.Exit(0)
	return nil
}

func (a *App) ShowWindow() {
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowCenter(a.ctx)
	runtime.WindowSetAlwaysOnTop(a.ctx, true)
	runtime.WindowSetAlwaysOnTop(a.ctx, false)
}

func (a *App) HideWindow() {
	runtime.WindowHide(a.ctx)
}

// SetServiceAutoStart sets whether a service should start automatically at boot
func (a *App) SetServiceAutoStart(serviceID string, enabled bool) error {
	return a.serviceManager.SetServiceAutoStart(serviceID, enabled)
}

// GetServiceAutoStart retrieves the auto-start status of a service
func (a *App) GetServiceAutoStart(serviceID string) bool {
	return a.serviceManager.GetServiceAutoStart(serviceID)
}

// AddSystemEnvironmentVariable adds a system environment variable
func (a *App) AddSystemEnvironmentVariable(varName, varValue string) error {
	return a.environmentManager.AddSystemEnvironmentVariable(varName, varValue)
}

// AddPathVariable adds a PATH environment variable
func (a *App) AddPathVariable(pathValue string) error {
	return a.environmentManager.AddPathVariable(pathValue)
}

// OpenSystemEnvironmentSettings opens the system environment variables settings window
func (a *App) OpenSystemEnvironmentSettings() error {
	return a.environmentManager.OpenSystemEnvironmentSettings()
}

// ValidatePathExists checks whether a path exists
func (a *App) ValidatePathExists(path string) bool {
	return a.environmentManager.ValidatePathExists(path)
}

// DiagnoseEnvironmentAccess diagnoses access permissions for environment variables
func (a *App) DiagnoseEnvironmentAccess() (map[string]interface{}, error) {
	return a.environmentManager.DiagnoseEnvironmentAccess()
}
