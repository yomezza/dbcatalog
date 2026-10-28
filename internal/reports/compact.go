package reports

import (
	"fmt"
	"math"
	"strings"
)

const compactX = 14.0
const compactWidth = 88.0
const compactBottom = 272.0

type compactRow struct {
	x, y, height float64
	label, value []string
	heading      string
	shade        bool
}

func (r *renderer) compact() error {
	p := r.p
	p.SetFont("Noto", "B", 17)
	title := p.SplitText(text(r.report.Name), 182)
	p.SetFont("Noto", "", 8)
	path := p.SplitText(text(r.report.Path), 182)
	headerBottom := 24 + float64(len(title))*7 + float64(len(path))*4 + 5
	sections := append([]Section{}, r.report.Sections...)
	if len(r.report.Summary) > 0 {
		sections = append([]Section{{Title: "Сводка", Fields: r.report.Summary}}, sections...)
	}
	sections = append(sections, metadata(r.report))
	var plan []compactRow
	var size float64
	for _, candidate := range []float64{9, 8.5, 8, 7.5, 7} {
		if rows, ok := r.planCompact(sections, headerBottom+6, candidate); ok {
			plan = rows
			size = candidate
			break
		}
	}
	if plan == nil {
		return fmt.Errorf("%w: «%s». Выберите подробный вид, чтобы сохранить весь текст", ErrCompactOverflow, r.report.Name)
	}
	p.AddPage()
	p.SetFillColor(22, 39, 57)
	p.Rect(0, 0, 210, headerBottom, "F")
	p.SetFillColor(0, 177, 156)
	p.Rect(compactX, 8, 1.5, 8, "F")
	r.compactText(19, 8, "CatalogDB", 11, "B", color{255, 255, 255})
	r.compactText(137, 9, "КРАТКИЙ ПАСПОРТ · "+strings.ToUpper(r.report.Kind), 6.5, "B", color{157, 187, 204})
	for i, s := range title {
		r.compactText(compactX, 23+float64(i)*7, s, 17, "B", color{255, 255, 255})
	}
	pathY := 24 + float64(len(title))*7
	for i, s := range path {
		r.compactText(compactX, pathY+float64(i)*4, s, 8, "", color{190, 209, 222})
	}
	for _, row := range plan {
		if row.heading != "" {
			p.SetFillColor(229, 244, 240)
			p.RoundedRect(row.x, row.y, compactWidth, 6, 1, "1234", "F")
			r.compactText(row.x+2, row.y+1, row.heading, size, "B", teal)
			continue
		}
		if row.shade {
			p.SetFillColor(light.r, light.g, light.b)
			p.Rect(row.x, row.y, compactWidth, row.height, "F")
		}
		line := size * 0.352778 * 1.35
		for i, s := range row.label {
			r.compactText(row.x+2, row.y+1+float64(i)*line, s, math.Max(7, size-0.7), "", muted)
		}
		for i, s := range row.value {
			r.compactText(row.x+35, row.y+1+float64(i)*line, s, size, "", ink)
		}
	}
	return p.Error()
}

func (r *renderer) compactText(x, y float64, s string, size float64, style string, c color) {
	r.p.SetFont("Noto", style, size)
	r.p.SetTextColor(c.r, c.g, c.b)
	r.p.Text(x, y+size*0.352778, text(s))
}

// Measure every line before drawing. No clipping, ellipsis or hidden overflow.
func (r *renderer) planCompact(sections []Section, top, size float64) ([]compactRow, bool) {
	x, y, col := compactX, top, 0
	rows := []compactRow{}
	line := size * 0.352778 * 1.35
	next := func() bool {
		col++
		if col > 1 {
			return false
		}
		x = 108
		y = top
		return true
	}
	for _, section := range sections {
		if len(section.Fields) == 0 {
			continue
		}
		fields := make([]compactRow, 0, len(section.Fields))
		height := 9.0
		for i, f := range section.Fields {
			r.p.SetFont("Noto", "", math.Max(7, size-0.7))
			labels := r.p.SplitText(text(f.Label), 31)
			r.p.SetFont("Noto", "", size)
			values := r.p.SplitText(valueText(f.Value), 51)
			h := 2 + float64(max(len(labels), len(values)))*line
			fields = append(fields, compactRow{height: h, label: labels, value: values, shade: i%2 == 0})
			height += h
		}
		if height <= compactBottom-top && y+height > compactBottom {
			if !next() {
				return nil, false
			}
		}
		if y+7+fields[0].height > compactBottom {
			if !next() {
				return nil, false
			}
		}
		rows = append(rows, compactRow{x: x, y: y, heading: section.Title})
		y += 7
		for _, f := range fields {
			if y+f.height > compactBottom {
				if !next() {
					return nil, false
				}
				rows = append(rows, compactRow{x: x, y: y, heading: section.Title + " (продолжение)"})
				y += 7
			}
			if y+f.height > compactBottom {
				return nil, false
			}
			f.x = x
			f.y = y
			rows = append(rows, f)
			y += f.height
		}
		y += 3
	}
	return rows, true
}
