package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateBaseURL(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "empty", raw: "", wantErr: true},
		{name: "https origin", raw: "https://catalog.example.com", want: "https://catalog.example.com"},
		{name: "trailing slash trimmed", raw: "https://catalog.example.com/", want: "https://catalog.example.com"},
		{name: "http allowed for dev", raw: "http://localhost:8080", want: "http://localhost:8080"},
		{name: "ftp rejected", raw: "ftp://catalog.example.com", wantErr: true},
		{name: "no host rejected", raw: "https://", wantErr: true},
		{name: "query rejected", raw: "https://catalog.example.com?x=1", wantErr: true},
		{name: "fragment rejected", raw: "https://catalog.example.com#f", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateBaseURL(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("validateBaseURL(%q) succeeded, want error", tc.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateBaseURL(%q): %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("validateBaseURL(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestNewRequiresBaseURL(t *testing.T) {
	_, err := New(context.Background(), Config{
		DBPath:     filepath.Join(t.TempDir(), "catalog.db"),
		SessionTTL: time.Hour,
		UploadDir:  t.TempDir(),
	})
	if err == nil {
		t.Fatal("New without BaseURL succeeded, want error")
	}
}
