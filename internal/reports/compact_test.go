package reports

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

func samplePassport(t *testing.T) Passport {
	t.Helper()
	raw, err := os.ReadFile("testdata/passport.json")
	if err != nil {
		t.Fatal(err)
	}
	var item Passport
	if err = json.Unmarshal(raw, &item); err != nil {
		t.Fatal(err)
	}
	return item
}

func pdfText(t *testing.T, data []byte) string {
	t.Helper()
	bin, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext is not installed; binary/PDF page assertions run independently")
	}
	path := filepath.Join(t.TempDir(), "passport.pdf")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	raw, err := exec.Command(bin, "-enc", "UTF-8", "-raw", path, "-").CombinedOutput()
	if err != nil {
		t.Fatalf("pdftotext: %v %s", err, raw)
	}
	return string(raw)
}

func TestCompactBatchKeepsEveryFieldAndOnePagePerDatabase(t *testing.T) {
	first := samplePassport(t)
	first.Sections = append(first.Sections, Section{Title: "Сведения и сопровождение сервера", Fields: []Field{{"Состояние сервера", "Эксплуатируется"}, {"Тип эксплуатации сервера", "Промышленный"}, {"Организация сервера", "Демонстрационная ИТ-служба"}, {"Руководитель сервера", "Анна Смирнова"}, {"Ответственный за сервер", "Иван Петров"}, {"Работа АБД на сервере", "Мониторинг и контроль"}}})
	second := samplePassport(t)
	second.Name = "Вторая выбранная БД"
	second.ID = "00000000-0000-4000-8000-000000000002"
	data, err := RenderPassports([]Passport{first, second}, "compact")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(regexp.MustCompile(`/Type /Page\b`).FindAll(data, -1)); n != 2 {
		t.Fatalf("expected exactly two pages, got %d", n)
	}
	normalize := func(s string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return r
		}, s)
	}
	text := normalize(pdfText(t, data))
	for _, item := range []Passport{first, second} {
		if !strings.Contains(text, normalize(item.Name)) {
			t.Error("missing object title", item.Name)
		}
		sections := append(append([]Section{}, item.Sections...), metadata(item))
		sections = append(sections, Section{Title: "Сводка", Fields: item.Summary})
		for _, section := range sections {
			for _, f := range section.Fields {
				if !strings.Contains(text, normalize(valueText(f.Value))) {
					t.Errorf("field lost: %s = %s", f.Label, f.Value)
				}
			}
		}
	}
}

func TestCompactOverflowReturnsNoTruncatedOrPartialPDF(t *testing.T) {
	first := samplePassport(t)
	second := samplePassport(t)
	second.Name = "Слишком длинное описание"
	second.Sections = append(second.Sections, Section{Title: "Описание", Fields: []Field{{"Описание", strings.Repeat("Длинное значение без усечения. ", 600) + "КОНЕЦ-ОПИСАНИЯ"}}})
	data, err := RenderPassports([]Passport{first, second}, "compact")
	if !errors.Is(err, ErrCompactOverflow) || len(data) != 0 || !strings.Contains(err.Error(), second.Name) {
		t.Fatal("compact silently truncated or returned a partial PDF", len(data), err)
	}
	data, err = RenderPassports([]Passport{first, second}, "detailed")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pdfText(t, data), "КОНЕЦ-ОПИСАНИЯ") {
		t.Fatal("detailed fallback lost the final text")
	}
}
