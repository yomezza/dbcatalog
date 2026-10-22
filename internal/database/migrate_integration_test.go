package database

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yomezza/test/internal/security"
)

func testPool(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("CATALOGDB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CATALOGDB_TEST_DATABASE_URL is not set: PostgreSQL integration tests not run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	p, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	if err = p.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	return ctx, p
}
func testSchema(t *testing.T, ctx context.Context, p *pgxpool.Pool) string {
	t.Helper()
	s := "test_" + strings.ReplaceAll(security.ID(), "-", "")
	t.Cleanup(func() {
		_, err := p.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+pgx.Identifier{s}.Sanitize()+" CASCADE")
		if err != nil {
			t.Error(err)
		}
	})
	return s
}
func TestMigrationIsolationRepeatabilityConcurrency(t *testing.T) {
	ctx, p := testPool(t)
	a, b := testSchema(t, ctx, p), testSchema(t, ctx, p)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- Migrate(ctx, p, a) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := Migrate(ctx, p, b); err != nil {
		t.Fatal(err)
	}
	for _, schema := range []string{a, b} {
		var count int
		if err := p.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{schema, "schema_migrations"}.Sanitize()).Scan(&count); err != nil || count != 3 {
			t.Fatalf("ledger count %d: %v", count, err)
		}
	}
	_, err := p.Exec(ctx, "INSERT INTO "+pgx.Identifier{a, "users"}.Sanitize()+"(id,login,password_hash,role,state) VALUES($1,'retained','hash','admin','active')", security.ID())
	if err != nil {
		t.Fatal(err)
	}
	if err = Migrate(ctx, p, a); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		schema string
		want   int
	}{{a, 1}, {b, 0}} {
		var count int
		if err = p.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{tc.schema, "users"}.Sanitize()).Scan(&count); err != nil || count != tc.want {
			t.Fatalf("cross-schema data leak or lost data: %d %v", count, err)
		}
	}
}
func TestFailedMigrationRollsBackAndCanRetry(t *testing.T) {
	ctx, p := testPool(t)
	schema := testSchema(t, ctx, p)
	bad := fstest.MapFS{"migrations/001_ok.sql": {Data: []byte(`CREATE TABLE retained(id integer);`)}, "migrations/002_bad.sql": {Data: []byte(`INSERT INTO table_that_does_not_exist VALUES(1);`)}}
	if err := migrateFS(ctx, p, schema, bad); err == nil {
		t.Fatal("failed SQL accepted")
	}
	var exists bool
	if err := p.QueryRow(ctx, `SELECT EXISTS(SELECT FROM pg_namespace WHERE nspname=$1)`, schema).Scan(&exists); err != nil || exists {
		t.Fatalf("failed migration left partial schema: %v %v", exists, err)
	}
	delete(bad, "migrations/002_bad.sql")
	if err := migrateFS(ctx, p, schema, bad); err != nil {
		t.Fatal("retry failed:", err)
	}
	bad["migrations/002_bad.sql"] = &fstest.MapFile{Data: []byte(`CREATE TABLE should_rollback(id int); SELECT 1/0;`)}
	if err := migrateFS(ctx, p, schema, bad); err == nil {
		t.Fatal("division by zero accepted")
	}
	var count int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema=$1 AND table_name='should_rollback'`, schema).Scan(&count); err != nil || count != 0 {
		t.Fatal("partial DDL persisted", err)
	}
	if err := p.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s.schema_migrations", pgx.Identifier{schema}.Sanitize())).Scan(&count); err != nil || count != 1 {
		t.Fatal("ledger changed after failure", err)
	}
}
func TestMigrationChecksumAndFutureVersion(t *testing.T) {
	ctx, p := testPool(t)
	schema := testSchema(t, ctx, p)
	if err := Migrate(ctx, p, schema); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "UPDATE "+pgx.Identifier{schema, "schema_migrations"}.Sanitize()+" SET checksum='changed' WHERE version=1"); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, p, schema); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatal("changed applied migration not detected", err)
	}
	if _, err := p.Exec(ctx, "INSERT INTO "+pgx.Identifier{schema, "schema_migrations"}.Sanitize()+"(version,name,checksum) VALUES(999,'future','future')"); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, p, schema); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatal("old client accepted newer schema", err)
	}
}

func TestUpgradePreservesExistingData(t *testing.T) {
	ctx, p := testPool(t)
	schema := testSchema(t, ctx, p)
	initial, err := migrationFiles.ReadFile("migrations/001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	old := fstest.MapFS{"migrations/001_initial.sql": {Data: initial}}
	if err = migrateFS(ctx, p, schema, old); err != nil {
		t.Fatal(err)
	}
	id := security.ID()
	if _, err = p.Exec(ctx, "INSERT INTO "+pgx.Identifier{schema, "users"}.Sanitize()+"(id,login,password_hash,role,state) VALUES($1,'keep_me','hash','admin','active')", id); err != nil {
		t.Fatal(err)
	}
	if err = Migrate(ctx, p, schema); err != nil {
		t.Fatal(err)
	}
	var login string
	if err = p.QueryRow(ctx, "SELECT login FROM "+pgx.Identifier{schema, "users"}.Sanitize()+" WHERE id=$1", id).Scan(&login); err != nil || login != "keep_me" {
		t.Fatal("upgrade lost user", err)
	}
	var count int
	if err = p.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{schema, "backup_policies"}.Sanitize()).Scan(&count); err != nil {
		t.Fatal("second migration not applied", err)
	}
}

func TestCurrentSchemaNeedsNoDDLRights(t *testing.T) {
	ctx, p := testPool(t)
	schema := testSchema(t, ctx, p)
	if err := Migrate(ctx, p, schema); err != nil {
		t.Fatal(err)
	}
	role := "reader_" + strings.ReplaceAll(security.ID(), "-", "")
	qr := pgx.Identifier{role}.Sanitize()
	if _, err := p.Exec(ctx, "CREATE ROLE "+qr); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := p.Exec(context.Background(), "DROP OWNED BY "+qr+"; DROP ROLE "+qr); err != nil {
			t.Error(err)
		}
	})
	if _, err := p.Exec(ctx, "GRANT USAGE ON SCHEMA "+pgx.Identifier{schema}.Sanitize()+" TO "+qr+"; GRANT SELECT ON "+pgx.Identifier{schema, "schema_migrations"}.Sanitize()+" TO "+qr); err != nil {
		t.Fatal(err)
	}
	c := p.Config().Copy()
	c.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error { _, err := conn.Exec(ctx, "SET ROLE "+qr); return err }
	limited, err := pgxpool.NewWithConfig(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	defer limited.Close()
	if err = Migrate(ctx, limited, schema); err != nil {
		t.Fatal("up-to-date schema should not request DDL rights:", err)
	}
	if err = Migrate(ctx, limited, schema+"_new"); err == nil {
		t.Fatal("role without CREATE created schema")
	}
}
