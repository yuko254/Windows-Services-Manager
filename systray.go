package main

import (
	"github.com/getlantern/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"os"
)

// SystrayManager manages the system tray
type SystrayManager struct {
	app      *App
	trayIcon []byte
	quitCh   chan struct{}
}

// NewSystrayManager creates a new system tray manager
func NewSystrayManager(app *App, trayIconData []byte) *SystrayManager {
	return &SystrayManager{
		app:      app,
		trayIcon: trayIconData,
		quitCh:   make(chan struct{}),
	}
}

// Start starts the system tray
func (s *SystrayManager) Start() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				println("system tray startup failed:", r)
			}
		}()

		systray.Run(s.onReady, s.onExit)
	}()
}

// onReady is called when tray initialization is complete
func (s *SystrayManager) onReady() {
	if len(s.trayIcon) > 0 {
		systray.SetIcon(s.trayIcon)
	} else {
		systray.SetIcon([]byte{})
	}

	systray.SetTitle("Windows Service Manager")
	systray.SetTooltip("Windows Service Manager - Right-click to show menu")

	mShow := systray.AddMenuItem("Show Window", "Show main window")
	systray.AddSeparator()
	mExit := systray.AddMenuItem("Exit Program", "Exit application")

	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				s.app.ShowWindow()

			case <-mExit.ClickedCh:
				s.ExitApp()
				return

			case <-s.quitCh:
				return
			}
		}
	}()
}

// ExitApp exits the application
func (s *SystrayManager) ExitApp() {
	select {
	case s.quitCh <- struct{}{}:
	default:
	}

	systray.Quit()

	runtime.Quit(s.app.ctx)

	os.Exit(0)
}

// onExit is called when the tray exits
func (s *SystrayManager) onExit() {
	// Cleanup work is handled in Cleanup()
}

// Cleanup cleans up system tray resources
func (s *SystrayManager) Cleanup() {
	select {
	case s.quitCh <- struct{}{}:
	default:
	}

	systray.Quit()
}