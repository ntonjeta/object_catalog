package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"biblio-oggetto/internal/domain"
	"biblio-oggetto/internal/usecase"
)

func TestObjectRepoImagesRoundTrip(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	repo := NewObjectRepo(db)

	now := time.Now().UTC()
	uploaded := domain.Object{
		ID:        "uploaded",
		Name:      "Dune",
		ImagePath: "cover-a.png",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repo.Create(ctx, uploaded, domain.OutboxEntry{}); err != nil {
		t.Fatalf("Create uploaded cover: %v", err)
	}
	remote := domain.Object{
		ID:        "remote",
		Name:      "Foundation",
		ImagePath: "https://example.com/cover.jpg",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repo.Create(ctx, remote, domain.OutboxEntry{}); err != nil {
		t.Fatalf("Create remote cover: %v", err)
	}

	// Create with a bare uploaded file name seeds row 0; URLs stay outside.
	got, err := repo.ByID(ctx, uploaded.ID)
	if err != nil {
		t.Fatalf("ByID uploaded: %v", err)
	}
	if len(got.Images) != 1 || got.Images[0].FileName != "cover-a.png" || got.Images[0].ID == "" {
		t.Fatalf("uploaded images = %+v, want one row mirroring the cover", got.Images)
	}
	gotRemote, err := repo.ByID(ctx, remote.ID)
	if err != nil {
		t.Fatalf("ByID remote: %v", err)
	}
	if len(gotRemote.Images) != 0 {
		t.Fatalf("remote images = %+v, want empty", gotRemote.Images)
	}

	// Replace rewrites positions in slice order.
	images := []domain.ObjectImage{
		{ID: "img-1", FileName: "cover-a.png"},
		{ID: "img-2", FileName: "extra-b.png"},
		{ID: "img-3", FileName: "extra-c.png"},
	}
	if err := repo.ReplaceImages(ctx, uploaded.ID, images); err != nil {
		t.Fatalf("ReplaceImages: %v", err)
	}
	got, err = repo.ByID(ctx, uploaded.ID)
	if err != nil {
		t.Fatalf("ByID after replace: %v", err)
	}
	if !sameImageOrder(got.Images, images) {
		t.Fatalf("images = %+v, want %+v", got.Images, images)
	}

	// List loads the same ordered images per object.
	objects, _, err := repo.List(ctx, usecase.ObjectFilter{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var listed []domain.ObjectImage
	for _, object := range objects {
		if object.ID == uploaded.ID {
			listed = object.Images
		}
	}
	if !sameImageOrder(listed, images) {
		t.Fatalf("listed images = %+v, want %+v", listed, images)
	}

	// Reordering with fewer rows renumbers positions with no leftovers.
	renumbered := []domain.ObjectImage{
		{ID: "img-3", FileName: "extra-c.png"},
		{ID: "img-1", FileName: "cover-a.png"},
	}
	if err := repo.ReplaceImages(ctx, uploaded.ID, renumbered); err != nil {
		t.Fatalf("ReplaceImages renumber: %v", err)
	}
	got, err = repo.ByID(ctx, uploaded.ID)
	if err != nil {
		t.Fatalf("ByID after renumber: %v", err)
	}
	if !sameImageOrder(got.Images, renumbered) {
		t.Fatalf("renumbered images = %+v, want %+v", got.Images, renumbered)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM object_images WHERE object_id = ?`, uploaded.ID).Scan(&count); err != nil {
		t.Fatalf("count images: %v", err)
	}
	if count != len(renumbered) {
		t.Fatalf("image rows = %d, want %d", count, len(renumbered))
	}

	// Deleting the object cascades to its image rows.
	if err := repo.Delete(ctx, uploaded.ID, domain.OutboxEntry{}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM object_images WHERE object_id = ?`, uploaded.ID).Scan(&count); err != nil {
		t.Fatalf("count images after delete: %v", err)
	}
	if count != 0 {
		t.Fatalf("image rows after object delete = %d, want 0 (FK cascade)", count)
	}
}

func sameImageOrder(got, want []domain.ObjectImage) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
