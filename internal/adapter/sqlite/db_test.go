package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"biblio-oggetto/internal/domain"
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
	var busy int
	var busyUntil sql.NullString
	if err := db.QueryRow(`SELECT image_path, busy, busy_until FROM objects WHERE id = 'legacy'`).
		Scan(&imagePath, &busy, &busyUntil); err != nil {
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

func TestObjectRepoByName(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	repo := NewObjectRepo(db)

	now := time.Now().UTC()
	object := domain.Object{
		ID:          "one",
		Name:        "Dune",
		Description: "desert",
		Tags:        []string{"scifi"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := repo.Create(ctx, object); err != nil {
		t.Fatalf("Create: %v", err)
	}
	found, err := repo.ByName(ctx, "dune")
	if err != nil {
		t.Fatalf("ByName: %v", err)
	}
	if found.ID != object.ID || len(found.Tags) != 1 || found.Tags[0] != "scifi" {
		t.Fatalf("found = %+v", found)
	}
	if _, err := repo.ByName(ctx, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing error = %v, want ErrNotFound", err)
	}

	second := object
	second.ID = "two"
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("create duplicate: %v", err)
	}
	if _, err := repo.ByName(ctx, "Dune"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate error = %v, want ErrConflict", err)
	}
}
