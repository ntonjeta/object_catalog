package spreadsheet

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"

	"biblio-oggetto/internal/domain"
	"biblio-oggetto/internal/usecase"
)

const (
	MaxFileBytes = 10 << 20
	MaxRows      = 5000
)

type field int

const (
	fieldUnknown field = iota
	fieldName
	fieldDescription
	fieldCategory
	fieldTags
	fieldPhotoURL
)

var headerFields = map[string]field{
	"name":          fieldName,
	"object":        fieldName,
	"object_name":   fieldName,
	"title":         fieldName,
	"description":   fieldDescription,
	"desc":          fieldDescription,
	"notes":         fieldDescription,
	"category":      fieldCategory,
	"category_name": fieldCategory,
	"tags":          fieldTags,
	"tag":           fieldTags,
	"photo_url":     fieldPhotoURL,
	"photo":         fieldPhotoURL,
	"photo_path":    fieldPhotoURL,
	"image_url":     fieldPhotoURL,
	"image":         fieldPhotoURL,
}

// Parse reads an XLSX or CSV file and returns its data rows. The first
// non-empty row must contain headers. Unknown columns are ignored.
func Parse(filename string, reader io.Reader) ([]usecase.ImportRow, error) {
	extension := strings.ToLower(filepath.Ext(filename))
	switch extension {
	case ".xlsx":
		return parseXLSX(reader)
	case ".csv":
		return parseCSV(reader)
	case ".xls":
		return nil, invalidf("legacy .xls files are not supported; save the file as .xlsx or CSV")
	default:
		return nil, invalidf("unsupported file type %q; expected .xlsx or .csv", extension)
	}
}

func parseCSV(reader io.Reader) ([]usecase.ImportRow, error) {
	data, err := readAll(reader)
	if err != nil {
		return nil, err
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(data) {
		return nil, invalidf("CSV file must be UTF-8 encoded")
	}
	csvReader := csv.NewReader(bytes.NewReader(data))
	csvReader.FieldsPerRecord = -1
	csvReader.TrimLeadingSpace = true
	rows, err := csvReader.ReadAll()
	if err != nil {
		return nil, invalidf("invalid CSV: %v", err)
	}
	return dataRows(rows)
}

func parseXLSX(reader io.Reader) ([]usecase.ImportRow, error) {
	data, err := readAll(reader)
	if err != nil {
		return nil, err
	}
	file, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{
		UnzipSizeLimit:    64 << 20,
		UnzipXMLSizeLimit: 16 << 20,
	})
	if err != nil {
		return nil, invalidf("invalid XLSX file: %v", err)
	}
	defer file.Close()

	sheets := file.GetSheetList()
	if len(sheets) == 0 {
		return nil, invalidf("XLSX file has no worksheets")
	}
	rows, err := file.GetRows(sheets[0])
	if err != nil {
		return nil, invalidf("read XLSX worksheet: %v", err)
	}
	return dataRows(rows)
}

func readAll(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, MaxFileBytes+1))
	if err != nil {
		return nil, invalidf("read import file: %v", err)
	}
	if len(data) > MaxFileBytes {
		return nil, invalidf("import file must be at most %d bytes", MaxFileBytes)
	}
	return data, nil
}

func dataRows(rows [][]string) ([]usecase.ImportRow, error) {
	headerIndex := -1
	for index, row := range rows {
		if !blankRow(row) {
			headerIndex = index
			break
		}
	}
	if headerIndex == -1 {
		return nil, invalidf("import file has no header row")
	}

	columns, err := headerColumns(rows[headerIndex])
	if err != nil {
		return nil, err
	}

	importRows := make([]usecase.ImportRow, 0)
	for index := headerIndex + 1; index < len(rows); index++ {
		row := rows[index]
		if blankRow(row) {
			continue
		}
		if len(importRows) >= MaxRows {
			return nil, invalidf("import file has more than %d data rows", MaxRows)
		}
		importRow := usecase.ImportRow{Row: index + 1}
		for column, field := range columns {
			value := cell(row, column)
			switch field {
			case fieldName:
				importRow.Name = value
			case fieldDescription:
				importRow.Description = value
				importRow.HasDescription = true
			case fieldCategory:
				importRow.Category = value
				importRow.HasCategory = true
			case fieldTags:
				importRow.Tags = splitTags(value)
				importRow.HasTags = true
			case fieldPhotoURL:
				importRow.PhotoURL = value
				importRow.HasPhotoURL = true
			}
		}
		importRows = append(importRows, importRow)
	}
	if len(importRows) == 0 {
		return nil, invalidf("import file has no data rows")
	}
	return importRows, nil
}

func headerColumns(header []string) (map[int]field, error) {
	columns := make(map[int]field, len(header))
	seen := make(map[field]string)
	for index, cell := range header {
		key := normalizeHeader(cell)
		if key == "" {
			continue
		}
		field, ok := headerFields[key]
		if !ok || field == fieldUnknown {
			continue
		}
		if previous, ok := seen[field]; ok {
			return nil, invalidf("duplicate headers %q and %q", previous, strings.TrimSpace(cell))
		}
		seen[field] = strings.TrimSpace(cell)
		columns[index] = field
	}
	if _, ok := seen[fieldName]; !ok {
		return nil, invalidf("import file must contain a name column")
	}
	return columns, nil
}

func normalizeHeader(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.Join(strings.Fields(value), "_")
	return value
}

func splitTags(value string) []string {
	parts := strings.Split(value, ",")
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		if tag := strings.TrimSpace(part); tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func blankRow(row []string) bool {
	for _, value := range row {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

func cell(row []string, index int) string {
	if index >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[index])
}

func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", domain.ErrInvalid, fmt.Sprintf(format, args...))
}
