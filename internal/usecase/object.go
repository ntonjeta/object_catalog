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
	objects    ObjectRepository
	categories CategoryRepository
	photos     PhotoStore
}

func NewObjects(objects ObjectRepository, categories CategoryRepository, photos PhotoStore) *Objects {
	return &Objects{objects: objects, categories: categories, photos: photos}
}

type CreateObjectInput struct {
	Name        string
	Description string
	CategoryID  string
	Tags        []string
	Photo       *PhotoUpload
}

type UpdateObjectInput struct {
	Name        *string
	Description *string
	CategoryID  *string
	Tags        *[]string
	Photo       *PhotoUpload
	RemovePhoto bool
}

type ListObjectsInput struct {
	Query    string
	Category string
	Tag      string
	Limit    int
	Offset   int
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
	category, err := uc.category(ctx, input.CategoryID)
	if err != nil {
		return domain.Object{}, err
	}
	id, err := domain.NewID()
	if err != nil {
		return domain.Object{}, fmt.Errorf("generate object id: %w", err)
	}
	photoPath, err := uc.savePhoto(ctx, input.Photo)
	if err != nil {
		return domain.Object{}, err
	}
	now := time.Now().UTC()
	object := domain.Object{
		ID:          id,
		Name:        name,
		Description: description,
		PhotoPath:   photoPath,
		Category:    category,
		Tags:        tags,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := uc.objects.Create(ctx, object); err != nil {
		uc.deletePhoto(ctx, photoPath)
		return domain.Object{}, err
	}
	return object, nil
}

func (uc *Objects) Update(ctx context.Context, id string, input UpdateObjectInput) (domain.Object, error) {
	if input.Photo != nil && input.RemovePhoto {
		return domain.Object{}, fmt.Errorf("%w: cannot replace and remove the photo at the same time", domain.ErrInvalid)
	}
	object, err := uc.objects.ByID(ctx, id)
	if err != nil {
		return domain.Object{}, err
	}
	oldPhotoPath := object.PhotoPath
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
	newPhotoPath, err := uc.savePhoto(ctx, input.Photo)
	if err != nil {
		return domain.Object{}, err
	}
	if newPhotoPath != "" {
		object.PhotoPath = newPhotoPath
	} else if input.RemovePhoto {
		object.PhotoPath = ""
	}
	object.UpdatedAt = time.Now().UTC()
	if err := uc.objects.Update(ctx, object); err != nil {
		uc.deletePhoto(ctx, newPhotoPath)
		return domain.Object{}, err
	}
	if oldPhotoPath != object.PhotoPath {
		uc.deletePhoto(ctx, oldPhotoPath)
	}
	return object, nil
}

func (uc *Objects) Get(ctx context.Context, id string) (domain.Object, error) {
	return uc.objects.ByID(ctx, id)
}

func (uc *Objects) Delete(ctx context.Context, id string) error {
	object, err := uc.objects.ByID(ctx, id)
	if err != nil {
		return err
	}
	if err := uc.objects.Delete(ctx, id); err != nil {
		return err
	}
	uc.deletePhoto(ctx, object.PhotoPath)
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
	return uc.objects.List(ctx, ObjectFilter{
		Query:    strings.TrimSpace(input.Query),
		Category: strings.TrimSpace(input.Category),
		Tag:      strings.ToLower(strings.TrimSpace(input.Tag)),
		Limit:    input.Limit,
		Offset:   input.Offset,
	})
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
