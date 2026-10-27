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
	if newImagePath != "" {
		object.ImagePath = newImagePath
	} else if input.RemovePhoto {
		object.ImagePath = ""
	}
	object.UpdatedAt = time.Now().UTC()
	if err := uc.objects.Update(ctx, object); err != nil {
		uc.deletePhoto(ctx, newImagePath)
		return domain.Object{}, err
	}
	if oldImagePath != object.ImagePath {
		uc.deletePhoto(ctx, oldImagePath)
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
	if err := uc.objects.Delete(ctx, id); err != nil {
		return err
	}
	uc.deletePhoto(ctx, object.ImagePath)
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

func normalizeBusy(object *domain.Object, now time.Time) {
	if object.Busy && !object.IsBusyAt(now) {
		object.Busy = false
		object.BusyUntil = nil
	}
}
