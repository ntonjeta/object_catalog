package usecase

import (
	"context"
	"time"

	"biblio-oggetto/internal/domain"
)

type UserRepository interface {
	Create(ctx context.Context, user domain.User) error
	ByEmail(ctx context.Context, email string) (domain.User, error)
	ByID(ctx context.Context, id string) (domain.User, error)
	UpdatePassword(ctx context.Context, id, passwordHash string) error
	Delete(ctx context.Context, id string) error
}

type SessionRepository interface {
	Create(ctx context.Context, tokenHash, userID string, expiresAt time.Time) error
	ByTokenHash(ctx context.Context, tokenHash string) (domain.Session, error)
	Delete(ctx context.Context, tokenHash string) error
	DeleteByUser(ctx context.Context, userID string) error
	DeleteExpired(ctx context.Context, now time.Time) error
}

type CategoryRepository interface {
	Create(ctx context.Context, category domain.Category) error
	List(ctx context.Context) ([]domain.Category, error)
	ByID(ctx context.Context, id string) (domain.Category, error)
	Update(ctx context.Context, category domain.Category) error
	Delete(ctx context.Context, id string) error
}

type ObjectFilter struct {
	Query    string
	Category string
	Tag      string
	Limit    int
	Offset   int
}

type ObjectRepository interface {
	Create(ctx context.Context, object domain.Object) error
	Update(ctx context.Context, object domain.Object) error
	Delete(ctx context.Context, id string) error
	ByID(ctx context.Context, id string) (domain.Object, error)
	List(ctx context.Context, filter ObjectFilter) ([]domain.Object, int, error)
}
