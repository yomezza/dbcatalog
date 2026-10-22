package application

import (
	"context"
	"errors"
	"net"
	"regexp"
	"strings"
	"time"
)

type ServerConnection struct {
	Host   string `json:"host"`
	Method string `json:"method"`
}

var dnsHost = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9.-]{0,251}[a-zA-Z0-9])?$`)

func validateServerHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if !dnsHost.MatchString(host) {
		return false
	}
	for _, part := range strings.Split(host, ".") {
		if len(part) == 0 || len(part) > 63 || part[0] == '-' || part[len(part)-1] == '-' {
			return false
		}
	}
	return true
}
func (s *Service) GetServerConnection(kind, id string) (ServerConnection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := ServerConnection{}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	query := `SELECT s.name,s.address,coalesce(s.attributes->>'connectionMethod','') FROM servers s WHERE s.id=$1`
	switch kind {
	case "server":
	case "instance":
		query = `SELECT s.name,s.address,coalesce(s.attributes->>'connectionMethod','') FROM instances i JOIN servers s ON s.id=i.server_id WHERE i.id=$1`
	case "database":
		query = `SELECT s.name,s.address,coalesce(s.attributes->>'connectionMethod','') FROM databases d LEFT JOIN instances i ON i.id=d.instance_id JOIN servers s ON s.id=coalesce(d.server_id,i.server_id) WHERE d.id=$1`
	default:
		return out, errors.New("Неизвестный тип объекта")
	}
	var name, address string
	if err = tx.QueryRow(ctx, query, id).Scan(&name, &address, &out.Method); err != nil {
		return out, errors.New("Сервер не найден")
	}
	out.Host = strings.TrimSpace(address)
	if out.Host == "" {
		out.Host = strings.TrimSpace(name)
	}
	if out.Method != "RDP" && out.Method != "SSH" {
		return out, errors.New("Укажите RDP или SSH в поле «Подключение к серверу» карточки сервера")
	}
	if !validateServerHost(out.Host) {
		return out, errors.New("Укажите корректное имя сервера или IP-адрес без команд, имени пользователя и пути")
	}
	return out, nil
}
