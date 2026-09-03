package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSSHWrapperEnablesSuccessMarker(t *testing.T) {
	directory, marker, _, err := prepareSSHWrapper("test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	output, err := exec.Command(filepath.Join(directory, "ssh"), "-G", "example.org").CombinedOutput()
	if err != nil {
		t.Fatalf("wrapper rejected OpenSSH options: %v\n%s", err, output)
	}
	config := strings.ToLower(string(output))
	if !strings.Contains(config, "permitlocalcommand yes") || !strings.Contains(config, strings.ToLower(marker)) {
		t.Fatalf("wrapper did not install its success marker:\n%s", output)
	}
}

func TestPasswordIsCapturedOnlyAtSSHPrompt(t *testing.T) {
	app := NewApp()
	connection := &ConnectionCommand{Protocol: "ssh", Target: "root@example.org", User: "root", Address: "example.org", Port: 22}
	session := &terminalSession{id: "test", pendingSSH: connection}

	app.inspectTerminalOutput(session, "root@example.org's password: ")
	if !session.capturingPassword {
		t.Fatal("password prompt was not recognised")
	}
	app.inspectTerminalInput(session, "s3cret\r")
	if session.candidatePassword != "s3cret" || session.capturingPassword {
		t.Fatalf("password capture state is wrong: candidate=%q capturing=%v", session.candidatePassword, session.capturingPassword)
	}
	if len(app.GetHosts()) != 0 {
		t.Fatal("host was saved before authentication success")
	}
}

func TestFailedSSHIsNotSaved(t *testing.T) {
	app := NewApp()
	connection := &ConnectionCommand{Protocol: "ssh", Target: "root@example.org", User: "root", Address: "example.org", Port: 22}
	session := &terminalSession{id: "test", pendingSSH: connection, candidatePassword: "wrong"}

	app.inspectTerminalOutput(session, "root@example.org: Permission denied (publickey,password).\r\n")
	if session.pendingSSH != nil || session.candidatePassword != "" {
		t.Fatal("failed SSH connection was left pending")
	}
	if len(app.GetHosts()) != 0 {
		t.Fatal("failed SSH connection was saved")
	}
}

func TestLocalPromptDoesNotConfirmSSH(t *testing.T) {
	app := NewApp()
	connection := &ConnectionCommand{Protocol: "ssh", Target: "root@example.org", User: "root", Address: "example.org", Port: 22}
	session := &terminalSession{id: "test", pendingSSH: connection}

	app.inspectTerminalOutput(session, "➜  ~ ssh root@example.org\r\n➜  ~ ")
	if session.pendingSSH == nil || len(app.GetHosts()) != 0 {
		t.Fatal("local shell prompt incorrectly confirmed the SSH connection")
	}
}

func TestOpenSSHMarkerConfirmsConnection(t *testing.T) {
	app := NewApp()
	app.hostsFile = filepath.Join(t.TempDir(), "hosts.json")
	connection := &ConnectionCommand{Protocol: "ssh", Target: "root@example.org", User: "root", Address: "example.org", Port: 22}
	session := &terminalSession{id: "test", pendingSSH: connection}
	app.confirmSSHConnection(session)

	deadline := time.Now().Add(time.Second)
	pending := false
	for !pending && time.Now().Before(deadline) {
		app.pendingHostMu.Lock()
		_, pending = app.pendingHosts[session.id]
		app.pendingHostMu.Unlock()
		time.Sleep(time.Millisecond)
	}
	if !pending || len(app.GetHosts()) != 0 || session.pendingSSH != nil {
		t.Fatal("new successful SSH connection did not request confirmation")
	}
	app.ResolveHostSave(session.id, true)
	if len(app.GetHosts()) != 1 {
		t.Fatal("confirmed SSH host was not saved")
	}
}
