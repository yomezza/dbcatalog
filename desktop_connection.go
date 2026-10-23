package main

import (
	"errors"
	"net"
	"strings"
	"time"

	"github.com/yomezza/test/internal/application"
)

// Platform implementations share protocol validation and the connectivity check.
func connectionPort(t application.ServerConnection) (string, error) {
	if t.Host == "" || strings.HasPrefix(t.Host, "-") || strings.ContainsAny(t.Host, " \t\r\n/\\\x00") {
		return "", errors.New("Некорректный адрес сервера")
	}
	switch t.Method {
	case "SSH":
		return "22", nil
	case "RDP":
		return "3389", nil
	default:
		return "", errors.New("Укажите способ подключения RDP или SSH")
	}
}

func checkServerReachable(host, port string) error {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 4*time.Second)
	if err != nil {
		return errors.New("Сервер недоступен на порту " + port + ". Проверьте адрес, сеть и службу")
	}
	return conn.Close()
}
