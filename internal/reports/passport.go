// Package reports renders portable object passports without a browser or a printer.
package reports

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/go-pdf/fpdf"
)

// The bundled, unmodified Noto fonts are distributed under fonts/OFL.txt.
//
//go:embed fonts/NotoSans-Regular.ttf
var regular []byte

//go:embed fonts/NotoSans-Bold.ttf
var bold []byte

type Field struct{ Label, Value string }
type Section struct {
	Title  string
	Fields []Field
}
type Passport struct {
	Kind, Name, Path, ID, Author, AppVersion string
	Version                                  int64
	GeneratedAt                              time.Time
	Badges                                   []string
	Summary                                  []Field
	Sections                                 []Section
}

const (
	margin     = 18.0
	width      = 174.0
	bottom     = 272.0
	lineHeight = 5.2
)

type color struct{ r, g, b int }

var (
	ink   = color{28, 44, 63}
	muted = color{102, 119, 137}
	teal  = color{0, 139, 126}
	light = color{243, 247, 250}
)

type renderer struct {
	p      *fpdf.Fpdf
	report Passport
	y      float64
}

// text removes PDF control characters and replaces supplementary-plane symbols
// unsupported by the PDF engine's BMP font tables, rather than panicking.
func text(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == '\t' {
			return ' '
		}
		if r == '\r' {
			return -1
		}
		if r != '\n' && unicode.IsControl(r) {
			return -1
		}
		if r > 0xffff {
			return '\ufffd'
		}
		return r
	}, s))
}

func valueText(s string) string {
	if s = text(s); s == "" {
		return "Не указано"
	}
	return s
}

var ErrCompactOverflow = errors.New("Краткий паспорт не помещается на одной странице")

func RenderPassport(in Passport) ([]byte, error) {
	return RenderPassports([]Passport{in}, "detailed")
}

