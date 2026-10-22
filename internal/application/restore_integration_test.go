package application

import (
	"context"
	"github.com/jackc/pgx/v5"
	"testing"
)

func TestRestoreExistingSnapshotPreservesLaterHistoryAndVersions(t *testing.T) {
	s, p, schema := catalogFixture(t)
	server, err := s.SaveObject(Object{Kind: "server", Name: "before", Description: "original"})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := s.GetHistory("server", server.ID)
	if err != nil {
		t.Fatal(err)
	}
	createID := entries[0].ID
	server.Name = "after"
	server.Description = "new"
	server, err = s.SaveObject(server)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := s.RestoreObject("server", server.ID, server.Version, createID, "after")
	if err != nil || restored.Name != "before" || restored.Version != 3 {
		t.Fatal(restored, err)
	}
	entries, err = s.GetHistory("server", server.ID)
	if err != nil || len(entries) != 3 || entries[0].Action != "restore" {
		t.Fatal(entries, err)
	}
	if _, err = s.RestoreObject("server", server.ID, server.Version, createID, "after"); err == nil {
		t.Fatal("stale restore accepted")
	}
	other, err := s.SaveObject(Object{Kind: "server", Name: "other"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RestoreObject("server", other.ID, other.Version, createID, "after"); err == nil {
		t.Fatal("cross-object history accepted")
	}
	ctx := context.Background()
	q := pgx.Identifier{schema}.Sanitize()
	if _, err = p.Exec(ctx, "UPDATE "+q+".users SET role='editor' WHERE id=$1", s.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RestoreObject("server", server.ID, restored.Version, createID, "after"); err == nil {
		t.Fatal("non-admin restore accepted")
	}
}
func TestRestoreOracleSnapshotRejectsMissingParent(t *testing.T) {
	s, _, _ := catalogFixture(t)
	a, err := s.SaveObject(Object{Kind: "server", Name: "a"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.SaveObject(Object{Kind: "server", Name: "b"})
	if err != nil {
		t.Fatal(err)
	}
	db, err := s.SaveObject(Object{Kind: "database", ParentID: a.ID, Name: "SID", DBMS: "Oracle Database"})
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := s.GetHistory("database", db.ID)
	oldID := entries[0].ID
	db.ParentID = b.ID
	db, err = s.SaveObject(db)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteObject("server", a.ID, a.Version); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RestoreObject("database", db.ID, db.Version, oldID, "after"); err == nil {
		t.Fatal("missing parent restored")
	}
	c, err := s.GetCatalog()
	if err != nil || len(c.Databases) != 1 || c.Databases[0].ParentID != b.ID {
		t.Fatal("failed restore changed data", c, err)
	}
}
