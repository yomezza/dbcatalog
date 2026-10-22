package application

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yomezza/test/internal/config"
	"github.com/yomezza/test/internal/security"
)

func catalogFixture(t *testing.T) (*Service, *pgxpool.Pool, string) {
	t.Helper()
	dsn := os.Getenv("CATALOGDB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PostgreSQL integration tests not run")
	}
	ctx := context.Background()
	p, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	schema := "fields_" + strings.ReplaceAll(security.ID(), "-", "")
	t.Cleanup(func() {
		if _, err := p.Exec(ctx, "DROP SCHEMA IF EXISTS "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	pc := p.Config().ConnConfig
	c := config.Connection{Host: pc.Host, Port: pc.Port, Database: pc.Database, Username: pc.User, Schema: schema, SSLMode: "disable"}
	s := New(filepath.Join(t.TempDir(), "connection.json"), memoryStore{})
	t.Cleanup(s.Close)
	if _, err = s.Connect(c, pc.Password, false, false); err != nil {
		t.Fatal(err)
	}
	if err = s.Register("admin", "long-enough-password", true); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Login("admin", "long-enough-password"); err != nil {
		t.Fatal(err)
	}
	return s, p, schema
}
func TestCatalogFieldsDictionariesAndPersonalFavorites(t *testing.T) {
	s, p, schema := catalogFixture(t)
	refs, err := s.GetLookups()
	if err != nil {
		t.Fatal(err)
	}
	if len(refs["status"]) != 10 || len(refs["administration"]) != 9 || len(refs["backupType"]) != 3 {
		t.Fatal("dictionary values missing")
	}
	server, err := s.SaveObject(Object{Kind: "server", Name: "host", Environment: "Промышленный", Attributes: Attributes{Location: "Собственный ЦОД", Domain: "oms.admsurgut.ru", ConnectionMethod: "SSH", Supervisor: "Руководитель", Responsible: "Администратор"}})
	if err != nil {
		t.Fatal(err)
	}
	instance, err := s.SaveObject(Object{Kind: "instance", Name: "instance", ParentID: server.ID, DBMS: "PostgreSQL"})
	if err != nil {
		t.Fatal(err)
	}
	db, err := s.SaveObject(Object{Kind: "database", Name: "sid", ParentID: instance.ID, Environment: "Тестовый", Attributes: Attributes{Administration: []string{"Мониторинг и контроль", "Резервное копирование", "Резервное копирование"}, ServiceOrganization: "Своя организация", InformationSystem: "ИС"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(db.Attributes.Administration) != 2 {
		t.Fatal("multiselect was not normalized")
	}
	c, err := s.GetCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if c.Servers[0].Attributes.Location != "Собственный ЦОД" || c.Databases[0].Environment != "Тестовый" || len(c.Databases[0].Attributes.Administration) != 2 {
		t.Fatal("fields lost on round trip")
	}
	refs, err = s.GetLookups()
	if err != nil || !slices.Contains(refs["responsible"], "Администратор") || !slices.Contains(refs["supervisor"], "Руководитель") || !slices.Contains(refs["serviceOrganization"], "Своя организация") {
		t.Fatal("DISTINCT suggestions missing", err)
	}
	invalid := db
	invalid.Attributes.Administration = []string{"arbitrary value"}
	if _, err = s.SaveObject(invalid); err == nil {
		t.Fatal("invalid dictionary value accepted")
	}
	for _, o := range []Object{server, instance, db} {
		if err = s.SetFavorite(o.Kind, o.ID, true); err != nil {
			t.Fatal(err)
		}
		if err = s.SetFavorite(o.Kind, o.ID, true); err != nil {
			t.Fatal("idempotent add", err)
		}
	}
	favorites, err := s.ListFavorites()
	if err != nil || len(favorites) != 3 {
		t.Fatal("favorites", len(favorites), err)
	}
	if err = s.Register("viewer", "long-enough-password", false); err != nil {
		t.Fatal(err)
	}
	users, err := s.ListUsers()
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range users {
		if u.Login == "viewer" {
			if err = s.UpdateUser(u.ID, "viewer", "active"); err != nil {
				t.Fatal(err)
			}
		}
	}
	s.Logout()
	s.nextLogin = time.Time{}
	if _, err = s.Login("viewer", "long-enough-password"); err != nil {
		t.Fatal(err)
	}
	favorites, err = s.ListFavorites()
	if err != nil || len(favorites) != 0 {
		t.Fatal("another user's favorites leaked", err)
	}
	if err = s.SetFavorite("database", db.ID, true); err != nil {
		t.Fatal("viewer cannot use own favorites", err)
	}
	if err = s.SetFavorite("server", security.ID(), true); err == nil {
		t.Fatal("nonexistent object favorited")
	}
	if err = s.SetFavorite("arbitrary", db.ID, true); err == nil {
		t.Fatal("unknown kind accepted")
	}
	s.Logout()
	s.nextLogin = time.Time{}
	if _, err = s.Login("admin", "long-enough-password"); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteObject("database", db.ID, db.Version); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = p.QueryRow(context.Background(), "SELECT count(*) FROM "+pgx.Identifier{schema, "favorites"}.Sanitize()+" WHERE database_id=$1", db.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("deleted object left stale favorites", err)
	}
	if err = s.SetFavorite("server", server.ID, false); err != nil {
		t.Fatal(err)
	}
	if err = s.SetFavorite("server", server.ID, false); err != nil {
		t.Fatal(err)
	}
	favorites, err = s.ListFavorites()
	if err != nil || len(favorites) != 1 || favorites[0].Kind != "instance" {
		t.Fatal("personal removal failed", err)
	}
}
