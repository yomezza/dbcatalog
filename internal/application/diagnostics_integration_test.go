package application

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestDiagnosticsChecksActualLedgerAndListener(t *testing.T) {
	s, p, schema := catalogFixture(t)
	ctx := context.Background()
	d := s.GetDiagnostics()
	if d.DatabaseState != "connected" || d.MigrationState != "current" || len(d.Migrations) != 16 || d.PostgreSQLVersion == "" {
		t.Fatalf("unexpected database report: %+v", d)
	}
	ledger := pgx.Identifier{schema, "schema_migrations"}.Sanitize()
	var original string
	if err := p.QueryRow(ctx, "SELECT checksum FROM "+ledger+" WHERE version=1").Scan(&original); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "UPDATE "+ledger+" SET checksum='corrupt-test-checksum' WHERE version=1"); err != nil {
		t.Fatal(err)
	}
	d = s.GetDiagnostics()
	if d.MigrationState != "attention" || d.Migrations[0].State != "mismatch" {
		t.Fatal("migration corruption was not detected")
	}
	var actual string
	if err := p.QueryRow(ctx, "SELECT checksum FROM "+ledger+" WHERE version=1").Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if actual != "corrupt-test-checksum" {
		t.Fatal("diagnostics changed the ledger")
	}
	if _, err := p.Exec(ctx, "UPDATE "+ledger+" SET checksum=$1 WHERE version=1", original); err != nil {
		t.Fatal(err)
	}
	events := make(chan Change, 8)
	s.SetChangeHandler(func(c Change) {
		select {
		case events <- c:
		default:
		}
	})
	select {
	case <-events:
	case <-time.After(5 * time.Second):
		t.Fatal("listener did not connect")
	}
	d = s.GetDiagnostics()
	if d.Listener.State != "listening" || d.Listener.ConnectedAt == "" {
		t.Fatalf("listener status: %+v", d.Listener)
	}
	s.Close()
	if d = s.GetDiagnostics(); d.Listener.State != "stopped" || d.DatabaseState != "disconnected" {
		t.Fatal("stale connected state after close")
	}
}
