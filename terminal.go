package main

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"al.essio.dev/pkg/shellescape"
	"github.com/creack/pty"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type terminalSession struct {
	id  string
	cmd *exec.Cmd
	pty *os.File

	mu                sync.Mutex
	commandBuffer     strings.Builder
	pendingSSH        *ConnectionCommand
	passwordBuffer    strings.Builder
	capturingPassword bool
	candidatePassword string
	autoPassword      string
	outputProbe       string
	markerPath        string
	disconnectPath    string
	wrapperDir        string
	done              chan struct{}
	closeOnce         sync.Once
}

type TerminalDataEvent struct {
	SessionID string `json:"sessionId"`
	Data      string `json:"data"`
}

type TerminalExitEvent struct {
	SessionID string `json:"sessionId"`
}

var (
	validSessionID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)
	ansiPattern    = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07]*(?:\x07|\x1b\\))`)
	passwordPrompt = regexp.MustCompile(`(?i)password[^:\r\n]*:\s*$`)
	authRetry      = regexp.MustCompile(`(?i)(permission denied, please try again|authentication failed)`)
	authFailure    = regexp.MustCompile(`(?i)(permission denied \(|connection (?:closed|refused|timed out)|could not resolve|no route to host|host key verification failed)`)
)

// StartTerminal starts a normal login shell in a named tab.
func (a *App) StartTerminal(sessionID string, cols, rows int) error {
	_, err := a.startTerminal(sessionID, cols, rows)
	return err
}

// StartSSHSession starts a new shell tab and immediately connects to a saved
// host. If a password exists in the OS keyring it is supplied directly to the
// PTY when OpenSSH asks for it; JavaScript never receives the secret.
func (a *App) StartSSHSession(sessionID, hostID string, cols, rows int) error {
	host, ok := a.getHost(hostID)
	if !ok {
		return errors.New("saved SSH host not found")
	}
	session, err := a.startTerminal(sessionID, cols, rows)
	if err != nil {
		return err
	}
	command := strings.TrimSpace(host.Command)
	if command == "" {
		command = buildSSHCommand(ConnectionCommand{Address: host.Address, User: host.User, Port: host.Port})
	}
	connection := ConnectionCommand{Protocol: "ssh", Command: command, Target: host.Target, User: host.User, Address: host.Address, Port: host.Port}
	session.mu.Lock()
	session.pendingSSH = &connection
	session.autoPassword = a.getCredential(host.ID)
	session.outputProbe = ""
	session.mu.Unlock()

	_, err = session.pty.Write([]byte(command + "\r"))
	return err
}

func (a *App) startTerminal(sessionID string, cols, rows int) (*terminalSession, error) {
	if !validSessionID.MatchString(sessionID) {
		return nil, errors.New("invalid terminal session id")
	}
	a.terminalMu.Lock()
	if existing := a.terminals[sessionID]; existing != nil {
		a.terminalMu.Unlock()
		return existing, nil
	}
	a.terminalMu.Unlock()

	shell := os.Getenv("SHELL")
	if shell == "" {
		if runtime.GOOS == "windows" {
			shell = "powershell.exe"
		} else {
			shell = "/bin/sh"
		}
	}
	args := []string{}
	if runtime.GOOS != "windows" {
		args = append(args, "-l")
	}
	cmd := exec.Command(shell, args...)
	wrapperDir, markerPath, disconnectPath, err := prepareSSHWrapper(sessionID)
	if err != nil {
		return nil, err
	}
	cmd.Env = withTerminalEnv(os.Environ(), wrapperDir)
	if cwd, err := os.UserHomeDir(); err == nil {
		cmd.Dir = cwd
	}
	if cols < 2 {
		cols = 80
	}
	if rows < 2 {
		rows = 24
	}
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		_ = os.RemoveAll(wrapperDir)
		return nil, err
	}
	session := &terminalSession{id: sessionID, cmd: cmd, pty: ptmx, markerPath: markerPath, disconnectPath: disconnectPath, wrapperDir: wrapperDir, done: make(chan struct{})}
	a.terminalMu.Lock()
	if existing := a.terminals[sessionID]; existing != nil {
		a.terminalMu.Unlock()
		_ = ptmx.Close()
		_ = cmd.Process.Kill()
		_ = os.RemoveAll(wrapperDir)
		return existing, nil
	}
	a.terminals[sessionID] = session
	a.terminalMu.Unlock()
	go a.readTerminal(session)
	go a.watchSSHSuccess(session)
	return session, nil
}

func withTerminalEnv(env []string, sshWrapperDir string) []string {
	result := make([]string, 0, len(env)+3)
	currentPath := os.Getenv("PATH")
	for _, value := range env {
		if !strings.HasPrefix(value, "TERM=") && !strings.HasPrefix(value, "COLORTERM=") && !strings.HasPrefix(value, "PATH=") {
			result = append(result, value)
		}
	}
	return append(result, "TERM=xterm-256color", "COLORTERM=truecolor", "PATH="+sshWrapperDir+string(os.PathListSeparator)+currentPath)
}

func prepareSSHWrapper(sessionID string) (string, string, string, error) {
	realSSH, err := exec.LookPath("ssh")
	if err != nil {
		return "", "", "", errors.New("OpenSSH client was not found in PATH")
	}
	directory, err := os.MkdirTemp("", "relay-ssh-"+sessionID+"-")
	if err != nil {
		return "", "", "", err
	}
	markerPath := filepath.Join(directory, "connected")
	disconnectPath := filepath.Join(directory, "disconnected")
	localCommand := "LocalCommand=printf connected > " + shellescape.Quote(markerPath)
	script := "#!/bin/sh\n" + shellescape.Quote(realSSH) + " -o PermitLocalCommand=yes -o " + shellescape.Quote(localCommand) + " \"$@\"\nstatus=$?\nprintf disconnected > " + shellescape.Quote(disconnectPath) + "\nexit $status\n"
	if err := os.WriteFile(filepath.Join(directory, "ssh"), []byte(script), 0o700); err != nil {
		_ = os.RemoveAll(directory)
		return "", "", "", err
	}
	return directory, markerPath, disconnectPath, nil
}

// OpenSSH runs LocalCommand only after authentication has succeeded. Watching
// its private marker gives us an exact success signal without parsing prompts,
// banners or shell themes.
func (a *App) watchSSHSuccess(session *terminalSession) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-session.done:
			return
		case <-ticker.C:
			if _, err := os.Stat(session.markerPath); err == nil {
				_ = os.Remove(session.markerPath)
				a.confirmSSHConnection(session)
			}
			if _, err := os.Stat(session.disconnectPath); err == nil {
				_ = os.Remove(session.disconnectPath)
				a.closeSFTP(session.id)
			}
		}
	}
}

func (a *App) confirmSSHConnection(session *terminalSession) {
	var connection *ConnectionCommand
	var password string
	session.mu.Lock()
	if session.pendingSSH != nil {
		copy := *session.pendingSSH
		connection = &copy
		password = session.candidatePassword
		session.candidatePassword = ""
		session.pendingSSH = nil
		session.capturingPassword = false
		session.passwordBuffer.Reset()
	}
	session.mu.Unlock()
	if connection != nil {
		go a.finishSSHConnection(session.id, *connection, password)
	}
}

func (a *App) readTerminal(session *terminalSession) {
	buffer := make([]byte, 32*1024)
	for {
		n, err := session.pty.Read(buffer)
		if n > 0 {
			data := string(buffer[:n])
			a.inspectTerminalOutput(session, data)
			if a.ctx != nil {
				wailsruntime.EventsEmit(a.ctx, "terminal:data", TerminalDataEvent{SessionID: session.id, Data: data})
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && a.ctx != nil {
				wailsruntime.EventsEmit(a.ctx, "terminal:data", TerminalDataEvent{SessionID: session.id, Data: "\r\n\x1b[31m" + err.Error() + "\x1b[0m"})
			}
			break
		}
	}
	_ = session.cmd.Wait()
	a.removeTerminal(session)
	a.closeTerminalResources(session)
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "terminal:exit", TerminalExitEvent{SessionID: session.id})
	}
}

func (a *App) inspectTerminalOutput(session *terminalSession, data string) {
	clean := ansiPattern.ReplaceAllString(data, "")
	var autoPassword string

	session.mu.Lock()
	if session.pendingSSH == nil {
		session.mu.Unlock()
		return
	}
	session.outputProbe = tail(session.outputProbe+clean, 4096)
	if authFailure.MatchString(session.outputProbe) {
		session.pendingSSH = nil
		session.candidatePassword = ""
		session.autoPassword = ""
		session.capturingPassword = false
		session.passwordBuffer.Reset()
		session.outputProbe = ""
	} else if authRetry.MatchString(session.outputProbe) {
		session.candidatePassword = ""
		session.autoPassword = ""
		session.capturingPassword = passwordPrompt.MatchString(session.outputProbe)
		session.passwordBuffer.Reset()
		session.outputProbe = tail(clean, 512)
	} else if passwordPrompt.MatchString(session.outputProbe) && !session.capturingPassword && session.candidatePassword == "" {
		if session.autoPassword != "" {
			autoPassword = session.autoPassword
			session.candidatePassword = autoPassword
			session.autoPassword = ""
		} else {
			session.capturingPassword = true
			session.passwordBuffer.Reset()
		}
	}
	session.mu.Unlock()

	if autoPassword != "" {
		_, _ = session.pty.Write([]byte(autoPassword + "\r"))
	}
}

func (a *App) WriteTerminal(sessionID, data string) error {
	session := a.getTerminal(sessionID)
	if session == nil {
		return errors.New("terminal is not running")
	}
	a.inspectTerminalInput(session, data)
	_, err := session.pty.Write([]byte(data))
	return err
}

func (a *App) inspectTerminalInput(session *terminalSession, data string) {
	session.mu.Lock()
	if session.capturingPassword {
		for len(data) > 0 {
			r, size := utf8.DecodeRuneInString(data)
			data = data[size:]
			switch r {
			case '\r', '\n':
				session.candidatePassword = session.passwordBuffer.String()
				session.passwordBuffer.Reset()
				session.capturingPassword = false
				session.outputProbe = ""
			case '\x7f', '\b':
				value := session.passwordBuffer.String()
				if value != "" {
					_, size := utf8.DecodeLastRuneInString(value)
					session.passwordBuffer.Reset()
					session.passwordBuffer.WriteString(value[:len(value)-size])
				}
			case '\x03':
				session.passwordBuffer.Reset()
				session.capturingPassword = false
				session.pendingSSH = nil
			default:
				if r >= 0x20 {
					session.passwordBuffer.WriteRune(r)
				}
			}
		}
		session.mu.Unlock()
		return
	}

	for _, r := range data {
		switch r {
		case '\r', '\n':
			line := session.commandBuffer.String()
			session.commandBuffer.Reset()
			if connection := parseConnectionCommand(line); connection != nil && connection.Protocol == "ssh" {
				session.pendingSSH = connection
				session.candidatePassword = ""
				session.autoPassword = ""
				session.outputProbe = ""
			}
		case '\x7f', '\b':
			value := session.commandBuffer.String()
			if value != "" {
				_, size := utf8.DecodeLastRuneInString(value)
				session.commandBuffer.Reset()
				session.commandBuffer.WriteString(value[:len(value)-size])
			}
		case '\x03', '\x15':
			session.commandBuffer.Reset()
		default:
			if r >= 0x20 && r != '\x1b' {
				session.commandBuffer.WriteRune(r)
			}
		}
	}
	session.mu.Unlock()
}

func (a *App) finishSSHConnection(sessionID string, connection ConnectionCommand, password string) {
	knownHost := a.hasSavedHost(connection)
	if knownHost {
		a.rememberSuccessfulHost(connection, password)
	} else {
		a.pendingHostMu.Lock()
		a.pendingHosts[sessionID] = pendingHostSave{connection: connection, password: password}
		a.pendingHostMu.Unlock()
	}
	if a.ctx != nil {
		if knownHost {
			wailsruntime.EventsEmit(a.ctx, "hosts:changed")
		} else {
			wailsruntime.EventsEmit(a.ctx, "host:save-request", map[string]interface{}{"sessionId": sessionID, "connection": connection})
		}
		wailsruntime.EventsEmit(a.ctx, "ssh:connected", map[string]string{"sessionId": sessionID, "target": connection.Target})
	}
	a.connectSFTP(sessionID, connection, password)
}

func tail(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[len(value)-limit:]
}

func (a *App) getTerminal(sessionID string) *terminalSession {
	a.terminalMu.Lock()
	defer a.terminalMu.Unlock()
	return a.terminals[sessionID]
}

func (a *App) ResizeTerminal(sessionID string, cols, rows int) error {
	if cols < 2 || rows < 2 {
		return nil
	}
	session := a.getTerminal(sessionID)
	if session == nil {
		return nil
	}
	return pty.Setsize(session.pty, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (a *App) StopTerminal(sessionID string) {
	session := a.getTerminal(sessionID)
	if session != nil {
		a.stopTerminal(session)
	}
}

func (a *App) StopAllTerminals() {
	a.terminalMu.Lock()
	sessions := make([]*terminalSession, 0, len(a.terminals))
	for _, session := range a.terminals {
		sessions = append(sessions, session)
	}
	a.terminals = make(map[string]*terminalSession)
	a.terminalMu.Unlock()
	for _, session := range sessions {
		a.stopTerminal(session)
	}
}

func (a *App) stopTerminal(session *terminalSession) {
	a.removeTerminal(session)
	a.closeTerminalResources(session)
	_ = session.pty.Close()
	if session.cmd.Process != nil {
		_ = session.cmd.Process.Signal(syscall.SIGHUP)
		_ = session.cmd.Process.Kill()
	}
}

func (a *App) removeTerminal(session *terminalSession) {
	a.terminalMu.Lock()
	if a.terminals[session.id] == session {
		delete(a.terminals, session.id)
	}
	a.terminalMu.Unlock()
}

func (a *App) closeTerminalResources(session *terminalSession) {
	session.closeOnce.Do(func() {
		close(session.done)
		a.closeSFTP(session.id)
		_ = os.RemoveAll(session.wrapperDir)
	})
}
