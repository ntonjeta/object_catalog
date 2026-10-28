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
	uc := NewObjects(objects, categories, newFakePhotos(), newFakeReservations(), newFakeActivities())

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
	uc := NewObjects(objects, newFakeCategories(), newFakePhotos(), newFakeReservations(), newFakeActivities())
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
	uc := NewObjects(objects, newFakeCategories(), newFakePhotos(), newFakeReservations(), newFakeActivities())
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
	if reserved.Busy || reserved.Reservation == nil || !reserved.Reservation.From.Equal(from) {
		t.Fatalf("reserved object = %+v, want pending reservation from %v", reserved, from)
	}
	if reserved.IsBusyAt(now) {
		t.Fatal("reserved object is busy before reservation start")
	}

	got, err := uc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Busy || got.Reservation == nil {
		t.Fatalf("get reserved object = %+v, want reservation", got)
	}

	released, err := uc.SetBusy(ctx, created.ID, SetObjectBusyInput{})
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released.Busy || released.Reservation != nil {
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
	uc := NewObjects(objects, newFakeCategories(), newFakePhotos(), newFakeReservations(), newFakeActivities())
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
	uc := NewObjects(newFakeObjects(), newFakeCategories(), newFakePhotos(), newFakeReservations(), newFakeActivities())
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
	uc := NewObjects(objects, newFakeCategories(), photos, newFakeReservations(), newFakeActivities())

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
	// The sqlite repo seeds row 0 from an uploaded cover; the fake mirrors it.
	stored, err := objects.Images(ctx, created.ID)
	if err != nil {
		t.Fatalf("stored images: %v", err)
	}
	if len(stored) != 1 || stored[0].FileName != created.ImagePath {
		t.Fatalf("stored images = %+v, want one row %q", stored, created.ImagePath)
	}
	coverRowID := stored[0].ID

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
	stored, err = objects.Images(ctx, created.ID)
	if err != nil {
		t.Fatalf("stored images after replace: %v", err)
	}
	if len(stored) != 1 || stored[0].ID != coverRowID || stored[0].FileName != replaced.ImagePath {
		t.Fatalf("stored images = %+v, want row 0 id %s overwritten with %q", stored, coverRowID, replaced.ImagePath)
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
	stored, err = objects.Images(ctx, created.ID)
	if err != nil {
		t.Fatalf("stored images after remove: %v", err)
	}
	if len(stored) != 0 {
		t.Fatalf("stored images = %+v, want empty list after RemovePhoto", stored)
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
	extra, err := uc.AddImages(ctx, withPhoto.ID, []PhotoUpload{*upload()})
	if err != nil {
		t.Fatalf("AddImages before delete: %v", err)
	}
	if len(extra.Images) != 2 {
		t.Fatalf("images = %+v, want cover plus one extra", extra.Images)
	}
	if err := uc.Delete(ctx, withPhoto.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !containsString(photos.deleted, withPhoto.ImagePath) {
		t.Fatalf("photo %q was not deleted with object", withPhoto.ImagePath)
	}
	if !containsString(photos.deleted, extra.Images[1].FileName) {
		t.Fatalf("extra image %q was not deleted with object", extra.Images[1].FileName)
	}
}

func TestObjectsAddImages(t *testing.T) {
	ctx := context.Background()
	photos := newFakePhotos()
	objects := newFakeObjects()
	uc := NewObjects(objects, newFakeCategories(), photos, newFakeReservations(), newFakeActivities())

	created, err := uc.Create(ctx, CreateObjectInput{Name: "Dune"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	two, err := uc.AddImages(ctx, created.ID, []PhotoUpload{
		{Reader: strings.NewReader("one"), Filename: "one.png"},
		{Reader: strings.NewReader("two"), Filename: "two.png"},
	})
	if err != nil {
		t.Fatalf("AddImages: %v", err)
	}
	if len(two.Images) != 2 {
		t.Fatalf("images = %+v, want two rows", two.Images)
	}
	if two.Images[0].FileName != photos.saved[0] || two.Images[1].FileName != photos.saved[1] {
		t.Fatalf("image files = %+v, want %q then %q", two.Images, photos.saved[0], photos.saved[1])
	}
	if two.Images[0].ID == "" || two.Images[0].ID == two.Images[1].ID {
		t.Fatalf("image ids not distinct: %+v", two.Images)
	}
	// The list was empty, so the first uploaded file takes over the cover.
	if two.ImagePath != two.Images[0].FileName {
		t.Fatalf("cover = %q, want %q", two.ImagePath, two.Images[0].FileName)
	}
	if len(photos.deleted) != 0 {
		t.Fatalf("AddImages deleted files: %v", photos.deleted)
	}
	stored, err := objects.Images(ctx, created.ID)
	if err != nil {
		t.Fatalf("stored images: %v", err)
	}
	if len(stored) != 2 || stored[0].ID != two.Images[0].ID || stored[1].ID != two.Images[1].ID {
		t.Fatalf("stored images = %+v, want %+v", stored, two.Images)
	}
	if got, err := uc.Get(ctx, created.ID); err != nil || got.ImagePath != two.ImagePath || len(got.Images) != 2 {
		t.Fatalf("Get = (%+v, %v), want synced cover and two images", got, err)
	}

	// Appending to a non-empty list never displaces row 0: cover unchanged.
	third, err := uc.AddImages(ctx, created.ID, []PhotoUpload{{Reader: strings.NewReader("three"), Filename: "three.png"}})
	if err != nil {
		t.Fatalf("AddImages third: %v", err)
	}
	if len(third.Images) != 3 || third.Images[0].ID != two.Images[0].ID || third.Images[0].FileName != two.Images[0].FileName {
		t.Fatalf("images = %+v, want row 0 preserved", third.Images)
	}
	if third.ImagePath != two.ImagePath {
		t.Fatalf("cover = %q, want unchanged %q", third.ImagePath, two.ImagePath)
	}

	// A legacy URL cover is replaced by images[0] on the first upload (Q7).
	urlObject, err := uc.Create(ctx, CreateObjectInput{Name: "Hyperion", PhotoPath: "https://example.com/cover.jpg"})
	if err != nil {
		t.Fatalf("Create with URL cover: %v", err)
	}
	added, err := uc.AddImages(ctx, urlObject.ID, []PhotoUpload{{Reader: strings.NewReader("first"), Filename: "first.png"}})
	if err != nil {
		t.Fatalf("AddImages over URL cover: %v", err)
	}
	if added.ImagePath != added.Images[0].FileName {
		t.Fatalf("cover = %q, want first image file %q", added.ImagePath, added.Images[0].FileName)
	}

	// The cap is validated before any file is saved.
	for i := 2; i <= 10; i++ {
		if _, err := uc.AddImages(ctx, urlObject.ID, []PhotoUpload{{Reader: strings.NewReader("fill"), Filename: "fill.png"}}); err != nil {
			t.Fatalf("fill to %d images: %v", i, err)
		}
	}
	savedBefore := len(photos.saved)
	if _, err := uc.AddImages(ctx, urlObject.ID, []PhotoUpload{{Reader: strings.NewReader("late"), Filename: "late.png"}}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("11th image error = %v, want ErrInvalid", err)
	}
	if len(photos.saved) != savedBefore {
		t.Fatal("over-cap AddImages saved a file")
	}
	if stored, err := objects.Images(ctx, urlObject.ID); err != nil || len(stored) != 10 {
		t.Fatalf("stored images = %d (%v), want 10 rows", len(stored), err)
	}

	if _, err := uc.AddImages(ctx, "missing", []PhotoUpload{{Reader: strings.NewReader("x"), Filename: "x.png"}}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing object error = %v, want ErrNotFound", err)
	}
}

func TestObjectsRemoveImageCoverSync(t *testing.T) {
	ctx := context.Background()
	photos := newFakePhotos()
	objects := newFakeObjects()
	uc := NewObjects(objects, newFakeCategories(), photos, newFakeReservations(), newFakeActivities())

	created, err := uc.Create(ctx, CreateObjectInput{Name: "Dune", Photo: &PhotoUpload{Reader: strings.NewReader("cover"), Filename: "cover.png"}})
	if err != nil {
		t.Fatalf("Create with photo: %v", err)
	}
	coverFile := created.ImagePath
	withExtras, err := uc.AddImages(ctx, created.ID, []PhotoUpload{
		{Reader: strings.NewReader("second"), Filename: "second.png"},
		{Reader: strings.NewReader("third"), Filename: "third.png"},
	})
	if err != nil {
		t.Fatalf("AddImages: %v", err)
	}
	if len(withExtras.Images) != 3 || withExtras.ImagePath != coverFile {
		t.Fatalf("images = %+v, cover = %q, want three rows under %q", withExtras.Images, withExtras.ImagePath, coverFile)
	}
	coverID := withExtras.Images[0].ID
	second := withExtras.Images[1]
	third := withExtras.Images[2]

	// Unknown ids surface ErrNotFound without touching files.
	if _, err := uc.RemoveImage(ctx, created.ID, "ghost"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown image error = %v, want ErrNotFound", err)
	}
	if _, err := uc.RemoveImage(ctx, "missing", coverID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing object error = %v, want ErrNotFound", err)
	}
	if len(photos.deleted) != 0 {
		t.Fatalf("failed removes deleted files: %v", photos.deleted)
	}

	// Removing a non-cover row leaves the cover alone and deletes only that
	// file after the repository write.
	afterMiddle, err := uc.RemoveImage(ctx, created.ID, second.ID)
	if err != nil {
		t.Fatalf("RemoveImage middle: %v", err)
	}
	if len(afterMiddle.Images) != 2 || afterMiddle.Images[0].ID != coverID || afterMiddle.Images[1].ID != third.ID {
		t.Fatalf("images = %+v, want cover then third", afterMiddle.Images)
	}
	if afterMiddle.ImagePath != coverFile {
		t.Fatalf("cover = %q, want unchanged %q", afterMiddle.ImagePath, coverFile)
	}
	if !containsString(photos.deleted, second.FileName) {
		t.Fatalf("removed image file was not deleted; deleted = %v", photos.deleted)
	}
	if containsString(photos.deleted, coverFile) || containsString(photos.deleted, third.FileName) {
		t.Fatalf("surviving files were deleted: %v", photos.deleted)
	}

	// Removing the cover row resyncs image_path to the new row 0 and only
	// then deletes the displaced file.
	afterCover, err := uc.RemoveImage(ctx, created.ID, coverID)
	if err != nil {
		t.Fatalf("RemoveImage cover: %v", err)
	}
	if len(afterCover.Images) != 1 || afterCover.Images[0].ID != third.ID {
		t.Fatalf("images = %+v, want only third", afterCover.Images)
	}
	if afterCover.ImagePath != third.FileName {
		t.Fatalf("cover = %q, want new row 0 %q", afterCover.ImagePath, third.FileName)
	}
	if !containsString(photos.deleted, coverFile) {
		t.Fatalf("displaced cover file was not deleted; deleted = %v", photos.deleted)
	}
	if containsString(photos.deleted, third.FileName) {
		t.Fatalf("new cover file was deleted: %v", photos.deleted)
	}
	if stored, err := objects.Images(ctx, created.ID); err != nil || len(stored) != 1 || stored[0].ID != third.ID {
		t.Fatalf("stored images = %+v (%v), want only third", stored, err)
	}

	// Removing the last row clears the cover.
	empty, err := uc.RemoveImage(ctx, created.ID, third.ID)
	if err != nil {
		t.Fatalf("RemoveImage last: %v", err)
	}
	if len(empty.Images) != 0 || empty.ImagePath != "" {
		t.Fatalf("images = %+v, cover = %q, want empty list and cleared cover", empty.Images, empty.ImagePath)
	}
	if !containsString(photos.deleted, third.FileName) {
		t.Fatalf("last image file was not deleted; deleted = %v", photos.deleted)
	}

	// Failure windows: a second object proves files stay on disk until every
	// repository write commits, and the stale cover is swept up afterwards.
	secondObject, err := uc.Create(ctx, CreateObjectInput{Name: "Hyperion", Photo: &PhotoUpload{Reader: strings.NewReader("cover"), Filename: "cover.png"}})
	if err != nil {
		t.Fatalf("Create second object: %v", err)
	}
	withExtra, err := uc.AddImages(ctx, secondObject.ID, []PhotoUpload{{Reader: strings.NewReader("extra"), Filename: "extra.png"}})
	if err != nil {
		t.Fatalf("AddImages second object: %v", err)
	}
	coverID2 := withExtra.Images[0].ID
	extra := withExtra.Images[1]

	deletedBefore := len(photos.deleted)
	objects.replaceImageErr = errors.New("database unavailable")
	if _, err := uc.RemoveImage(ctx, secondObject.ID, coverID2); err == nil {
		t.Fatal("RemoveImage succeeded despite ReplaceImages error")
	}
	if len(photos.deleted) != deletedBefore {
		t.Fatalf("failed RemoveImage deleted files: %v", photos.deleted[deletedBefore:])
	}
	if stored, err := objects.Images(ctx, secondObject.ID); err != nil || len(stored) != 2 {
		t.Fatalf("stored images = %+v (%v), want list intact after failure", stored, err)
	}
	objects.replaceImageErr = nil

	// The cover row write fails after the list rewrite committed: the
	// displaced file must survive because the stale cover still references
	// it, and the retry that removes the last row sweeps it up.
	objects.updateErr = errors.New("database unavailable")
	if _, err := uc.RemoveImage(ctx, secondObject.ID, coverID2); err == nil {
		t.Fatal("RemoveImage succeeded despite update error")
	}
	if containsString(photos.deleted[deletedBefore:], withExtra.ImagePath) {
		t.Fatalf("cover file deleted despite repository failure: %v", photos.deleted[deletedBefore:])
	}
	if containsString(photos.deleted[deletedBefore:], extra.FileName) {
		t.Fatalf("listed file deleted despite failure: %v", photos.deleted[deletedBefore:])
	}
	objects.updateErr = nil

	final, err := uc.RemoveImage(ctx, secondObject.ID, extra.ID)
	if err != nil {
		t.Fatalf("RemoveImage sweep: %v", err)
	}
	if len(final.Images) != 0 || final.ImagePath != "" {
		t.Fatalf("images = %+v, cover = %q, want cleared", final.Images, final.ImagePath)
	}
	if !containsString(photos.deleted, withExtra.ImagePath) {
		t.Fatalf("stale cover file was not swept up; deleted = %v", photos.deleted)
	}
	if !containsString(photos.deleted, extra.FileName) {
		t.Fatalf("removed image file was not deleted; deleted = %v", photos.deleted)
	}
}

func TestObjectsReorderImages(t *testing.T) {
	ctx := context.Background()
	photos := newFakePhotos()
	objects := newFakeObjects()
	uc := NewObjects(objects, newFakeCategories(), photos, newFakeReservations(), newFakeActivities())

	created, err := uc.Create(ctx, CreateObjectInput{Name: "Dune", Photo: &PhotoUpload{Reader: strings.NewReader("cover"), Filename: "cover.png"}})
	if err != nil {
		t.Fatalf("Create with photo: %v", err)
	}
	withExtras, err := uc.AddImages(ctx, created.ID, []PhotoUpload{
		{Reader: strings.NewReader("second"), Filename: "second.png"},
		{Reader: strings.NewReader("third"), Filename: "third.png"},
	})
	if err != nil {
		t.Fatalf("AddImages: %v", err)
	}
	current := withExtras.Images

	if _, err := uc.ReorderImages(ctx, "missing", []string{current[0].ID}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing object error = %v, want ErrNotFound", err)
	}
	if _, err := uc.ReorderImages(ctx, created.ID, []string{current[1].ID, current[2].ID}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("short permutation error = %v, want ErrInvalid", err)
	}
	if _, err := uc.ReorderImages(ctx, created.ID, []string{current[0].ID, current[1].ID, "ghost"}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("unknown id error = %v, want ErrInvalid", err)
	}
	if _, err := uc.ReorderImages(ctx, created.ID, []string{current[0].ID, current[0].ID, current[2].ID}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("duplicate id error = %v, want ErrInvalid", err)
	}
	if stored, err := objects.Images(ctx, created.ID); err != nil || len(stored) != 3 || stored[0].ID != current[0].ID {
		t.Fatalf("stored images after failed reorders = %+v (%v), want unchanged order", stored, err)
	}

	reordered, err := uc.ReorderImages(ctx, created.ID, []string{current[2].ID, current[0].ID, current[1].ID})
	if err != nil {
		t.Fatalf("ReorderImages: %v", err)
	}
	want := []string{current[2].ID, current[0].ID, current[1].ID}
	for i, image := range reordered.Images {
		if image.ID != want[i] {
			t.Fatalf("image order = %+v, want ids %v", reordered.Images, want)
		}
	}
	if reordered.ImagePath != current[2].FileName {
		t.Fatalf("cover = %q, want new row 0 %q", reordered.ImagePath, current[2].FileName)
	}
	if stored, err := objects.Images(ctx, created.ID); err != nil || len(stored) != 3 || stored[0].ID != current[2].ID {
		t.Fatalf("stored images = %+v (%v), want third first", stored, err)
	}
	// Reordering deletes nothing: every file is still referenced.
	if len(photos.deleted) != 0 {
		t.Fatalf("ReorderImages deleted files: %v", photos.deleted)
	}

	// Reordering back restores the original cover.
	back, err := uc.ReorderImages(ctx, created.ID, []string{current[0].ID, current[2].ID, current[1].ID})
	if err != nil {
		t.Fatalf("ReorderImages back: %v", err)
	}
	if back.ImagePath != created.ImagePath {
		t.Fatalf("cover = %q, want %q", back.ImagePath, created.ImagePath)
	}
	if len(photos.deleted) != 0 {
		t.Fatalf("ReorderImages deleted files: %v", photos.deleted)
	}
}

func TestObjectsRemovePhotoClearsImages(t *testing.T) {
	ctx := context.Background()
	photos := newFakePhotos()
	objects := newFakeObjects()
	uc := NewObjects(objects, newFakeCategories(), photos, newFakeReservations(), newFakeActivities())

	created, err := uc.Create(ctx, CreateObjectInput{Name: "Dune", Photo: &PhotoUpload{Reader: strings.NewReader("cover"), Filename: "cover.png"}})
	if err != nil {
		t.Fatalf("Create with photo: %v", err)
	}
	extra, err := uc.AddImages(ctx, created.ID, []PhotoUpload{{Reader: strings.NewReader("second"), Filename: "second.png"}})
	if err != nil {
		t.Fatalf("AddImages: %v", err)
	}

	removed, err := uc.Update(ctx, created.ID, UpdateObjectInput{RemovePhoto: true})
	if err != nil {
		t.Fatalf("RemovePhoto: %v", err)
	}
	if removed.ImagePath != "" {
		t.Fatalf("image path = %q, want empty", removed.ImagePath)
	}
	if len(removed.Images) != 0 {
		t.Fatalf("images = %+v, want emptied list", removed.Images)
	}
	if stored, err := objects.Images(ctx, created.ID); err != nil || len(stored) != 0 {
		t.Fatalf("stored images = %+v (%v), want empty list", stored, err)
	}
	if !containsString(photos.deleted, created.ImagePath) {
		t.Fatalf("cover file %q was not deleted; deleted = %v", created.ImagePath, photos.deleted)
	}
	if !containsString(photos.deleted, extra.Images[1].FileName) {
		t.Fatalf("extra image file %q was not deleted; deleted = %v", extra.Images[1].FileName, photos.deleted)
	}

	// Failure injection: a rejected ReplaceImages must leave every file on
	// disk because the surviving state still references them.
	rebuild, err := uc.Create(ctx, CreateObjectInput{Name: "Foundation", Photo: &PhotoUpload{Reader: strings.NewReader("cover"), Filename: "cover.png"}})
	if err != nil {
		t.Fatalf("Create rebuild: %v", err)
	}
	rebuildExtra, err := uc.AddImages(ctx, rebuild.ID, []PhotoUpload{{Reader: strings.NewReader("second"), Filename: "second.png"}})
	if err != nil {
		t.Fatalf("AddImages rebuild: %v", err)
	}
	if len(rebuildExtra.Images) != 2 {
		t.Fatalf("rebuilt images = %+v, want cover plus one extra", rebuildExtra.Images)
	}
	deletedBefore := len(photos.deleted)
	objects.replaceImageErr = errors.New("database unavailable")
	if _, err := uc.Update(ctx, rebuild.ID, UpdateObjectInput{RemovePhoto: true}); err == nil {
		t.Fatal("RemovePhoto succeeded despite ReplaceImages error")
	}
	objects.replaceImageErr = nil
	if len(photos.deleted) != deletedBefore {
		t.Fatalf("failed RemovePhoto deleted files: %v", photos.deleted[deletedBefore:])
	}
	if stored, err := objects.Images(ctx, rebuild.ID); err != nil || len(stored) != 2 {
		t.Fatalf("stored images = %+v (%v), want list untouched after failure", stored, err)
	}
}

func TestObjectsAddImagesRollback(t *testing.T) {
	ctx := context.Background()
	photos := newFakePhotos()
	objects := newFakeObjects()
	uc := NewObjects(objects, newFakeCategories(), photos, newFakeReservations(), newFakeActivities())

	created, err := uc.Create(ctx, CreateObjectInput{Name: "Dune", Photo: &PhotoUpload{Reader: strings.NewReader("cover"), Filename: "cover.png"}})
	if err != nil {
		t.Fatalf("Create with photo: %v", err)
	}

	objects.replaceImageErr = errors.New("database unavailable")
	savedBefore := len(photos.saved)
	_, err = uc.AddImages(ctx, created.ID, []PhotoUpload{
		{Reader: strings.NewReader("one"), Filename: "one.png"},
		{Reader: strings.NewReader("two"), Filename: "two.png"},
	})
	if err == nil {
		t.Fatal("AddImages succeeded despite ReplaceImages error")
	}
	newFiles := photos.saved[savedBefore:]
	if len(newFiles) != 2 {
		t.Fatalf("AddImages saved %d files, want two before the failure", len(newFiles))
	}
	for _, fileName := range newFiles {
		if !containsString(photos.deleted, fileName) {
			t.Fatalf("newly saved file %q was not rolled back; deleted = %v", fileName, photos.deleted)
		}
	}
	if containsString(photos.deleted, created.ImagePath) {
		t.Fatalf("existing image file %q was deleted during rollback", created.ImagePath)
	}
	if stored, err := objects.Images(ctx, created.ID); err != nil || len(stored) != 1 || stored[0].FileName != created.ImagePath {
		t.Fatalf("stored images = %+v (%v), want unchanged cover row", stored, err)
	}
	objects.replaceImageErr = nil

	// After clearing the failure the same call succeeds and keeps files.
	deletedBefore := len(photos.deleted)
	recovered, err := uc.AddImages(ctx, created.ID, []PhotoUpload{
		{Reader: strings.NewReader("one"), Filename: "one.png"},
		{Reader: strings.NewReader("two"), Filename: "two.png"},
	})
	if err != nil {
		t.Fatalf("AddImages after rollback: %v", err)
	}
	if len(recovered.Images) != 3 {
		t.Fatalf("images = %+v, want cover plus two", recovered.Images)
	}
	if len(photos.deleted) != deletedBefore {
		t.Fatalf("successful AddImages deleted files: %v", photos.deleted[deletedBefore:])
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
