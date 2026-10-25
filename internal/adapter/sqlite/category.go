package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"biblio-oggetto/internal/domain"
)

type CategoryRepo struct {
	db *sql.DB
}

func NewCategoryRepo(db *sql.DB) *CategoryRepo {
	return &CategoryRepo{db: db}
}

func (r *CategoryRepo) Create(ctx context.Context, category domain.Category) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO categories (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		category.ID, category.Name, formatTime(category.CreatedAt), formatTime(category.UpdatedAt),
	)
	return mapError(err)
}

func (r *CategoryRepo) List(ctx context.Context) ([]domain.Category, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, created_at, updated_at FROM categories ORDER BY name COLLATE NOCASE, id`,
	)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()

	categories := make([]domain.Category, 0)
	for rows.Next() {
		category, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		categories = append(categories, category)
	}
	if err := rows.Err(); err != nil {
		return nil, mapError(err)
	}
	return categories, nil
}

func (r *CategoryRepo) ByID(ctx context.Context, id string) (domain.Category, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, created_at, updated_at FROM categories WHERE id = ?`, id,
	)
	return scanCategory(row)
}

func (r *CategoryRepo) Update(ctx context.Context, category domain.Category) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE categories SET name = ?, updated_at = ? WHERE id = ?`,
		category.Name, formatTime(category.UpdatedAt), category.ID,
	)
	if err != nil {
		return mapError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check category update: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *CategoryRepo) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM categories WHERE id = ?`, id)
	if err != nil {
		return mapError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check category deletion: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanCategory(row scanner) (domain.Category, error) {
	var (
		category  domain.Category
		createdAt string
		updatedAt string
	)
	if err := row.Scan(&category.ID, &category.Name, &createdAt, &updatedAt); err != nil {
		return domain.Category{}, mapError(err)
	}
	var err error
	category.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.Category{}, err
	}
	category.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.Category{}, err
	}
	return category, nil
}
