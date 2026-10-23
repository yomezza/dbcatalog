//go:build linux

package main

import (
	"errors"
	"github.com/yomezza/test/internal/security"
	"github.com/zalando/go-keyring"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/yomezza/test/internal/application"
)

func fakeLookup(names ...string) pathLookup {
	return func(name string) (string, error) {
		for _, available := range names {
			if name == available {
				return "/usr/bin/" + name, nil
			}
		}
		return "", errors.New("not installed")
	}
}

func TestLinuxConnectionCommands(t *testing.T) {
	tests := []struct {
		name, method, host string
		installed          []string
		path               string
		args               []string
	}{
		{"FreeRDP IPv6", "RDP", "::1", []string{"xfreerdp3"}, "/usr/bin/xfreerdp3", []string{"/v:[::1]:3389", "/dynamic-resolution"}},
		{"Remmina fallback", "RDP", "server.example", []string{"remmina"}, "/usr/bin/remmina", []string{"-c", "rdp://server.example:3389"}},
		{"Debian terminal", "SSH", "server.example", []string{"ssh", "x-terminal-emulator"}, "/usr/bin/x-terminal-emulator", []string{"-e", "/usr/bin/ssh", "--", "server.example"}},
		{"GNOME terminal", "SSH", "::1", []string{"ssh", "gnome-terminal"}, "/usr/bin/gnome-terminal", []string{"--", "/usr/bin/ssh", "--", "::1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := linuxConnectionCommand(application.ServerConnection{Host: tt.host, Method: tt.method}, fakeLookup(tt.installed...))
			if err != nil {
				t.Fatal(err)
			}
			if got.path != tt.path || !reflect.DeepEqual(got.args, tt.args) {
				t.Fatalf("unexpected command: %#v", got)
			}
		})
	}
}

func TestLinuxMissingClientsAndInvalidTargets(t *testing.T) {
	for _, tt := range []struct {
		host, method string
		installed    []string
	}{
		{"server", "RDP", nil}, {"server", "SSH", nil}, {"server", "SSH", []string{"ssh"}},
		{"-oProxyCommand=bad", "SSH", []string{"ssh", "xterm"}}, {"host/name", "RDP", []string{"xfreerdp"}},
		{"server", "unknown", nil},
	} {
		if _, err := linuxConnectionCommand(application.ServerConnection{Host: tt.host, Method: tt.method}, fakeLookup(tt.installed...)); err == nil {
			t.Fatalf("expected error: %#v", tt)
		}
	}
}

func TestLinuxFilePathsRemainSingleArgument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report $(touch unwanted); test.xlsx")
	for _, reveal := range []bool{false, true} {
		got, err := linuxFileCommand(path, reveal, fakeLookup("xdg-open"))
		if err != nil {
			t.Fatal(err)
		}
		want := path
		if reveal {
			want = filepath.Dir(path)
		}
		if !reflect.DeepEqual(got.args, []string{want}) {
			t.Fatalf("unexpected arguments: %#v", got.args)
		}
	}
	if _, err := linuxFileCommand(path, false, fakeLookup()); err == nil {
		t.Fatal("missing opener must fail")
	}
}

// Opt-in integration check: run within an isolated D-Bus session with an unlocked keyring.
func TestLinuxSecretService(t *testing.T) {
	if os.Getenv("CATALOGDB_TEST_KEYRING") != "1" {
		t.Skip("requires isolated unlocked Secret Service")
	}
	service := "CatalogDB-linux-test-" + security.ID()
	account, secret := "synthetic-account", security.ID()
	store := application.OSStore{}
	if err := store.Set(service, account, secret); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := keyring.Delete(service, account); err != nil {
			t.Error(err)
		}
	})
	got, err := store.Get(service, account)
	if err != nil || got != secret {
		t.Fatalf("Secret Service round trip failed: %v", err)
	}
}

func TestLinuxImmediateLaunchFailure(t *testing.T) {
	if err := startDesktopCommand(desktopCommand{path: "/usr/bin/false"}); err == nil {
		t.Fatal("immediate client failure must be reported")
	}
	if err := startDesktopCommand(desktopCommand{path: "/usr/bin/true"}); err != nil {
		t.Fatal(err)
	}
	if err := startDesktopCommand(desktopCommand{path: "/nonexistent/catalogdb-client"}); err == nil {
		t.Fatal("missing executable must fail")
	}
}
