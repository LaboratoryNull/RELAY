package main

import (
	"path/filepath"
	"testing"
)

func TestTrackCommand(t *testing.T) {
	app := NewApp()
	app.hostsFile = filepath.Join(t.TempDir(), "hosts.json")

	tests := []struct {
		command, protocol, target, user, address string
		port                                     int
	}{
		{"ssh root@19.4.2.1", "ssh", "root@19.4.2.1", "root", "19.4.2.1", 22},
		{"ssh -p 2222 deploy@example.org", "ssh", "deploy@example.org", "deploy", "example.org", 2222},
		{"sftp -P 2200 files.local", "sftp", "files.local", "", "files.local", 2200},
		{"ssh -vvv -J jump.example -i ~/.ssh/prod -p2223 deploy@example.org", "ssh", "deploy@example.org", "deploy", "example.org", 2223},
		{"ssh -l admin -o Port=2201 -o StrictHostKeyChecking=no example.org", "ssh", "example.org", "admin", "example.org", 2201},
		{"ssh -oUser=service -oPort=2202 example.org uptime", "ssh", "example.org", "service", "example.org", 2202},
		{"ssh -P production-tag root@example.org", "ssh", "root@example.org", "root", "example.org", 22},
	}
	for _, test := range tests {
		got := app.TrackCommand(test.command)
		if got == nil || got.Protocol != test.protocol || got.Target != test.target || got.User != test.user || got.Address != test.address || got.Port != test.port {
			t.Fatalf("TrackCommand(%q) = %#v", test.command, got)
		}
	}
	if got := app.TrackCommand("echo ssh root@example.org"); got != nil {
		t.Fatalf("non-ssh command was tracked: %#v", got)
	}
	if len(app.GetHosts()) != 0 {
		t.Fatalf("commands must not be saved before authentication, got %d hosts", len(app.GetHosts()))
	}
	connection := app.TrackCommand("ssh root@19.4.2.1")
	app.rememberSuccessfulHost(*connection, "")
	if len(app.GetHosts()) != 1 {
		t.Fatalf("successful SSH connection was not saved")
	}
}

func TestUpsertHostPreservesFullSSHCommand(t *testing.T) {
	app := NewApp()
	app.hostsFile = filepath.Join(t.TempDir(), "hosts.json")
	command := "ssh -J bastion -i ~/.ssh/prod -o StrictHostKeyChecking=accept-new -p 2222 deploy@example.org"
	hosts, err := app.UpsertHost(Host{Name: "Production", Command: command})
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0].Name != "Production" || hosts[0].Command != command {
		t.Fatalf("saved host = %#v", hosts)
	}
	if hosts[0].User != "deploy" || hosts[0].Address != "example.org" || hosts[0].Port != 2222 {
		t.Fatalf("parsed host fields = %#v", hosts[0])
	}
}

func TestHostsWithDifferentSSHOptionsRemainDistinct(t *testing.T) {
	app := NewApp()
	app.hostsFile = filepath.Join(t.TempDir(), "hosts.json")
	commands := []string{
		"ssh -J bastion-a deploy@example.org",
		"ssh -J bastion-b deploy@example.org",
	}
	for _, command := range commands {
		if _, err := app.UpsertHost(Host{Command: command}); err != nil {
			t.Fatal(err)
		}
	}
	hosts := app.GetHosts()
	if len(hosts) != 2 || hosts[0].ID == hosts[1].ID {
		t.Fatalf("hosts with distinct options were merged: %#v", hosts)
	}
}

func TestGetHostsReturnsEmptyArray(t *testing.T) {
	app := NewApp()
	hosts := app.GetHosts()
	if hosts == nil || len(hosts) != 0 {
		t.Fatalf("empty host history must be a non-nil empty slice, got %#v", hosts)
	}
}
