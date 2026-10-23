//go:build !windows && !linux

package main

import (
	"errors"
	"github.com/yomezza/test/internal/application"
)

func openLocalFile(string, bool) error {
	return errors.New("Открытие файлов поддерживается только в Windows и Linux")
}
func startServerConnection(application.ServerConnection) error {
	return errors.New("Подключение к серверу поддерживается только в Windows и Linux")
}
