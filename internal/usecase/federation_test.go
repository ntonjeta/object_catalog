package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"biblio-oggetto/internal/domain"
)

func newTestFederation() (*Federation, *fakeOutbox, *fakeFollowers, *fakeRemoteActors, *fakeInboxEvents, *fakePeers, *fakeRemoteObjects) {
	outbox := newFakeOutbox()
	followers := newFakeFollowers()
	actors := newFakeRemoteActors()
	inbox := newFakeInboxEvents()
	peers := newFakePeers()
	remoteObjects := newFakeRemoteObjects()
	callLog := &fakeCallLog{}
	peers.log = callLog
	remoteObjects.log = callLog
	identity := NewIdentity(&fakeInstanceKeys{}, "https://catalog.example.com")
	return NewFederation(outbox, followers, actors, inbox, identity, peers, remoteObjects), outbox, followers, actors, inbox, peers, remoteObjects
}

func TestFederationFollowAddsFollowerAndQueuesAccept(t *testing.T) {
	ctx := context.Background()
	federation, outbox, followers, actors, _, _, _ := newTestFederation()
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
	federation, outbox, followers, _, _, _, _ := newTestFederation()

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
	federation, _, _, _, _, _, _ := newTestFederation()
	if err := federation.Unfollow(context.Background(), FollowRequest{ActorID: "https://remote.example/ghost"}); err != nil {
		t.Fatalf("unfollow missing: %v", err)
	}
}

func TestFederationOutboxLimitValidation(t *testing.T) {
	federation, _, _, _, _, _, _ := newTestFederation()
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
	federation, outbox, _, _, _, _, _ := newTestFederation()
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
	federation, _, _, _, inbox, _, _ := newTestFederation()

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
	federation, outbox, followers, actors, inbox, _, _ := newTestFederation()
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

func TestAddPeerStoresPendingAndQueuesFollow(t *testing.T) {
	ctx := context.Background()
	federation, outbox, _, actors, _, peers, _ := newTestFederation()
	actors.actors["https://peer/ap/actors/catalog"] = domain.RemoteActor{
		ID:       "https://peer/ap/actors/catalog",
		InboxURL: "https://peer/ap/inbox",
	}

	peer, err := federation.AddPeer(ctx, "  https://peer/ap/actors/catalog  ")
	if err != nil {
		t.Fatalf("add peer: %v", err)
	}
	if len(peers.peers) != 1 {
		t.Fatalf("peers = %+v, want one stored peer", peers.peers)
	}
	if peers.peers[0] != peer {
		t.Fatalf("stored peer = %+v, want %+v", peers.peers[0], peer)
	}
	if peer.ActorID != "https://peer/ap/actors/catalog" || peer.InboxURL != "https://peer/ap/inbox" {
		t.Fatalf("peer = %+v", peer)
	}
	if peer.State != domain.PeerPending {
		t.Fatalf("state = %q, want pending", peer.State)
	}
	wantFollowID := "https://catalog.example.com/ap/outbox/" + peer.ID
	if peer.FollowActivityID != wantFollowID {
		t.Fatalf("follow activity id = %q, want %q", peer.FollowActivityID, wantFollowID)
	}
	if len(outbox.appended) != 1 {
		t.Fatalf("appended = %+v, want one follow", outbox.appended)
	}
	entry := outbox.appended[0]
	if entry.Kind != domain.OutboxFollow || entry.Target != peer.ActorID {
		t.Fatalf("entry = %+v, want follow targeting the peer actor", entry)
	}
	if entry.ID != peer.ID || entry.FollowID != wantFollowID {
		t.Fatalf("entry = %+v, want id %q and follow id %q", entry, peer.ID, wantFollowID)
	}
}

func TestAddPeerValidatesTarget(t *testing.T) {
	ctx := context.Background()
	federation, outbox, _, actors, _, peers, _ := newTestFederation()
	ownID, err := federation.ActorID(ctx)
	if err != nil {
		t.Fatalf("actor id: %v", err)
	}

	if _, err := federation.AddPeer(ctx, "   "); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("empty target error = %v, want ErrInvalid", err)
	}
	actors.err = fmt.Errorf("%w: target must be a handle or absolute URL", domain.ErrInvalid)
	if _, err := federation.AddPeer(ctx, "not a target"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("resolver ErrInvalid error = %v, want ErrInvalid", err)
	}
	actors.err = nil
	actors.resolved = domain.RemoteActor{ID: ownID, InboxURL: "https://catalog.example.com/ap/inbox"}
	if _, err := federation.AddPeer(ctx, "catalog@catalog.example.com"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("self target error = %v, want ErrInvalid", err)
	}
	if len(peers.peers) != 0 || len(outbox.appended) != 0 {
		t.Fatalf("invalid AddPeer wrote peers %+v / outbox %+v", peers.peers, outbox.appended)
	}
}

func TestAddPeerUnresolvableTarget(t *testing.T) {
	ctx := context.Background()
	federation, outbox, _, actors, _, peers, _ := newTestFederation()
	actors.err = errors.New("dial tcp: lookup peer.example: no such host")

	_, err := federation.AddPeer(ctx, "handle@peer.example")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "handle@peer.example") {
		t.Fatalf("error = %q, want it to name the target", err)
	}
	if len(peers.peers) != 0 || len(outbox.appended) != 0 {
		t.Fatalf("failed join wrote peers %+v / outbox %+v", peers.peers, outbox.appended)
	}
}

