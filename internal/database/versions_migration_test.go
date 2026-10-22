package database

import (
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/yomezza/test/internal/security"
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestVersionMigrationBackfillsHistoryAndCurrentBaselineAtomically(t *testing.T) {
	ctx, p := testPool(t)
	schema := testSchema(t, ctx, p)
	old := fstest.MapFS{}
	entries, _ := fs.ReadDir(migrationFiles, "migrations")
	for _, e := range entries {
		if e.Name() < "007" {
			raw, _ := migrationFiles.ReadFile("migrations/" + e.Name())
			old["migrations/"+e.Name()] = &fstest.MapFile{Data: raw}
		}
	}
	if err := migrateFS(ctx, p, schema, old); err != nil {
		t.Fatal(err)
	}
	q := pgx.Identifier{schema}.Sanitize()
	uid, id, deleted := security.ID(), security.ID(), security.ID()
	if _, err := p.Exec(ctx, "INSERT INTO "+q+".users(id,login,password_hash,role,state)VALUES($1,'admin','hash','admin','active')", uid); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "INSERT INTO "+q+".servers(id,name,description,version,updated_by)VALUES($1,'current','baseline',3,$2)", id, uid); err != nil {
		t.Fatal(err)
	}
	for version := 1; version <= 2; version++ {
		raw, _ := json.Marshal(map[string]any{"id": id, "kind": "server", "name": "historical", "description": "history", "version": version})
		if _, err := p.Exec(ctx, "INSERT INTO "+q+".object_history(entity,object_id,action,actor_id,after_state)VALUES('server',$1,'update',$2,$3)", id, uid, raw); err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := json.Marshal(map[string]any{"id": deleted, "kind": "server", "name": "deleted", "version": 1})
	if _, err := p.Exec(ctx, "INSERT INTO "+q+".object_history(entity,object_id,action,actor_id,before_state)VALUES('server',$1,'delete',$2,$3)", deleted, uid, raw); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, p, schema); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := p.QueryRow(ctx, "SELECT count(*) FROM "+q+".record_versions WHERE object_id=$1", id).Scan(&count); err != nil || count != 3 {
		t.Fatal("missing historical or baseline versions", count, err)
	}
	var kind, description string
	if err := p.QueryRow(ctx, "SELECT snapshot->>'kind',snapshot->>'description' FROM "+q+".record_versions WHERE object_id=$1 AND version=3", id).Scan(&kind, &description); err != nil || kind != "server" || description != "baseline" {
		t.Fatal(kind, description, err)
	}
	if err := p.QueryRow(ctx, "SELECT count(*) FROM "+q+".record_versions WHERE object_id=$1", deleted).Scan(&count); err != nil || count != 1 {
		t.Fatal("deleted snapshots lost", count, err)
	}
	// An invalid duplicate snapshot must abort its surrounding data update.
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)

	if _, err = tx.Exec(ctx, "UPDATE "+q+".servers SET version=4 WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	duplicate, _ := json.Marshal(map[string]any{"id": id, "kind": "server", "name": "forged", "version": 2})
	if _, err = tx.Exec(ctx, "INSERT INTO "+q+".object_history(entity,object_id,action,actor_id,after_state)VALUES('server',$1,'update',$2,$3)", id, uid, duplicate); err == nil {
		t.Fatal("duplicate version overwrote archive")
	}
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "23505" {
		t.Fatal("unexpected trigger error", err)
	}
	tx.Rollback(ctx)
	if err = p.QueryRow(ctx, "SELECT version FROM "+q+".servers WHERE id=$1", id).Scan(&count); err != nil || count != 3 {
		t.Fatal("failed version capture did not roll back data", count, err)
	}
	if err = Migrate(ctx, p, schema); err != nil {
		t.Fatal("repeat migration", err)
	}
}
