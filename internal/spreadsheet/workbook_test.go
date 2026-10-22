package spreadsheet

import (
	"bytes"
	"github.com/xuri/excelize/v2"
	"os"
	"testing"
)

func fixture(t *testing.T, columns int) []byte {
	t.Helper()
	wb := excelize.NewFile()
	defer wb.Close()
	for c, h := range Headers[:columns] {
		a, _ := excelize.CoordinatesToCellName(c+1, 1)
		if err := wb.SetCellStr("Sheet1", a, h); err != nil {
			t.Fatal(err)
		}
	}
	for c, v := range []string{"sid", "default", "Эксплуатируется", "Промышленный", "host", "PostgreSQL"} {
		a, _ := excelize.CoordinatesToCellName(c+1, 2)
		_ = wb.SetCellStr("Sheet1", a, v)
	}
	_ = wb.SetCellStr("Sheet1", "B1000", "default")
	if columns == 28 {
		_ = wb.SetCellStr("Sheet1", "AA2", "DO-NOT-STORE-THIS-PASSWORD")
		_ = wb.SetCellStr("Sheet1", "AB2", "OLD-PASSWORD")
	}
	b, e := wb.WriteToBuffer()
	if e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestReadBothFormatsIgnoresTemplateAndSecrets(t *testing.T) {
	for _, n := range []int{21, 28} {
		wb, e := Read(fixture(t, n))
		if e != nil {
			t.Fatal(e)
		}
		if wb.Columns != n || len(wb.Rows) != 1 || wb.Rows[0].Values[0] != "sid" || wb.IgnoredRows != 998 {
			t.Fatal("template rows imported", n, len(wb.Rows), wb.IgnoredRows)
		}
		if n == 28 && wb.SecretRows != 1 {
			t.Fatal("secret presence not reported")
		}
	}
}
func TestExportUsesTextNotFormula(t *testing.T) {
	var r [21]string
	r[0] = "=1+1"
	b, e := Write([][21]string{r})
	if e != nil {
		t.Fatal(e)
	}
	wb, e := excelize.OpenReader(bytes.NewReader(b))
	if e != nil {
		t.Fatal(e)
	}
	defer wb.Close()
	value, e := wb.GetCellValue("Каталог", "A2")
	if e != nil || value != "=1+1" {
		t.Fatal("export altered value", e)
	}
	formula, e := wb.GetCellFormula("Каталог", "A2")
	if e != nil || formula != "" {
		t.Fatal("export produced an executable formula", e)
	}
}
func TestUploadedXLSStructure(t *testing.T) {
	p := os.Getenv("CATALOGDB_XLS_SAMPLE")
	if p == "" {
		t.Skip("optional private XLS sample not supplied")
	}
	data, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	wb, e := Read(data)
	if e != nil {
		t.Fatal(e)
	}
	if wb.Columns != 28 || len(wb.Rows) != 608 {
		t.Fatalf("sample structure mismatch: columns=%d rows=%d ignored=%d", wb.Columns, len(wb.Rows), wb.IgnoredRows)
	}
	t.Logf("XLS verified: %d catalogue rows, %d template rows ignored", len(wb.Rows), wb.IgnoredRows)
}
