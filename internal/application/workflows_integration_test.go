package application

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/xuri/excelize/v2"
	"os"
	"strings"
	"testing"
)

func workflowBranch(t *testing.T, s *Service) (Object, Object, []Object) {
	t.Helper()
	server, e := s.SaveObject(Object{Kind: "server", Name: "host"})
	if e != nil {
		t.Fatal(e)
	}
	instance, e := s.SaveObject(Object{Kind: "instance", Name: "pg", ParentID: server.ID, DBMS: "PostgreSQL"})
	if e != nil {
		t.Fatal(e)
	}
	dbs := []Object{}
	for _, name := range []string{"one", "two"} {
		db, e := s.SaveObject(Object{Kind: "database", Name: name, ParentID: instance.ID, Environment: "Промышленный"})
		if e != nil {
			t.Fatal(e)
		}
		dbs = append(dbs, db)
	}
	return server, instance, dbs
}
func TestBulkPreviewAtomicityStaleSelectionAndFreshPermissions(t *testing.T) {
	s, p, schema := catalogFixture(t)
	_, _, dbs := workflowBranch(t, s)
	refs := []ObjectRef{{dbs[0].ID, dbs[0].Version}, {dbs[1].ID, dbs[1].Version}}
	patch := BulkPatch{SetResponsible: true, Responsible: "Новый ответственный", SetAdministration: true, Administration: []string{"Мониторинг и контроль"}}
	preview, e := s.PreviewBulkEdit(refs, patch)
	if e != nil || preview.Changed != 2 {
		t.Fatal(preview, e)
	}
	c, _ := s.GetCatalog()
	if c.Databases[0].Attributes.Responsible != "" {
		t.Fatal("preview wrote data")
	}
	if n, e := s.ApplyBulkEdit(refs, patch, "wrong"); e == nil || n != 0 {
		t.Fatal("invalid confirmation accepted")
	}
	n, e := s.ApplyBulkEdit(refs, patch, preview.Fingerprint)
	if e != nil || n != 2 {
		t.Fatal(n, e)
	}
	c, e = s.GetCatalog()
	if e != nil {
		t.Fatal(e)
	}
	for _, d := range c.Databases {
		if d.Attributes.Responsible != patch.Responsible || d.Version != 2 || len(d.Attributes.Administration) != 1 {
			t.Fatal(d)
		}
	}
	if _, e = s.ApplyBulkEdit(refs, patch, preview.Fingerprint); e == nil {
		t.Fatal("stale selection accepted")
	}
	refs = []ObjectRef{{c.Databases[0].ID, c.Databases[0].Version}}
	preview, e = s.PreviewBulkEdit(refs, BulkPatch{SetResponsible: true})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = p.Exec(context.Background(), "UPDATE "+schema+".users SET role='viewer' WHERE id=$1", s.user.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.ApplyBulkEdit(refs, BulkPatch{SetResponsible: true}, preview.Fingerprint); e == nil {
		t.Fatal("role downgrade ignored")
	}
}
func TestPersonalFiltersViewerScopeAndOptimisticVersion(t *testing.T) {
	s, p, schema := catalogFixture(t)
	f, e := s.SaveFilterPreset(FilterPreset{Name: "Мой", Criteria: CatalogCriteria{Query: "host | one", Policy: "missing"}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.SaveFilterPreset(FilterPreset{Name: " мой "}); e == nil {
		t.Fatal("duplicate preset accepted")
	}
	if _, e = p.Exec(context.Background(), "UPDATE "+schema+".users SET role='viewer' WHERE id=$1", s.user.ID); e != nil {
		t.Fatal(e)
	}
	saved, e := s.SaveFilterPreset(f)
	if e != nil || saved.Version != 2 {
		t.Fatal(saved, e)
	}
	if e = s.DeleteFilterPreset(f.ID, f.Version); e == nil {
		t.Fatal("stale delete accepted")
	}
	if e = s.DeleteFilterPreset(saved.ID, saved.Version); e != nil {
		t.Fatal(e)
	}
	if e = s.Register("other-reader", "long-password-123", false); e != nil {
		t.Fatal(e)
	}
	// Row ownership is enforced even if a caller supplies another user's preset UUID.
	var other string
	if e = p.QueryRow(context.Background(), "SELECT id::text FROM "+schema+".users WHERE login='other-reader'").Scan(&other); e != nil {
		t.Fatal(e)
	}
	f, e = s.SaveFilterPreset(FilterPreset{Name: "Личный"})
	if e != nil {
		t.Fatal(e)
	}
	s.user = &User{ID: other}
	if _, e = p.Exec(context.Background(), "UPDATE "+schema+".users SET state='active',role='viewer' WHERE id=$1", other); e != nil {
		t.Fatal(e)
	}
	list, e := s.ListFilterPresets()
	if e != nil || len(list) != 0 {
		t.Fatal(list, e)
	}
	if _, e = s.SaveFilterPreset(f); e == nil {
		t.Fatal("another user's preset updated")
	}
}
func TestQualityEffectivePoliciesAndScopedDuplicateSIDs(t *testing.T) {
	s, _, _ := catalogFixture(t)
	server, instance, dbs := workflowBranch(t, s)
	_, e := s.SavePolicy(Policy{Kind: "instance", ObjectID: instance.ID, Name: "Общая", Enabled: true, BackupType: "Локальное", Scheduler: "cron", Schedule: "02:00 ежедневно", Storage: "/backup", RetentionDays: 7})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.SavePolicy(Policy{Kind: "database", ObjectID: dbs[0].ID, Name: "Выключена"})
	if e != nil {
		t.Fatal(e)
	}
	page, e := s.GetQualityReport("", 0)
	if e != nil {
		t.Fatal(e)
	}
	if page.Counts["missing_responsible"] != 2 || page.Counts["missing_address"] != 1 || page.Counts["production_no_policy"] != 0 || page.Counts["production_disabled_policy"] != 1 {
		t.Fatal(page)
	}
	_, e = s.SaveObject(Object{Kind: "database", DBMS: "Oracle Database", ParentID: server.ID, Name: "one"})
	if e != nil {
		t.Fatal(e)
	}
	page, e = s.GetQualityReport("similar_sid", 0)
	if e != nil || page.Total != 0 {
		t.Fatal("SID across parents treated as duplicate", page, e)
	}
}
func TestDeletedBranchPreviewConflictAndVersionContinuity(t *testing.T) {
	s, _, _ := catalogFixture(t)
	server, instance, dbs := workflowBranch(t, s)
	policy, e := s.SavePolicy(Policy{Kind: "instance", ObjectID: instance.ID, Name: "Политика"})
	if e != nil {
		t.Fatal(e)
	}
	oracle, e := s.SaveObject(Object{Kind: "database", Name: "ORCL", DBMS: "Oracle Database", ParentID: server.ID})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.DeleteObject("server", server.ID, server.Version); e != nil {
		t.Fatal(e)
	}
	deleted, e := s.ListDeleted(0)
	if e != nil || deleted.Total != 6 {
		t.Fatal(deleted, e)
	}
	child, e := s.PreviewDeletedRestore("database", dbs[0].ID, false)
	if e != nil || len(child.Conflicts) == 0 {
		t.Fatal("missing parent allowed", child, e)
	}
	preview, e := s.PreviewDeletedRestore("server", server.ID, true)
	if e != nil || len(preview.Items) != 6 || len(preview.Conflicts) > 0 {
		t.Fatal(preview, e)
	}
	c, _ := s.GetCatalog()
	if len(c.Servers) != 0 {
		t.Fatal("preview resurrected data")
	}
	deleted, _ = s.ListDeleted(0)
	if deleted.Total != 6 {
		t.Fatal("preview marked restored")
	}
	conflict, e := s.SaveObject(Object{Kind: "server", Name: "HOST"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.RestoreDeleted("server", server.ID, true, preview.Fingerprint); e == nil {
		t.Fatal("stale preview accepted")
	}
	conflicted, e := s.PreviewDeletedRestore("server", server.ID, true)
	if e != nil || len(conflicted.Conflicts) == 0 {
		t.Fatal("name conflict missed", conflicted, e)
	}
	if e = s.DeleteObject("server", conflict.ID, conflict.Version); e != nil {
		t.Fatal(e)
	}
	preview, e = s.PreviewDeletedRestore("server", server.ID, true)
	if e != nil || len(preview.Conflicts) > 0 {
		t.Fatal(preview, e)
	}
	n, e := s.RestoreDeleted("server", server.ID, true, preview.Fingerprint)
	if e != nil || n != 6 {
		t.Fatal(n, e)
	}
	c, e = s.GetCatalog()
	if e != nil || len(c.Servers) != 1 || len(c.Instances) != 1 || len(c.Databases) != 3 {
		t.Fatal(c, e)
	}
	for _, o := range append(append(c.Servers, c.Instances...), c.Databases...) {
		if o.Version != 2 {
			t.Fatal("version continuity lost", o)
		}
	}
	policies, e := s.ListPolicies()
	if e != nil || len(policies) != 1 || policies[0].ID != policy.ID || policies[0].Version != 2 {
		t.Fatal(policies, e)
	}
	if _, e = s.RestoreDeleted("database", oracle.ID, false, preview.Fingerprint); e == nil {
		t.Fatal("restored tombstone reused")
	}
	versions, e := s.GetVersions("database", dbs[0].ID, 0)
	if e != nil || versions.Total != 2 {
		t.Fatal(versions, e)
	}
	// Delete and recover one database again; current parent remains and version continues.
	if e = s.DeleteObject("database", dbs[0].ID, 2); e != nil {
		t.Fatal(e)
	}
	preview, e = s.PreviewDeletedRestore("database", dbs[0].ID, false)
	if e != nil || len(preview.Conflicts) > 0 {
		t.Fatal(preview, e)
	}
	if _, e = s.RestoreDeleted("database", dbs[0].ID, false, preview.Fingerprint); e != nil {
		t.Fatal(e)
	}
	versions, _ = s.GetVersions("database", dbs[0].ID, 0)
	if versions.CurrentVersion != 3 {
		t.Fatal(versions)
	}
}
func TestSelectedExportContainsOnlyRequestedRowsAndKeepsDownloads(t *testing.T) {
	s, _, _ := catalogFixture(t)
	_, _, dbs := workflowBranch(t, s)
	entry, e := s.ExportSelection([]string{dbs[1].ID})
	if e != nil {
		t.Fatal(e)
	}
	_, path, e := s.ResolveDownload(entry.ID)
	if e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	wb, e := excelize.OpenReader(bytes.NewReader(data))
	if e != nil {
		t.Fatal(e)
	}
	defer wb.Close()
	rows, e := wb.GetRows("Каталог")
	if e != nil || len(rows) != 2 || rows[1][0] != "two" {
		t.Fatal(rows, e)
	}
	if _, e = s.ExportSelection([]string{}); e == nil {
		t.Fatal("empty selection exported all")
	}
	if _, e = s.ExportSelection([]string{dbs[0].ID, dbs[0].ID}); e == nil {
		t.Fatal("duplicate selection accepted")
	}
	if e = s.DeleteObject("database", dbs[0].ID, dbs[0].Version); e != nil {
		t.Fatal(e)
	}
	if _, e = s.ExportSelection([]string{dbs[0].ID}); e == nil {
		t.Fatal("deleted selection silently omitted")
	}
}
func policyWorkbook(t *testing.T, mutate func(*excelize.File)) string {
	t.Helper()
	data, e := base64.StdEncoding.DecodeString(importFixture(t, 28, "one", "two"))
	if e != nil {
		t.Fatal(e)
	}
	wb, e := excelize.OpenReader(bytes.NewReader(data))
	if e != nil {
		t.Fatal(e)
	}
	defer wb.Close()
	for _, cell := range []string{"Y2", "Y3"} {
		if e = wb.SetCellStr("Sheet1", cell, "Включена"); e != nil {
			t.Fatal(e)
		}
	}
	if mutate != nil {
		mutate(wb)
	}
	buf, e := wb.WriteToBuffer()
	if e != nil {
		t.Fatal(e)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}
func TestImportPoliciesExplicitDefaultsCorrectionConflictAndAtomicSave(t *testing.T) {
	s, _, _ := catalogFixture(t)
	preview, e := s.PreviewExcel(policyWorkbook(t, nil))
	if e != nil {
		t.Fatal(e)
	}
	cfg := PolicyImportConfig{Enabled: true, TargetKind: "database"}
	preview, e = s.PreviewExcelPolicies(preview.Token, cfg)
	if e != nil || preview.Valid != 0 || len(preview.Policies) != 2 {
		t.Fatal(preview, e)
	}
	cfg.RetentionUnit = "days"
	cfg.Defaults.Schedule = "Ежедневно 02:00"
	preview, e = s.PreviewExcelPolicies(preview.Token, cfg)
	if e != nil || preview.Valid != 2 {
		t.Fatal(preview, e)
	}
	catalog, _ := s.GetCatalog()
	if len(catalog.Servers) > 0 {
		t.Fatal("policy preview wrote catalog")
	}
	if _, e = s.ApplyExcel(preview.Token, false, true); e == nil {
		t.Fatal("policy confirmation bypassed")
	}
	raw, _ := json.Marshal(preview)
	if strings.Contains(string(raw), "PASSWORD") {
		t.Fatal("archive secrets leaked")
	}
	result, e := s.ApplyExcel(preview.Token, false, false)
	if e != nil || result.PoliciesCreated != 2 || result.DatabasesCreated != 2 {
		t.Fatal(result, e)
	}
	policies, e := s.ListPolicies()
	if e != nil || len(policies) != 2 || policies[0].RetentionDays != 7 || !policies[0].Enabled {
		t.Fatal(policies, e)
	}
	// A policy changed after preview cannot be overwritten by a stale import.
	preview, e = s.PreviewExcel(policyWorkbook(t, nil))
	if e != nil {
		t.Fatal(e)
	}
	preview, e = s.PreviewExcelPolicies(preview.Token, cfg)
	if e != nil || preview.Policies[0].Before == nil {
		t.Fatal(preview, e)
	}
	policies[0].Description = "Другое изменение"
	if _, e = s.SavePolicy(policies[0]); e != nil {
		t.Fatal(e)
	}
	if _, e = s.ApplyExcel(preview.Token, false, false); e == nil {
		t.Fatal("concurrent policy change overwritten")
	}
	// Instance assignment deduplicates identical rows, but differing paths are errors for every affected row.
	preview, e = s.PreviewExcel(policyWorkbook(t, func(wb *excelize.File) { wb.SetCellStr("Sheet1", "W3", "/different") }))
	if e != nil {
		t.Fatal(e)
	}
	cfg.TargetKind = "instance"
	preview, e = s.PreviewExcelPolicies(preview.Token, cfg)
	if e != nil || preview.Valid != 0 {
		t.Fatal("policy conflicts not caught", preview, e)
	}
	cfg.Overrides = []PolicyImportOverride{{Row: 3, Skip: true}}
	preview, e = s.PreviewExcelPolicies(preview.Token, cfg)
	if e != nil || preview.Valid != 2 {
		t.Fatal(preview, e)
	}
	result, e = s.ApplyExcel(preview.Token, false, false)
	if e != nil || result.PoliciesCreated != 1 {
		t.Fatal(result, e)
	}
}

func TestDeletedPolicyRestoreConflictAndAdministratorOnly(t *testing.T) {
	s, p, schema := catalogFixture(t)
	_, _, dbs := workflowBranch(t, s)
	old, e := s.SavePolicy(Policy{Kind: "database", ObjectID: dbs[0].ID, Name: "Старая"})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.DeletePolicy(old.ID, old.Version); e != nil {
		t.Fatal(e)
	}
	preview, e := s.PreviewDeletedRestore("policy", old.ID, false)
	if e != nil || len(preview.Conflicts) != 0 {
		t.Fatal(preview, e)
	}
	newer, e := s.SavePolicy(Policy{Kind: "database", ObjectID: dbs[0].ID, Name: "Новая"})
	if e != nil {
		t.Fatal(e)
	}
	conflict, e := s.PreviewDeletedRestore("policy", old.ID, false)
	if e != nil || len(conflict.Conflicts) == 0 {
		t.Fatal("assignment conflict not caught", conflict, e)
	}
	if _, e = s.RestoreDeleted("policy", old.ID, false, conflict.Fingerprint); e == nil {
		t.Fatal("duplicate policy restored")
	}
	policies, _ := s.ListPolicies()
	if len(policies) != 1 || policies[0].ID != newer.ID {
		t.Fatal("restore changed catalog on failure")
	}
	if _, e = p.Exec(context.Background(), "UPDATE "+schema+".users SET role='editor' WHERE id=$1", s.user.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.ListDeleted(0); e == nil {
		t.Fatal("editor read deleted snapshots")
	}
	if _, e = s.PreviewDeletedRestore("policy", old.ID, false); e == nil {
		t.Fatal("editor previewed restoration")
	}
	if _, e = s.RestoreDeleted("policy", old.ID, false, preview.Fingerprint); e == nil {
		t.Fatal("editor restored deleted object")
	}
}
func TestImportPolicyOverridesRetainConfigurationAfterExclusionsAndRejectOracleInstance(t *testing.T) {
	s, _, _ := catalogFixture(t)
	preview, e := s.PreviewExcel(policyWorkbook(t, func(wb *excelize.File) {
		wb.SetCellStr("Sheet1", "F2", "Oracle Database")
		wb.SetCellStr("Sheet1", "B2", "")
	}))
	if e != nil {
		t.Fatal(e)
	}
	cfg := PolicyImportConfig{Enabled: true, TargetKind: "instance", RetentionUnit: "days", Defaults: Policy{Schedule: "02:00"}}
	preview, e = s.PreviewExcelPolicies(preview.Token, cfg)
	if e != nil || preview.Valid != 1 {
		t.Fatal(preview, e)
	}
	cfg.Overrides = []PolicyImportOverride{{Row: 2, Policy: Policy{Kind: "database", Name: "Oracle", Enabled: true, BackupType: "Локальное", Scheduler: "cron", Schedule: "03:00", Storage: "/backup", RetentionDays: 14}}}
	preview, e = s.PreviewExcelPolicies(preview.Token, cfg)
	if e != nil || preview.Valid != 2 {
		t.Fatal(preview, e)
	}
	preview, e = s.SetImportRowsExcluded(preview.Token, []int{3}, true)
	if e != nil || !preview.WithPolicies || preview.Valid != 1 || len(preview.Policies) != 1 || preview.Policies[0].Policy.RetentionDays != 14 {
		t.Fatal(preview, e)
	}
	result, e := s.ApplyExcel(preview.Token, false, false)
	if e != nil || result.PoliciesCreated != 1 || result.DatabasesCreated != 1 || result.InstancesCreated != 0 {
		t.Fatal(result, e)
	}
}
