package e2e

import (
	"net/http"
	"strings"
	"testing"
)

func TestFrontendIsServed(t *testing.T) {
	environment := newEnv(t)

	response, data := environment.do(environment.anonymous, http.MethodGet, "/", nil)
	assertStatus(t, response, http.StatusOK, data)
	if contentType := response.Header.Get("Content-Type"); !strings.Contains(contentType, "text/html") {
		t.Fatalf("index content type = %q, want text/html", contentType)
	}
	if cache := response.Header.Get("Cache-Control"); cache != "no-cache" {
		t.Fatalf("index cache control = %q, want no-cache", cache)
	}
	html := string(data)
	for _, fragment := range []string{`id="object-grid"`, `id="login-dialog"`, `id="busy-dialog"`, `id="object-photo"`, `/static/app.js`, `/static/styles.css`} {
		if !strings.Contains(html, fragment) {
			t.Fatalf("index is missing %q", fragment)
		}
	}

	assets := []struct {
		path        string
		contentType string
		fragment    string
	}{
		{path: "/static/app.js", contentType: "javascript", fragment: "/api/objects"},
		{path: "/static/styles.css", contentType: "text/css", fragment: ".object-card"},
		{path: "/static/favicon.svg", contentType: "image/svg+xml", fragment: "<svg"},
	}
	for _, asset := range assets {
		t.Run(asset.path, func(t *testing.T) {
			response, data := environment.do(environment.anonymous, http.MethodGet, asset.path, nil)
			assertStatus(t, response, http.StatusOK, data)
			if contentType := response.Header.Get("Content-Type"); !strings.Contains(contentType, asset.contentType) {
				t.Fatalf("content type = %q, want %q", contentType, asset.contentType)
			}
			if !strings.Contains(string(data), asset.fragment) {
				t.Fatalf("asset is missing %q", asset.fragment)
			}
		})
	}

	response, data = environment.do(environment.anonymous, http.MethodGet, "/static/missing.js", nil)
	assertStatus(t, response, http.StatusNotFound, data)

	response, data = environment.do(environment.anonymous, http.MethodGet, "/static/", nil)
	assertStatus(t, response, http.StatusNotFound, data)
	assertErrorCode(t, data, "not_found")

	response, data = environment.do(environment.anonymous, http.MethodGet, "/static/uploads/missing.png", nil)
	assertStatus(t, response, http.StatusNotFound, data)
	assertErrorCode(t, data, "not_found")

	response, data = environment.do(environment.anonymous, http.MethodGet, "/api/not-real", nil)
	assertStatus(t, response, http.StatusNotFound, data)
	if contentType := response.Header.Get("Content-Type"); !strings.Contains(contentType, "application/json") {
		t.Fatalf("unknown API content type = %q, want application/json", contentType)
	}
	assertErrorCode(t, data, "not_found")
}
