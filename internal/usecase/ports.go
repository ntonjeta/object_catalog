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

type ReservationRepository interface {
	Create(ctx context.Context, reservation domain.Reservation) error
	Update(ctx context.Context, reservation domain.Reservation) error
	ByID(ctx context.Context, id string) (domain.Reservation, error)
	ActiveByObject(ctx context.Context, objectID string) ([]domain.Reservation, error)
}

type ActivityFilter struct {
	ObjectID string
	Limit    int
	Offset   int
}

type ActivityRepository interface {
	Create(ctx context.Context, activity domain.Activity) error
	List(ctx context.Context, filter ActivityFilter) ([]domain.Activity, int, error)
}

type ObjectFilter struct {
	Query    string
	Category string
	Tag      string
	Limit    int
	Offset   int
}

type ObjectRepository interface {
	Create(ctx context.Context, object domain.Object, entry domain.OutboxEntry) error
	Update(ctx context.Context, object domain.Object, entry domain.OutboxEntry) error
	Delete(ctx context.Context, id string, entry domain.OutboxEntry) error
	ByID(ctx context.Context, id string) (domain.Object, error)
	ByName(ctx context.Context, name string) (domain.Object, error)
	List(ctx context.Context, filter ObjectFilter) ([]domain.Object, int, error)
	Images(ctx context.Context, objectID string) ([]domain.ObjectImage, error)
	ReplaceImages(ctx context.Context, objectID string, images []domain.ObjectImage) error
}

type InstanceKeyRepository interface {
	Create(ctx context.Context, key domain.InstanceKey) error
	Get(ctx context.Context) (domain.InstanceKey, error)
}

// OutboxRepository stores federated activities. Append is used standalone by
// the Federation use case; the object mutations embed the entry in the same
// SQLite transaction through ObjectRepository.
type OutboxRepository interface {
	Append(ctx context.Context, entry domain.OutboxEntry) error
	ByID(ctx context.Context, id string) (domain.OutboxEntry, error)
	List(ctx context.Context, limit, offset int) ([]domain.OutboxEntry, int, error)
	Pending(ctx context.Context, now time.Time, limit int) ([]domain.OutboxEntry, error)
	MarkDelivered(ctx context.Context, id string, at time.Time) error
	MarkAttempt(ctx context.Context, id string, attempts int, nextAttemptAt time.Time) error
}

type FollowerRepository interface {
	Add(ctx context.Context, follower domain.Follower) error
	Remove(ctx context.Context, actorID string) error
	List(ctx context.Context) ([]domain.Follower, error)
}

// RemoteActorRepository resolves a remote actor id to its document (inbox).
type RemoteActorRepository interface {
	FetchActor(ctx context.Context, actorID string) (domain.RemoteActor, error)
}
