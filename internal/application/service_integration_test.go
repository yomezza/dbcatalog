package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yomezza/test/internal/config"
	"github.com/yomezza/test/internal/security"
)

type memoryStore map[string]string

func (m memoryStore) Get(s, k string) (string, error) {
	v, ok := m[s+k]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}
func (m memoryStore) Set(s, k, v string) error { m[s+k] = v; return nil }
func TestCatalogWorkflow(t *testing.T) {
	dsn := os.Getenv("CATALOGDB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PostgreSQL integration tests not run")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	pc := admin.Config().ConnConfig
	schema := "app_" + strings.ReplaceAll(security.ID(), "-", "")
	defer func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	c := config.Connection{Host: pc.Host, Port: pc.Port, Database: pc.Database, Username: pc.User, Schema: schema, SSLMode: "disable"}
	s := New(filepath.Join(t.TempDir(), "connection.json"), memoryStore{})
	defer s.Close()
	status, err := s.Connect(c, pc.Password, false, true)
	if err != nil || !status.NeedsAdmin {
		t.Fatal("connect/bootstrap", status, err)
	}
	if err = s.Register("administrator", "long-enough-password", true); err != nil {
		t.Fatal(err)
	}
	if err = s.Register("intruder", "long-enough-password", true); err == nil {
		t.Fatal("second bootstrap accepted")
	}
	if _, err = s.Login("administrator", "long-enough-password"); err != nil {
		t.Fatal(err)
	}
	server, err := s.SaveObject(Object{Kind: "server", Name: "host-a", Address: "10.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	instance, err := s.SaveObject(Object{Kind: "instance", Name: "pg-1", ParentID: server.ID, DBMS: "PostgreSQL"})
	if err != nil {
		t.Fatal(err)
	}
	db, err := s.SaveObject(Object{Kind: "database", Name: "payroll", ParentID: instance.ID})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := s.GetCatalog()
	if err != nil || len(catalog.Servers) != 1 || len(catalog.Instances) != 1 || len(catalog.Databases) != 1 {
		t.Fatal("catalog", catalog, err)
	}
	stale := db
	db.Description = "updated"
	db, err = s.SaveObject(db)
	if err != nil || db.Version != 2 {
		t.Fatal("update", db, err)
	}
	if _, err = s.SaveObject(stale); err == nil {
		t.Fatal("stale update accepted")
	}
	hist, err := s.GetHistory("database", db.ID)
	if err != nil || len(hist) != 2 {
		t.Fatal("history", hist, err)
	}
	if preview, err := s.PreviewDeletion("server", server.ID, server.Version); err != nil || preview.Instances != 1 || preview.Databases != 1 {
		t.Fatal("cascade preview", preview, err)
	}
	if err = s.Register("newuser", "another-long-password", false); err != nil {
		t.Fatal(err)
	}
	users, err := s.ListUsers()
	if err != nil || len(users) != 2 {
		t.Fatal("list users", users, err)
	}
	var newID string
	for _, u := range users {
		if u.Login == "newuser" {
			newID = u.ID
		}
	}
	if err = s.UpdateUser(newID, "viewer", "active"); err != nil {
		t.Fatal(err)
	}
	s.Logout()
	s.nextLogin = time.Time{}
	if _, err = s.Login("newuser", "another-long-password"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.GetCatalog(); err != nil {
		t.Fatal("viewer cannot read", err)
	}
	if _, err = s.SaveObject(Object{Kind: "server", Name: "forbidden"}); err == nil {
		t.Fatal("viewer wrote catalog")
	}
	if _, err = admin.Exec(ctx, "UPDATE "+pgx.Identifier{schema, "users"}.Sanitize()+" SET state='blocked' WHERE id=$1", newID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.GetCatalog(); err == nil {
		t.Fatal("blocked session still accepted")
	}
	// Saved config contains no password and retains the chosen schema.
	raw, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), pc.Password) {
		t.Fatal("password leaked to JSON")
	}
	loaded, err := config.Load(s.path)
	if err != nil || loaded.Schema != schema {
		t.Fatal("schema persistence", err)
	}
}

func TestConcurrentBootstrapAndSchemaSwitch(t *testing.T) {
	dsn := os.Getenv("CATALOGDB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PostgreSQL integration tests not run")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	pc := pool.Config().ConnConfig
	schema := "boot_" + strings.ReplaceAll(security.ID(), "-", "")
	other := schema + "_b"
	defer func() {
		for _, name := range []string{schema, other} {
			if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS "+pgx.Identifier{name}.Sanitize()+" CASCADE"); err != nil {
				t.Error(err)
			}
		}
	}()
	c := config.Connection{Host: pc.Host, Port: pc.Port, Database: pc.Database, Username: pc.User, Schema: schema, SSLMode: "disable"}
	first := New(filepath.Join(t.TempDir(), "one.json"), memoryStore{})
	defer first.Close()
	second := New(filepath.Join(t.TempDir(), "two.json"), memoryStore{})
	defer second.Close()
	for _, s := range []*Service{first, second} {
		if _, err = s.Connect(c, pc.Password, false, true); err != nil {
			t.Fatal(err)
		}
	}
	results := make(chan error, 2)
	go func() { results <- first.Register("admin_one", "long-enough-password", true) }()
	go func() { results <- second.Register("admin_two", "long-enough-password", true) }()
	successes := 0
	for i := 0; i < 2; i++ {
		if <-results == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatal("bootstrap must have exactly one winner", successes)
	}
	var login string
	if err = pool.QueryRow(ctx, "SELECT login FROM "+pgx.Identifier{schema, "users"}.Sanitize()).Scan(&login); err != nil {
		t.Fatal(err)
	}
	if _, err = first.Login(login, "long-enough-password"); err != nil {
		t.Fatal(err)
	}
	c.Schema = other
	if _, err = first.Connect(c, "", true, false); err == nil {
		t.Fatal("old schema credential reused for new schema")
	}
	st, err := first.Connect(c, pc.Password, false, false)
	if err != nil || !st.NeedsAdmin || st.User != nil || st.Schema != other {
		t.Fatal("schema switch must clear session and use isolated new schema", st, err)
	}
}
