package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"biblio-oggetto/internal/domain"
)

const (
	defaultObjectLimit = 20
	maxObjectLimit     = 100
)

type Objects struct {
	objects      ObjectRepository
	categories   CategoryRepository
	photos       PhotoStore
	reservations ReservationRepository
	activities   ActivityRepository
}

func NewObjects(objects ObjectRepository, categories CategoryRepository, photos PhotoStore, reservations ReservationRepository, activities ActivityRepository) *Objects {
	return &Objects{objects: objects, categories: categories, photos: photos, reservations: reservations, activities: activities}
}

type CreateObjectInput struct {
	Name        string
	Description string
	CategoryID  string
	Tags        []string
	Photo       *PhotoUpload
	PhotoPath   string
}

type UpdateObjectInput struct {
	Name        *string
	Description *string
	CategoryID  *string
	Tags        *[]string
	Photo       *PhotoUpload
	PhotoPath   *string
	RemovePhoto bool
}

type ListObjectsInput struct {
	Query    string
	Category string
	Tag      string
	Limit    int
	Offset   int
}

type SetObjectBusyInput struct {
	Busy      bool
	BusyUntil *time.Time
}

func (uc *Objects) Create(ctx context.Context, input CreateObjectInput) (domain.Object, error) {
	name := domain.NormalizeObjectName(input.Name)
	if err := domain.ValidateObjectName(name); err != nil {
		return domain.Object{}, err
	}
	description := strings.TrimSpace(input.Description)
	if err := domain.ValidateObjectDescription(description); err != nil {
		return domain.Object{}, err
	}
	tags, err := domain.NormalizeTags(input.Tags)
	if err != nil {
		return domain.Object{}, err
	}
	imageFile := strings.TrimSpace(input.PhotoPath)
	if err := domain.ValidateObjectPhotoPath(imageFile); err != nil {
		return domain.Object{}, err
	}
	uploadedPhoto := false
	if input.Photo != nil {
		if imageFile != "" {
			return domain.Object{}, fmt.Errorf("%w: cannot upload and set a photo path at the same time", domain.ErrInvalid)
		}
		imageFile, err = uc.savePhoto(ctx, input.Photo)
		if err != nil {
			return domain.Object{}, err
		}
		uploadedPhoto = true
	}
	category, err := uc.category(ctx, input.CategoryID)
	if err != nil {
		if uploadedPhoto {
			uc.deletePhoto(ctx, imageFile)
		}
		return domain.Object{}, err
	}
	id, err := domain.NewID()
	if err != nil {
		if uploadedPhoto {
			uc.deletePhoto(ctx, imageFile)
		}
		return domain.Object{}, fmt.Errorf("generate object id: %w", err)
	}
	now := time.Now().UTC()
	object := domain.Object{
		ID:          id,
		Name:        name,
		Description: description,
		ImagePath:   imageFile,
		Category:    category,
		Tags:        tags,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := uc.objects.Create(ctx, object); err != nil {
		if uploadedPhoto {
			uc.deletePhoto(ctx, imageFile)
		}
		return domain.Object{}, err
	}
	return object, nil
}

func (uc *Objects) Update(ctx context.Context, id string, input UpdateObjectInput) (domain.Object, error) {
	if input.Photo != nil && input.RemovePhoto {
		return domain.Object{}, fmt.Errorf("%w: cannot replace and remove the photo at the same time", domain.ErrInvalid)
	}
	if input.Photo != nil && input.PhotoPath != nil {
		return domain.Object{}, fmt.Errorf("%w: cannot upload and set a photo path at the same time", domain.ErrInvalid)
	}
	if input.RemovePhoto && input.PhotoPath != nil {
		return domain.Object{}, fmt.Errorf("%w: cannot remove and set a photo path at the same time", domain.ErrInvalid)
	}
	object, err := uc.objects.ByID(ctx, id)
	if err != nil {
		return domain.Object{}, err
	}
	oldImagePath := object.ImagePath
	oldImages := object.Images
	if input.Name != nil {
		name := domain.NormalizeObjectName(*input.Name)
		if err := domain.ValidateObjectName(name); err != nil {
			return domain.Object{}, err
		}
		object.Name = name
	}
	if input.Description != nil {
		description := strings.TrimSpace(*input.Description)
		if err := domain.ValidateObjectDescription(description); err != nil {
			return domain.Object{}, err
		}
		object.Description = description
	}
	if input.Tags != nil {
		tags, err := domain.NormalizeTags(*input.Tags)
		if err != nil {
			return domain.Object{}, err
		}
		object.Tags = tags
	}
	if input.CategoryID != nil {
		category, err := uc.category(ctx, *input.CategoryID)
		if err != nil {
			return domain.Object{}, err
		}
		object.Category = category
	}
	if input.PhotoPath != nil {
		imageFile := strings.TrimSpace(*input.PhotoPath)
		if err := domain.ValidateObjectPhotoPath(imageFile); err != nil {
			return domain.Object{}, err
		}
		object.ImagePath = imageFile
	}
	newImagePath, err := uc.savePhoto(ctx, input.Photo)
	if err != nil {
		return domain.Object{}, err
	}
	images := oldImages
	if newImagePath != "" {
		object.ImagePath = newImagePath
		if len(images) > 0 {
			// The cover is row 0 of the list: overwrite that row in place,
			// keeping its id so existing API addresses stay valid. When the
			// list is empty the upload stays a pure image_path swap; no row
			// is invented.
			replaced := append([]domain.ObjectImage{}, images...)
			replaced[0].FileName = newImagePath
			images = replaced
		}
	} else if input.RemovePhoto {
		object.ImagePath = ""
		images = nil
	}
	object.Images = images
	object.UpdatedAt = time.Now().UTC()
	if err := uc.objects.Update(ctx, object); err != nil {
		uc.deletePhoto(ctx, newImagePath)
		return domain.Object{}, err
	}
	if !imageListsEqual(oldImages, images) {
		if err := uc.objects.ReplaceImages(ctx, object.ID, images); err != nil {
			// The cover row is already committed, so every file this call
			// could delete is still referenced by the committed cover or the
			// surviving old list. Touch nothing and let the caller retry.
			return domain.Object{}, err
		}
	}
	for _, fileName := range removedImageFiles(oldImagePath, oldImages, object.ImagePath, images) {
		uc.deletePhoto(ctx, fileName)
	}
	uc.attachReservation(ctx, &object, time.Now().UTC())
	return object, nil
}

// AddImages appends uploaded photos to the object's ordered image list. The
// prospective count is validated before any file is saved, and every newly
// saved file is discarded when a repository write fails, mirroring the Create
// rollback. Appending never displaces row 0, so the cover only changes when
// the list was empty and the first uploaded file takes over a URL cover or an
// empty image_path.
func (uc *Objects) AddImages(ctx context.Context, objectID string, uploads []PhotoUpload) (domain.Object, error) {
	object, err := uc.objects.ByID(ctx, objectID)
	if err != nil {
		return domain.Object{}, err
	}
	if err := domain.ValidateObjectImageCount(len(object.Images) + len(uploads)); err != nil {
		return domain.Object{}, err
	}
	oldImagePath := object.ImagePath
	oldImages := object.Images
	images := append([]domain.ObjectImage{}, oldImages...)
	saved := make([]string, 0, len(uploads))
	discardSaved := func() {
		for _, fileName := range saved {
			uc.deletePhoto(ctx, fileName)
		}
	}
	for i := range uploads {
		fileName, err := uc.savePhoto(ctx, &uploads[i])
		if err != nil {
			discardSaved()
			return domain.Object{}, err
		}
		saved = append(saved, fileName)
		imageID, err := domain.NewID()
		if err != nil {
			discardSaved()
			return domain.Object{}, fmt.Errorf("generate object image id: %w", err)
		}
		images = append(images, domain.ObjectImage{ID: imageID, FileName: fileName})
	}
	if err := uc.objects.ReplaceImages(ctx, objectID, images); err != nil {
		discardSaved()
		return domain.Object{}, err
	}
	object.Images = images
	if len(oldImages) == 0 && len(images) > 0 && images[0].FileName != oldImagePath {
		object.ImagePath = images[0].FileName
		object.UpdatedAt = time.Now().UTC()
		if err := uc.objects.Update(ctx, object); err != nil {
			// The list is committed and references the new files, so they
			// cannot be discarded here; the cover resyncs on retry.
			return domain.Object{}, err
		}
		for _, fileName := range removedImageFiles(oldImagePath, oldImages, object.ImagePath, images) {
			uc.deletePhoto(ctx, fileName)
		}
	}
	uc.attachReservation(ctx, &object, time.Now().UTC())
	return object, nil
}

// RemoveImage drops one image from the object's list and renumbers the
// remaining positions. When the removed row was the cover, image_path follows
// the new row 0 (or clears). Displaced files are deleted only after every
// repository write succeeded, matching Update's oldImagePath discipline.
func (uc *Objects) RemoveImage(ctx context.Context, objectID, imageID string) (domain.Object, error) {
	object, err := uc.objects.ByID(ctx, objectID)
	if err != nil {
		return domain.Object{}, err
	}
	index := -1
	for i, image := range object.Images {
		if image.ID == imageID {
			index = i
			break
		}
	}
	if index < 0 {
		return domain.Object{}, fmt.Errorf("%w: object %s has no image %s", domain.ErrNotFound, objectID, imageID)
	}
	oldImagePath := object.ImagePath
	oldImages := object.Images
	rest := make([]domain.ObjectImage, 0, len(oldImages)-1)
	rest = append(rest, oldImages[:index]...)
	rest = append(rest, oldImages[index+1:]...)
	if err := uc.objects.ReplaceImages(ctx, objectID, rest); err != nil {
		return domain.Object{}, err
	}
	if index == 0 {
		if len(rest) > 0 {
			object.ImagePath = rest[0].FileName
		} else {
			object.ImagePath = ""
		}
		object.UpdatedAt = time.Now().UTC()
		if err := uc.objects.Update(ctx, object); err != nil {
			// The new list is committed; the displaced cover may still be
			// referenced by the stale cover row, so nothing is deleted.
			return domain.Object{}, err
		}
	}
	object.Images = rest
	for _, fileName := range removedImageFiles(oldImagePath, oldImages, object.ImagePath, rest) {
		uc.deletePhoto(ctx, fileName)
	}
	uc.attachReservation(ctx, &object, time.Now().UTC())
	return object, nil
}

// ReorderImages rewrites the image list with the submitted id order. The ids
// must be a permutation of the current list; no files are deleted because
// every file stays referenced. The cover follows the new row 0.
func (uc *Objects) ReorderImages(ctx context.Context, objectID string, orderedIDs []string) (domain.Object, error) {
	object, err := uc.objects.ByID(ctx, objectID)
	if err != nil {
		return domain.Object{}, err
	}
	if len(orderedIDs) != len(object.Images) {
		return domain.Object{}, fmt.Errorf("%w: image ids must be a permutation of the object's images", domain.ErrInvalid)
	}
	byID := make(map[string]domain.ObjectImage, len(object.Images))
	for _, image := range object.Images {
		byID[image.ID] = image
	}
	ordered := make([]domain.ObjectImage, 0, len(orderedIDs))
	seen := make(map[string]struct{}, len(orderedIDs))
	for _, imageID := range orderedIDs {
		image, ok := byID[imageID]
		if !ok {
			return domain.Object{}, fmt.Errorf("%w: unknown image id %s", domain.ErrInvalid, imageID)
		}
		if _, duplicate := seen[imageID]; duplicate {
			return domain.Object{}, fmt.Errorf("%w: duplicate image id %s", domain.ErrInvalid, imageID)
		}
		seen[imageID] = struct{}{}
		ordered = append(ordered, image)
	}
	if err := uc.objects.ReplaceImages(ctx, objectID, ordered); err != nil {
		return domain.Object{}, err
	}
	object.Images = ordered
	if len(ordered) > 0 && ordered[0].FileName != object.ImagePath {
		object.ImagePath = ordered[0].FileName
		object.UpdatedAt = time.Now().UTC()
		if err := uc.objects.Update(ctx, object); err != nil {
			return domain.Object{}, err
		}
	}
	uc.attachReservation(ctx, &object, time.Now().UTC())
	return object, nil
}

func (uc *Objects) Get(ctx context.Context, id string) (domain.Object, error) {
	object, err := uc.objects.ByID(ctx, id)
	if err != nil {
		return domain.Object{}, err
	}
	now := time.Now().UTC()
	normalizeBusy(&object, now)
	uc.attachReservation(ctx, &object, now)
	return object, nil
}

func (uc *Objects) SetBusy(ctx context.Context, id string, input SetObjectBusyInput) (domain.Object, error) {
	object, err := uc.objects.ByID(ctx, id)
	if err != nil {
		return domain.Object{}, err
	}
	now := time.Now().UTC()
	normalizeBusy(&object, now)
	actives, err := uc.reservations.ActiveByObject(ctx, id)
	if err != nil {
		return domain.Object{}, err
	}
	if input.Busy {
		if input.BusyUntil == nil {
			return domain.Object{}, fmt.Errorf("%w: busy_until is required when busy is true", domain.ErrInvalid)
		}
		until := input.BusyUntil.UTC()
		if !until.After(now) {
			return domain.Object{}, fmt.Errorf("%w: busy_until must be in the future", domain.ErrInvalid)
		}
		for _, reservation := range actives {
			if reservation.TakesAt(now) {
				return domain.Object{}, fmt.Errorf("%w: confirmed reservation is active %s", domain.ErrConflict, reservationWindow(reservation))
			}
			if reservation.Status == domain.ReservationPending && reservation.From.Before(until) {
				return domain.Object{}, fmt.Errorf("%w: loan would overlap pending reservation starting %s", domain.ErrConflict, reservation.From.Format(time.RFC3339))
			}
		}
		object.Busy = true
		object.BusyUntil = &until
	} else {
		object.Busy = false
		object.BusyUntil = nil
		for _, reservation := range actives {
			if err := uc.cancelReservation(ctx, reservation, now); err != nil {
				return domain.Object{}, err
			}
		}
	}
	object.UpdatedAt = now
	if err := uc.objects.Update(ctx, object); err != nil {
		return domain.Object{}, err
	}
	activityType := domain.ActivityObjectBusy
	detail := "busy until " + formatOptionalTime(object.BusyUntil)
	if !input.Busy {
		activityType = domain.ActivityObjectReleased
		detail = "released"
	}
	if err := recordActivity(ctx, uc.activities, activityType, id, "", detail, now); err != nil {
		return domain.Object{}, err
	}
	uc.attachReservation(ctx, &object, now)
	return object, nil
}

type ReserveObjectInput struct {
	From  time.Time
	Until *time.Time
}

// Reserve places a pending reservation on the object, replacing any live
// reservation. The object becomes unavailable only once the window starts and
// the reservation is confirmed; until then it stays lendable.
func (uc *Objects) Reserve(ctx context.Context, id string, input ReserveObjectInput) (domain.Object, error) {
	object, err := uc.objects.ByID(ctx, id)
	if err != nil {
		return domain.Object{}, err
	}
	now := time.Now().UTC()
	normalizeBusy(&object, now)
	from := input.From.UTC()
	if !from.After(now) {
		return domain.Object{}, fmt.Errorf("%w: reservation start must be in the future", domain.ErrInvalid)
	}
	var until *time.Time
	if input.Until != nil {
		value := input.Until.UTC()
		if !value.After(from) {
			return domain.Object{}, fmt.Errorf("%w: reservation end must be after the start", domain.ErrInvalid)
		}
		until = &value
	}
	if object.IsBusyAt(now) {
		return domain.Object{}, fmt.Errorf("%w: object is on loan until %s", domain.ErrConflict, formatOptionalTime(object.BusyUntil))
	}
	actives, err := uc.reservations.ActiveByObject(ctx, id)
	if err != nil {
		return domain.Object{}, err
	}
	for _, reservation := range actives {
		if reservation.TakesAt(now) {
			return domain.Object{}, fmt.Errorf("%w: confirmed reservation is active %s", domain.ErrConflict, reservationWindow(reservation))
		}
	}
	for _, reservation := range actives {
		if err := uc.cancelReservation(ctx, reservation, now); err != nil {
			return domain.Object{}, err
		}
	}
	reservationID, err := domain.NewID()
	if err != nil {
		return domain.Object{}, fmt.Errorf("generate reservation id: %w", err)
	}
	reservation := domain.Reservation{
		ID:        reservationID,
		ObjectID:  id,
		From:      from,
		Until:     until,
		Status:    domain.ReservationPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := uc.reservations.Create(ctx, reservation); err != nil {
		return domain.Object{}, err
	}
	if err := recordActivity(ctx, uc.activities, domain.ActivityReservationMade, id, reservationID, "reserved "+reservationWindow(reservation), now); err != nil {
		return domain.Object{}, err
	}
	uc.attachReservation(ctx, &object, now)
	return object, nil
}

func (uc *Objects) cancelReservation(ctx context.Context, reservation domain.Reservation, now time.Time) error {
	if reservation.Status == domain.ReservationCancelled || reservation.Status == domain.ReservationLapsed {
		return nil
	}
	updated := reservation
	updated.Status = domain.ReservationCancelled
	updated.UpdatedAt = now
	if err := uc.reservations.Update(ctx, updated); err != nil {
		return err
	}
	return recordActivity(ctx, uc.activities, domain.ActivityReservationCancelled, reservation.ObjectID, reservation.ID, "cancelled "+reservationWindow(reservation), now)
}

func lapseStaleReservation(ctx context.Context, repo ReservationRepository, acts ActivityRepository, reservation domain.Reservation, now time.Time) domain.Reservation {
	if reservation.IsActiveAt(now) {
		return reservation
	}
	updated := reservation
	updated.Status = domain.ReservationLapsed
	updated.UpdatedAt = now
	if err := repo.Update(ctx, updated); err != nil {
		return reservation
	}
	_ = recordActivity(ctx, acts, domain.ActivityReservationLapsed, reservation.ObjectID, reservation.ID, "lapsed "+reservationWindow(reservation), now)
	return updated
}

func recordActivity(ctx context.Context, repo ActivityRepository, activityType domain.ActivityType, objectID, reservationID, detail string, at time.Time) error {
	if repo == nil {
		return nil
	}
	id, err := domain.NewID()
	if err != nil {
		return fmt.Errorf("generate activity id: %w", err)
	}
	return repo.Create(ctx, domain.Activity{
		ID:            id,
		Type:          activityType,
		ObjectID:      objectID,
		ReservationID: reservationID,
		Detail:        detail,
		CreatedAt:     at,
	})
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return "(open ended)"
	}
	return value.Format(time.RFC3339)
}

func reservationWindow(reservation domain.Reservation) string {
	return reservation.From.Format(time.RFC3339) + " → " + formatOptionalTime(reservation.Until)
}

func (uc *Objects) Delete(ctx context.Context, id string) error {
	object, err := uc.objects.ByID(ctx, id)
	if err != nil {
		return err
	}
	// The object_images rows vanish with the object (FK cascade); the use
	// case owns only the best-effort file cleanup.
	if err := uc.objects.Delete(ctx, id); err != nil {
		return err
	}
	uc.deletePhoto(ctx, object.ImagePath)
	for _, image := range object.Images {
		if image.FileName != object.ImagePath {
			uc.deletePhoto(ctx, image.FileName)
		}
	}
	return nil
}

func (uc *Objects) List(ctx context.Context, input ListObjectsInput) ([]domain.Object, int, error) {
	if input.Limit == 0 {
		input.Limit = defaultObjectLimit
	}
	if input.Limit < 0 || input.Limit > maxObjectLimit {
		return nil, 0, fmt.Errorf("%w: limit must be between 1 and %d", domain.ErrInvalid, maxObjectLimit)
	}
	if input.Offset < 0 {
		return nil, 0, fmt.Errorf("%w: offset must not be negative", domain.ErrInvalid)
	}
	objects, total, err := uc.objects.List(ctx, ObjectFilter{
		Query:    strings.TrimSpace(input.Query),
		Category: strings.TrimSpace(input.Category),
		Tag:      strings.ToLower(strings.TrimSpace(input.Tag)),
		Limit:    input.Limit,
		Offset:   input.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	now := time.Now().UTC()
	for i := range objects {
		normalizeBusy(&objects[i], now)
		uc.attachReservation(ctx, &objects[i], now)
	}
	return objects, total, nil
}

// attachReservation loads the object's earliest live reservation, lapses
// stale pending records, and best-effort persists the transition. A failed
// lookup simply leaves Reservation nil.
func (uc *Objects) attachReservation(ctx context.Context, object *domain.Object, now time.Time) {
	actives, err := uc.reservations.ActiveByObject(ctx, object.ID)
	if err != nil {
		return
	}
	var next *domain.Reservation
	for _, reservation := range actives {
		updated := lapseStaleReservation(ctx, uc.reservations, uc.activities, reservation, now)
		if updated.Status == domain.ReservationLapsed {
			continue
		}
		if next == nil || updated.From.Before(next.From) {
			item := updated
			next = &item
		}
	}
	object.Reservation = next
	if next != nil && next.TakesAt(now) {
		object.Busy = true
		if next.Until != nil && (object.BusyUntil == nil || next.Until.After(*object.BusyUntil)) {
			object.BusyUntil = next.Until
		}
	}
}

func (uc *Objects) category(ctx context.Context, id string) (*domain.Category, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	category, err := uc.categories.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &category, nil
}

func (uc *Objects) savePhoto(ctx context.Context, upload *PhotoUpload) (string, error) {
	if upload == nil {
		return "", nil
	}
	return uc.photos.Save(ctx, *upload)
}

func (uc *Objects) deletePhoto(ctx context.Context, path string) {
	if path == "" {
		return
	}
	_ = uc.photos.Delete(ctx, path)
}

// imageListsEqual compares image lists by id and file name.
func imageListsEqual(a, b []domain.ObjectImage) bool {
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

// removedImageFiles lists the files referenced by the old cover and image
// list that the new cover and image list no longer reference, deduplicated in
// old-first order. Callers run it only after every repository write has
// succeeded, so a file still referenced by the surviving state is never
// deleted and a failed write never orphans a saved file.
func removedImageFiles(oldImagePath string, oldImages []domain.ObjectImage, newPath string, newImages []domain.ObjectImage) []string {
	surviving := make(map[string]struct{}, len(newImages)+1)
	if newPath != "" {
		surviving[newPath] = struct{}{}
	}
	for _, image := range newImages {
		surviving[image.FileName] = struct{}{}
	}
	removed := make([]string, 0, len(oldImages)+1)
	seen := make(map[string]struct{}, len(oldImages)+1)
	claim := func(fileName string) {
		if fileName == "" {
			return
		}
		if _, ok := surviving[fileName]; ok {
			return
		}
		if _, duplicate := seen[fileName]; duplicate {
			return
		}
		seen[fileName] = struct{}{}
		removed = append(removed, fileName)
	}
	claim(oldImagePath)
	for _, image := range oldImages {
		claim(image.FileName)
	}
	return removed
}

func normalizeBusy(object *domain.Object, now time.Time) {
	if object.Busy && !object.IsBusyAt(now) {
		object.Busy = false
		object.BusyUntil = nil
	}
}
