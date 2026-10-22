package application

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"testing"
)

func TestVersionArchiveBeyondHistoryLimitAndRestoration(t *testing.T) {
	s, _, _ := catalogFixture(t)
	o, err := s.SaveObject(Object{Kind: "server", Name: "versioned", Description: "original"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 102; i++ {
		o.Description = fmt.Sprintf("revision %d", i)
		o, err = s.SaveObject(o)
		if err != nil {
			t.Fatal(err)
		}
	}
	page, err := s.GetVersions("server", o.ID, 999)
	if err != nil || page.Total != 103 || page.Page != 4 || page.CurrentVersion != 103 {
		t.Fatal(page, err)
	}
	snapshot, err := s.GetRecordVersion("server", o.ID, 1)
	if err != nil || snapshot.(map[string]any)["description"] != "original" {
		t.Fatal(snapshot, err)
	}
	h, err := s.GetHistoryPage("server", o.ID, 4)
	if err != nil || h.Total != 103 || len(h.Items) != 3 {
		t.Fatal(h, err)
	}
	if err = s.RestoreVersion("server", o.ID, o.Version, 1); err != nil {
		t.Fatal(err)
	}
	c, _ := s.GetCatalog()
	if c.Servers[0].Version != 104 || c.Servers[0].Description != "original" {
		t.Fatal(c)
	}
	if err = s.RestoreVersion("server", o.ID, o.Version, 2); err == nil {
		t.Fatal("stale restoration accepted")
	}
	old, err := s.GetRecordVersion("server", o.ID, 2)
	if err != nil || old.(map[string]any)["description"] != "revision 1" {
		t.Fatal("intermediate version lost", old, err)
	}
	h, err = s.GetHistoryPage("server", o.ID, 0)
	if err != nil || h.Total != 104 || h.Items[0].Action != "restore" {
		t.Fatal(h, err)
	}
	audit, err := s.GetAudit(AuditFilter{Action: "version_restore"})
	if err != nil || audit.Total != 1 {
		t.Fatal(audit, err)
	}
	current := c.Servers[0]
	if err = s.DeleteObject("server", o.ID, current.Version); err != nil {
		t.Fatal(err)
	}
	page, err = s.GetVersions("server", o.ID, 0)
	if err != nil || page.Exists || page.Total != 104 {
		t.Fatal("archive deleted with object", page, err)
	}
	if err = s.RestoreVersion("server", o.ID, 104, 1); err == nil {
		t.Fatal("deleted object implicitly recreated")
	}
}
func TestPolicyVersionRestorationAndRoleEnforcement(t *testing.T) {
	s, p, schema := catalogFixture(t)
	srv, err := s.SaveObject(Object{Kind: "server", Name: "host"})
	if err != nil {
		t.Fatal(err)
	}
	db, err := s.SaveObject(Object{Kind: "database", Name: "SID", DBMS: "Oracle Database", ParentID: srv.ID})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := s.SavePolicy(Policy{Kind: "database", ObjectID: db.ID, Name: "Original", Enabled: true, BackupType: "Локальное", Scheduler: "cron", Schedule: "daily", Storage: "/backup", RetentionDays: 7})
	if err != nil {
		t.Fatal(err)
	}
	policy.Name = "Changed"
	policy.Enabled = false
	policy, err = s.SavePolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RestoreVersion("policy", policy.ID, policy.Version, 1); err != nil {
		t.Fatal(err)
	}
	policies, err := s.ListPolicies()
	if err != nil || len(policies) != 1 || policies[0].Version != 3 || !policies[0].Enabled || policies[0].Name != "Original" {
		t.Fatal(policies, err)
	}
	versions, err := s.GetVersions("policy", policy.ID, 0)
	if err != nil || versions.Total != 3 || versions.Items[0].Action != "restore" {
		t.Fatal(versions, err)
	}
	if err = s.RestoreVersion("policy", policy.ID, 3, 999); err == nil {
		t.Fatal("unknown version accepted")
	}
	for _, role := range []string{"editor", "viewer"} {
		if _, err = p.Exec(context.Background(), "UPDATE "+pgx.Identifier{schema}.Sanitize()+".users SET role=$2 WHERE id=$1", s.user.ID, role); err != nil {
			t.Fatal(err)
		}
		if _, err = s.GetVersions("policy", policy.ID, 0); err == nil {
			t.Fatal("non-admin listed versions", role)
		}
		if _, err = s.GetRecordVersion("policy", policy.ID, 1); err == nil {
			t.Fatal("non-admin read version", role)
		}
		if err = s.RestoreVersion("policy", policy.ID, 3, 2); err == nil {
			t.Fatal("non-admin restored version", role)
		}
		if _, err = s.GetHistoryPage("policy", policy.ID, 0); err != nil {
			t.Fatal("history should remain readable", err)
		}
	}
}
func TestVersionRestoreRejectsRemovedParentAtomically(t *testing.T) {
	s, _, _ := catalogFixture(t)
	a, _ := s.SaveObject(Object{Kind: "server", Name: "a"})
	b, _ := s.SaveObject(Object{Kind: "server", Name: "b"})
	db, err := s.SaveObject(Object{Kind: "database", Name: "SID", DBMS: "Oracle Database", ParentID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	db.ParentID = b.ID
	db, err = s.SaveObject(db)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteObject("server", a.ID, a.Version); err != nil {
		t.Fatal(err)
	}
	if err = s.RestoreVersion("database", db.ID, db.Version, 1); err == nil {
		t.Fatal("missing parent accepted")
	}
	versions, err := s.GetVersions("database", db.ID, 0)
	if err != nil || versions.Total != 2 || versions.CurrentVersion != 2 {
		t.Fatal("failed restore modified archive", versions, err)
	}
}
