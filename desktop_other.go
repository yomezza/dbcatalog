//go:build !windows

package main

import (
	"errors"
	"github.com/yomezza/test/internal/application"
)

func openLocalFile(string, bool) error {
	return errors.New("Открытие файлов поддерживается в Windows-сборке")
}
func startServerConnection(application.ServerConnection) error {
	return errors.New("Подключение к серверу поддерживается в Windows-сборке")
}
