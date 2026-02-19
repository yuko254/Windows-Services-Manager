package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// WindowsServiceManager manages services using the Windows Service Control Manager API
type WindowsServiceManager struct {
	mutex       sync.RWMutex
	dataFile    string
	services    map[string]*Service
	statusCache *ServiceStatusCache
	ctx         context.Context
}

// getDataConfigPath returns the path to the data config file
func getDataConfigPath() (string, error) {
    configDir, err := os.UserConfigDir()
    if err != nil {
        return "", err
    }
    return filepath.Join(configDir, "Windows-Services-Manager", "data.json"), nil
}


// NewWindowsServiceManager creates a new Windows service manager
func NewWindowsServiceManager() *WindowsServiceManager {
	cache := NewServiceStatusCache()
	cache.StartCleanupRoutine()
	path, err := getDataConfigPath()
	if err != nil {
		fmt.Printf("Warning: failed to get data config path: %v\n", err)
		path = "services_data.json"
		return nil
	}

	return &WindowsServiceManager{
		services:    make(map[string]*Service),
		dataFile:    path,
		statusCache: cache,
	}
}

// SetContext sets the context for emitting events
func (wsm *WindowsServiceManager) SetContext(ctx context.Context) {
	wsm.ctx = ctx
}

// emitServiceStatusChanged emits a service status change event
func (wsm *WindowsServiceManager) emitServiceStatusChanged(serviceID, status string, pid int) {
	if wsm.ctx != nil {
		runtime.EventsEmit(wsm.ctx, "service-status-changed", map[string]interface{}{
			"serviceId": serviceID,
			"status":    status,
			"pid":       pid,
		})
	}
}

// emitServicesUpdated emits a service list update event
func (wsm *WindowsServiceManager) emitServicesUpdated() {
	if wsm.ctx != nil {
		services := make([]*Service, 0, len(wsm.services))
		for _, service := range wsm.services {
			services = append(services, service)
		}
		runtime.EventsEmit(wsm.ctx, "services-updated", services)
	}
}

// connectSCM connects to the Windows Service Control Manager
func (wsm *WindowsServiceManager) connectSCM() (*mgr.Mgr, error) {
	return mgr.Connect()
}

// withSCM is a helper to perform operations using SCM
func (wsm *WindowsServiceManager) withSCM(operation func(*mgr.Mgr) error) error {
	scm, err := wsm.connectSCM()
	if err != nil {
		return fmt.Errorf("failed to connect to service control manager: %v", err)
	}
	defer scm.Disconnect()

	return operation(scm)
}

// waitForServiceState waits for a service to reach a specific state
func (wsm *WindowsServiceManager) waitForServiceState(windowsService *mgr.Service, targetState svc.State, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		status, err := windowsService.Query()
		if err != nil {
			return fmt.Errorf("failed to query service status: %v", err)
		}

		if status.State == targetState {
			return nil
		}

		if targetState == svc.Running && status.State == svc.Stopped {
			return fmt.Errorf("service failed to start")
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for service state")
}

// setServiceRegistryValue sets a registry value for a service
func (wsm *WindowsServiceManager) setServiceRegistryValue(serviceName, subKey, valueName, value string) error {
	keyPath := fmt.Sprintf(`SYSTEM\CurrentControlSet\Services\%s`, serviceName)
	if subKey != "" {
		keyPath = fmt.Sprintf(`%s\%s`, keyPath, subKey)
	}

	var key registry.Key
	var err error

	if subKey != "" {
		parentKey, err := registry.OpenKey(registry.LOCAL_MACHINE, fmt.Sprintf(`SYSTEM\CurrentControlSet\Services\%s`, serviceName), registry.SET_VALUE)
		if err != nil {
			return fmt.Errorf("failed to open service registry key: %v", err)
		}
		defer parentKey.Close()

		key, _, err = registry.CreateKey(parentKey, subKey, registry.SET_VALUE)
		if err != nil {
			return fmt.Errorf("failed to create registry subkey: %v", err)
		}
	} else {
		key, err = registry.OpenKey(registry.LOCAL_MACHINE, keyPath, registry.SET_VALUE)
		if err != nil {
			return fmt.Errorf("failed to open service registry key: %v", err)
		}
	}
	defer key.Close()

	err = key.SetStringValue(valueName, value)
	if err != nil {
		return fmt.Errorf("failed to set registry value: %v", err)
	}

	return nil
}

// setServiceWorkingDirectory sets the working directory for a service via registry
func (wsm *WindowsServiceManager) setServiceWorkingDirectory(serviceName, workingDir string) error {
	return wsm.setServiceRegistryValue(serviceName, "Parameters", "AppDirectory", workingDir)
}

// setServiceImagePathDirect directly sets the ImagePath value of a service
func (wsm *WindowsServiceManager) setServiceImagePathDirect(serviceName, imagePath string) error {
	return wsm.setServiceRegistryValue(serviceName, "", "ImagePath", imagePath)
}

// createServiceWrapper sets up the built-in service wrapper (using current program + arguments mode)
func (wsm *WindowsServiceManager) createServiceWrapper(serviceName, exePath, args, workingDir string) (string, error) {
	currentExe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get current executable path: %v", err)
	}

	err = wsm.storeServiceConfigInRegistry(serviceName, exePath, args, workingDir)
	if err != nil {
		return "", fmt.Errorf("failed to store service configuration: %v", err)
	}

	return fmt.Sprintf(`"%s" --service-wrapper %s`, currentExe, serviceName), nil
}

