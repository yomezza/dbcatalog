package documents

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

func TestWordPackageEscapesContentAndContainsNoActiveInstructions(t *testing.T) {
	data, err := Word(Instruction{Title: "<script>", Steps: "A & B\nШаг 2\x00"}, []Field{{Label: "Объект", Value: "База <1>"}})
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		decoder := xml.NewDecoder(bytes.NewReader(content))
		for {
			_, err = decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(f.Name, err)
			}
		}
	}
	f, err := z.Open("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	raw, _ := io.ReadAll(f)
	if !strings.Contains(string(raw), "&lt;script&gt;") || !strings.Contains(string(raw), "A &amp; B") {
		t.Fatal("text not escaped")
	}
	if _, err = Word(Instruction{Title: "Empty"}, nil); err == nil {
		t.Fatal("instruction without steps accepted")
	}
}
