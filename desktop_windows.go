//go:build windows

package main

import (
	"errors"
	"github.com/yomezza/test/internal/application"
	"golang.org/x/sys/windows"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

// ShellExecute gives console applications a real interactive console with standard input.
func shellOpen(file, params string) error {
	f, err := syscall.UTF16PtrFromString(file)
	if err != nil {
		return err
	}
	p, err := syscall.UTF16PtrFromString(params)
	if err != nil {
		return err
	}
	verb, _ := syscall.UTF16PtrFromString("open")
	result, _, _ := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteW").Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(f)), uintptr(unsafe.Pointer(p)), 0, 1)
	if result <= 32 {
		return errors.New("Windows не смог открыть файл или приложение")
	}
	return nil
}
func openLocalFile(path string, reveal bool) error {
	file, params := path, ""
	if reveal {
		file = "explorer.exe"
		params = "/select," + syscall.EscapeArg(path)
	}
	if err := shellOpen(file, params); err != nil {
		return errors.New("Не удалось открыть файл. Проверьте ассоциацию файлов и наличие подходящего приложения")
	}
	return nil
}
func startServerConnection(t application.ServerConnection) error {
	port, err := connectionPort(t)
	if err != nil {
		return err
	}
	client := "mstsc.exe"
	if t.Method == "SSH" {
		client = "ssh.exe"
	}
	path, err := exec.LookPath(client)
	if err != nil {
		return errors.New("Не найден клиент " + client + ". Установите соответствующий компонент Windows")
	}
	if err := checkServerReachable(t.Host, port); err != nil {
		return err
	}
	if t.Method == "SSH" {
		if err = shellOpen(path, "-- "+syscall.EscapeArg(t.Host)); err != nil {
			return errors.New("Не удалось открыть окно SSH-клиента")
		}
		return nil
	}
	host := t.Host
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if err = shellOpen(path, syscall.EscapeArg("/v:"+host)); err != nil {
		return errors.New("Не удалось запустить клиент " + client)
	}
	return nil
}
