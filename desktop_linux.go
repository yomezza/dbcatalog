//go:build linux

package main

import (
	"errors"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/yomezza/test/internal/application"
)

type desktopCommand struct {
	path string
	args []string
}
type pathLookup func(string) (string, error)

// No shell is involved: file paths and server addresses are separate arguments.
func linuxFileCommand(path string, reveal bool, lookup pathLookup) (desktopCommand, error) {
	opener, err := lookup("xdg-open")
	if err != nil {
		return desktopCommand{}, errors.New("Не найден xdg-open. Установите пакет xdg-utils")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return desktopCommand{}, errors.New("Некорректный путь к файлу")
	}
	if reveal {
		absolute = filepath.Dir(absolute)
	}
	return desktopCommand{opener, []string{absolute}}, nil
}

func linuxConnectionCommand(t application.ServerConnection, lookup pathLookup) (desktopCommand, error) {
	port, err := connectionPort(t)
	if err != nil {
		return desktopCommand{}, err
	}
	if t.Method == "RDP" {
		// FreeRDP accepts both DNS and bracketed IPv6 addresses in /v:.
		for _, client := range []string{"xfreerdp3", "xfreerdp"} {
			if path, err := lookup(client); err == nil {
				return desktopCommand{path, []string{"/v:" + net.JoinHostPort(t.Host, port), "/dynamic-resolution"}}, nil
			}
		}
		if path, err := lookup("remmina"); err == nil {
			return desktopCommand{path, []string{"-c", "rdp://" + net.JoinHostPort(t.Host, port)}}, nil
		}
		return desktopCommand{}, errors.New("Не найден RDP-клиент. Установите FreeRDP (xfreerdp3/xfreerdp) или Remmina")
	}
	ssh, err := lookup("ssh")
	if err != nil {
		return desktopCommand{}, errors.New("Не найден SSH-клиент. Установите openssh-client")
	}
	for _, terminal := range []struct {
		name string
		args []string
	}{
		{"x-terminal-emulator", []string{"-e"}},
		{"gnome-terminal", []string{"--"}},
		{"konsole", []string{"-e"}},
		{"xfce4-terminal", []string{"-x"}},
		{"xterm", []string{"-e"}},
	} {
		if path, err := lookup(terminal.name); err == nil {
			args := append(append([]string{}, terminal.args...), ssh, "--", t.Host)
			return desktopCommand{path, args}, nil
		}
	}
	return desktopCommand{}, errors.New("Не найден терминал для SSH. Установите x-terminal-emulator, GNOME Terminal, Konsole, XFCE Terminal или xterm")
}

func startDesktopCommand(command desktopCommand) error {
	cmd := exec.Command(command.path, command.args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Не удалось запустить %s: %w", filepath.Base(command.path), err)
	}
	// Catch immediate failures (missing handler/display) and reap long-lived clients
	// without waiting for the user to close their terminal or remote session.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("Приложение %s завершилось с ошибкой. Проверьте клиент и настройки рабочего стола: %w", filepath.Base(command.path), err)
		}
		return nil
	case <-time.After(300 * time.Millisecond):
		return nil
	}
}

func openLocalFile(path string, reveal bool) error {
	command, err := linuxFileCommand(path, reveal, exec.LookPath)
	if err != nil {
		return err
	}
	return startDesktopCommand(command)
}

func startServerConnection(t application.ServerConnection) error {
	command, err := linuxConnectionCommand(t, exec.LookPath)
	if err != nil {
		return err
	}
	port, _ := connectionPort(t)
	if err := checkServerReachable(t.Host, port); err != nil {
		return err
	}
	return startDesktopCommand(command)
}
