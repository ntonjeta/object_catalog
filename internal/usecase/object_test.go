package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"biblio-oggetto/internal/domain"
)

func TestObjectsCreateAndUpdate(t *testing.T) {
	ctx := context.Background()
	categories := newFakeCategories()
	objects := newFakeObjects()
	uc := NewObjects(objects, categories, newFakePhotos())

	category, err := NewCategories(categories).Create(ctx, "Books")
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	created, err := uc.Create(ctx, CreateObjectInput{
		Name:        "  Dune   Messiah ",
		Description: "  aftermath  ",
		CategoryID:  category.ID,
		Tags:        []string{" Sequel ", "classic", "sequel"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Name != "Dune Messiah" || created.Description != "aftermath" {
		t.Fatalf("created object = %+v", created)
	}
	if created.Category == nil || created.Category.ID != category.ID {
		t.Fatalf("category = %+v, want %s", created.Category, category.ID)
	}
	if len(created.Tags) != 2 || created.Tags[0] != "sequel" || created.Tags[1] != "classic" {
		t.Fatalf("tags = %v, want [sequel classic]", created.Tags)
	}

	if _, err := uc.Create(ctx, CreateObjectInput{Name: " "}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("empty name error = %v, want ErrInvalid", err)
	}
	if _, err := uc.Create(ctx, CreateObjectInput{Name: "x", Description: strings.Repeat("a", 5001)}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("long description error = %v, want ErrInvalid", err)
	}
	if _, err := uc.Create(ctx, CreateObjectInput{Name: "x", CategoryID: "missing"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing category error = %v, want ErrNotFound", err)
	}

	name := "Hyperion"
	description := "pilgrims"
	tags := []string{"Space"}
	emptyCategory := ""
	updated, err := uc.Update(ctx, created.ID, UpdateObjectInput{
		Name:        &name,
		Description: &description,
		Tags:        &tags,
		CategoryID:  &emptyCategory,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "Hyperion" || updated.Description != "pilgrims" {
		t.Fatalf("updated object = %+v", updated)
	}
	if updated.Category != nil {
		t.Fatalf("category = %+v, want nil", updated.Category)
	}
	if len(updated.Tags) != 1 || updated.Tags[0] != "space" {
		t.Fatalf("tags = %v, want [space]", updated.Tags)
	}

	invalid := " "
	if _, err := uc.Update(ctx, created.ID, UpdateObjectInput{Name: &invalid}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("invalid update error = %v, want ErrInvalid", err)
	}
	if _, err := uc.Update(ctx, "missing", UpdateObjectInput{Name: &name}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing object error = %v, want ErrNotFound", err)
	}
	if got, err := uc.Get(ctx, created.ID); err != nil || got.ID != created.ID {
		t.Fatalf("Get = (%+v, %v), want created category", got, err)
	}
	if err := uc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := uc.Get(ctx, created.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("get deleted object error = %v, want ErrNotFound", err)
	}
}

func TestObjectsSetBusy(t *testing.T) {
	ctx := context.Background()
	objects := newFakeObjects()
	uc := NewObjects(objects, newFakeCategories(), newFakePhotos())
	created, err := uc.Create(ctx, CreateObjectInput{Name: "Dune"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	until := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	busy, err := uc.SetBusy(ctx, created.ID, SetObjectBusyInput{Busy: true, BusyUntil: &until})
	if err != nil {
		t.Fatalf("SetBusy: %v", err)
	}
	if !busy.Busy || busy.BusyUntil == nil || !busy.BusyUntil.Equal(until) {
		t.Fatalf("busy object = %+v, want busy until %v", busy, until)
	}
	if got, err := uc.Get(ctx, created.ID); err != nil || !got.Busy {
		t.Fatalf("Get busy = (%+v, %v), want busy", got, err)
	}

	released, err := uc.SetBusy(ctx, created.ID, SetObjectBusyInput{})
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released.Busy || released.BusyUntil != nil {
		t.Fatalf("released object = %+v, want available", released)
	}

	past := time.Now().UTC().Add(-time.Minute)
	if _, err := uc.SetBusy(ctx, created.ID, SetObjectBusyInput{Busy: true, BusyUntil: &past}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("past expiry error = %v, want ErrInvalid", err)
	}
	if _, err := uc.SetBusy(ctx, created.ID, SetObjectBusyInput{Busy: true}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("missing expiry error = %v, want ErrInvalid", err)
	}
	if _, err := uc.SetBusy(ctx, "missing", SetObjectBusyInput{}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing object error = %v, want ErrNotFound", err)
	}
}

func TestObjectsReserve(t *testing.T) {
	ctx := context.Background()
	objects := newFakeObjects()
	uc := NewObjects(objects, newFakeCategories(), newFakePhotos())
	created, err := uc.Create(ctx, CreateObjectInput{Name: "Dune"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	now := time.Now().UTC()
	from := now.Add(24 * time.Hour).Truncate(time.Second)
	until := now.Add(48 * time.Hour).Truncate(time.Second)
	reserved, err := uc.Reserve(ctx, created.ID, ReserveObjectInput{From: from, Until: &until})
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if !reserved.Busy || reserved.ReservedFrom == nil || !reserved.ReservedFrom.Equal(from) {
		t.Fatalf("reserved object = %+v, want reserved from %v", reserved, from)
	}
	if reserved.IsBusyAt(now) {
		t.Fatal("reserved object is busy before reservation start")
	}

	got, err := uc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.Busy || got.ReservedFrom == nil {
		t.Fatalf("get reserved object = %+v, want reservation", got)
	}

	released, err := uc.SetBusy(ctx, created.ID, SetObjectBusyInput{})
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released.Busy || released.ReservedFrom != nil {
		t.Fatalf("released object = %+v, want available", released)
	}

	past := now.Add(-time.Hour)
	if _, err := uc.Reserve(ctx, created.ID, ReserveObjectInput{From: past}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("past start error = %v, want ErrInvalid", err)
	}
	if _, err := uc.Reserve(ctx, created.ID, ReserveObjectInput{From: from, Until: &past}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("end before start error = %v, want ErrInvalid", err)
	}
	if _, err := uc.Reserve(ctx, "missing", ReserveObjectInput{From: from}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing object error = %v, want ErrNotFound", err)
	}

	busyUntil := now.Add(time.Hour)
	if _, err := uc.SetBusy(ctx, created.ID, SetObjectBusyInput{Busy: true, BusyUntil: &busyUntil}); err != nil {
		t.Fatalf("mark busy: %v", err)
	}
	if _, err := uc.Reserve(ctx, created.ID, ReserveObjectInput{From: from}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("busy object error = %v, want ErrConflict", err)
	}
}

func TestObjectsExpiredBusyIsAvailable(t *testing.T) {
	ctx := context.Background()
	objects := newFakeObjects()
	uc := NewObjects(objects, newFakeCategories(), newFakePhotos())
	past := time.Now().UTC().Add(-time.Hour)
	expired := domain.Object{
		ID:        "expired",
		Name:      "Expired",
		Busy:      true,
		BusyUntil: &past,
	}
	if err := objects.Create(ctx, expired); err != nil {
		t.Fatalf("create expired object: %v", err)
	}
	got, err := uc.Get(ctx, expired.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Busy || got.BusyUntil != nil {
		t.Fatalf("expired object = %+v, want available", got)
	}
}

func TestObjectsListValidation(t *testing.T) {
	uc := NewObjects(newFakeObjects(), newFakeCategories(), newFakePhotos())
	ctx := context.Background()

	if _, _, err := uc.List(ctx, ListObjectsInput{Limit: 101}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("limit 101 error = %v, want ErrInvalid", err)
	}
	if _, _, err := uc.List(ctx, ListObjectsInput{Limit: -1}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("negative limit error = %v, want ErrInvalid", err)
	}
	if _, _, err := uc.List(ctx, ListObjectsInput{Offset: -1}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("negative offset error = %v, want ErrInvalid", err)
	}
	if _, _, err := uc.List(ctx, ListObjectsInput{}); err != nil {
		t.Fatalf("default list: %v", err)
	}
}

func TestCategoriesValidation(t *testing.T) {
	ctx := context.Background()
	repo := newFakeCategories()
	uc := NewCategories(repo)

	created, err := uc.Create(ctx, "  Science   Fiction ")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Name != "Science Fiction" {
		t.Fatalf("name = %q, want Science Fiction", created.Name)
	}
	if _, err := uc.Create(ctx, "science fiction"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate error = %v, want ErrConflict", err)
	}
	if _, err := uc.Create(ctx, "  "); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("empty name error = %v, want ErrInvalid", err)
	}
	renamed, err := uc.Update(ctx, created.ID, "Speculative Fiction")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if renamed.Name != "Speculative Fiction" {
		t.Fatalf("name = %q, want Speculative Fiction", renamed.Name)
	}
	if _, err := uc.Update(ctx, "missing", "Other"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing update error = %v, want ErrNotFound", err)
	}
	if got, err := uc.Get(ctx, created.ID); err != nil || got.ID != created.ID {
		t.Fatalf("Get = (%+v, %v), want created category", got, err)
	}
	if err := uc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := uc.Delete(ctx, created.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second Delete error = %v, want ErrNotFound", err)
	}
}

func TestObjectsPhotoLifecycle(t *testing.T) {
	ctx := context.Background()
	photos := newFakePhotos()
	objects := newFakeObjects()
	uc := NewObjects(objects, newFakeCategories(), photos)

	upload := func() *PhotoUpload {
		return &PhotoUpload{Reader: strings.NewReader("photo"), Filename: "photo.png"}
	}
	created, err := uc.Create(ctx, CreateObjectInput{Name: "Dune", Photo: upload()})
	if err != nil {
		t.Fatalf("Create with photo: %v", err)
	}
	if created.ImagePath == "" || created.ImagePath != photos.saved[0] {
		t.Fatalf("image path = %q, want %q", created.ImagePath, photos.saved[0])
	}

	replaced, err := uc.Update(ctx, created.ID, UpdateObjectInput{Photo: upload()})
	if err != nil {
		t.Fatalf("Update with photo: %v", err)
	}
	if replaced.ImagePath == created.ImagePath {
		t.Fatal("image path was not replaced")
	}
	if !containsString(photos.deleted, created.ImagePath) {
		t.Fatalf("old photo %q was not deleted; deleted = %v", created.ImagePath, photos.deleted)
	}

	removed, err := uc.Update(ctx, created.ID, UpdateObjectInput{RemovePhoto: true})
	if err != nil {
		t.Fatalf("Remove photo: %v", err)
	}
	if removed.ImagePath != "" {
		t.Fatalf("image path = %q, want empty", removed.ImagePath)
	}
	if !containsString(photos.deleted, replaced.ImagePath) {
		t.Fatalf("replaced photo %q was not deleted; deleted = %v", replaced.ImagePath, photos.deleted)
	}

	savedCount := len(photos.saved)
	if _, err := uc.Update(ctx, created.ID, UpdateObjectInput{Photo: upload(), RemovePhoto: true}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("replace+remove error = %v, want ErrInvalid", err)
	}
	if len(photos.saved) != savedCount {
		t.Fatal("ambiguous photo update saved a file")
	}

	emptyName := " "
	if _, err := uc.Update(ctx, created.ID, UpdateObjectInput{Name: &emptyName, Photo: upload()}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("invalid name with photo error = %v, want ErrInvalid", err)
	}
	if len(photos.saved) != savedCount {
		t.Fatal("invalid update saved a photo")
	}

	objects.createErr = errors.New("database unavailable")
	if _, err := uc.Create(ctx, CreateObjectInput{Name: "Hyperion", Photo: upload()}); err == nil {
		t.Fatal("Create succeeded, want database error")
	}
	if !containsString(photos.deleted, photos.saved[len(photos.saved)-1]) {
		t.Fatal("photo was not cleaned up after create failed")
	}
	objects.createErr = nil

	withPhoto, err := uc.Create(ctx, CreateObjectInput{Name: "Foundation", Photo: upload()})
	if err != nil {
		t.Fatalf("Create before delete: %v", err)
	}
	if err := uc.Delete(ctx, withPhoto.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !containsString(photos.deleted, withPhoto.ImagePath) {
		t.Fatalf("photo %q was not deleted with object", withPhoto.ImagePath)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
