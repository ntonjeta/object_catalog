package e2e

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

type importFailureDTO struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

type importResultDTO struct {
	Created  int                `json:"created"`
	Updated  int                `json:"updated"`
	Failed   int                `json:"failed"`
	Failures []importFailureDTO `json:"failures"`
}

func (e *env) upload(client *http.Client, path, field, filename string, data []byte) (*http.Response, []byte) {
	e.t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(field, filename)
	if err != nil {
		e.t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		e.t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		e.t.Fatalf("close multipart writer: %v", err)
	}
	request, err := http.NewRequest(http.MethodPost, e.server.URL+path, &body)
	if err != nil {
		e.t.Fatalf("new request: %v", err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := client.Do(request)
	if err != nil {
		e.t.Fatalf("upload %s: %v", path, err)
	}
	responseBody, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		e.t.Fatalf("read upload response: %v", err)
	}
	return response, responseBody
}

func TestImportCSVUpsertAndPartialFailures(t *testing.T) {
	environment := newEnv(t)
	environment.login()
	category := environment.createCategory("Books")
	existing := environment.createObject(map[string]any{
		"name":        "Dune",
		"description": "old",
		"category_id": category.ID,
		"tags":        []string{"old"},
	})

	csv := strings.Join([]string{
		"name,description,category,tags,photo_url",
		`Dune,Updated,books,"scifi, classic",https://example.com/dune.jpg`,
		"Hyperion,Pilgrims,Space Opera,space,",
		",Missing name,,,",
	}, "\n")
	response, data := environment.upload(environment.admin, "/api/objects/import", "file", "objects.csv", []byte(csv))
	assertStatus(t, response, http.StatusOK, data)
	result := decode[importResultDTO](t, data)
	if result.Created != 1 || result.Updated != 1 || result.Failed != 1 {
		t.Fatalf("import result = %+v", result)
	}
	if len(result.Failures) != 1 || result.Failures[0].Row != 4 {
		t.Fatalf("failures = %+v", result.Failures)
	}

	response, data = environment.do(environment.anonymous, http.MethodGet, "/api/objects?limit=100", nil)
	assertStatus(t, response, http.StatusOK, data)
	objects := decode[objectsEnvelope](t, data).Objects
	dune := findObject(t, objects, "Dune")
	if dune.ID != existing.ID || dune.Description != "Updated" || dune.ImagePath != "https://example.com/dune.jpg" {
		t.Fatalf("updated object = %+v", dune)
	}
	if dune.Category == nil || dune.Category.Name != "Books" {
		t.Fatalf("updated category = %+v", dune.Category)
	}
	if !equalStrings(dune.Tags, []string{"classic", "scifi"}) {
		t.Fatalf("updated tags = %v", dune.Tags)
	}
	if len(dune.Images) != 0 {
		t.Fatalf("imported URL cover images = %+v, want empty list", dune.Images)
	}
	findObject(t, objects, "Hyperion")

	response, data = environment.do(environment.anonymous, http.MethodGet, "/api/categories", nil)
	assertStatus(t, response, http.StatusOK, data)
	if !containsCategory(decode[categoriesEnvelope](t, data).Categories, "Space Opera") {
		t.Fatal("auto-created category is missing")
	}
}

func TestImportAuthorizationAndErrors(t *testing.T) {
	environment := newEnv(t)

	response, data := environment.upload(environment.anonymous, "/api/objects/import", "file", "objects.csv", []byte("name\nDune\n"))
	assertStatus(t, response, http.StatusUnauthorized, data)
	assertErrorCode(t, data, "unauthorized")

	environment.login()
	response, data = environment.upload(environment.admin, "/api/objects/import", "file", "objects.xls", []byte("name\nDune\n"))
	assertStatus(t, response, http.StatusBadRequest, data)
	assertErrorCode(t, data, "invalid_input")

	response, data = environment.upload(environment.admin, "/api/objects/import", "file", "objects.csv", []byte("description\nDune\n"))
	assertStatus(t, response, http.StatusBadRequest, data)
	assertErrorCode(t, data, "invalid_input")
}

func TestImportXLSX(t *testing.T) {
	environment := newEnv(t)
	environment.login()

	file := excelize.NewFile()
	defer file.Close()
	sheet := "Sheet1"
	if err := file.SetSheetRow(sheet, "A1", &[]any{"name", "description", "category", "tags"}); err != nil {
		t.Fatalf("set header: %v", err)
	}
	if err := file.SetSheetRow(sheet, "A2", &[]any{"Dune", "A desert planet", "Books", "scifi, classic"}); err != nil {
		t.Fatalf("set row: %v", err)
	}
	buffer, err := file.WriteToBuffer()
	if err != nil {
		t.Fatalf("write xlsx: %v", err)
	}

	response, data := environment.upload(environment.admin, "/api/objects/import", "file", "objects.xlsx", buffer.Bytes())
	assertStatus(t, response, http.StatusOK, data)
	result := decode[importResultDTO](t, data)
	if result.Created != 1 || result.Updated != 0 || result.Failed != 0 {
		t.Fatalf("import result = %+v", result)
	}

	response, data = environment.do(environment.anonymous, http.MethodGet, "/api/objects?limit=100", nil)
	assertStatus(t, response, http.StatusOK, data)
	dune := findObject(t, decode[objectsEnvelope](t, data).Objects, "Dune")
	if !equalStrings(dune.Tags, []string{"classic", "scifi"}) {
		t.Fatalf("tags = %v", dune.Tags)
	}
}

func findObject(t *testing.T, objects []objectDTO, name string) objectDTO {
	t.Helper()
	for _, object := range objects {
		if object.Name == name {
			return object
		}
	}
	t.Fatalf("object %q not found", name)
	return objectDTO{}
}

func containsCategory(categories []categoryDTO, name string) bool {
	for _, category := range categories {
		if category.Name == name {
			return true
		}
	}
	return false
}
