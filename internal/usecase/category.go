package usecase

import (
	"context"
	"fmt"
	"time"

	"biblio-oggetto/internal/domain"
)

type Categories struct {
	categories CategoryRepository
}

func NewCategories(categories CategoryRepository) *Categories {
	return &Categories{categories: categories}
}

func (uc *Categories) Create(ctx context.Context, name string) (domain.Category, error) {
	name = domain.NormalizeCategoryName(name)
	if err := domain.ValidateCategoryName(name); err != nil {
		return domain.Category{}, err
	}
	id, err := domain.NewID()
	if err != nil {
		return domain.Category{}, fmt.Errorf("generate category id: %w", err)
	}
	now := time.Now().UTC()
	category := domain.Category{
		ID:        id,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := uc.categories.Create(ctx, category); err != nil {
		return domain.Category{}, err
	}
	return category, nil
}

func (uc *Categories) List(ctx context.Context) ([]domain.Category, error) {
	return uc.categories.List(ctx)
}

func (uc *Categories) Get(ctx context.Context, id string) (domain.Category, error) {
	return uc.categories.ByID(ctx, id)
}

func (uc *Categories) Update(ctx context.Context, id, name string) (domain.Category, error) {
	category, err := uc.categories.ByID(ctx, id)
	if err != nil {
		return domain.Category{}, err
	}
	name = domain.NormalizeCategoryName(name)
	if err := domain.ValidateCategoryName(name); err != nil {
		return domain.Category{}, err
	}
	category.Name = name
	category.UpdatedAt = time.Now().UTC()
	if err := uc.categories.Update(ctx, category); err != nil {
		return domain.Category{}, err
	}
	return category, nil
}

func (uc *Categories) Delete(ctx context.Context, id string) error {
	return uc.categories.Delete(ctx, id)
}
