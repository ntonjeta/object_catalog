package usecase

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"biblio-oggetto/internal/domain"
)

const (
	defaultOutboxLimit = 50
	maxOutboxLimit     = 100
	deliveryBatchSize  = 10
)

// FollowRequest describes a received Follow/Undo activity targeting the
// instance actor.
type FollowRequest struct {
	ActivityID string
	ActorID    string
	ObjectID   string
}

type Federation struct {
	outbox    OutboxRepository
	followers FollowerRepository
	actors    RemoteActorRepository
	identity  *Identity
}

func NewFederation(outbox OutboxRepository, followers FollowerRepository, actors RemoteActorRepository, identity *Identity) *Federation {
	return &Federation{outbox: outbox, followers: followers, actors: actors, identity: identity}
}

// ActorID is the canonical identifier of the instance actor.
func (f *Federation) ActorID(ctx context.Context) (string, error) {
	actor, err := f.identity.Actor(ctx)
	if err != nil {
		return "", err
	}
	return actor.ID, nil
}

// Follow records a remote follower and queues the Accept activity.
func (f *Federation) Follow(ctx context.Context, req FollowRequest) error {
	actorID := strings.TrimSpace(req.ActorID)
	if err := validateRemoteURL("actor id", actorID); err != nil {
		return err
	}
	remote, err := f.actors.FetchActor(ctx, actorID)
	if err != nil {
		return fmt.Errorf("resolve follower %q: %w", actorID, err)
	}
	if err := f.followers.Add(ctx, domain.Follower{
		ActorID:   remote.ID,
		InboxURL:  remote.InboxURL,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		return err
	}
	id, err := domain.NewID()
	if err != nil {
		return fmt.Errorf("generate outbox id: %w", err)
	}
	return f.outbox.Append(ctx, domain.OutboxEntry{
		ID:        id,
		Kind:      domain.OutboxAccept,
		Target:    remote.ID,
		FollowID:  strings.TrimSpace(req.ActivityID),
		Published: time.Now().UTC(),
	})
}

// Unfollow removes a follower. Missing followers are ignored.
func (f *Federation) Unfollow(ctx context.Context, req FollowRequest) error {
	actorID := strings.TrimSpace(req.ActorID)
	if err := validateRemoteURL("actor id", actorID); err != nil {
		return err
	}
	if err := f.followers.Remove(ctx, actorID); err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	return nil
}

func (f *Federation) Followers(ctx context.Context) ([]domain.Follower, error) {
	return f.followers.List(ctx)
}

// Outbox lists entries newest first.
func (f *Federation) Outbox(ctx context.Context, limit, offset int) ([]domain.OutboxEntry, int, error) {
	if limit == 0 {
		limit = defaultOutboxLimit
	}
	if limit < 0 || limit > maxOutboxLimit {
		return nil, 0, fmt.Errorf("%w: limit must be between 1 and %d", domain.ErrInvalid, maxOutboxLimit)
	}
	if offset < 0 {
		return nil, 0, fmt.Errorf("%w: offset must not be negative", domain.ErrInvalid)
	}
	return f.outbox.List(ctx, limit, offset)
}

func (f *Federation) OutboxEntry(ctx context.Context, id string) (domain.OutboxEntry, error) {
	return f.outbox.ByID(ctx, id)
}

// PendingDeliveries returns queued entries whose retry window has elapsed.
func (f *Federation) PendingDeliveries(ctx context.Context, now time.Time) ([]domain.OutboxEntry, error) {
	return f.outbox.Pending(ctx, now.UTC(), deliveryBatchSize)
}

func (f *Federation) MarkDelivered(ctx context.Context, id string, at time.Time) error {
	return f.outbox.MarkDelivered(ctx, id, at.UTC())
}

func (f *Federation) MarkAttempt(ctx context.Context, id string, attempts int, nextAttemptAt time.Time) error {
	return f.outbox.MarkAttempt(ctx, id, attempts, nextAttemptAt.UTC())
}

func validateRemoteURL(what, raw string) error {
	if raw == "" {
		return fmt.Errorf("%w: %s is required", domain.ErrInvalid, what)
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("%w: %s must be an absolute http(s) URL", domain.ErrInvalid, what)
	}
	return nil
}
