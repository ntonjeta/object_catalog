package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestOpenMigratesExistingObjectsWithoutImagePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	if _, err := legacy.Exec(`CREATE TABLE objects (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		category_id TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if _, err := legacy.Exec(`INSERT INTO objects (id, name, created_at, updated_at)
		VALUES ('legacy', 'Legacy', '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z')`); err != nil {
		t.Fatalf("insert legacy object: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	var imagePath string
	if err := db.QueryRow(`SELECT image_path FROM objects WHERE id = 'legacy'`).Scan(&imagePath); err != nil {
		t.Fatalf("query migrated image_path: %v", err)
	}
	if imagePath != "" {
		t.Fatalf("image_path = %q, want empty default", imagePath)
	}
}

func TestOpenRenamesLegacyPhotoPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	if _, err := legacy.Exec(`CREATE TABLE objects (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		photo_path TEXT NOT NULL DEFAULT '',
		category_id TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if _, err := legacy.Exec(`INSERT INTO objects (id, name, photo_path, created_at, updated_at)
		VALUES ('legacy', 'Legacy', '/static/uploads/old.png', '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z')`); err != nil {
		t.Fatalf("insert legacy object: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	var imagePath string
	if err := db.QueryRow(`SELECT image_path FROM objects WHERE id = 'legacy'`).Scan(&imagePath); err != nil {
		t.Fatalf("query migrated image_path: %v", err)
	}
	if imagePath != "/static/uploads/old.png" {
		t.Fatalf("image_path = %q, want legacy value", imagePath)
	}
	if busy != 0 {
		t.Fatalf("busy = %d, want 0 default", busy)
	}
	if busyUntil.Valid {
		t.Fatalf("busy_until = %q, want NULL default", busyUntil.String)
	}
}
