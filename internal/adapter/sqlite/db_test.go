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
	var reservedFrom sql.NullString
	if err := db.QueryRow(`SELECT image_path, busy, busy_until, reserved_from FROM objects WHERE id = 'legacy'`).
		Scan(&imagePath, &busy, &busyUntil, &reservedFrom); err != nil {
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
	if reservedFrom.Valid {
		t.Fatalf("reserved_from = %q, want NULL default", reservedFrom.String)
	}
}

func TestOpenBackfillsObjectImages(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	if _, err := legacy.Exec(`CREATE TABLE objects (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		image_path TEXT NOT NULL DEFAULT '',
		category_id TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	seed := []struct {
		id        string
		imagePath string
	}{
		{"uploaded", "cover.png"},
		{"remote", "https://example.com/photo.jpg"},
		{"root-relative", "/static/uploads/old.png"},
		{"empty", ""},
	}
	for _, item := range seed {
		if _, err := legacy.Exec(`INSERT INTO objects (id, name, image_path, created_at, updated_at)
			VALUES (?, ?, ?, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z')`,
			item.id, item.id, item.imagePath); err != nil {
			t.Fatalf("insert legacy object %q: %v", item.id, err)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	// Rerun-safe: migrating a second time must not duplicate or add rows.
	if err := migrate(ctx, db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	rows, err := db.QueryContext(ctx, `SELECT object_id, position, file_name FROM object_images ORDER BY object_id`)
	if err != nil {
		t.Fatalf("query object_images: %v", err)
	}
	defer rows.Close()
	type imageRow struct {
		objectID string
		position int
		fileName string
	}
	var found []imageRow
	for rows.Next() {
		var row imageRow
		if err := rows.Scan(&row.objectID, &row.position, &row.fileName); err != nil {
			t.Fatalf("scan object_images row: %v", err)
		}
		found = append(found, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read object_images: %v", err)
	}
	want := []imageRow{{objectID: "uploaded", position: 0, fileName: "cover.png"}}
	if len(found) != len(want) {
		t.Fatalf("object_images rows = %+v, want exactly %+v (one per legacy cover, position 0, no URL or root-relative rows)", found, want)
	}
	for i := range want {
		if found[i] != want[i] {
			t.Fatalf("object_images row %d = %+v, want %+v", i, found[i], want[i])
		}
	}
	var imageID string
	if err := db.QueryRowContext(ctx, `SELECT id FROM object_images WHERE object_id = 'uploaded'`).Scan(&imageID); err != nil {
		t.Fatalf("query backfilled image id: %v", err)
	}
	if len(imageID) != 32 {
		t.Fatalf("backfilled image id = %q, want 32 hex characters", imageID)
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
