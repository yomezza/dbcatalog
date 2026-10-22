package application

import (
	"testing"
	"time"
)

func TestAuditFiltersPaginationAndDeletedNames(t *testing.T) {
	s, _, _ := catalogFixture(t)
	server, _, _, _ := seedDeletionBranch(t, s, "audit-target")
	preview, err := s.PreviewDeletion("server", server.ID, server.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DeleteObjectConfirmed("server", server.ID, server.Version, preview.Fingerprint); err != nil {
		t.Fatal(err)
	}
	page, err := s.GetAudit(AuditFilter{Query: "admin | audit-target | Удаление", Entity: "server", PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Events) != 1 || page.Events[0].ObjectName != "audit-target" {
		t.Fatal("AND search or deleted-name lookup failed", page.Total, len(page.Events))
	}
	next, err := s.GetAudit(AuditFilter{Query: "admin | audit-target | Удаление", Entity: "server", Page: 1, PageSize: 1})
	if err != nil || len(next.Events) != 1 || next.Events[0].ID == page.Events[0].ID {
		t.Fatal("pagination duplicate", err)
	}
	none, err := s.GetAudit(AuditFilter{Query: "admin | NONEXISTENT"})
	if err != nil || none.Total != 0 {
		t.Fatal("AND filter behaved as OR", err)
	}
	only, err := s.GetAudit(AuditFilter{ActorID: s.user.ID, Action: "cascade_delete", From: time.Now().Add(-time.Hour).Format(time.RFC3339), To: time.Now().Add(time.Hour).Format(time.RFC3339)})
	if err != nil || only.Total != 1 || only.Events[0].Details["databases"] == nil {
		t.Fatal("filters or operation details missing", err)
	}
	future, err := s.GetAudit(AuditFilter{From: time.Now().Add(time.Hour).Format(time.RFC3339)})
	if err != nil || future.Total != 0 {
		t.Fatal("date boundary ignored", err)
	}
	if _, err = s.GetAudit(AuditFilter{From: "bad-date"}); err == nil {
		t.Fatal("invalid date accepted")
	}
	if _, err = s.GetAudit(AuditFilter{From: "2026-10-23T00:00:00Z", To: "2026-10-22T00:00:00Z"}); err == nil {
		t.Fatal("reversed dates accepted")
	}
	clamped, err := s.GetAudit(AuditFilter{Page: 100000, PageSize: 1000})
	if err != nil || clamped.PageSize != 100 || clamped.Page < 0 {
		t.Fatal("page size not bounded", err)
	}
}
