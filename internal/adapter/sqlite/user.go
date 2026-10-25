package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"biblio-oggetto/internal/domain"
)

type UserRepo struct {
	db *sql.DB
}

func NewUserRepo(db *sql.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) Create(ctx context.Context, user domain.User) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, ?, ?)`,
		user.ID, user.Email, user.PasswordHash, formatTime(user.CreatedAt),
	)
	return mapError(err)
}

func (r *UserRepo) ByEmail(ctx context.Context, email string) (domain.User, error) {
	return r.byField(ctx, "email", email)
}

func (r *UserRepo) ByID(ctx context.Context, id string) (domain.User, error) {
	return r.byField(ctx, "id", id)
}

func (r *UserRepo) byField(ctx context.Context, field, value string) (domain.User, error) {
	query := `SELECT id, email, password_hash, created_at FROM users WHERE ` + field + ` = ?`
	var (
		user      domain.User
		createdAt string
	)
	err := r.db.QueryRowContext(ctx, query, value).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &createdAt,
	)
	if err != nil {
		return domain.User{}, mapError(err)
	}
	user.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (r *UserRepo) UpdatePassword(ctx context.Context, id, passwordHash string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, id,
	)
	if err != nil {
		return mapError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check password update: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *UserRepo) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return mapError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check user deletion: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