// RenderPassports shares embedded fonts and page numbering across the selection.
// A failed compact layout returns no partial document.
func RenderPassports(items []Passport, layout string) ([]byte, error) {
	if len(items) == 0 {
		return nil, errors.New("Не выбраны объекты для PDF")
	}
	if layout != "detailed" && layout != "compact" {
		return nil, errors.New("Неизвестный вид PDF")
	}
	in := items[0]
	p := fpdf.New("P", "mm", "A4", "")
	p.SetMargins(margin, margin, margin)
	p.SetCellMargin(0)
	p.SetAutoPageBreak(false, 20)
	p.AddUTF8FontFromBytes("Noto", "", regular)
	p.AddUTF8FontFromBytes("Noto", "B", bold)
	title := "Паспорт объекта — " + text(in.Name)
	if len(items) > 1 {
		title = fmt.Sprintf("Паспорта выбранных БД — %d", len(items))
	}
	p.SetTitle(title, true)
	p.SetAuthor(text(in.Author), true)
	p.SetCreator("CatalogDB "+in.AppVersion, true)
	p.SetCreationDate(in.GeneratedAt.UTC())
	p.SetModificationDate(in.GeneratedAt.UTC())
	p.AliasNbPages("{pages}")
	r := renderer{p: p}
	p.SetFooterFunc(func() {
		p.SetDrawColor(219, 229, 235)
		p.Line(margin, 280, 192, 280)
		r.line(margin, 284, "CatalogDB  /  Паспорт объекта", 8, "", muted)
		p.SetXY(150, 284)
		p.CellFormat(42, 4, fmt.Sprintf("%d / {pages}", p.PageNo()), "", 0, "R", false, 0, "")
	})
	for _, item := range items {
		r.report = item
		if layout == "compact" {
			if err := r.compact(); err != nil {
				return nil, err
			}
		} else {
			p.AddPage()
			r.cover()
			r.summary()
			for i, section := range item.Sections {
				r.section(fmt.Sprintf("%02d", i+1), section)
			}
			r.section("ID", metadata(item))
		}
	}
	var out bytes.Buffer
	if err := p.Output(&out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func metadata(in Passport) Section {
	return Section{Title: "Реквизиты документа", Fields: []Field{
		{"Идентификатор объекта", in.ID}, {"Версия записи", fmt.Sprint(in.Version)},
		{"Сформировал", in.Author}, {"Дата формирования", in.GeneratedAt.Format("02.01.2006 · 15:04:05 UTC-07:00")},
	}}
}

func (r *renderer) line(x, y float64, s string, size float64, style string, c color) {
	r.p.SetFont("Noto", style, size)
	r.p.SetTextColor(c.r, c.g, c.b)
	r.p.SetXY(x, y)
	r.p.CellFormat(0, 5, text(s), "", 0, "L", false, 0, "")
}

func (r *renderer) cover() {
	p := r.p
	p.SetFont("Noto", "B", 25)
	title := p.SplitText(text(r.report.Name), width)
	p.SetFont("Noto", "", 9)
	path := p.SplitText(text(r.report.Path), width)
	titleY := 48.0
	pathY := titleY + float64(len(title))*11 + 4
	r.y = pathY + float64(len(path))*4.8 + 10
	p.SetFillColor(22, 39, 57)
	p.Rect(0, 0, 210, r.y, "F")
	p.SetFillColor(0, 177, 156)
	p.Rect(margin, 18, 2, 14, "F")
	r.line(24, 18, "CatalogDB", 15, "B", color{255, 255, 255})
	r.line(24, 25, "КАТАЛОГ ИНФРАСТРУКТУРЫ", 7.5, "", color{157, 187, 204})
	r.line(149, 19, r.report.GeneratedAt.Format("02.01.2006"), 9, "", color{209, 225, 235})
	r.line(149, 25, "ПАСПОРТ ОБЪЕКТА", 7, "B", color{157, 187, 204})
	r.line(margin, 38, "ПАСПОРТ · "+strings.ToUpper(r.report.Kind), 9, "B", color{78, 219, 193})
	for i, s := range title {
		r.line(margin, titleY+float64(i)*11, s, 25, "B", color{255, 255, 255})
	}
	for i, s := range path {
		r.line(margin, pathY+float64(i)*4.8, s, 9, "", color{190, 209, 222})
	}
	r.y += 8
	x := margin
	for _, badge := range r.report.Badges {
		badge = text(badge)
		if badge == "" {
			continue
		}
		p.SetFont("Noto", "B", 8)
		w := p.GetStringWidth(badge) + 8
		if x+w > 192 {
			x = margin
			r.y += 10
		}
		bg, fg := color{229, 244, 240}, teal
		if badge == "Промышленный" {
			bg, fg = color{230, 238, 252}, color{45, 86, 153}
		}
		if badge == "Тестовый" {
			bg, fg = color{252, 242, 221}, color{147, 98, 23}
		}
		switch badge {
		case "Выведена из эксплуатации", "Архив", "Отсоединена", "Справочник":
			bg, fg = color{237, 240, 244}, muted
		case "К удалению":
			bg, fg = color{252, 232, 233}, color{160, 51, 65}
		case "Устаревшая", "Актуализировать":
			bg, fg = color{252, 242, 221}, color{147, 98, 23}
		case "Заморожена", "Разрабатывается":
			bg, fg = color{230, 238, 252}, color{45, 86, 153}
		}
		p.SetFillColor(bg.r, bg.g, bg.b)
		p.RoundedRect(x, r.y, w, 8, 2, "1234", "F")
		r.line(x+4, r.y+1.4, badge, 8, "B", fg)
		x += w + 3
	}
	if x > margin {
		r.y += 14
	}
}

func (r *renderer) summary() {
	if len(r.report.Summary) == 0 {
		return
	}
	items := r.report.Summary
	columns := min(3, len(items))
	w := (width - float64(columns-1)*4) / float64(columns)
	for start := 0; start < len(items); start += 3 {
		end := min(start+3, len(items))
		height := 24.0
		for _, f := range items[start:end] {
			r.p.SetFont("Noto", "B", 11)
			height = math.Max(height, 14+float64(len(r.p.SplitText(valueText(f.Value), w-12)))*5.2)
		}
		r.ensure(height + 4)
		for i, f := range items[start:end] {
			x := margin + float64(i)*(w+4)
			r.p.SetFillColor(light.r, light.g, light.b)
			r.p.RoundedRect(x, r.y, w, height, 2, "1234", "F")
			r.line(x+6, r.y+4, f.Label, 7.5, "", muted)
			r.p.SetFont("Noto", "B", 11)
			for j, s := range r.p.SplitText(valueText(f.Value), w-12) {
				r.line(x+6, r.y+11+float64(j)*5.2, s, 11, "B", ink)
			}
		}
		r.y += height + 4
	}
	r.y += 4
}

func (r *renderer) ensure(h float64) {
	if r.y+h <= bottom {
		return
	}
	r.p.AddPage()
	r.line(margin, 17, "CatalogDB  /  "+r.report.Kind, 9, "B", teal)
	r.p.SetFont("Noto", "B", 12)
	lines := r.p.SplitText(text(r.report.Name), width)
	for i, s := range lines {
		r.line(margin, 26+float64(i)*6, s, 12, "B", ink)
	}
	r.y = 34 + float64(len(lines))*6
}

func (r *renderer) section(number string, s Section) {
	if len(s.Fields) == 0 {
		return
	}
	// Keep ordinary sections together. Exceptionally long descriptions still
	// flow across as many pages as needed without truncating their contents.
	height := 19.0
	for _, f := range s.Fields {
		r.p.SetFont("Noto", "", 9.5)
		value := text(f.Value)
		if value == "" {
			value = "Не указано"
		}
		n := len(r.p.SplitText(value, 112))
		r.p.SetFont("Noto", "", 8)
		height += 4 + float64(max(n, len(r.p.SplitText(text(f.Label), 48))))*lineHeight
	}
	if height <= 220 {
		r.ensure(height)
	} else {
		r.ensure(30)
	}
	r.line(margin, r.y, number, 9, "B", teal)
	r.line(margin+11, r.y-0.5, s.Title, 12, "B", ink)
	r.y += 11
	for i, field := range s.Fields {
		r.field(field, i%2 == 0)
	}
	r.y += 8
}

func (r *renderer) field(f Field, shade bool) {
	value := text(f.Value)
	if value == "" {
		value = "Не указано"
	}
	r.p.SetFont("Noto", "", 9.5)
	values := r.p.SplitText(value, 112)
	r.p.SetFont("Noto", "", 8)
	labels := r.p.SplitText(text(f.Label), 48)
	for len(values) > 0 {
		r.ensure(8 + float64(len(labels))*lineHeight)
		capacity := max(1, int((bottom-r.y-4)/lineHeight))
		n := min(len(values), capacity)
		height := 4 + float64(max(n, len(labels)))*lineHeight
		if shade {
			r.p.SetFillColor(light.r, light.g, light.b)
			r.p.Rect(margin, r.y, width, height, "F")
		}
		for i, s := range labels {
			r.line(margin+4, r.y+2+float64(i)*lineHeight, s, 8, "", muted)
		}
		for i, s := range values[:n] {
			r.line(margin+58, r.y+2+float64(i)*lineHeight, s, 9.5, "", ink)
		}
		r.y += height
		values = values[n:]
		if len(values) > 0 {
			labels = []string{"Продолжение"}
			r.ensure(12)
		}
	}
}
