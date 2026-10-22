package database

import (
	"github.com/yomezza/test/internal/config"
	"testing"
)

func TestDSNOptionsCannotBeInjected(t *testing.T) {
	c := config.Connection{Host: "localhost", Port: 5432, Database: "db?sslmode=disable", Username: "user", Schema: "team_a", SSLMode: "verify-full"}
	password := "p@ss' sslmode=disable&x=1\\"
	p, err := PoolConfig(c, password)
	if err != nil {
		t.Fatal(err)
	}
	if p.ConnConfig.Password != password || p.ConnConfig.Database != c.Database || p.ConnConfig.TLSConfig == nil {
		t.Fatal("DSN options were altered")
	}
	if p.ConnConfig.RuntimeParams["search_path"] != `"team_a", pg_catalog, pg_temp` {
		t.Fatal("selected schema not propagated")
	}
}

func TestMigrationStateMessages(t *testing.T) {
	if PublicError(&MigrationStateError{Version: 999, Reason: "newer schema"}) == PublicError(&MigrationStateError{Version: 1, Reason: "checksum mismatch"}) {
		t.Fatal("incompatible schema diagnoses must differ")
	}
}
