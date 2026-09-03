package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"
)

// App owns the long-lived terminal process and the small amount of local state
// used by the connection manager.
type App struct {
	ctx context.Context
	// Restores the user's locale environment after GTK has initialised. This
	// keeps spawned shells and SSH tools in the locale selected by the user.
	restoreLocale func()

	terminalMu sync.Mutex
	terminals  map[string]*terminalSession
	sftpMu     sync.Mutex
	sftp       map[string]*remoteSession
	transferMu sync.Mutex
	transfers  map[string]*transferControl

	hostsMu       sync.Mutex
	hosts         []Host
	hostsFile     string
	pendingHostMu sync.Mutex
	pendingHosts  map[string]pendingHostSave
}

func NewApp() *App {
	return &App{
		terminals:    make(map[string]*terminalSession),
		sftp:         make(map[string]*remoteSession),
		transfers:    make(map[string]*transferControl),
		pendingHosts: make(map[string]pendingHostSave),
	}
}

func (a *App) startup(ctx context.Context) {
	if a.restoreLocale != nil {
		a.restoreLocale()
		a.restoreLocale = nil
	}
	a.ctx = ctx
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = os.TempDir()
	}
	a.hostsFile = filepath.Join(configDir, "ssh-manager-ui", "hosts.json")
	a.loadHosts()
}

func (a *App) shutdown(_ context.Context) {
	a.StopAllTerminals()
	a.closeAllSFTP()
}
