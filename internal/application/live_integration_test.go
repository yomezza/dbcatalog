package application

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

func TestLiveEventsCommitRollbackSchemaIsolationAndReconnect(t *testing.T) {
	s, p, schema := catalogFixture(t)
	ctx := context.Background()
	events := make(chan Change, 100)
	s.SetChangeHandler(func(c Change) {
		select {
		case events <- c:
		default:
		}
	})
	wait := func(kind string) {
		t.Helper()
		deadline := time.NewTimer(6 * time.Second)
		defer deadline.Stop()
		for {
			select {
			case c := <-events:
				if c.Type == kind && c.Schema == schema {
					return
				}
			case <-deadline.C:
				t.Fatal("missing live event", kind)
			}
		}
	}
	quiet := func() {
		t.Helper()
		select {
		case c := <-events:
			t.Fatal("unexpected live event", c)
		case <-time.After(120 * time.Millisecond):
		}
	}
	wait("resync")
	tx, err := s.readTransaction(ctx, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.saveObjectTx(ctx, tx, Object{Kind: "server", Name: "commit-event"}); err != nil {
		t.Fatal(err)
	}
	quiet()
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	wait("catalog_changed")
	tx, err = s.readTransaction(ctx, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.saveObjectTx(ctx, tx, Object{Kind: "server", Name: "rollback-event"}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	quiet()
	raw, _ := json.Marshal(Change{Schema: schema + "_other", Type: "catalog_changed"})
	if _, err = p.Exec(ctx, `SELECT pg_notify('catalogdb_events',$1)`, string(raw)); err != nil {
		t.Fatal(err)
	}
	quiet()
	// Reconnect emits resync so missed signals never become the only source of truth.
	var pid int
	if err = p.QueryRow(ctx, `SELECT pid FROM pg_stat_activity WHERE application_name=$1 LIMIT 1`, "CatalogDB.Listener:"+schema).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Exec(ctx, `SELECT pg_terminate_backend($1)`, pid); err != nil {
		t.Fatal(err)
	}
	wait("resync")
	// Statement signals with the same payload coalesce even for a transaction with many writes.
	tx, err = s.readTransaction(ctx, true, false)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 3; n++ {
		if _, err = tx.Exec(ctx, `UPDATE servers SET description=description||'x'`); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	wait("catalog_changed")
	quiet()
	if _, err = p.Exec(ctx, "UPDATE "+pgx.Identifier{schema, "servers"}.Sanitize()+" SET description='qualified SQL'"); err != nil {
		t.Fatal(err)
	}
	wait("catalog_changed")
}
