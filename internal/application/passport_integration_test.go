package application

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestObjectPassportUsesCurrentDataAndEffectivePolicy(t *testing.T) {
	s, p, schema := catalogFixture(t)
	server := mustObject(t, s, Object{Kind: "server", Name: "PDF-DEMO", Address: "192.0.2.15", OS: "Linux", Attributes: Attributes{Location: "Учебная площадка", ConnectionMethod: "SSH"}})
	instance := mustObject(t, s, Object{Kind: "instance", ParentID: server.ID, Name: "pg-main", DBMS: "PostgreSQL", DBMSVersion: "17"})
	db := mustObject(t, s, Object{Kind: "database", ParentID: instance.ID, Name: "Документооборот", Environment: "Промышленный", Attributes: Attributes{Responsible: "Иван Петров", InformationSystem: "Учебная ИС"}})
	policy, err := s.SavePolicy(Policy{Kind: "instance", ObjectID: instance.ID, Name: "Ночная копия", Enabled: true, BackupType: "Централизованное", Scheduler: "cron", Schedule: "0 2 * * *", Storage: "/demo/backups", RetentionDays: 14})
	if err != nil {
		t.Fatal(err)
	}
	load := func(kind, id string) string {
		t.Helper()
		tx, err := s.readTransaction(context.Background(), false, false)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		r, err := buildObjectPassport(context.Background(), tx, kind, id, s.user.Login, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(r)
		return string(raw)
	}
	raw := load("database", db.ID)
	for _, term := range []string{"192.0.2.15", "Иван Петров", "PostgreSQL", "Ночная копия", "Унаследована", "pg-main"} {
		if !strings.Contains(raw, term) {
			t.Error("missing report field", term)
		}
	}
	direct, err := s.SavePolicy(Policy{Kind: "database", ObjectID: db.ID, Name: "Собственная отключена", Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	raw = load("database", db.ID)
	if strings.Contains(raw, policy.Name) || !strings.Contains(raw, direct.Name) || !strings.Contains(raw, "Отключена") {
		t.Fatal("disabled own policy did not override inherited policy", raw)
	}
	oracle := mustObject(t, s, Object{Kind: "database", ParentID: server.ID, Name: "ORCL", DBMS: "Oracle Database", DBMSVersion: "19c"})
	if raw = load("database", oracle.ID); strings.Contains(raw, instance.Name) || !strings.Contains(raw, "Непосредственно на сервере") {
		t.Fatal("Oracle report incorrectly requires an instance", raw)
	}
	if raw = load("server", server.ID); !strings.Contains(raw, "Из них Oracle SID") {
		t.Fatal("missing server composition")
	}
	if raw = load("instance", instance.ID); !strings.Contains(raw, "Базы данных") {
		t.Fatal("missing instance composition")
	}
	// Viewer exports are allowed; blocked users and missing objects must not
	// create files, and the export reads the updated backend record.
	db.Name = "Актуальное имя"
	db = mustObject(t, s, db)
	if err = s.Register("pdfviewer", "viewer-password-123", false); err != nil {
		t.Fatal(err)
	}
	users, _ := s.ListUsers()
	var viewer string
	for _, u := range users {
		if u.Login == "pdfviewer" {
			viewer = u.ID
		}
	}
	if err = s.UpdateUser(viewer, "viewer", "active"); err != nil {
		t.Fatal(err)
	}
	s.Logout()
	s.nextLogin = time.Time{}
	if _, err = s.Login("pdfviewer", "viewer-password-123"); err != nil {
		t.Fatal(err)
	}
	e, err := s.ExportObjectPDF("database", db.ID)
	if err != nil || !strings.Contains(e.Name, db.Name) || e.Category != "PDF-паспорт объекта" {
		t.Fatal("viewer PDF export", e, err)
	}
	_, path, err := s.ResolveDownload(e.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.HasPrefix(string(data), "%PDF-") {
		t.Fatal("not a PDF", err)
	}
	if _, err = s.ExportObjectPDF("database", "00000000-0000-4000-8000-000000000000"); err == nil {
		t.Fatal("missing object exported")
	}
	var count int
	if err = p.QueryRow(context.Background(), "SELECT count(*) FROM "+pgx.Identifier{schema, "audit_events"}.Sanitize()+" WHERE action='pdf_export' AND actor_id=$1 AND object_id=$2", viewer, db.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("PDF audit", count, err)
	}
	if _, err = p.Exec(context.Background(), "UPDATE "+pgx.Identifier{schema, "users"}.Sanitize()+" SET state='blocked' WHERE id=$1", viewer); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExportObjectPDF("database", db.ID); err == nil {
		t.Fatal("blocked user exported PDF")
	}
}

func mustObject(t *testing.T, s *Service, o Object) Object {
	t.Helper()
	out, err := s.SaveObject(o)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
