// Package documents generates portable Word instructions without running templates or macros.
package documents

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"strings"
)

type Instruction struct {
	Title         string `json:"title"`
	Template      string `json:"template"`
	Purpose       string `json:"purpose"`
	Prerequisites string `json:"prerequisites"`
	Steps         string `json:"steps"`
	Verification  string `json:"verification"`
	Rollback      string `json:"rollback"`
	Notes         string `json:"notes"`
}
type Field struct {
	Label string
	Value string
}

func Validate(in Instruction) error {
	if strings.TrimSpace(in.Title) == "" || len([]rune(in.Title)) > 200 {
		return errors.New("Название инструкции: от 1 до 200 символов")
	}
	if strings.TrimSpace(in.Steps) == "" {
		return errors.New("Добавьте шаги выполнения инструкции")
	}
	for _, v := range []string{in.Template, in.Purpose, in.Prerequisites, in.Steps, in.Verification, in.Rollback, in.Notes} {
		if len(v) > 100000 {
			return errors.New("Раздел инструкции слишком длинный")
		}
	}
	return nil
}
func Word(in Instruction, fields []Field) ([]byte, error) {
	if err := Validate(in); err != nil {
		return nil, err
	}
	var content strings.Builder
	escape := func(s string) string {
		var b bytes.Buffer
		xml.EscapeText(&b, []byte(strings.Map(func(r rune) rune {
			if r < 32 && r != '\n' && r != '\t' && r != '\r' {
				return -1
			}
			return r
		}, s)))
		return b.String()
	}
	paragraph := func(text, style string) {
		content.WriteString("<w:p><w:pPr><w:pStyle w:val=\"" + style + "\"/></w:pPr><w:r><w:t xml:space=\"preserve\">" + escape(text) + "</w:t></w:r></w:p>")
	}
	paragraph(in.Title, "Title")
	for _, f := range fields {
		if f.Value != "" {
			paragraph(f.Label+": "+f.Value, "Normal")
		}
	}
	for _, s := range []Field{{"Назначение", in.Purpose}, {"Предварительные условия", in.Prerequisites}, {"Порядок выполнения", in.Steps}, {"Проверка результата", in.Verification}, {"Откат и восстановление", in.Rollback}, {"Примечания", in.Notes}} {
		if strings.TrimSpace(s.Value) == "" {
			continue
		}
		paragraph(s.Label, "Heading1")
		for _, line := range strings.Split(strings.ReplaceAll(s.Value, "\r\n", "\n"), "\n") {
			paragraph(line, "Normal")
		}
	}
	files := map[string]string{
		"[Content_Types].xml":          `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/></Types>`,
		"_rels/.rels":                  `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/_rels/document.xml.rels": `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`,
		"word/styles.xml":              `<?xml version="1.0" encoding="UTF-8"?><w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:cs="Calibri"/><w:sz w:val="22"/></w:rPr></w:rPrDefault></w:docDefaults><w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:pPr><w:spacing w:after="120"/></w:pPr></w:style><w:style w:type="paragraph" w:styleId="Title"><w:name w:val="Title"/><w:basedOn w:val="Normal"/><w:rPr><w:b/><w:color w:val="146D80"/><w:sz w:val="36"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:basedOn w:val="Normal"/><w:pPr><w:keepNext/><w:spacing w:before="240" w:after="120"/></w:pPr><w:rPr><w:b/><w:color w:val="146D80"/><w:sz w:val="26"/></w:rPr></w:style></w:styles>`,
		"word/document.xml":            `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + content.String() + `<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1134" w:right="1134" w:bottom="1134" w:left="1134"/></w:sectPr></w:body></w:document>`,
	}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml", "word/styles.xml", "word/_rels/document.xml.rels"} {
		f, err := z.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err = f.Write([]byte(files[name])); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
