package application

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/xuri/excelize/v2"
	"github.com/yomezza/test/internal/spreadsheet"
)

func importFixture(t *testing.T, columns int, sids ...string) string {
	t.Helper()
	wb := excelize.NewFile()
	defer wb.Close()
	for c, h := range spreadsheet.Headers[:columns] {
		addr, _ := excelize.CoordinatesToCellName(c+1, 1)
		if err := wb.SetCellStr("Sheet1", addr, h); err != nil {
			t.Fatal(err)
		}
	}
	for r, sid := range sids {
		values := []string{sid, "default", "Эксплуатируется", "Промышленный", "host", "PostgreSQL", "17", "SSH", "отсутствует", "10.0.0.1", `МКУ "УИТС г. Сургута"`, "ИС", "Организация", "Руководитель", "Ответственный", "Linux", "1", "Linux", "Назначение", "Мониторинг и контроль;Резервное копирование", "Нет", "Локальное", "/backups", "cron", "Отключен", "7", "DO-NOT-IMPORT-PASSWORD", "OLD-PASSWORD"}
		if r == 1 {
			values[4] = "HOST"
		}
		for c, v := range values[:columns] {
			addr, _ := excelize.CoordinatesToCellName(c+1, r+2)
			if err := wb.SetCellStr("Sheet1", addr, v); err != nil {
				t.Fatal(err)
			}
		}
	}
	buf, err := wb.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}
