package usecase

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"biblio-oggetto/internal/domain"
)

func newTestFederation() (*Federation, *fakeOutbox, *fakeFollowers, *fakeRemoteActors, *fakeInboxEvents) {
	outbox := newFakeOutbox()
	followers := newFakeFollowers()
	actors := newFakeRemoteActors()
	inbox := newFakeInboxEvents()
	identity := NewIdentity(&fakeInstanceKeys{}, "https://catalog.example.com")
	return NewFederation(outbox, followers, actors, inbox, identity), outbox, followers, actors, inbox
}

func TestFederationFollowAddsFollowerAndQueuesAccept(t *testing.T) {
	ctx := context.Background()
	federation, outbox, followers, actors, _ := newTestFederation()
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
	federation, outbox, followers, _, _ := newTestFederation()

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
	federation, _, _, _, _ := newTestFederation()
	if err := federation.Unfollow(context.Background(), FollowRequest{ActorID: "https://remote.example/ghost"}); err != nil {
		t.Fatalf("unfollow missing: %v", err)
	}
}

func TestFederationOutboxLimitValidation(t *testing.T) {
	federation, _, _, _, _ := newTestFederation()
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
	federation, outbox, _, _, _ := newTestFederation()
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

func TestFederationEnqueueActivityValidation(t *testing.T) {
	ctx := context.Background()
	federation, _, _, _, inbox := newTestFederation()

	payload := []byte(`{"id":"https://remote.example/f/1","type":"Follow","actor":"https://remote.example/alice","object":"https://catalog.example.com/ap/actors/catalog"}`)
	if err := federation.EnqueueActivity(ctx, "https://remote.example/alice", payload); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if len(inbox.events) != 1 {
		t.Fatalf("events = %d, want 1", len(inbox.events))
	}
	event := inbox.events[0]
	if event.Type != "Follow" || event.Actor != "https://remote.example/alice" ||
		event.ObjectID != "https://catalog.example.com/ap/actors/catalog" || event.Sender != "https://remote.example/alice" {
		t.Fatalf("event = %+v", event)
	}
	if err := federation.EnqueueActivity(ctx, "https://remote.example/alice", payload); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate enqueue error = %v, want ErrConflict", err)
	}

	cases := [][]byte{
		[]byte(`{`),
		[]byte(`{"type":"Follow"}`),
		[]byte(`{"id":"not-a-url","type":"Follow"}`),
		[]byte(`{"id":"https://remote.example/f/2","type":" "}`),
	}
	for i, payload := range cases {
		if err := federation.EnqueueActivity(ctx, "https://remote.example/alice", payload); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("case %d error = %v, want ErrInvalid", i, err)
		}
	}
}

func TestFederationProcessPendingInbox(t *testing.T) {
	ctx := context.Background()
	federation, outbox, followers, actors, inbox := newTestFederation()
	actorID, err := federation.ActorID(ctx)
	if err != nil {
		t.Fatalf("actor id: %v", err)
	}
	actors.actors["https://remote.example/alice"] = domain.RemoteActor{
		ID:       "https://remote.example/alice",
		InboxURL: "https://remote.example/inbox/alice",
	}

	follow := fmt.Sprintf(`{"id":"https://remote.example/f/1","type":"Follow","actor":"https://remote.example/alice","object":"%s"}`, actorID)
	like := `{"id":"https://remote.example/l/1","type":"Like","actor":"https://remote.example/alice","object":"https://catalog.example.com/ap/objects/x"}`
	foreign := `{"id":"https://remote.example/f/2","type":"Follow","actor":"https://remote.example/alice","object":"https://other.example/actors/bob"}`
	unresolvable := `{"id":"https://remote.example/f/3","type":"Follow","actor":"https://remote.example/ghost","object":"` + actorID + `"}`

	for _, payload := range []string{follow, like, foreign, unresolvable} {
		if err := federation.EnqueueActivity(ctx, "https://remote.example/alice", []byte(payload)); err != nil {
			t.Fatalf("enqueue %q: %v", payload, err)
		}
	}

	now := time.Now().UTC()
	if err := federation.ProcessPendingInbox(ctx, now); err != nil {
		t.Fatalf("process: %v", err)
	}

	if len(followers.followers) != 1 {
		t.Fatalf("followers = %v, want alice", followers.followers)
	}
	if len(outbox.appended) != 1 || outbox.appended[0].Kind != domain.OutboxAccept {
		t.Fatalf("accept entries = %+v", outbox.appended)
	}
	for _, event := range inbox.events {
		switch event.ID {
		case "https://remote.example/f/3":
			if event.ProcessedAt != nil {
				t.Fatalf("unresolvable follow marked processed, want retry scheduled (attempts %d)", event.Attempts)
			}
			if event.Attempts != 1 || !event.NextAttemptAt.After(now) {
				t.Fatalf("unresolvable event = %+v, want attempt 1 with future window", event)
			}
		case "https://remote.example/f/1", "https://remote.example/l/1", "https://remote.example/f/2":
			if event.ProcessedAt == nil {
				t.Fatalf("event %s not processed", event.ID)
			}
		}
	}

	// The retry window defers the failing event; second pass does not re-run it.
	if err := federation.ProcessPendingInbox(ctx, now); err != nil {
		t.Fatalf("second process: %v", err)
	}
	for _, event := range inbox.events {
		if event.ID == "https://remote.example/f/3" && event.Attempts != 1 {
			t.Fatalf("failing event retried inside backoff window: attempts %d", event.Attempts)
		}
	}
}
