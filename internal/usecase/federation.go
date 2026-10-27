package usecase

import (
	"context"
	"encoding/json"
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
	inboxBatchSize     = 10
)

// RetryDelay returns the exponential backoff after the given attempt count
// (30 s base, doubling, capped at 30 minutes).
func RetryDelay(attempts int) time.Duration {
	const (
		base = 30 * time.Second
		cap  = 30 * time.Minute
	)
	delay := base
	for i := 1; i < attempts; i++ {
		delay *= 2
		if delay >= cap {
			return cap
		}
	}
	if delay > cap {
		return cap
	}
	return delay
}

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
	inbox     InboxEventRepository
	identity  *Identity
}

func NewFederation(outbox OutboxRepository, followers FollowerRepository, actors RemoteActorRepository, inbox InboxEventRepository, identity *Identity) *Federation {
	return &Federation{outbox: outbox, followers: followers, actors: actors, inbox: inbox, identity: identity}
}

// EnqueueActivity stores a signature-verified incoming activity for
// asynchronous processing. Duplicate activity ids are reported with
// domain.ErrConflict so callers can answer 202 without reprocessing.
func (f *Federation) EnqueueActivity(ctx context.Context, sender string, payload []byte) error {
	event, err := decodeInboxEvent(sender, payload)
	if err != nil {
		return err
	}
	event.ReceivedAt = time.Now().UTC()
	if err := f.inbox.Add(ctx, event); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return fmt.Errorf("%w: duplicate activity %q", domain.ErrConflict, event.ID)
		}
		return err
	}
	return nil
}

// ProcessPendingInbox applies queued inbox activities. Unknown types and
// activities not targeting the instance actor are dropped after being
// marked processed.
func (f *Federation) ProcessPendingInbox(ctx context.Context, now time.Time) error {
	events, err := f.inbox.Pending(ctx, now.UTC(), inboxBatchSize)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}
	actor, err := f.identity.Actor(ctx)
	if err != nil {
		return err
	}
	for _, event := range events {
		err := error(nil)
		switch {
		case event.Type == "Follow" && event.ObjectID == actor.ID:
			err = f.Follow(ctx, FollowRequest{ActivityID: event.ID, ActorID: event.Actor, ObjectID: event.ObjectID})
		case event.Type == "Undo" && event.ObjectID == actor.ID:
			err = f.Unfollow(ctx, FollowRequest{ActivityID: event.ID, ActorID: event.Actor, ObjectID: event.ObjectID})
		}
		if err != nil {
			attempts := event.Attempts + 1
			if markErr := f.inbox.MarkAttempt(ctx, event.ID, attempts, now.Add(RetryDelay(attempts))); markErr != nil {
				return markErr
			}
			continue
		}
		if err := f.inbox.MarkProcessed(ctx, event.ID, now.UTC()); err != nil {
			return err
		}
	}
	return nil
}

func decodeInboxEvent(sender string, payload []byte) (domain.InboxEvent, error) {
	var activity struct {
		ID     string          `json:"id"`
		Type   string          `json:"type"`
		Actor  string          `json:"actor"`
		Object json.RawMessage `json:"object"`
	}
	if err := json.Unmarshal(payload, &activity); err != nil {
		return domain.InboxEvent{}, fmt.Errorf("%w: body must be a valid JSON activity", domain.ErrInvalid)
	}
	if err := validateRemoteURL("activity id", activity.ID); err != nil {
		return domain.InboxEvent{}, err
	}
	if strings.TrimSpace(activity.Type) == "" {
		return domain.InboxEvent{}, fmt.Errorf("%w: activity type is required", domain.ErrInvalid)
	}
	return domain.InboxEvent{
		ID:       activity.ID,
		Sender:   sender,
		Type:     activity.Type,
		Actor:    activity.Actor,
		ObjectID: activityObjectID(activity.Object),
		Payload:  string(payload),
	}, nil
}

func activityObjectID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString
	}
	var asObject struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &asObject); err == nil {
		return asObject.ID
	}
	return ""
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
