package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"biblio-oggetto/internal/app"
)

const (
	adminEmail         = "admin@example.com"
	adminPassword      = "password123"
	federationBaseURL  = "https://catalog.example.com"
	federationAcctHost = "catalog.example.com"
)

type env struct {
	t             *testing.T
	app           *app.App
	server        *httptest.Server
	admin         *http.Client
	anonymous     *http.Client
	adminEmail    string
	adminPassword string
	uploadRoot    string
}

type categoryDTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type categoryEnvelope struct {
	Category categoryDTO `json:"category"`
}

type categoriesEnvelope struct {
	Categories []categoryDTO `json:"categories"`
}

type objectImageDTO struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	Position int    `json:"position"`
}

type objectDTO struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	ImagePath    string           `json:"image_path"`
	Images       []objectImageDTO `json:"images"`
	Busy         bool             `json:"busy"`
	BusyUntil    *time.Time       `json:"busy_until"`
	ReservedFrom *time.Time       `json:"reserved_from"`
	Category     *categoryDTO     `json:"category"`
	Tags         []string         `json:"tags"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

type objectEnvelope struct {
	Object objectDTO `json:"object"`
}

type objectsEnvelope struct {
	Objects []objectDTO `json:"objects"`
	Total   int         `json:"total"`
	Limit   int         `json:"limit"`
	Offset  int         `json:"offset"`
}

type userDTO struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type userEnvelope struct {
	User userDTO `json:"user"`
}

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func newEnv(t *testing.T) *env {
	t.Helper()
	uploadRoot := filepath.Join(t.TempDir(), "static")
	application, err := app.New(context.Background(), app.Config{
		DBPath:     filepath.Join(t.TempDir(), "catalog.db"),
		SessionTTL: time.Hour,
		UploadDir:  uploadRoot,
		BaseURL:    federationBaseURL,
	})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() {
		if err := application.Close(); err != nil {
			t.Errorf("close app: %v", err)
		}
	})
	if _, err := application.CreateAdmin(context.Background(), adminEmail, adminPassword); err != nil {
		t.Fatalf("create admin: %v", err)
	}

	server := httptest.NewServer(application.Handler)
	t.Cleanup(server.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return &env{
		t:             t,
		app:           application,
		server:        server,
		admin:         &http.Client{Jar: jar},
		anonymous:     &http.Client{},
		adminEmail:    adminEmail,
		adminPassword: adminPassword,
		uploadRoot:    uploadRoot,
	}
}

func (e *env) do(client *http.Client, method, path string, body any) (*http.Response, []byte) {
	e.t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			e.t.Fatalf("marshal request body: %v", err)
		}
	}
	return e.doRaw(client, method, path, payload)
}

func (e *env) doRaw(client *http.Client, method, path string, payload []byte) (*http.Response, []byte) {
	e.t.Helper()
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	request, err := http.NewRequest(method, e.server.URL+path, reader)
	if err != nil {
		e.t.Fatalf("new request: %v", err)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, path, err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		e.t.Fatalf("read response: %v", err)
	}
	return response, data
}

func (e *env) login() {
	e.t.Helper()
	response, data := e.do(e.admin, http.MethodPost, "/api/auth/login", map[string]string{
		"email":    e.adminEmail,
		"password": e.adminPassword,
	})
	assertStatus(e.t, response, http.StatusOK, data)
}

func (e *env) createCategory(name string) categoryDTO {
	e.t.Helper()
	response, data := e.do(e.admin, http.MethodPost, "/api/categories", map[string]string{"name": name})
	assertStatus(e.t, response, http.StatusCreated, data)
	return decode[categoryEnvelope](e.t, data).Category
}

func (e *env) createObject(body map[string]any) objectDTO {
	e.t.Helper()
	response, data := e.do(e.admin, http.MethodPost, "/api/objects", body)
	assertStatus(e.t, response, http.StatusCreated, data)
	return decode[objectEnvelope](e.t, data).Object
}

func assertStatus(t *testing.T, response *http.Response, want int, body []byte) {
	t.Helper()
	if response.StatusCode != want {
		t.Fatalf("status = %d, want %d; body: %s", response.StatusCode, want, body)
	}
}

func assertErrorCode(t *testing.T, body []byte, want string) {
	t.Helper()
	var envelope errorEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode error envelope: %v; body: %s", err, body)
	}
	if envelope.Error.Code != want {
		t.Fatalf("error code = %q, want %q; body: %s", envelope.Error.Code, want, body)
	}
}

func decode[T any](t *testing.T, body []byte) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("decode response: %v; body: %s", err, body)
	}
	return value
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
