package application

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/yomezza/test/internal/downloads"
	"github.com/yomezza/test/internal/reports"
)

func TestSelectedPDFSnapshotPoliciesPermissionsAndAtomicFailures(t *testing.T) {
	s, p, schema := catalogFixture(t)
	ctx := context.Background()
	server := mustObject(t, s, Object{Kind: "server", Name: "A-DEMO", Address: "192.0.2.10", Description: "Описание сервера целиком", Attributes: Attributes{Responsible: "Ответственный сервера", Supervisor: "Руководитель сервера"}})
	instance := mustObject(t, s, Object{Kind: "instance", Name: "PG-DEMO", ParentID: server.ID, DBMS: "PostgreSQL", DBMSVersion: "17"})
	db := mustObject(t, s, Object{Kind: "database", Name: "copy", ParentID: instance.ID, Description: "Описание базы целиком", Attributes: Attributes{InformationSystem: "ИС на русском", Administration: []string{"Резервное копирование"}}})
	other := mustObject(t, s, Object{Kind: "server", Name: "B-DEMO", Address: "192.0.2.11"})
	oracle := mustObject(t, s, Object{Kind: "database", Name: "copy", ParentID: other.ID, DBMS: "Oracle Database", DBMSVersion: "19c"})
	huge := mustObject(t, s, Object{Kind: "database", Name: "HUGE-TEXT", ParentID: instance.ID, Description: strings.Repeat("Long text with all content. ", 500) + "END-OF-LONG-DESCRIPTION"})
	policy, err := s.SavePolicy(Policy{Kind: "instance", ObjectID: instance.ID, Name: "POLICY-PRIVATE-MARKER", Enabled: true, BackupType: "Централизованное", Scheduler: "cron", Schedule: "0 2 * * *", Storage: "/ONLY-POLICY-STORAGE", RetentionDays: 14})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SavePolicy(Policy{Kind: "database", ObjectID: oracle.ID, Name: "ORACLE-POLICY-MARKER", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	// Establish a repeatable-read snapshot, then change a parent and a policy
	// from another client. Both batch queries must continue seeing the old state.
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM servers`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Exec(ctx, "UPDATE "+pgx.Identifier{schema, "servers"}.Sanitize()+" SET address='192.0.2.25' WHERE id=$1", server.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Exec(ctx, "UPDATE "+pgx.Identifier{schema, "backup_policies"}.Sanitize()+" SET name='POLICY-NEW-MARKER' WHERE id=$1", policy.ID); err != nil {
		t.Fatal(err)
	}
	items, err := loadSelectionPassports(ctx, tx, []string{oracle.ID, db.ID}, s.user.Login, time.Now())
	if err != nil || len(items) != 2 {
		t.Fatal(items, err)
	}
	raw, _ := json.Marshal(items)
	if !strings.Contains(string(raw), "192.0.2.10") || strings.Contains(string(raw), "192.0.2.25") || !strings.Contains(string(raw), "POLICY-PRIVATE-MARKER") {
		t.Fatal("batch mixed different snapshots", string(raw))
	}
	if items[0].Path != "A-DEMO / PG-DEMO / copy" || items[1].Path != "B-DEMO / Oracle Database / copy" {
		t.Fatal("same SID on different parents was collapsed or misordered", items)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	// A viewer can print the selection. IDs are resolved in PostgreSQL, without
	// depending on loaded tree pages or combining equal SID names.
	if _, err = p.Exec(ctx, "UPDATE "+pgx.Identifier{schema, "users"}.Sanitize()+" SET role='viewer' WHERE id=$1", s.user.ID); err != nil {
		t.Fatal(err)
	}
	read := func(entry downloads.Entry) []byte {
		t.Helper()
		_, path, err := s.ResolveDownload(entry.ID)
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	entry, err := s.ExportSelectionPDF([]string{oracle.ID, db.ID}, PDFOptions{Layout: "compact", IncludePolicies: true})
	if err != nil {
		t.Fatal(err)
	}
	data := read(entry)
	if n := len(regexp.MustCompile(`/Type /Page\b`).FindAll(data, -1)); n != 2 {
		t.Fatalf("wanted two database pages, got %d", n)
	}
	if bin, e := exec.LookPath("pdftotext"); e == nil {
		assertText := func(data []byte, present, absent []string) {
			t.Helper()
			path := filepath.Join(t.TempDir(), "selected.pdf")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			raw, err := exec.Command(bin, "-enc", "UTF-8", "-raw", path, "-").CombinedOutput()
			if err != nil {
				t.Fatal(err, string(raw))
			}
			for _, s := range present {
				if !strings.Contains(string(raw), s) {
					t.Error("PDF missing", s)
				}
			}
			for _, s := range absent {
				if strings.Contains(string(raw), s) {
					t.Error("excluded policy leaked", s)
				}
			}
		}
		assertText(data, []string{"192.0.2.25", "POLICY-NEW-MARKER", "ORACLE-POLICY-MARKER", "Описание сервера целиком", "Ответственный сервера"}, nil)
		for _, layout := range []string{"compact", "detailed"} {
			e, err := s.ExportSelectionPDF([]string{db.ID, oracle.ID}, PDFOptions{Layout: layout, IncludePolicies: false})
			if err != nil {
				t.Fatal(err)
			}
			assertText(read(e), []string{"copy", "192.0.2.25"}, []string{"POLICY-NEW-MARKER", "ORACLE-POLICY-MARKER", "ONLY-POLICY-STORAGE", "Политика РК", "Состояние политики"})
		}
	}
	before, _ := s.ListDownloads()
	for _, ids := range [][]string{nil, {db.ID, db.ID}, {db.ID, "00000000-0000-4000-8000-000000000000"}, make([]string, MaxPDFDatabases+1)} {
		if _, err = s.ExportSelectionPDF(ids, PDFOptions{Layout: "compact", IncludePolicies: true}); err == nil {
			t.Fatal("invalid selection accepted", ids)
		}
	}
	if _, err = s.ExportSelectionPDF([]string{db.ID}, PDFOptions{Layout: "unknown"}); err == nil {
		t.Fatal("invalid layout accepted")
	}
	if _, err = s.ExportSelectionPDF([]string{db.ID, huge.ID}, PDFOptions{Layout: "compact", IncludePolicies: false}); !errors.Is(err, reports.ErrCompactOverflow) {
		t.Fatal("oversize compact must refuse, never truncate", err)
	}
	after, _ := s.ListDownloads()
	if len(after) != len(before) {
		t.Fatal("failed batch left a partial file")
	}
	if _, err = s.ExportSelectionPDF([]string{db.ID, huge.ID}, PDFOptions{Layout: "detailed", IncludePolicies: false}); err != nil {
		t.Fatal("detailed fallback failed", err)
	}
	if _, err = p.Exec(ctx, "UPDATE "+pgx.Identifier{schema, "users"}.Sanitize()+" SET state='blocked' WHERE id=$1", s.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExportSelectionPDF([]string{db.ID}, PDFOptions{Layout: "compact"}); err == nil {
		t.Fatal("blocked user printed")
	}
}
