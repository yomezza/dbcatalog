package reports

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestPassportEmbedsCyrillicFontsAndPaginatesLongValues(t *testing.T) {
	in := Passport{Kind: "База данных", Name: "Документооборот — основная БД", Path: "DB-DEMO-01 / PostgreSQL / Документооборот", ID: "00000000-0000-4000-8000-000000000001", Version: 7, Author: "Демонстрационный пользователь", AppVersion: "test", GeneratedAt: time.Date(2026, 10, 26, 12, 0, 0, 0, time.FixedZone("UTC+05", 5*3600)), Badges: []string{"Промышленный", "Эксплуатируется", "PostgreSQL"}, Summary: []Field{{"СУБД", "PostgreSQL 17"}, {"Сервер", "DB-DEMO-01"}, {"Политика РК", "Включена"}}, Sections: []Section{{"Длинное описание", []Field{{"Описание", strings.Repeat("Проверка кириллицы: Ёжик, перенос строки и сохранность текста.\n", 180) + "КОНЕЦ-ДЛИННОГО-ТЕКСТА"}, {"Длинный путь", strings.Repeat("/длинный-каталог", 90) + "/КОНЕЦ-ПУТИ"}, {"Специальные символы", "<script>alert('text')</script> 🙂 \x00\t\r\nОбычный текст"}}}}}
	data, err := RenderPassport(in)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-1.3")) || !bytes.Contains(data, []byte("/ToUnicode")) || !bytes.Contains(data, []byte("/FontFile2")) {
		t.Fatal("missing PDF header, Unicode mapping or embedded fonts")
	}
	if pages := len(regexp.MustCompile(`/Type /Page\b`).FindAll(data, -1)); pages < 4 {
		t.Fatalf("long content was not paginated: %d pages", pages)
	}
	path := filepath.Join(t.TempDir(), "passport.pdf")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if bin, err := exec.LookPath("pdftotext"); err == nil {
		// Windows PDF tools may default to a legacy console encoding. Specify
		// UTF-8 so the same Cyrillic assertions apply on every platform.
		raw, err := exec.Command(bin, "-enc", "UTF-8", "-layout", path, "-").CombinedOutput()
		if err != nil {
			t.Fatalf("pdftotext: %v: %s", err, raw)
		}
		for _, term := range []string{"Документооборот", "Ёжик", "КОНЕЦ-ДЛИННОГО-ТЕКСТА", "КОНЕЦ-ПУТИ", "Обычный текст", "Демонстрационный пользователь", "UTC+05:00"} {
			if !strings.Contains(string(raw), term) {
				t.Errorf("PDF lost text %q", term)
			}
		}
	}
}