func TestImportPreviewConfirmationIdempotenceAndConflicts(t *testing.T) {
	s, _, _ := catalogFixture(t)
	encoded := importFixture(t, 28, "one", "two", "one")
	preview, err := s.PreviewExcel(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Total != 3 || preview.Valid != 2 || preview.SecretsIgnored != 3 {
		t.Fatal("preview mismatch")
	}
	jsonPreview, _ := json.Marshal(preview)
	jsonPlan, _ := json.Marshal(s.pendingImport.items[0].database)
	if bytes.Contains(jsonPreview, []byte("DO-NOT-IMPORT")) || bytes.Contains(jsonPlan, []byte("DO-NOT-IMPORT")) {
		t.Fatal("password leaked into preview or plan")
	}
	if _, err = s.ApplyExcel(preview.Token, false, true); err == nil {
		t.Fatal("invalid rows silently skipped")
	}
	if _, err = s.ApplyExcel(preview.Token, true, false); err == nil {
		t.Fatal("catalog-only warning not acknowledged")
	}
	c, err := s.GetCatalog()
	if err != nil || len(c.Servers) != 0 {
		t.Fatal("preview wrote data", err)
	}
	result, err := s.ApplyExcel(preview.Token, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.ServersCreated != 1 || result.InstancesCreated != 1 || result.DatabasesCreated != 2 || result.Skipped != 1 {
		t.Fatal("result mismatch", result)
	}
	if _, err = s.ApplyExcel(preview.Token, true, true); err == nil {
		t.Fatal("applied plan reused")
	}
	preview, err = s.PreviewExcel(encoded)
	if err != nil {
		t.Fatal(err)
	}
	result, err = s.ApplyExcel(preview.Token, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.ServersCreated+result.InstancesCreated+result.DatabasesCreated+result.ObjectsUpdated != 0 {
		t.Fatal("repeat import not idempotent", result)
	}
	c, err = s.GetCatalog()
	if err != nil {
		t.Fatal(err)
	}
	preview, err = s.PreviewExcel(encoded)
	if err != nil {
		t.Fatal(err)
	}
	db := c.Databases[0]
	db.Description = "concurrent edit"
	if _, err = s.SaveObject(db); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApplyExcel(preview.Token, true, true); err == nil {
		t.Fatal("stale import overwrote concurrent edit")
	}
	preview, err = s.PreviewExcel(encoded)
	if err != nil {
		t.Fatal(err)
	}
	s.Logout()
	s.nextLogin = time.Time{}
	if _, err = s.Login("admin", "long-enough-password"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApplyExcel(preview.Token, true, true); err == nil {
		t.Fatal("plan survived session change")
	}
}
func TestImportRollbackAndExportWithDuplicateServerNames(t *testing.T) {
	s, p, schema := catalogFixture(t)
	ctx := context.Background()
	quoted := pgx.Identifier{schema}.Sanitize()
	_, err := p.Exec(ctx, `CREATE FUNCTION `+quoted+`.reject_import() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.name='reject' THEN RAISE EXCEPTION 'test rejection'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_import BEFORE INSERT ON `+quoted+`.databases FOR EACH ROW EXECUTE FUNCTION `+quoted+`.reject_import()`)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.PreviewExcel(importFixture(t, 21, "one", "reject"))
	if err != nil || preview.Valid != 2 {
		t.Fatal("preview", err)
	}
	if _, err = s.ApplyExcel(preview.Token, false, false); err == nil {
		t.Fatal("database rejection ignored")
	}
	c, err := s.GetCatalog()
	if err != nil || len(c.Servers)+len(c.Instances)+len(c.Databases) != 0 {
		t.Fatal("partial import persisted", err)
	}
	if _, err = p.Exec(ctx, `DROP TRIGGER reject_import ON `+quoted+`.databases`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApplyExcel(preview.Token, false, false); err != nil {
		t.Fatal("retry after correction", err)
	}
	server, err := s.SaveObject(Object{Kind: "server", Name: "host", Address: "10.0.0.2"})
	if err != nil {
		t.Fatal(err)
	}
	instance, err := s.SaveObject(Object{Kind: "instance", Name: "other", ParentID: server.ID, DBMS: "PostgreSQL"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveObject(Object{Kind: "database", Name: "=not-a-formula", ParentID: instance.ID}); err != nil {
		t.Fatal(err)
	}
	exported, err := s.ExportExcel()
	if err != nil {
		t.Fatal(err)
	}
	data, err := base64.StdEncoding.DecodeString(exported)
	if err != nil {
		t.Fatal(err)
	}
	wb, err := spreadsheet.Read(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(wb.Rows) != 3 {
		t.Fatal("duplicate hostname lost databases in export", len(wb.Rows))
	}
	found := false
	for _, r := range wb.Rows {
		if r.Values[0] == "=not-a-formula" {
			found = true
			if r.Values[9] != "10.0.0.2" {
				t.Fatal("export mixed server identities")
			}
		}
	}
	if !found {
		t.Fatal("text cell beginning with = was lost")
	}
	preview, err = s.PreviewExcel(importFixture(t, 21, "new"))
	if err != nil || preview.Valid != 0 {
		t.Fatal("ambiguous hostname accepted", err)
	}
}
func TestPrivateXLSPreview(t *testing.T) {
	path := os.Getenv("CATALOGDB_XLS_SAMPLE")
	if path == "" {
		t.Skip("optional private XLS sample not supplied")
	}
	s, _, _ := catalogFixture(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.PreviewExcel(base64.StdEncoding.EncodeToString(data))
	if err != nil {
		t.Fatal(err)
	}
	if preview.Columns != 28 || preview.Total != 608 || preview.Ignored != 64235 {
		t.Fatal("private file structure mismatch")
	}
	c, err := s.GetCatalog()
	if err != nil || len(c.Servers) != 0 {
		t.Fatal("private preview must not write customer records", err)
	}
	counts := map[string]int{}
	for _, r := range preview.Rows {
		for _, e := range r.Errors {
			if strings.Contains(e, "справочника") {
				counts["dictionary"]++
			} else {
				counts["identity/conflict"]++
			}
		}
	}
	t.Logf("Private XLS: total=%d valid=%d ignored=%d issue categories=%v; no customer data imported", preview.Total, preview.Valid, preview.Ignored, counts)
}

func TestImportRejectsRecreatedIdentityWithSameVersion(t *testing.T) {
	s, _, _ := catalogFixture(t)
	data := importFixture(t, 21, "one")
	preview, err := s.PreviewExcel(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApplyExcel(preview.Token, false, false); err != nil {
		t.Fatal(err)
	}
	preview, err = s.PreviewExcel(data)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := s.GetCatalog()
	if err != nil {
		t.Fatal(err)
	}
	old := catalog.Databases[0]
	if err = s.DeleteObject("database", old.ID, old.Version); err != nil {
		t.Fatal(err)
	}
	replacement := old
	replacement.ID = ""
	replacement.Version = 0
	replacement, err = s.SaveObject(replacement)
	if err != nil {
		t.Fatal(err)
	}
	if old.Version != replacement.Version || old.ID == replacement.ID {
		t.Fatal("test must recreate an identity with the same version")
	}
	if _, err = s.ApplyExcel(preview.Token, false, false); err == nil {
		t.Fatal("import accepted a replaced identity")
	}
}
