package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"biblio-oggetto/internal/domain"
)

func newTestFederation() (*Federation, *fakeOutbox, *fakeFollowers, *fakeRemoteActors) {
	outbox := newFakeOutbox()
	followers := newFakeFollowers()
	actors := newFakeRemoteActors()
	identity := NewIdentity(&fakeInstanceKeys{}, "https://catalog.example.com")
	return NewFederation(outbox, followers, actors, identity), outbox, followers, actors
}

func TestFederationFollowAddsFollowerAndQueuesAccept(t *testing.T) {
	ctx := context.Background()
	federation, outbox, followers, actors := newTestFederation()
	actors.actors["https://remote.example/alice"] = domain.RemoteActor{
		ID:       "https://remote.example/alice",
		InboxURL: "https://remote.example/inbox/alice",
	}
	actorID, err := federation.ActorID(ctx)
	if err != nil {
		t.Fatalf("actor id: %v", err)
	}

	err = federation.Follow(ctx, FollowRequest{
		ActivityID: "https://remote.example/follows/1",
		ActorID:    "https://remote.example/alice",
		ObjectID:   actorID,
	})
	if err != nil {
		t.Fatalf("follow: %v", err)
	}
	stored, ok := followers.followers["https://remote.example/alice"]
	if !ok || stored.InboxURL != "https://remote.example/inbox/alice" {
		t.Fatalf("followers = %+v", followers.followers)
	}
	if len(outbox.appended) != 1 {
		t.Fatalf("appended = %+v, want one accept", outbox.appended)
	}
	entry := outbox.appended[0]
	if entry.Kind != domain.OutboxAccept || entry.Target != "https://remote.example/alice" {
		t.Fatalf("entry = %+v", entry)
	}
	if entry.FollowID != "https://remote.example/follows/1" {
		t.Fatalf("entry follow id = %q", entry.FollowID)
	}
}

func TestFederationFollowValidatesInput(t *testing.T) {
	ctx := context.Background()
	federation, outbox, followers, _ := newTestFederation()

	if err := federation.Follow(ctx, FollowRequest{ActorID: ""}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("empty actor error = %v, want ErrInvalid", err)
	}
	if err := federation.Follow(ctx, FollowRequest{ActorID: "not-a-url"}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("bad actor error = %v, want ErrInvalid", err)
	}
	if len(outbox.appended) != 0 || len(followers.followers) != 0 {
		t.Fatal("invalid follow produced side effects")
	}
}

func TestFederationUnfollowMissingIsIgnored(t *testing.T) {
	federation, _, _, _ := newTestFederation()
	if err := federation.Unfollow(context.Background(), FollowRequest{ActorID: "https://remote.example/ghost"}); err != nil {
		t.Fatalf("unfollow missing: %v", err)
	}
}

func TestFederationOutboxLimitValidation(t *testing.T) {
	federation, _, _, _ := newTestFederation()
	ctx := context.Background()

	if _, _, err := federation.Outbox(ctx, 101, 0); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("limit 101 error = %v, want ErrInvalid", err)
	}
	if _, _, err := federation.Outbox(ctx, 0, -1); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("offset -1 error = %v, want ErrInvalid", err)
	}
	if _, _, err := federation.Outbox(ctx, 0, 0); err != nil {
		t.Fatalf("default list: %v", err)
	}
}

func TestFederationDeliveryMarkers(t *testing.T) {
	ctx := context.Background()
	federation, outbox, _, _ := newTestFederation()
	outbox.entries = append(outbox.entries, domain.OutboxEntry{
		ID: "e1", Kind: domain.OutboxCreate, Published: time.Now().UTC(),
	})

	pending, err := federation.PendingDeliveries(ctx, time.Now().UTC())
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending = (%v, %v), want one entry", pending, err)
	}
	next := time.Now().UTC().Add(time.Minute)
	if err := federation.MarkAttempt(ctx, "e1", 1, next); err != nil {
		t.Fatalf("mark attempt: %v", err)
	}
	pending, err = federation.PendingDeliveries(ctx, time.Now().UTC())
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending after backoff = (%v, %v), want none", pending, err)
	}
	if err := federation.MarkDelivered(ctx, "e1", time.Now().UTC()); err != nil {
		t.Fatalf("mark delivered: %v", err)
	}
	if err := federation.MarkDelivered(ctx, "missing", time.Now().UTC()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing delivered error = %v, want ErrNotFound", err)
	}
}
