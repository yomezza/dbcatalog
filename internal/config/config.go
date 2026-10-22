package config

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const DefaultSchema = "dbcatalog"

var schemaPattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// Connection contains no credentials. Passwords belong in the OS credential store.
type Connection struct {
	Host        string `json:"host"`
	Port        uint16 `json:"port"`
	Database    string `json:"database"`
	Username    string `json:"username"`
	Schema      string `json:"schema"`
	SSLMode     string `json:"sslMode"`
	SSLRootCert string `json:"sslRootCert"`
}

func (c Connection) Normalize() Connection {
	c.Host = strings.TrimSpace(c.Host)
	c.Database = strings.TrimSpace(c.Database)
	c.Username = strings.TrimSpace(c.Username)
	c.Schema = strings.TrimSpace(c.Schema)
	if c.Schema == "" {
		c.Schema = DefaultSchema
	}
	if c.Port == 0 {
		c.Port = 5432
	}
	if c.SSLMode == "" {
		c.SSLMode = "verify-full"
	}
	return c
}
func (c Connection) Validate() error {
	if c.Host == "" || c.Database == "" || c.Username == "" {
		return errors.New("Укажите сервер, базу данных и пользователя PostgreSQL")
	}
	if strings.ContainsAny(c.Host, "/\\\x00") {
		return errors.New("Укажите имя сервера или IP-адрес без пути")
	}
	if !schemaPattern.MatchString(c.Schema) || len(c.Schema) > 63 {
		return errors.New("Схема: 1–63 символа, строчные латинские буквы, цифры и подчёркивание; первый символ — буква или подчёркивание")
	}
	if c.Schema == "public" || c.Schema == "information_schema" || strings.HasPrefix(c.Schema, "pg_") {
		return errors.New("Выберите отдельную прикладную схему, не public, information_schema или pg_*")
	}
	switch c.SSLMode {
	case "disable", "require", "verify-ca", "verify-full":
	default:
		return errors.New("Неизвестный режим SSL")
	}
	if c.Port == 0 {
		return errors.New("Некорректный порт")
	}
	return nil
}
func (c Connection) Address() string { return net.JoinHostPort(c.Host, fmt.Sprint(c.Port)) }
func (c Connection) CredentialKey() string {
	// Length-delimited JSON avoids collisions when database/user names contain '/'.
	key, _ := json.Marshal([]string{c.Host, fmt.Sprint(c.Port), c.Database, c.Username, c.Schema})
	return fmt.Sprintf("connection-%x", sha256.Sum256(key))
}
func Path() (string, error) {
	var root string
	if runtime.GOOS == "windows" {
		root = os.Getenv("LOCALAPPDATA")
	} else {
		var err error
		root, err = os.UserConfigDir()
		if err != nil {
			return "", err
		}
	}
	if root == "" {
		return "", errors.New("Не найден каталог настроек пользователя")
	}
	return filepath.Join(root, "DBCatalog", "connection.json"), nil
}
func Load(path string) (Connection, error) {
	var c Connection
	data, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(data, &c); err != nil {
		return c, errors.New("Некорректный файл настройки подключения")
	}
	c = c.Normalize()
	return c, c.Validate()
}
func Save(path string, c Connection) error {
	c = c.Normalize()
	if err := c.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "connection-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, path)
}
