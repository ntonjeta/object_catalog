package spreadsheet

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"biblio-oggetto/internal/domain"
)

func TestParseCSV(t *testing.T) {
	input := "\xef\xbb\xbfObject Name,Desc,Category Name,Tag,Image URL,Ignored\n" +
		"Dune,A desert planet,Books,\"SciFi, classic\",https://example.com/dune.jpg,ignored\n" +
		",Only description,,,,\n"
	rows, err := Parse("objects.csv", strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	first := rows[0]
	if first.Row != 2 || first.Name != "Dune" || first.Description != "A desert planet" || first.Category != "Books" {
		t.Fatalf("first row = %+v", first)
	}
	if !equalStrings(first.Tags, []string{"SciFi", "classic"}) {
		t.Fatalf("tags = %v", first.Tags)
	}
	if first.PhotoURL != "https://example.com/dune.jpg" {
		t.Fatalf("photo url = %q", first.PhotoURL)
	}
	if !first.HasDescription || !first.HasCategory || !first.HasTags || !first.HasPhotoURL {
		t.Fatalf("field presence flags = %+v", first)
	}
	second := rows[1]
	if second.Row != 3 || second.Name != "" || second.Description != "Only description" {
		t.Fatalf("second row = %+v", second)
	}
}

func TestParseCSVOptionalColumnsAbsent(t *testing.T) {
	rows, err := Parse("objects.csv", strings.NewReader("name\nDune\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	row := rows[0]
	if row.HasDescription || row.HasCategory || row.HasTags || row.HasPhotoURL {
		t.Fatalf("absent columns reported present: %+v", row)
	}
}

func TestParseCSVValidation(t *testing.T) {
	for name, input := range map[string]string{
		"missing name":   "description,category\nhello,Books\n",
		"duplicate name": "name,object name\nDune,Dune Messiah\n",
		"no header":      "\n\n",
		"invalid utf8":   "name\n\xff\n",
		"no data rows":   "name\n",
		"invalid csv":    "name\n\"unterminated\n",
		"empty csv":      "",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse("objects.csv", strings.NewReader(input)); !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("Parse error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestParseXLSX(t *testing.T) {
	file := excelize.NewFile()
	defer file.Close()
	sheet := "Sheet1"
	if err := file.SetSheetRow(sheet, "A1", &[]any{"name", "description", "category", "tags", "photo_url"}); err != nil {
		t.Fatalf("set header: %v", err)
	}
	if err := file.SetSheetRow(sheet, "A2", &[]any{"Dune", "A desert planet", "Books", "scifi, classic", "https://example.com/dune.jpg"}); err != nil {
		t.Fatalf("set row: %v", err)
	}
	buffer, err := file.WriteToBuffer()
	if err != nil {
		t.Fatalf("write xlsx: %v", err)
	}
	rows, err := Parse("objects.xlsx", bytes.NewReader(buffer.Bytes()))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.Row != 2 || row.Name != "Dune" || row.Category != "Books" {
		t.Fatalf("row = %+v", row)
	}
	if !equalStrings(row.Tags, []string{"scifi", "classic"}) {
		t.Fatalf("tags = %v", row.Tags)
	}
}

func TestParseRejectsUnsupportedFiles(t *testing.T) {
	for name, filename := range map[string]string{
		"legacy":      "objects.xls",
		"unknown ext": "objects.txt",
		"no ext":      "objects",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(filename, strings.NewReader("name\nDune\n")); !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("Parse error = %v, want ErrInvalid", err)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
