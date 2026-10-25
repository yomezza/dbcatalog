package application

import (
	"context"
	"errors"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/yomezza/test/internal/buildinfo"
	"github.com/yomezza/test/internal/config"
	"github.com/yomezza/test/internal/database"
	"github.com/zalando/go-keyring"
)

type credentialObservation struct {
	sync.Mutex
	State     string
	CheckedAt string
}
type observedCredentials struct {
	CredentialStore
	health *credentialObservation
}

func (s observedCredentials) record(err error) {
	state := "available"
	if err != nil && !errors.Is(err, keyring.ErrNotFound) && !errors.Is(err, os.ErrNotExist) {
		state = "error"
	}
	s.health.Lock()
	defer s.health.Unlock()
	s.health.State = state
	s.health.CheckedAt = time.Now().UTC().Format(time.RFC3339)
}
func (s observedCredentials) Get(service, key string) (string, error) {
	v, err := s.CredentialStore.Get(service, key)
	s.record(err)
	return v, err
}
func (s observedCredentials) Set(service, key, value string) error {
	err := s.CredentialStore.Set(service, key, value)
	s.record(err)
	return err
}

type ListenerDiagnostic struct {
	State       string `json:"state"`
	ConnectedAt string `json:"connectedAt"`
	LastEventAt string `json:"lastEventAt"`
}
type Diagnostics struct {
	GeneratedAt         string                         `json:"generatedAt"`
	AppVersion          string                         `json:"appVersion"`
	GoVersion           string                         `json:"goVersion"`
	WailsVersion        string                         `json:"wailsVersion"`
	Platform            string                         `json:"platform"`
	Revision            string                         `json:"revision"`
	Modified            bool                           `json:"modified"`
	ConfigState         string                         `json:"configState"`
	CredentialBackend   string                         `json:"credentialBackend"`
	CredentialState     string                         `json:"credentialState"`
	CredentialCheckedAt string                         `json:"credentialCheckedAt"`
	DatabaseState       string                         `json:"databaseState"`
	PostgreSQLVersion   string                         `json:"postgresqlVersion"`
	ReadOnly            bool                           `json:"readOnly"`
	TLS                 bool                           `json:"tls"`
	PoolConnections     int32                          `json:"poolConnections"`
	PoolMaximum         int32                          `json:"poolMaximum"`
	MigrationState      string                         `json:"migrationState"`
	Migrations          []database.MigrationDiagnostic `json:"migrations"`
	Listener            ListenerDiagnostic             `json:"listener"`
}

// GetDiagnostics is available before login and when disconnected. Only explicitly
// selected technical fields are returned: no DSN, settings dump, identity, paths,
// credential values, SQL payloads, server errors or record contents are exported.
func (s *Service) GetDiagnostics() Diagnostics {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := Diagnostics{GeneratedAt: time.Now().UTC().Format(time.RFC3339), AppVersion: buildinfo.Version, GoVersion: runtime.Version(), Platform: runtime.GOOS + "/" + runtime.GOARCH, ConfigState: "missing", CredentialBackend: "Системное хранилище", CredentialState: "unexamined", DatabaseState: "disconnected", MigrationState: "unavailable", Migrations: []database.MigrationDiagnostic{}}
	if runtime.GOOS == "windows" {
		d.CredentialBackend = "Windows Credential Manager"
	} else if runtime.GOOS == "linux" {
		d.CredentialBackend = "Secret Service (D-Bus)"
	} else if runtime.GOOS == "android" {
		d.CredentialBackend = "Android Keystore"
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if dep.Path == "github.com/wailsapp/wails/v2" {
				d.WailsVersion = dep.Version
			}
		}
		for _, v := range info.Settings {
			if v.Key == "vcs.revision" {
				d.Revision = v.Value
			}
			if v.Key == "vcs.modified" {
				d.Modified = v.Value == "true"
			}
		}
	}
	if _, err := config.Load(s.path); err == nil {
		d.ConfigState = "valid"
	} else if !os.IsNotExist(err) {
		d.ConfigState = "error"
	}
	if s.credentialHealth != nil {
		s.credentialHealth.Lock()
		d.CredentialState = s.credentialHealth.State
		d.CredentialCheckedAt = s.credentialHealth.CheckedAt
		s.credentialHealth.Unlock()
	}
	s.live.healthMu.Lock()
	d.Listener = s.live.health
	s.live.healthMu.Unlock()
	if d.Listener.State == "" {
		d.Listener.State = "stopped"
	}
	if s.pool == nil {
		return d
	}
	stat := s.pool.Stat()
	d.PoolConnections = stat.TotalConns()
	d.PoolMaximum = stat.MaxConns()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d.DatabaseState = "error"
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return d
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	err = tx.QueryRow(ctx, `SELECT current_setting('server_version'),pg_is_in_recovery() OR current_setting('default_transaction_read_only')='on',coalesce((SELECT ssl FROM pg_catalog.pg_stat_ssl WHERE pid=pg_backend_pid()),false)`).Scan(&d.PostgreSQLVersion, &d.ReadOnly, &d.TLS)
	if err != nil {
		return d
	}
	d.DatabaseState = "connected"
	rows, err := database.InspectMigrations(ctx, tx)
	if err != nil {
		d.MigrationState = "error"
		return d
	}
	d.Migrations = rows
	d.MigrationState = "current"
	for _, m := range rows {
		if m.State != "applied" {
			d.MigrationState = "attention"
		}
	}
	return d
}
