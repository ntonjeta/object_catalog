package main

import (
	"path/filepath"
	"testing"
)

func TestRunCreateAdmin(t *testing.T) {
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "catalog.db"))
	t.Setenv("UPLOAD_DIR", t.TempDir())
	t.Setenv("ADMIN_PASSWORD", "password123")

	if err := run([]string{"create-admin", "-email", "admin@example.com"}); err != nil {
		t.Fatalf("create-admin: %v", err)
	}
	if err := run([]string{"create-admin", "-email", "admin@example.com"}); err == nil {
		t.Fatal("duplicate create-admin succeeded, want error")
	}
	if err := run([]string{"create-admin"}); err == nil {
		t.Fatal("create-admin without email succeeded, want error")
	}
	if err := run([]string{"nope"}); err == nil {
		t.Fatal("unknown command succeeded, want error")
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("DB_PATH", "custom.db")
	t.Setenv("SESSION_TTL", "2h")
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("UPLOAD_DIR", "custom-static")

	cfg, err := configFromEnv()
	if err != nil {
		t.Fatalf("configFromEnv: %v", err)
	}
	if cfg.DBPath != "custom.db" || cfg.CookieSecure != true || cfg.SessionTTL.String() != "2h0m0s" || cfg.UploadDir != "custom-static" {
		t.Fatalf("config = %+v", cfg)
	}

	t.Setenv("SESSION_TTL", "bad")
	if _, err := configFromEnv(); err == nil {
		t.Fatal("invalid SESSION_TTL succeeded, want error")
	}
	t.Setenv("SESSION_TTL", "1h")
	t.Setenv("COOKIE_SECURE", "bad")
	if _, err := configFromEnv(); err == nil {
		t.Fatal("invalid COOKIE_SECURE succeeded, want error")
	}
}
