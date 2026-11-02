package usecase

import (
	"context"
	"strings"
	"testing"

	"biblio-oggetto/internal/domain"
)

func TestObjectsImportCreatesUpdatesAndReportsFailures(t *testing.T) {
	ctx := context.Background()
	categories := newFakeCategories()
	objects := newFakeObjects()
	photos := newFakePhotos()
	uc := NewObjects(objects, categories, photos, newFakeReservations(), newFakeActivities())

	books, err := NewCategories(categories).Create(ctx, "Books")
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	existing, err := uc.Create(ctx, CreateObjectInput{
		Name:        "Dune",
		Description: "old description",
		CategoryID:  books.ID,
		Tags:        []string{"old"},
		PhotoPath:   "/static/uploads/old.png",
	})
	if err != nil {
		t.Fatalf("create existing object: %v", err)
	}

	result, err := uc.Import(ctx, []ImportRow{
		{Row: 2, Name: "  Dune  ", Description: "new description", Category: "books", Tags: []string{"SciFi", "classic"}, PhotoURL: "https://example.com/dune.jpg", HasDescription: true, HasCategory: true, HasTags: true, HasPhotoURL: true},
		{Row: 3, Name: "Hyperion", Description: "pilgrims", Category: "Space Opera", Tags: []string{"space"}, HasDescription: true, HasCategory: true, HasTags: true},
		{Row: 4, Name: " "},
		{Row: 5, Name: "Too long", Description: strings.Repeat("a", 5001), HasDescription: true},
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Created != 1 || result.Updated != 1 || len(result.Failures) != 2 {
		t.Fatalf("result = %+v, want created=1 updated=1 failures=2", result)
	}
	if result.Failures[0].Row != 4 || result.Failures[1].Row != 5 {
		t.Fatalf("failure rows = %+v", result.Failures)
	}

	updated := objects.objects[existing.ID]
	if updated.Name != "Dune" || updated.Description != "new description" || updated.ImagePath != "https://example.com/dune.jpg" {
		t.Fatalf("updated object = %+v", updated)
	}
	if updated.Category == nil || updated.Category.Name != "Books" {
		t.Fatalf("updated category = %+v", updated.Category)
	}
	if !equalStringSlice(updated.Tags, []string{"scifi", "classic"}) {
		t.Fatalf("updated tags = %v", updated.Tags)
	}
	if !containsString(photos.deleted, "/static/uploads/old.png") {
		t.Fatalf("old photo was not deleted; deleted = %v", photos.deleted)
	}

	created, err := objects.ByName(ctx, "Hyperion")
	if err != nil {
		t.Fatalf("find created object: %v", err)
	}
	if created.Category == nil || created.Category.Name != "Space Opera" {
		t.Fatalf("created category = %+v", created.Category)
	}
	if _, err := categoryByName(categories, "space opera"); err != nil {
		t.Fatalf("missing auto-created category: %v", err)
	}
}

func TestObjectsImportBlankFieldsClearAndPreservePhoto(t *testing.T) {
	ctx := context.Background()
	categories := newFakeCategories()
	objects := newFakeObjects()
	photos := newFakePhotos()
	uc := NewObjects(objects, categories, photos, newFakeReservations(), newFakeActivities())

	books, err := NewCategories(categories).Create(ctx, "Books")
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	existing, err := uc.Create(ctx, CreateObjectInput{
		Name:        "Dune",
		Description: "old description",
		CategoryID:  books.ID,
		Tags:        []string{"old"},
		PhotoPath:   "/static/uploads/old.png",
	})
	if err != nil {
		t.Fatalf("create existing object: %v", err)
	}

	result, err := uc.Import(ctx, []ImportRow{{Row: 2, Name: "Dune"}})
	if err != nil {
		t.Fatalf("Import with absent optional columns: %v", err)
	}
	if result.Updated != 1 {
		t.Fatalf("result = %+v, want updated=1", result)
	}
	updated := objects.objects[existing.ID]
	if updated.Description != "old description" || updated.Category == nil || len(updated.Tags) != 1 {
		t.Fatalf("absent columns changed fields: %+v", updated)
	}
	if updated.ImagePath != "/static/uploads/old.png" {
		t.Fatalf("absent photo url changed path to %q", updated.ImagePath)
	}

	result, err = uc.Import(ctx, []ImportRow{
		{Row: 2, Name: "Dune", HasDescription: true, HasCategory: true, HasTags: true},
	})
	if err != nil {
		t.Fatalf("Import with blank optional columns: %v", err)
	}
	if result.Updated != 1 {
		t.Fatalf("result = %+v, want updated=1", result)
	}
	updated = objects.objects[existing.ID]
	if updated.Description != "" || updated.Category != nil || len(updated.Tags) != 0 {
		t.Fatalf("blank fields were not cleared: %+v", updated)
	}
	if updated.ImagePath != "/static/uploads/old.png" {
		t.Fatalf("blank photo url changed path to %q", updated.ImagePath)
	}

	result, err = uc.Import(ctx, []ImportRow{
		{Row: 2, Name: "Dune", PhotoURL: "https://example.com/new.jpg", HasPhotoURL: true},
	})
	if err != nil {
		t.Fatalf("Import with photo: %v", err)
	}
	if result.Updated != 1 {
		t.Fatalf("result = %+v, want updated=1", result)
	}
	updated = objects.objects[existing.ID]
	if updated.ImagePath != "https://example.com/new.jpg" {
		t.Fatalf("photo path = %q", updated.ImagePath)
	}
	if !containsString(photos.deleted, "/static/uploads/old.png") {
		t.Fatalf("replaced photo was not deleted; deleted = %v", photos.deleted)
	}
}

func TestObjectsImportRejectsDuplicateExistingNames(t *testing.T) {
	ctx := context.Background()
	categories := newFakeCategories()
	objects := newFakeObjects()
	uc := NewObjects(objects, categories, newFakePhotos(), newFakeReservations(), newFakeActivities())

	for _, id := range []string{"one", "two"} {
		if err := objects.Create(ctx, domain.Object{ID: id, Name: "Dune"}, domain.OutboxEntry{}); err != nil {
			t.Fatalf("create object: %v", err)
		}
	}
	result, err := uc.Import(ctx, []ImportRow{{Row: 2, Name: "dune"}})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Created != 0 || result.Updated != 0 || len(result.Failures) != 1 {
		t.Fatalf("result = %+v, want one failure", result)
	}
}

func categoryByName(categories *fakeCategories, name string) (domain.Category, error) {
	for _, category := range categories.categories {
		if strings.EqualFold(category.Name, name) {
			return category, nil
		}
	}
	return domain.Category{}, domain.ErrNotFound
}

func equalStringSlice(a, b []string) bool {
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
