package application

import (
	"bytes"
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/xuri/excelize/v2"
	"os"
	"testing"
	"time"
)

func TestImportExclusionRechecksDuplicatesAndCanBeUndone(t *testing.T) {
	s, _, _ := catalogFixture(t)
	preview, err := s.PreviewExcel(importFixture(t, 28, "one", "one", "two"))
	if err != nil || preview.Valid != 2 {
		t.Fatal(preview, err)
	}
	next, err := s.SetImportRowsExcluded(preview.Token, []int{2}, true)
	if err != nil || next.Excluded != 1 || next.Total != 2 || next.Valid != 2 || next.Rows[0].Row != 3 || next.BackupRows != 2 || next.SecretsIgnored != 2 {
		t.Fatal(next, err)
	}
	if _, err = s.ApplyExcel(preview.Token, true, true); err == nil {
		t.Fatal("stale pre-exclusion token accepted")
	}
	if _, err = s.SetImportRowsExcluded(next.Token, []int{999}, true); err == nil {
		t.Fatal("unknown row accepted")
	}
	if _, err = s.RevalidateExcel(next.Token, []ImportRowEdit{{Row: 2, Values: next.ExcludedRows[0].Values}}); err == nil {
		t.Fatal("excluded row edited without restoring")
	}
	next, err = s.SetImportRowsExcluded(next.Token, []int{3, 4}, true)
	if err != nil || next.Total != 0 || next.Valid != 0 || next.Excluded != 3 {
		t.Fatal(next, err)
	}
	if _, err = s.ApplyExcel(next.Token, false, true); err == nil {
		t.Fatal("empty import accepted")
	}
	next, err = s.SetImportRowsExcluded(next.Token, []int{2, 4}, false)
	if err != nil || next.Valid != 2 || next.Total != 2 || next.Excluded != 1 {
		t.Fatal(next, err)
	}
	file, err := s.ExportImportPreview(next.Token)
	if err != nil {
		t.Fatal(err)
	}
	_, path, err := s.ResolveDownload(file.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wb, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := wb.GetRows("Каталог")
	wb.Close()
	if err != nil || len(rows) != 3 {
		t.Fatal("excluded rows exported", rows, err)
	}
	result, err := s.ApplyExcel(next.Token, false, true)
	if err != nil || result.DatabasesCreated != 2 || result.Excluded != 1 || result.Skipped != 0 {
		t.Fatal(result, err)
	}
}
func TestImportExclusionRequiresCurrentRoleAndPreview(t *testing.T) {
	s, p, schema := catalogFixture(t)
	preview, err := s.PreviewExcel(importFixture(t, 21, "one"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Exec(context.Background(), "UPDATE "+pgx.Identifier{schema}.Sanitize()+".users SET role='viewer' WHERE id=$1", s.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetImportRowsExcluded(preview.Token, []int{2}, true); err == nil {
		t.Fatal("viewer revised import")
	}
	if s.pendingImport.preview.Token != preview.Token || s.pendingImport.preview.Excluded != 0 {
		t.Fatal("rejected exclusion modified preview")
	}
	s.pendingImport.expires = time.Now().Add(-time.Second)
	if _, err = s.SetImportRowsExcluded(preview.Token, []int{2}, true); err == nil {
		t.Fatal("expired plan revised")
	}
}
