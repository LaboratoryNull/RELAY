package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"al.essio.dev/pkg/shellescape"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type Host struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Target        string `json:"target"`
	User          string `json:"user"`
	Address       string `json:"address"`
	Port          int    `json:"port"`
	Command       string `json:"command"`
	Favorite      bool   `json:"favorite"`
	HasPassword   bool   `json:"hasPassword"`
	LastConnected string `json:"lastConnected"`
}

type ConnectionCommand struct {
	Protocol string `json:"protocol"`
	Command  string `json:"command"`
	Target   string `json:"target"`
	User     string `json:"user"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
}

type pendingHostSave struct {
	connection ConnectionCommand
	password   string
}

func (a *App) loadHosts() {
	a.hostsMu.Lock()
	defer a.hostsMu.Unlock()
	data, err := os.ReadFile(a.hostsFile)
	if err == nil {
		_ = json.Unmarshal(data, &a.hosts)
	}
}

func (a *App) saveHostsLocked() error {
	if err := os.MkdirAll(filepath.Dir(a.hostsFile), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(a.hosts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(a.hostsFile, data, 0o600)
}

func (a *App) GetHosts() []Host {
	a.hostsMu.Lock()
	defer a.hostsMu.Unlock()
	// A nil slice is encoded as JSON null by Wails. The frontend contract is an
	// array, including when no connections have been recorded yet.
	hosts := make([]Host, len(a.hosts))
	copy(hosts, a.hosts)
	sort.SliceStable(hosts, func(i, j int) bool {
		if hosts[i].Favorite != hosts[j].Favorite {
			return hosts[i].Favorite
		}
		return hosts[i].LastConnected > hosts[j].LastConnected
	})
	return hosts
}

// TrackCommand recognises normal OpenSSH syntax without changing history.
// A host is persisted by the terminal only after authentication succeeds.
func (a *App) TrackCommand(line string) *ConnectionCommand {
	return parseConnectionCommand(line)
}

func parseConnectionCommand(line string) *ConnectionCommand {
	fields := splitCommandLine(line)
	if len(fields) < 2 {
		return nil
	}
	protocol := filepath.Base(fields[0])
	if protocol != "ssh" && protocol != "sftp" {
		return nil
	}

	port := 22
	userFromOption := ""
	target := ""
	optionsWithValue := map[string]bool{
		"-b": true, "-c": true, "-D": true, "-E": true, "-e": true,
		"-F": true, "-I": true, "-i": true, "-J": true, "-L": true,
		"-l": true, "-m": true, "-O": true, "-o": true, "-P": true, "-p": true,
		"-Q": true, "-R": true, "-S": true, "-W": true, "-w": true,
	}
	for i := 1; i < len(fields); i++ {
		value := fields[i]
		if value == "--" {
			if i+1 < len(fields) {
				target = fields[i+1]
			}
			break
		}
		portOption := value == "-p" || (protocol == "sftp" && value == "-P")
		if portOption {
			if i+1 < len(fields) {
				if parsed, err := strconv.Atoi(fields[i+1]); err == nil {
					port = parsed
				}
				i++
			}
			continue
		}
		attachedPort := strings.HasPrefix(value, "-p") || (protocol == "sftp" && strings.HasPrefix(value, "-P"))
		if attachedPort && len(value) > 2 {
			if parsed, err := strconv.Atoi(value[2:]); err == nil {
				port = parsed
			}
			continue
		}
		if value == "-l" && i+1 < len(fields) {
			userFromOption = fields[i+1]
			i++
			continue
		}
		if strings.HasPrefix(value, "-l") && len(value) > 2 {
			userFromOption = value[2:]
			continue
		}
		if value == "-o" && i+1 < len(fields) {
			applySSHConfigOption(fields[i+1], &port, &userFromOption)
			i++
			continue
		}
		if strings.HasPrefix(value, "-o") && len(value) > 2 {
			applySSHConfigOption(value[2:], &port, &userFromOption)
			continue
		}
		if strings.HasPrefix(value, "-") {
			option := value
			if len(value) > 2 {
				option = value[:2]
			}
			if optionsWithValue[option] && len(value) == 2 && i+1 < len(fields) {
				i++
			}
			continue
		}
		target = value
		break
	}
	if target == "" || strings.ContainsAny(target, "\r\n\t") {
		return nil
	}

	user, address := parseTarget(target)
	if user == "" {
		user = userFromOption
	}
	if address == "" {
		return nil
	}
	connection := &ConnectionCommand{Protocol: protocol, Command: strings.TrimSpace(line), Target: target, User: user, Address: address, Port: port}
	return connection
}

func applySSHConfigOption(option string, port *int, user *string) {
	key, value, found := strings.Cut(option, "=")
	if !found {
		parts := strings.Fields(option)
		if len(parts) != 2 {
			return
		}
		key, value = parts[0], parts[1]
	}
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "port":
		if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			*port = parsed
		}
	case "user":
		*user = strings.TrimSpace(value)
	}
}

func splitCommandLine(line string) []string {
	var fields []string
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			fields = append(fields, current.String())
			current.Reset()
		}
	}
	for _, char := range strings.TrimSpace(line) {
		if escaped {
			current.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			} else {
				current.WriteRune(char)
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
		} else if char == ' ' || char == '\t' {
			flush()
		} else {
			current.WriteRune(char)
		}
	}
	flush()
	return fields
}

func parseTarget(target string) (string, string) {
	user := ""
	address := target
	if index := strings.LastIndex(target, "@"); index >= 0 {
		user = target[:index]
		address = target[index+1:]
	}
	address = strings.Trim(address, "[]")
	return user, address
}

func (a *App) rememberSuccessfulHost(connection ConnectionCommand, password string) {
	hasPassword := false
	if password != "" {
		hasPassword = a.setCredential(hostID(connection), password) == nil
	}

	a.hostsMu.Lock()
	defer a.hostsMu.Unlock()
	id := hostID(connection)
	for index := range a.hosts {
		if sameHost(a.hosts[index], connection) {
			a.hosts[index].ID = id
			a.hosts[index].Target = connection.Target
			a.hosts[index].User = connection.User
			a.hosts[index].Address = connection.Address
			a.hosts[index].Port = connection.Port
			a.hosts[index].Command = connection.Command
			a.hosts[index].LastConnected = time.Now().UTC().Format(time.RFC3339)
			if hasPassword {
				a.hosts[index].HasPassword = true
			}
			_ = a.saveHostsLocked()
			return
		}
	}
	a.hosts = append(a.hosts, Host{
		ID: id, Target: connection.Target, User: connection.User, Command: connection.Command,
		Address: connection.Address, Port: connection.Port,
		HasPassword:   hasPassword,
		LastConnected: time.Now().UTC().Format(time.RFC3339),
	})
	_ = a.saveHostsLocked()
}

func hostID(connection ConnectionCommand) string {
	identity := connection.Address
	if connection.User != "" {
		identity = connection.User + "@" + identity
	}
	base := strings.ToLower(identity) + ":" + strconv.Itoa(connection.Port)
	if command := strings.TrimSpace(connection.Command); command != "" {
		digest := sha256.Sum256([]byte(command))
		return base + ":" + fmt.Sprintf("%x", digest[:6])
	}
	return base
}

func sameHost(host Host, connection ConnectionCommand) bool {
	if host.ID == hostID(connection) {
		return true
	}
	if strings.TrimSpace(host.Command) != "" && strings.TrimSpace(connection.Command) != "" {
		return strings.TrimSpace(host.Command) == strings.TrimSpace(connection.Command)
	}
	return strings.EqualFold(host.Address, connection.Address) && strings.EqualFold(host.User, connection.User) && host.Port == connection.Port
}

func buildSSHCommand(connection ConnectionCommand) string {
	target := connection.Address
	if connection.User != "" {
		target = connection.User + "@" + target
	}
	command := "ssh "
	if connection.Port != 0 && connection.Port != 22 {
		command += "-p " + strconv.Itoa(connection.Port) + " "
	}
	return command + "-- " + shellescape.Quote(target)
}

// UpsertHost validates and saves a manually added or edited host. Command is
// the source of truth so custom OpenSSH options survive quick reconnects.
func (a *App) UpsertHost(host Host) ([]Host, error) {
	command := strings.TrimSpace(host.Command)
	if command == "" {
		command = buildSSHCommand(ConnectionCommand{Address: strings.TrimSpace(host.Address), User: strings.TrimSpace(host.User), Port: host.Port})
	}
	connection := parseConnectionCommand(command)
	if connection == nil || connection.Protocol != "ssh" {
		return nil, errors.New("некорректная SSH-команда")
	}
	if connection.Port < 1 || connection.Port > 65535 {
		return nil, errors.New("порт должен быть от 1 до 65535")
	}

	newID := hostID(*connection)
	oldID := host.ID
	a.hostsMu.Lock()
	index := -1
	for i := range a.hosts {
		if a.hosts[i].ID == oldID || a.hosts[i].ID == newID {
			index = i
			break
		}
	}
	updated := Host{
		ID: newID, Name: strings.TrimSpace(host.Name), Target: connection.Target,
		User: connection.User, Address: connection.Address, Port: connection.Port,
		Command: command, LastConnected: host.LastConnected,
	}
	if index >= 0 {
		updated.Favorite = a.hosts[index].Favorite
		updated.HasPassword = a.hosts[index].HasPassword
		if updated.LastConnected == "" {
			updated.LastConnected = a.hosts[index].LastConnected
		}
		a.hosts[index] = updated
	} else {
		a.hosts = append(a.hosts, updated)
	}
	err := a.saveHostsLocked()
	a.hostsMu.Unlock()
	if err != nil {
		return nil, err
	}
	if oldID != "" && oldID != newID {
		if password := a.getCredential(oldID); password != "" {
			if a.setCredential(newID, password) == nil {
				_ = a.deleteCredential(oldID)
			}
		}
	}
	return a.GetHosts(), nil
}

func (a *App) ResolveHostSave(sessionID string, save bool) []Host {
	a.pendingHostMu.Lock()
	pending, exists := a.pendingHosts[sessionID]
	delete(a.pendingHosts, sessionID)
	a.pendingHostMu.Unlock()
	if exists && save {
		a.rememberSuccessfulHost(pending.connection, pending.password)
		if a.ctx != nil {
			wailsruntime.EventsEmit(a.ctx, "hosts:changed")
		}
	}
	return a.GetHosts()
}

func (a *App) getHost(id string) (Host, bool) {
	a.hostsMu.Lock()
	defer a.hostsMu.Unlock()
	for _, host := range a.hosts {
		if host.ID == id {
			return host, true
		}
	}
	return Host{}, false
}

func (a *App) hasSavedHost(connection ConnectionCommand) bool {
	a.hostsMu.Lock()
	defer a.hostsMu.Unlock()
	for _, host := range a.hosts {
		if sameHost(host, connection) {
			return true
		}
	}
	return false
}

func (a *App) ToggleFavorite(id string) []Host {
	a.hostsMu.Lock()
	for index := range a.hosts {
		if a.hosts[index].ID == id {
			a.hosts[index].Favorite = !a.hosts[index].Favorite
			break
		}
	}
	_ = a.saveHostsLocked()
	a.hostsMu.Unlock()
	return a.GetHosts()
}

func (a *App) DeleteHost(id string) []Host {
	a.hostsMu.Lock()
	for index := range a.hosts {
		if a.hosts[index].ID == id {
			a.hosts = append(a.hosts[:index], a.hosts[index+1:]...)
			break
		}
	}
	_ = a.saveHostsLocked()
	a.hostsMu.Unlock()
	_ = a.deleteCredential(id)
	return a.GetHosts()
}