// storeServiceConfigInRegistry stores service configuration in the registry
func (wsm *WindowsServiceManager) storeServiceConfigInRegistry(serviceName, exePath, args, workingDir string) error {
	if err := wsm.setServiceRegistryValue(serviceName, "Parameters", "ExePath", exePath); err != nil {
		return fmt.Errorf("failed to set ExePath: %v", err)
	}

	if args != "" {
		if err := wsm.setServiceRegistryValue(serviceName, "Parameters", "Args", args); err != nil {
			return fmt.Errorf("failed to set Args: %v", err)
		}
	}

	if workingDir != "" {
		if err := wsm.setServiceRegistryValue(serviceName, "Parameters", "WorkingDir", workingDir); err != nil {
			return fmt.Errorf("failed to set WorkingDir: %v", err)
		}
	}

	return nil
}

// GetServices returns all services managed by us
func (wsm *WindowsServiceManager) GetServices() ([]*Service, error) {
	wsm.mutex.RLock()
	defer wsm.mutex.RUnlock()

	var services []*Service

	err := wsm.withSCM(func(scm *mgr.Mgr) error {
		services = make([]*Service, 0, len(wsm.services))
		for _, service := range wsm.services {
			status, pid := wsm.getServiceRealTimeStatus(scm, service.ID)
			service.Status = status
			service.PID = pid
			service.UpdatedAt = time.Now()
			services = append(services, service)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	go wsm.saveServices()

	return services, nil
}

// CreateService creates a system service using Windows SCM
func (wsm *WindowsServiceManager) CreateService(config ServiceConfig) (*Service, error) {
	wsm.mutex.Lock()
	defer wsm.mutex.Unlock()

	if _, err := os.Stat(config.ExePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("executable does not exist: %s", config.ExePath)
	}

	serviceName := wsm.generateServiceName(config.Name)

	if _, exists := wsm.services[serviceName]; exists {
		return nil, fmt.Errorf("service name already exists: %s", serviceName)
	}

	workingDir := config.WorkingDir
	if workingDir == "" {
		workingDir = filepath.Dir(config.ExePath)
	}

	var service *Service

	err := wsm.withSCM(func(scm *mgr.Mgr) error {
		serviceConfig := mgr.Config{
			ServiceType:  windows.SERVICE_WIN32_OWN_PROCESS,
			StartType:    mgr.StartAutomatic,
			ErrorControl: mgr.ErrorNormal,
			DisplayName:  config.Name,
			Description:  fmt.Sprintf("Service created by Windows Service Manager: %s", config.Name),
		}

		binaryPath := config.ExePath
		if config.Args != "" {
			binaryPath = fmt.Sprintf("\"%s\" %s", config.ExePath, config.Args)
		}

		windowsService, err := scm.CreateService(serviceName, binaryPath, serviceConfig)
		if err != nil {
			return fmt.Errorf("failed to create Windows service: %v", err)
		}
		defer windowsService.Close()

		wrapperPath, err := wsm.createServiceWrapper(serviceName, config.ExePath, config.Args, workingDir)
		if err != nil {
			windowsService.Delete()
			return fmt.Errorf("failed to create service wrapper: %v", err)
		}

		err = wsm.setServiceImagePathDirect(serviceName, wrapperPath)
		if err != nil {
			windowsService.Delete()
			return fmt.Errorf("failed to set service path: %v", err)
		}

		err = wsm.setServiceWorkingDirectory(serviceName, workingDir)
		if err != nil {
			fmt.Printf("Warning: failed to set working directory: %v\n", err)
		}

		service = &Service{
			ID:         serviceName,
			Name:       config.Name,
			ExePath:    config.ExePath,
			Args:       config.Args,
			WorkingDir: workingDir,
			Status:     "stopped",
			PID:        0,
			AutoStart:  false,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	wsm.services[serviceName] = service
	wsm.saveServices()
	
	// Emit service list update event
	wsm.emitServicesUpdated()
	
	// Auto-start the service
	go func() {
		time.Sleep(1 * time.Second)
		wsm.StartService(serviceName)
	}()

	return service, nil
}

// StartService starts a Windows service
func (wsm *WindowsServiceManager) StartService(serviceID string) error {
	wsm.mutex.Lock()
	defer wsm.mutex.Unlock()

	service, exists := wsm.services[serviceID]
	if !exists {
		return fmt.Errorf("service does not exist: %s", serviceID)
	}

	return wsm.withSCM(func(scm *mgr.Mgr) error {
		windowsService, err := scm.OpenService(serviceID)
		if err != nil {
			return fmt.Errorf("failed to open service: %v", err)
		}
		defer windowsService.Close()

		status, err := windowsService.Query()
		if err != nil {
			return fmt.Errorf("failed to query service status: %v", err)
		}

		if status.State == svc.Running {
			return fmt.Errorf("service is already running")
		}

		err = windowsService.Start()
		if err != nil {
			return fmt.Errorf("failed to start service: %v", err)
		}

		err = wsm.waitForServiceState(windowsService, svc.Running, 30*time.Second)
		if err != nil {
			service.Status = "error"
			service.UpdatedAt = time.Now()
			wsm.saveServices()
			return err
		}

		status, _ = windowsService.Query()
		service.Status = "running"
		service.PID = int(status.ProcessId)
		service.UpdatedAt = time.Now()
		wsm.statusCache.Set(serviceID, "running", int(status.ProcessId))
		wsm.saveServices()
		
		// Emit status change event
		wsm.emitServiceStatusChanged(serviceID, "running", int(status.ProcessId))

		return nil
	})
}

// StopService stops a Windows service
func (wsm *WindowsServiceManager) StopService(serviceID string) error {
	wsm.mutex.Lock()
	defer wsm.mutex.Unlock()

	service, exists := wsm.services[serviceID]
	if !exists {
		return fmt.Errorf("service does not exist: %s", serviceID)
	}

	return wsm.withSCM(func(scm *mgr.Mgr) error {
		windowsService, err := scm.OpenService(serviceID)
		if err != nil {
			return fmt.Errorf("failed to open service: %v", err)
		}
		defer windowsService.Close()

		status, err := windowsService.Query()
		if err != nil {
			return fmt.Errorf("failed to query service status: %v", err)
		}

		if status.State == svc.Stopped {
			service.Status = "stopped"
			service.PID = 0
			service.UpdatedAt = time.Now()
			wsm.saveServices()
			return nil
		}

		_, err = windowsService.Control(svc.Stop)
		if err != nil {
			return fmt.Errorf("failed to send stop signal: %v", err)
		}

		err = wsm.waitForServiceState(windowsService, svc.Stopped, 30*time.Second)
		if err != nil {
			return err
		}

		service.Status = "stopped"
		service.PID = 0
		service.UpdatedAt = time.Now()
		wsm.statusCache.Set(serviceID, "stopped", 0)
		wsm.saveServices()
		
		// Emit status change event
		wsm.emitServiceStatusChanged(serviceID, "stopped", 0)

		return nil
	})
}

// DeleteService deletes a Windows service
func (wsm *WindowsServiceManager) DeleteService(serviceID string) error {
	wsm.mutex.Lock()
	defer wsm.mutex.Unlock()

	_, exists := wsm.services[serviceID]
	if !exists {
		return fmt.Errorf("service does not exist: %s", serviceID)
	}

	return wsm.withSCM(func(scm *mgr.Mgr) error {
		windowsService, err := scm.OpenService(serviceID)
		if err != nil {
			return fmt.Errorf("failed to open service: %v", err)
		}
		defer windowsService.Close()

		status, err := windowsService.Query()
		if err == nil && status.State != svc.Stopped {
			windowsService.Control(svc.Stop)

			wsm.waitForServiceState(windowsService, svc.Stopped, 30*time.Second)
		}

		err = windowsService.Delete()
		if err != nil {
			return fmt.Errorf("failed to delete service: %v", err)
		}

		delete(wsm.services, serviceID)
		wsm.statusCache.Remove(serviceID)
		wsm.saveServices()
		
		// Emit service list update event
		wsm.emitServicesUpdated()

		return nil
	})
}

// getServiceRealTimeStatus gets real-time service status (using cache optimization)
func (wsm *WindowsServiceManager) getServiceRealTimeStatus(scm *mgr.Mgr, serviceName string) (string, int) {
	if cachedStatus, found := wsm.statusCache.Get(serviceName); found {
		return cachedStatus.Status, cachedStatus.PID
	}

	windowsService, err := scm.OpenService(serviceName)
	if err != nil {
		wsm.statusCache.Set(serviceName, "error", 0)
		return "error", 0
	}
	defer windowsService.Close()

	status, err := windowsService.Query()
	if err != nil {
		wsm.statusCache.Set(serviceName, "error", 0)
		return "error", 0
	}

	var statusStr string
	var pid int

	switch status.State {
	case svc.Running:
		statusStr = "running"
		pid = int(status.ProcessId)
	case svc.Stopped:
		statusStr = "stopped"
		pid = 0
	case svc.StartPending:
		statusStr = "starting"
		pid = 0
	case svc.StopPending:
		statusStr = "stopping"
		pid = int(status.ProcessId)
	default:
		statusStr = "error"
		pid = 0
	}

	// Update cache
	wsm.statusCache.Set(serviceName, statusStr, pid)
	return statusStr, pid
}

// generateServiceName generates a unique service name
func (wsm *WindowsServiceManager) generateServiceName(displayName string) string {
	cleanName := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, displayName)

	return fmt.Sprintf("WSM_%s_%d", cleanName, time.Now().Unix())
}

// saveServices saves service data to file
func (wsm *WindowsServiceManager) saveServices() {
	data, err := json.MarshalIndent(wsm.services, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(wsm.dataFile, data, 0644)
}

// loadServices loads service data from file
func (wsm *WindowsServiceManager) loadServices() {
	if _, err := os.Stat(wsm.dataFile); os.IsNotExist(err) {
		return
	}

	data, err := os.ReadFile(wsm.dataFile)
	if err != nil {
		return
	}

	json.Unmarshal(data, &wsm.services)
}

// SetServiceAutoStart sets whether a service starts automatically at boot
func (wsm *WindowsServiceManager) SetServiceAutoStart(serviceID string, enabled bool) error {
	wsm.mutex.Lock()
	defer wsm.mutex.Unlock()

	service, exists := wsm.services[serviceID]
	if !exists {
		return fmt.Errorf("service does not exist: %s", serviceID)
	}

	return wsm.withSCM(func(scm *mgr.Mgr) error {
		windowsService, err := scm.OpenService(serviceID)
		if err != nil {
			return fmt.Errorf("failed to open service: %v", err)
		}
		defer windowsService.Close()

		// Get current service configuration
		config, err := windowsService.Config()
		if err != nil {
			return fmt.Errorf("failed to get service configuration: %v", err)
		}

		// Modify start type
		if enabled {
			config.StartType = mgr.StartAutomatic
		} else {
			config.StartType = mgr.StartManual
		}

		// Update service configuration
		err = windowsService.UpdateConfig(config)
		if err != nil {
			return fmt.Errorf("failed to update service configuration: %v", err)
		}

		// Update in-memory service info
		service.AutoStart = enabled
		service.UpdatedAt = time.Now()
		wsm.saveServices()

		return nil
	})
}

// GetServiceAutoStart gets whether a service is set to auto-start
func (wsm *WindowsServiceManager) GetServiceAutoStart(serviceID string) bool {
	wsm.mutex.RLock()
	defer wsm.mutex.RUnlock()

	service, exists := wsm.services[serviceID]
	if !exists {
		return false
	}

	return service.AutoStart
}