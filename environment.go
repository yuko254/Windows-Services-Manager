package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// EnvironmentManager manages environment variables
type EnvironmentManager struct{}

func NewEnvironmentManager() *EnvironmentManager {
	return &EnvironmentManager{}
}

// AddSystemEnvironmentVariable adds a system-level environment variable
func (em *EnvironmentManager) AddSystemEnvironmentVariable(varName, varValue string) error {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Control\Session Manager\Environment`,
		registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("cannot open system environment registry (administrator rights required): %v", err)
	}
	defer key.Close()

	var valueType uint32
	if strings.ToUpper(varName) == "PATH" || strings.Contains(varValue, "%") {
		valueType = registry.EXPAND_SZ
	} else {
		valueType = registry.SZ
	}

	// Special handling for PATH variable
	if strings.ToUpper(varName) == "PATH" {
		var existingPath string
		var readErr error

		existingPath, _, readErr = key.GetStringValue("PATH")
		if readErr != nil && readErr != registry.ErrNotExist {
			return fmt.Errorf("cannot read existing PATH variable: %v", readErr)
		}

		if existingPath != "" {
			pathEntries := strings.Split(existingPath, ";")
			for _, entry := range pathEntries {
				if strings.EqualFold(strings.TrimSpace(entry), strings.TrimSpace(varValue)) {
					return fmt.Errorf("path already exists in PATH: %s", varValue)
				}
			}
		}

		if existingPath != "" {
			if !strings.HasSuffix(existingPath, ";") {
				varValue = existingPath + ";" + varValue
			} else {
				varValue = existingPath + varValue
			}
		}
	}

	// Set registry value
	if valueType == registry.EXPAND_SZ {
		err = key.SetExpandStringValue(varName, varValue)
	} else {
		err = key.SetStringValue(varName, varValue)
	}

	if err != nil {
		return fmt.Errorf("cannot set environment variable: %v", err)
	}

	// Immediately notify system that environment variable has changed
	err = em.broadcastEnvironmentChange()
	if err != nil {
		return fmt.Errorf("environment variable set successfully, but failed to notify system: %v", err)
	}

	return nil
}

// AddPathVariable specifically adds a PATH environment variable
func (em *EnvironmentManager) AddPathVariable(pathValue string) error {
	pathValue = strings.Trim(pathValue, "\"")

	if !filepath.IsAbs(pathValue) {
		return fmt.Errorf("absolute path must be provided")
	}

	if strings.HasSuffix(strings.ToLower(pathValue), ".exe") {
		pathValue = filepath.Dir(pathValue)
	}

	return em.AddSystemEnvironmentVariable("PATH", pathValue)
}

// broadcastEnvironmentChange broadcasts environment change message
func (em *EnvironmentManager) broadcastEnvironmentChange() error {
	const (
		HWND_BROADCAST   = 0xffff
		WM_SETTINGCHANGE = 0x001A
		SMTO_ABORTIFHUNG = 0x0002
	)

	user32 := windows.NewLazySystemDLL("user32.dll")
	sendMessageTimeoutW := user32.NewProc("SendMessageTimeoutW")

	environmentPtr, _ := syscall.UTF16PtrFromString("Environment")

	ret, _, err := sendMessageTimeoutW.Call(
		uintptr(HWND_BROADCAST),
		uintptr(WM_SETTINGCHANGE),
		0,
		uintptr(unsafe.Pointer(environmentPtr)),
		uintptr(SMTO_ABORTIFHUNG),
		uintptr(5000), // 5 second timeout
		uintptr(0),
	)

	if ret == 0 {
		return fmt.Errorf("failed to broadcast environment change: %v", err)
	}

	return nil
}

// OpenSystemEnvironmentSettings opens system environment settings
func (em *EnvironmentManager) OpenSystemEnvironmentSettings() error {
	cmd := exec.Command("rundll32.exe", "sysdm.cpl,EditEnvironmentVariables")
	return cmd.Start()
}

// ValidatePathExists validates whether a path exists
func (em *EnvironmentManager) ValidatePathExists(path string) bool {
	path = strings.Trim(path, "\"")
	if _, err := windows.GetFileAttributes(windows.StringToUTF16Ptr(path)); err != nil {
		return false
	}
	return true
}

// GetSystemEnvironmentVariable gets a system environment variable value
func (em *EnvironmentManager) GetSystemEnvironmentVariable(varName string) (string, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Control\Session Manager\Environment`,
		registry.QUERY_VALUE)
	if err != nil {
		return "", fmt.Errorf("cannot open system environment registry: %v", err)
	}
	defer key.Close()

	value, _, err := key.GetStringValue(varName)
	if err != nil {
		if err == registry.ErrNotExist {
			return "", fmt.Errorf("environment variable does not exist: %s", varName)
		}
		return "", fmt.Errorf("cannot read environment variable: %v", err)
	}

	return value, nil
}

// DiagnoseEnvironmentAccess diagnoses environment variable access permissions
func (em *EnvironmentManager) DiagnoseEnvironmentAccess() (map[string]interface{}, error) {
	result := make(map[string]interface{})

	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Control\Session Manager\Environment`,
		registry.QUERY_VALUE)
	if err != nil {
		result["registry_read"] = false
		result["registry_read_error"] = err.Error()
	} else {
		result["registry_read"] = true
		key.Close()
	}

	key, err = registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Control\Session Manager\Environment`,
		registry.SET_VALUE)
	if err != nil {
		result["registry_write"] = false
		result["registry_write_error"] = err.Error()
	} else {
		result["registry_write"] = true
		key.Close()
	}

	key, err = registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Control\Session Manager\Environment`,
		registry.ALL_ACCESS)
	if err != nil {
		result["registry_full"] = false
		result["registry_full_error"] = err.Error()
	} else {
		result["registry_full"] = true
		key.Close()
	}

	pathValue, err := em.GetSystemEnvironmentVariable("PATH")
	if err != nil {
		result["path_read"] = false
		result["path_read_error"] = err.Error()
	} else {
		result["path_read"] = true
		result["path_length"] = len(pathValue)
	}

	return result, nil
}