func TestAddPeerRejoinResetsPending(t *testing.T) {
	ctx := context.Background()
	federation, outbox, _, actors, _, peers, _ := newTestFederation()
	actors.actors["https://peer/ap/actors/catalog"] = domain.RemoteActor{
		ID:       "https://peer/ap/actors/catalog",
		InboxURL: "https://peer/ap/inbox",
	}
	first, err := federation.AddPeer(ctx, "https://peer/ap/actors/catalog")
	if err != nil {
		t.Fatalf("first add: %v", err)
	}
	if err := peers.MarkAccepted(ctx, first.ActorID); err != nil {
		t.Fatalf("mark accepted: %v", err)
	}
	actors.actors["https://peer/ap/actors/catalog"] = domain.RemoteActor{
		ID:       "https://peer/ap/actors/catalog",
		InboxURL: "https://peer/ap/inbox/v2",
	}

	second, err := federation.AddPeer(ctx, "https://peer/ap/actors/catalog")
	if err != nil {
		t.Fatalf("rejoin: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("rejoin returned id %q, want the stored id %q from the first join", second.ID, first.ID)
	}
	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("rejoin CreatedAt = %v, want the first join's %v", second.CreatedAt, first.CreatedAt)
	}
	if len(peers.peers) != 1 {
		t.Fatalf("peers = %+v, want still one peer", peers.peers)
	}
	stored := peers.peers[0]
	if stored != second {
		t.Fatalf("returned peer %+v is not the stored row %+v", second, stored)
	}
	if stored.State != domain.PeerPending {
		t.Fatalf("stored state = %q, want pending", stored.State)
	}
	if stored.InboxURL != "https://peer/ap/inbox/v2" || stored.FollowActivityID != second.FollowActivityID {
		t.Fatalf("stored peer = %+v, want refreshed from %+v", stored, second)
	}
	if len(outbox.appended) != 2 {
		t.Fatalf("appended = %+v, want two follow entries", outbox.appended)
	}
	for _, entry := range outbox.appended {
		if entry.Kind != domain.OutboxFollow || entry.Target != first.ActorID {
			t.Fatalf("entry = %+v, want follow targeting the peer actor", entry)
		}
	}
	secondEntry := outbox.appended[1]
	if secondEntry.FollowID != second.FollowActivityID {
		t.Fatalf("second entry follow id = %q, want %q", secondEntry.FollowID, second.FollowActivityID)
	}
	if want := "https://catalog.example.com/ap/outbox/" + secondEntry.ID; secondEntry.FollowID != want {
		t.Fatalf("second entry follow id = %q, want it derived from its own entry id %q", secondEntry.FollowID, want)
	}
}

func TestRemovePeerQueuesUndoAndCascades(t *testing.T) {
	ctx := context.Background()
	federation, outbox, _, actors, _, peers, remoteObjects := newTestFederation()
	actors.actors["https://peer/ap/actors/catalog"] = domain.RemoteActor{
		ID:       "https://peer/ap/actors/catalog",
		InboxURL: "https://peer/ap/inbox",
	}
	peer, err := federation.AddPeer(ctx, "https://peer/ap/actors/catalog")
	if err != nil {
		t.Fatalf("add peer: %v", err)
	}
	remoteObjects.objects = append(remoteObjects.objects, domain.RemoteObject{
		ID: "https://peer/ap/objects/1", PeerActorID: peer.ActorID,
	})

	if err := federation.RemovePeer(ctx, peer.ID); err != nil {
		t.Fatalf("remove peer: %v", err)
	}
	if len(peers.peers) != 0 {
		t.Fatalf("peers = %+v, want empty", peers.peers)
	}
	if len(outbox.appended) != 2 {
		t.Fatalf("appended = %+v, want follow + undo", outbox.appended)
	}
	entry := outbox.appended[1]
	if entry.Kind != domain.OutboxUndo || entry.Target != peer.ActorID || entry.FollowID != peer.FollowActivityID {
		t.Fatalf("undo entry = %+v", entry)
	}
	if entry.ID == peer.ID {
		t.Fatalf("undo entry reuses peer id %q", entry.ID)
	}
	if len(remoteObjects.deletedPeers) != 1 || remoteObjects.deletedPeers[0] != peer.ActorID {
		t.Fatalf("delete-by-peer calls = %v, want [%s]", remoteObjects.deletedPeers, peer.ActorID)
	}
	if len(remoteObjects.objects) != 0 {
		t.Fatalf("remote objects = %+v, want cascaded away", remoteObjects.objects)
	}
	deleteIdx, removeIdx := -1, -1
	for i, op := range peers.log.ops {
		switch op {
		case "remoteObjects.DeleteByPeer":
			deleteIdx = i
		case "peers.Remove":
			removeIdx = i
		}
	}
	if deleteIdx < 0 || removeIdx < 0 || deleteIdx > removeIdx {
		t.Fatalf("ops = %v, want remoteObjects.DeleteByPeer before peers.Remove", peers.log.ops)
	}

	if err := federation.RemovePeer(ctx, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown id error = %v, want ErrNotFound", err)
	}
	if len(outbox.appended) != 2 {
		t.Fatalf("failed removal appended entries: %+v", outbox.appended)
	}
	if len(peers.log.ops) != 2 {
		t.Fatalf("failed removal ran repo writes: %v", peers.log.ops)
	}
}

func TestPeersLists(t *testing.T) {
	ctx := context.Background()
	federation, _, _, _, _, peers, _ := newTestFederation()
	peers.peers = []domain.Peer{
		{ID: "p1", ActorID: "https://a.example/ap/actors/catalog", State: domain.PeerAccepted},
		{ID: "p2", ActorID: "https://b.example/ap/actors/catalog", State: domain.PeerPending},
	}

	listed, err := federation.Peers(ctx)
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(listed) != 2 || listed[0].ID != "p1" || listed[1].ID != "p2" {
		t.Fatalf("listed = %+v, want the two seeded peers in order", listed)
	}
}
