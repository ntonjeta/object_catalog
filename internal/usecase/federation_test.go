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

const testPeerActor = "https://peer.example/ap/actors/catalog"

// seedTestPeer stores a peer directly in the fake, bypassing AddPeer: the
// ingest tests need peers in a given state without a resolvable actor.
func seedTestPeer(peers *fakePeers, actorID string, state domain.PeerState) domain.Peer {
	peer := domain.Peer{
		ID:               "peer-" + actorID,
		ActorID:          actorID,
		InboxURL:         actorID + "/inbox",
		FollowActivityID: "https://catalog.example.com/ap/outbox/peer-" + actorID,
		State:            state,
		CreatedAt:        time.Now().UTC(),
	}
	peers.peers = append(peers.peers, peer)
	return peer
}

func addTestInboxEvent(t *testing.T, ctx context.Context, inbox *fakeInboxEvents, event domain.InboxEvent) {
	t.Helper()
	event.ReceivedAt = time.Now().UTC()
	if err := inbox.Add(ctx, event); err != nil {
		t.Fatalf("enqueue %s: %v", event.ID, err)
	}
}

func assertAllProcessed(t *testing.T, inbox *fakeInboxEvents) {
	t.Helper()
	for _, event := range inbox.events {
		if event.ProcessedAt == nil {
			t.Fatalf("event %s not processed (attempts %d)", event.ID, event.Attempts)
		}
	}
}

func mustPeerTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time %q: %v", value, err)
	}
	return parsed.UTC()
}

func equalRemoteObjects(a, b domain.RemoteObject) bool {
	if a.ID != b.ID || a.PeerActorID != b.PeerActorID || a.Name != b.Name ||
		a.Description != b.Description || a.ImageURL != b.ImageURL ||
		a.CategoryName != b.CategoryName || a.Busy != b.Busy {
		return false
	}
	switch {
	case a.BusyUntil == nil || b.BusyUntil == nil:
		if a.BusyUntil != b.BusyUntil {
			return false
		}
	case !a.BusyUntil.Equal(*b.BusyUntil):
		return false
	}
	if len(a.Tags) != len(b.Tags) {
		return false
	}
	for i := range a.Tags {
		if a.Tags[i] != b.Tags[i] {
			return false
		}
	}
	return a.CreatedAt.Equal(b.CreatedAt) && a.UpdatedAt.Equal(b.UpdatedAt)
}

// peerActivityPayload wraps an object in an ActivityPub activity.
func peerActivityPayload(activityID, activityType, actor, object string) string {
	return `{"@context":"https://www.w3.org/ns/activitystreams","id":"` + activityID + `","type":"` +
		activityType + `","actor":"` + actor + `","object":` + object + `}`
}

func peerCreatePayload(actor, object string) string {
	return peerActivityPayload("https://peer.example/ap/outbox/a1", "Create", actor, object)
}

// peerArticleObject is a minimal origin-valid Article object, extended with
// the given raw JSON field fragments (each starting with a comma).
func peerArticleObject(fields string) string {
	return `{"id":"https://peer.example/ap/objects/obj-1","type":"Article","name":"Dune"` + fields + `}`
}

func TestInboxAcceptMarksPeerAccepted(t *testing.T) {
	ctx := context.Background()
	federation, _, _, _, inbox, peers, remoteObjects := newTestFederation()
	peers.peers = append(peers.peers, domain.Peer{
		ID:               "peer-1",
		ActorID:          testPeerActor,
		InboxURL:         "https://peer.example/ap/inbox",
		FollowActivityID: "https://us/ap/outbox/f1",
		State:            domain.PeerPending,
		CreatedAt:        time.Now().UTC(),
	})

	known := domain.InboxEvent{
		ID: "https://peer.example/ap/outbox/a1", Type: "Accept",
		Actor: testPeerActor, ObjectID: "https://us/ap/outbox/f1",
	}
	unknown := domain.InboxEvent{
		ID: "https://peer.example/ap/outbox/a2", Type: "Accept",
		Actor: testPeerActor, ObjectID: "https://us/ap/outbox/never-joined",
	}
	// Accept answering our follow but sent by an actor that is not the peer.
	spoofed := domain.InboxEvent{
		ID: "https://stranger.example/ap/outbox/a3", Type: "Accept",
		Actor: "https://stranger.example/ap/actors/catalog", ObjectID: "https://us/ap/outbox/f1",
	}
	for _, event := range []domain.InboxEvent{known, unknown, spoofed} {
		addTestInboxEvent(t, ctx, inbox, event)
	}

	if err := federation.ProcessPendingInbox(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(peers.accepted) != 1 || peers.accepted[0] != testPeerActor {
		t.Fatalf("accepted = %v, want one MarkAccepted for %s", peers.accepted, testPeerActor)
	}
	if peers.peers[0].State != domain.PeerAccepted {
		t.Fatalf("peer state = %q, want accepted", peers.peers[0].State)
	}
	if len(remoteObjects.puts) != 0 {
		t.Fatalf("puts = %+v, Accept must not ingest objects", remoteObjects.puts)
	}
	assertAllProcessed(t, inbox)
}

func TestIngestCreateStoresRemoteObject(t *testing.T) {
	ctx := context.Background()
	federation, _, _, _, inbox, peers, remoteObjects := newTestFederation()
	seedTestPeer(peers, testPeerActor, domain.PeerAccepted)

	const payload = `{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id": "https://peer.example/ap/outbox/create-1",
		"type": "Create",
		"actor": "https://peer.example/ap/actors/catalog",
		"published": "2026-10-01T10:00:00Z",
		"object": {
			"@context": "https://www.w3.org/ns/activitystreams",
			"id": "https://peer.example/ap/objects/obj-1",
			"type": "Article",
			"name": "Dune",
			"content": "A spice opera.",
			"image": "https://peer.example/static/uploads/dune.png",
			"published": "2026-10-01T10:00:00Z",
			"updated": "2026-10-05T12:30:00Z",
			"url": "https://peer.example/objects/obj-1",
			"attachment": [
				{"type": "Note", "name": "Availability", "content": "on loan until 2026-12-01T00:00:00Z"},
				{"type": "Note", "name": "Category", "content": "Books"},
				{"type": "Tag", "name": "#dune"},
				{"type": "Tag", "name": "#movie"},
				{"type": "Tag", "name": "#Dune"}
			]
		}
	}`
	addTestInboxEvent(t, ctx, inbox, domain.InboxEvent{
		ID:       "https://peer.example/ap/outbox/create-1",
		Type:     "Create",
		Actor:    testPeerActor,
		ObjectID: "https://peer.example/ap/objects/obj-1",
		Payload:  payload,
	})

	if err := federation.ProcessPendingInbox(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("process: %v", err)
	}
	assertAllProcessed(t, inbox)
	if len(remoteObjects.puts) != 1 {
		t.Fatalf("puts = %+v, want one mirrored object", remoteObjects.puts)
	}
	until := mustPeerTime(t, "2026-12-01T00:00:00Z")
	want := domain.RemoteObject{
		ID:           "https://peer.example/ap/objects/obj-1",
		PeerActorID:  testPeerActor,
		Name:         "Dune",
		Description:  "A spice opera.",
		ImageURL:     "https://peer.example/static/uploads/dune.png",
		CategoryName: "Books",
		Busy:         true,
		BusyUntil:    &until,
		Tags:         []string{"dune", "movie"},
		CreatedAt:    mustPeerTime(t, "2026-10-01T10:00:00Z"),
		UpdatedAt:    mustPeerTime(t, "2026-10-05T12:30:00Z"),
	}
	if got := remoteObjects.puts[0]; !equalRemoteObjects(got, want) {
		t.Fatalf("mirrored object =\n%+v\nwant\n%+v", got, want)
	}
}

func TestIngestUpdateAndDelete(t *testing.T) {
	ctx := context.Background()
	federation, _, _, _, inbox, peers, remoteObjects := newTestFederation()
	seedTestPeer(peers, testPeerActor, domain.PeerAccepted)

	create := peerActivityPayload("https://peer.example/ap/outbox/c1", "Create", testPeerActor, peerArticleObject(""))
	update := peerActivityPayload("https://peer.example/ap/outbox/c2", "Update", testPeerActor,
		peerArticleObject(`,"name":"Dune Messiah","content":"The spice must flow."`))
	deleteActivity := peerActivityPayload("https://peer.example/ap/outbox/d1", "Delete", testPeerActor,
		`{"id":"https://peer.example/ap/objects/obj-1","type":"Tombstone"}`)
	addTestInboxEvent(t, ctx, inbox, domain.InboxEvent{
		ID: "https://peer.example/ap/outbox/c1", Type: "Create", Actor: testPeerActor,
		ObjectID: "https://peer.example/ap/objects/obj-1", Payload: create,
	})
	addTestInboxEvent(t, ctx, inbox, domain.InboxEvent{
		ID: "https://peer.example/ap/outbox/c2", Type: "Update", Actor: testPeerActor,
		ObjectID: "https://peer.example/ap/objects/obj-1", Payload: update,
	})

	if err := federation.ProcessPendingInbox(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(remoteObjects.puts) != 2 {
		t.Fatalf("puts = %d, want Create then Update each calling Put", len(remoteObjects.puts))
	}
	if remoteObjects.puts[0].ID != "https://peer.example/ap/objects/obj-1" ||
		remoteObjects.puts[1].ID != remoteObjects.puts[0].ID {
		t.Fatalf("puts ids = %q, %q, want the second Put to reuse the first id",
			remoteObjects.puts[0].ID, remoteObjects.puts[1].ID)
	}
	if remoteObjects.puts[1].Name != "Dune Messiah" {
		t.Fatalf("second put name = %q, want the update to replace the article", remoteObjects.puts[1].Name)
	}
	if len(remoteObjects.objects) != 1 || remoteObjects.objects[0].Name != "Dune Messiah" {
		t.Fatalf("stored = %+v, want one replaced row", remoteObjects.objects)
	}

	addTestInboxEvent(t, ctx, inbox, domain.InboxEvent{
		ID: "https://peer.example/ap/outbox/d1", Type: "Delete", Actor: testPeerActor,
		ObjectID: "https://peer.example/ap/objects/obj-1", Payload: deleteActivity,
	})
	if err := federation.ProcessPendingInbox(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("process delete: %v", err)
	}
	if len(remoteObjects.deleted) != 1 || remoteObjects.deleted[0] != "https://peer.example/ap/objects/obj-1" {
		t.Fatalf("deleted = %v, want the tombstoned object id", remoteObjects.deleted)
	}
	if len(remoteObjects.objects) != 0 {
		t.Fatalf("stored = %+v, want the mirror gone", remoteObjects.objects)
	}
	if len(remoteObjects.puts) != 2 {
		t.Fatalf("puts = %d, want no new mirrors from the Delete", len(remoteObjects.puts))
	}
	assertAllProcessed(t, inbox)
}

// R6: duplicate and retried Delete deliveries are normal ActivityPub traffic,
// so a tombstone for an id that was never mirrored (or already removed) is a
// known content failure: processed, never requeued.
func TestIngestDeleteUnknownIDMarkedProcessed(t *testing.T) {
	ctx := context.Background()
	federation, _, _, _, inbox, peers, remoteObjects := newTestFederation()
	seedTestPeer(peers, testPeerActor, domain.PeerAccepted)

	const missing = "https://peer.example/ap/objects/never-mirrored"
	addTestInboxEvent(t, ctx, inbox, domain.InboxEvent{
		ID: "https://peer.example/ap/outbox/d9", Type: "Delete", Actor: testPeerActor,
		ObjectID: missing,
		Payload:  peerActivityPayload("https://peer.example/ap/outbox/d9", "Delete", testPeerActor, `{"id":"`+missing+`","type":"Tombstone"}`),
	})

	if err := federation.ProcessPendingInbox(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("process: %v", err)
	}
	assertAllProcessed(t, inbox)
	if inbox.events[0].Attempts != 0 {
		t.Fatalf("attempts = %d, want no retry scheduled for an unknown-id delete", inbox.events[0].Attempts)
	}
	if len(remoteObjects.deleted) != 0 {
		t.Fatalf("deleted = %v, want no removal recorded", remoteObjects.deleted)
	}

	// A second pass sees an empty queue: the event is done, not stuck.
	if err := federation.ProcessPendingInbox(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("second process: %v", err)
	}
	if inbox.events[0].Attempts != 0 {
		t.Fatalf("attempts after second pass = %d, want still 0", inbox.events[0].Attempts)
	}
}

// R7: ownership of a mirrored id belongs to its announcing peer. An accepted
// peer may not tombstone rows mirrored from another origin.
func TestIngestDeleteForeignOrigin(t *testing.T) {
	ctx := context.Background()
	federation, _, _, _, inbox, peers, remoteObjects := newTestFederation()
	peerA := "https://peer-a.example/ap/actors/catalog"
	peerB := "https://peer-b.example/ap/actors/catalog"
	seedTestPeer(peers, peerA, domain.PeerAccepted)
	seedTestPeer(peers, peerB, domain.PeerAccepted)
	mine := "https://peer-b.example/ap/objects/keep-me"
	remoteObjects.objects = append(remoteObjects.objects,
		domain.RemoteObject{ID: mine, PeerActorID: peerB, Name: "Dune"})

	addTestInboxEvent(t, ctx, inbox, domain.InboxEvent{
		ID: "https://peer-a.example/ap/outbox/d1", Type: "Delete", Actor: peerA,
		ObjectID: mine,
		Payload:  peerActivityPayload("https://peer-a.example/ap/outbox/d1", "Delete", peerA, `{"id":"`+mine+`","type":"Tombstone"}`),
	})

	if err := federation.ProcessPendingInbox(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("process: %v", err)
	}
	assertAllProcessed(t, inbox)
	if len(remoteObjects.deleted) != 0 || len(remoteObjects.objects) != 1 {
		t.Fatalf("deleted = %v, stored = %+v, want peer B's mirror untouched",
			remoteObjects.deleted, remoteObjects.objects)
	}
}

func TestIngestIgnoresNonPeer(t *testing.T) {
	ctx := context.Background()
	federation, _, _, _, inbox, peers, remoteObjects := newTestFederation()
	seedTestPeer(peers, testPeerActor, domain.PeerPending)
	stranger := "https://stranger.example/ap/actors/catalog"

	strangerArticle := `{"id":"https://stranger.example/ap/objects/x","type":"Article","name":"Contraband"}`
	pending := domain.InboxEvent{
		ID: "https://peer.example/ap/outbox/c1", Type: "Create", Actor: testPeerActor,
		ObjectID: "https://peer.example/ap/objects/obj-1",
		Payload:  peerCreatePayload(testPeerActor, peerArticleObject("")),
	}
	unknownCreate := domain.InboxEvent{
		ID: "https://stranger.example/ap/outbox/c2", Type: "Create", Actor: stranger,
		ObjectID: "https://stranger.example/ap/objects/x",
		Payload: `{"id":"https://stranger.example/ap/outbox/c2","type":"Create","actor":"` + stranger +
			`","object":` + strangerArticle + `}`,
	}
	unknownDelete := domain.InboxEvent{
		ID: "https://stranger.example/ap/outbox/d2", Type: "Delete", Actor: stranger,
		ObjectID: "https://stranger.example/ap/objects/x",
		Payload: `{"id":"https://stranger.example/ap/outbox/d2","type":"Delete","actor":"` + stranger +
			`","object":{"id":"https://stranger.example/ap/objects/x","type":"Tombstone"}}`,
	}
	for _, event := range []domain.InboxEvent{pending, unknownCreate, unknownDelete} {
		addTestInboxEvent(t, ctx, inbox, event)
	}

	if err := federation.ProcessPendingInbox(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(remoteObjects.puts) != 0 || len(remoteObjects.deleted) != 0 {
		t.Fatalf("puts = %+v, deleted = %v, want no mirrors from non-accepted actors",
			remoteObjects.puts, remoteObjects.deleted)
	}
	if len(peers.accepted) != 0 {
		t.Fatalf("accepted = %v, want no state change", peers.accepted)
	}
	assertAllProcessed(t, inbox)
}

func TestIngestArticleDefensiveParsing(t *testing.T) {
	dropped := func(t *testing.T, name, object string) {
		t.Helper()
		if _, ok := decodePeerArticle([]byte(peerCreatePayload(testPeerActor, object)), testPeerActor); ok {
			t.Fatalf("%s: article was ingested, want it dropped", name)
		}
	}

	t.Run("object as bare string id", func(t *testing.T) {
		dropped(t, "bare string object", `"https://peer.example/ap/objects/obj-1"`)
	})
	t.Run("missing name", func(t *testing.T) {
		dropped(t, "missing name", `{"id":"https://peer.example/ap/objects/obj-1","type":"Article"}`)
	})
	t.Run("whitespace name", func(t *testing.T) {
		dropped(t, "whitespace name", `{"id":"https://peer.example/ap/objects/obj-1","type":"Article","name":"   "}`)
	})
	t.Run("missing article id", func(t *testing.T) {
		dropped(t, "missing id", `{"type":"Article","name":"Dune"}`)
	})
	t.Run("non-http article id", func(t *testing.T) {
		dropped(t, "non-http id", `{"id":"ftp://peer.example/ap/objects/x","type":"Article","name":"Dune"}`)
	})
	t.Run("actor mismatch", func(t *testing.T) {
		// Same origin, but the activity names a different actor than the peer.
		impersonator := `{"@context":"https://www.w3.org/ns/activitystreams","id":"https://peer.example/ap/outbox/a1",` +
			`"type":"Create","actor":"https://peer.example/ap/actors/other","object":` + peerArticleObject("") + `}`
		if _, ok := decodePeerArticle([]byte(impersonator), testPeerActor); ok {
			t.Fatal("article from a mismatched activity actor was ingested")
		}
	})

	t.Run("oversized name truncates to 200", func(t *testing.T) {
		long := strings.Repeat("ü", 500)
		object := fmt.Sprintf(`{"id":"https://peer.example/ap/objects/obj-1","type":"Article","name":%q}`, long)
		got, ok := decodePeerArticle([]byte(peerCreatePayload(testPeerActor, object)), testPeerActor)
		if !ok {
			t.Fatal("long name dropped, want it truncated and stored")
		}
		if got.Name != string([]rune(long)[:200]) || len([]rune(got.Name)) != 200 {
			t.Fatalf("name = %d runes, want the first 200 runes", len([]rune(got.Name)))
		}
	})

	t.Run("oversized description truncates to 5000", func(t *testing.T) {
		long := strings.Repeat("d", 6000)
		object := fmt.Sprintf(`{"id":"https://peer.example/ap/objects/obj-1","type":"Article","name":"Dune","content":%q}`, long)
		got, ok := decodePeerArticle([]byte(peerCreatePayload(testPeerActor, object)), testPeerActor)
		if !ok {
			t.Fatal("long description dropped, want it truncated and stored")
		}
		if len([]rune(got.Description)) != 5000 {
			t.Fatalf("description = %d runes, want 5000", len([]rune(got.Description)))
		}
	})

	t.Run("non-http image dropped", func(t *testing.T) {
		got, ok := decodePeerArticle([]byte(peerCreatePayload(testPeerActor,
			peerArticleObject(`,"image":"ftp://peer.example/x.png"`))), testPeerActor)
		if !ok {
			t.Fatal("article dropped for a bad image, want the image only cleared")
		}
		if got.ImageURL != "" {
			t.Fatalf("image = %q, want the non-http(s) url dropped", got.ImageURL)
		}
	})

	t.Run("relative image dropped", func(t *testing.T) {
		got, ok := decodePeerArticle([]byte(peerCreatePayload(testPeerActor,
			peerArticleObject(`,"image":"/static/uploads/x.png"`))), testPeerActor)
		if !ok {
			t.Fatal("article dropped for a relative image, want the image only cleared")
		}
		if got.ImageURL != "" {
			t.Fatalf("image = %q, want the relative path dropped", got.ImageURL)
		}
	})

	availability := func(content string) (domain.RemoteObject, bool) {
		payload := peerCreatePayload(testPeerActor, peerArticleObject(
			`,"attachment":[{"type":"Note","name":"Availability","content":"`+content+`"}]`))
		return decodePeerArticle([]byte(payload), testPeerActor)
	}
	t.Run("availability on loan", func(t *testing.T) {
		got, ok := availability("on loan")
		if !ok || !got.Busy || got.BusyUntil != nil {
			t.Fatalf("object = %+v (ok %v), want Busy true with nil until", got, ok)
		}
	})
	t.Run("availability unparseable expiry fails closed", func(t *testing.T) {
		got, ok := availability("on loan until tomorrow")
		if !ok || !got.Busy || got.BusyUntil != nil {
			t.Fatalf("object = %+v (ok %v), want an unreadable deadline to stay Busy with nil until", got, ok)
		}
	})
	t.Run("availability missing timestamp fails closed", func(t *testing.T) {
		got, ok := availability("on loan until ")
		if !ok || !got.Busy || got.BusyUntil != nil {
			t.Fatalf("object = %+v (ok %v), want a missing deadline to stay Busy with nil until", got, ok)
		}
	})
	t.Run("availability garbage", func(t *testing.T) {
		got, ok := availability("churned")
		if !ok || got.Busy || got.BusyUntil != nil {
			t.Fatalf("object = %+v (ok %v), want non-loan text to read as available", got, ok)
		}
	})

	t.Run("tag list clamps to twenty", func(t *testing.T) {
		parts := make([]string, 0, 25)
		for i := 0; i < 25; i++ {
			parts = append(parts, fmt.Sprintf(`{"type":"Tag","name":"#tag%02d"}`, i))
		}
		got, ok := decodePeerArticle([]byte(peerCreatePayload(testPeerActor,
			peerArticleObject(`,"attachment":[`+strings.Join(parts, ",")+`]`))), testPeerActor)
		if !ok {
			t.Fatal("article dropped for 25 tags, want the tag list truncated")
		}
		if len(got.Tags) != 20 {
			t.Fatalf("tags = %v, want the first 20", got.Tags)
		}
	})
	t.Run("tag cap counts unique tags", func(t *testing.T) {
		parts := make([]string, 0, 30)
		for i := 0; i < 10; i++ {
			parts = append(parts, `{"type":"Tag","name":"#dupe"}`)
		}
		for i := 0; i < 20; i++ {
			parts = append(parts, fmt.Sprintf(`{"type":"Tag","name":"#tag%02d"}`, i))
		}
		got, ok := decodePeerArticle([]byte(peerCreatePayload(testPeerActor,
			peerArticleObject(`,"attachment":[`+strings.Join(parts, ",")+`]`))), testPeerActor)
		if !ok {
			t.Fatal("article dropped for repeated tags, want truncation")
		}
		// Duplicates fold before the cap: the 20 kept tags are dupe + tag00..tag18,
		// not 10 duplicates crowding out 19 unique numbered tags.
		if len(got.Tags) != 20 || got.Tags[0] != "dupe" || got.Tags[1] != "tag00" || got.Tags[19] != "tag18" {
			t.Fatalf("tags = %v, want dupe followed by tag00..tag18", got.Tags)
		}
	})
}

func TestIngestRejectsForeignOriginID(t *testing.T) {
	ctx := context.Background()
	federation, _, _, _, inbox, peers, remoteObjects := newTestFederation()
	seedTestPeer(peers, testPeerActor, domain.PeerAccepted)

	const articleID = "https://peer.example/ap/objects/ok"
	foreignID := "https://evil.example/ap/objects/x"
	suffixTrickID := "https://peer.example.evil.example/ap/objects/x"
	foreignArticle := `{"id":"` + foreignID + `","type":"Article","name":"Contraband"}`
	// Same path suffix, different origin: a naive suffix check would accept it.
	suffixTrickArticle := `{"id":"` + suffixTrickID + `","type":"Article","name":"Contraband"}`
	ownArticle := `{"id":"` + articleID + `","type":"Article","name":"Dune"}`
	for i, article := range []struct{ object, id string }{
		{foreignArticle, foreignID},
		{suffixTrickArticle, suffixTrickID},
		{ownArticle, articleID},
	} {
		activityID := fmt.Sprintf("https://peer.example/ap/outbox/c%d", i)
		addTestInboxEvent(t, ctx, inbox, domain.InboxEvent{
			ID: activityID, Type: "Create", Actor: testPeerActor,
			ObjectID: article.id,
			Payload:  `{"id":"` + activityID + `","type":"Create","actor":"` + testPeerActor + `","object":` + article.object + `}`,
		})
	}

	if err := federation.ProcessPendingInbox(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("process: %v", err)
	}
	assertAllProcessed(t, inbox)
	if len(remoteObjects.puts) != 1 {
		t.Fatalf("puts = %+v, want only the same-origin article mirrored", remoteObjects.puts)
	}
	if remoteObjects.puts[0].ID != articleID {
		t.Fatalf("put id = %q, want %q", remoteObjects.puts[0].ID, articleID)
	}

	// Origin comparison is case-insensitive on scheme+host.
	mixedCase := "https://Peer.Example/ap/actors/catalog"
	got, ok := decodePeerArticle([]byte(peerCreatePayload(mixedCase, ownArticle)), mixedCase)
	if !ok {
		t.Fatal("case-insensitive origin match was dropped")
	}
	if got.PeerActorID != mixedCase || got.ID != articleID {
		t.Fatalf("object = %+v, want the peer actor id kept verbatim", got)
	}
}

func TestIngestBadArticleDoesNotFailBatch(t *testing.T) {
	ctx := context.Background()
	federation, _, _, _, inbox, peers, remoteObjects := newTestFederation()
	seedTestPeer(peers, testPeerActor, domain.PeerAccepted)

	junk := domain.InboxEvent{
		ID: "https://junk.example/ap/outbox/j1", Type: "Create",
		Actor:   "https://junk.example/ap/actors/catalog",
		Payload: "this is not json at all {{{",
	}
	acceptedPeerJunk := domain.InboxEvent{
		ID: "https://peer.example/ap/outbox/bad-1", Type: "Create", Actor: testPeerActor,
		ObjectID: "https://peer.example/ap/objects/bad-1",
		Payload:  peerCreatePayload(testPeerActor, `"https://peer.example/ap/objects/bad-1"`),
	}
	good := domain.InboxEvent{
		ID: "https://peer.example/ap/outbox/good-1", Type: "Create", Actor: testPeerActor,
		ObjectID: "https://peer.example/ap/objects/obj-1",
		Payload:  peerCreatePayload(testPeerActor, peerArticleObject("")),
	}
	for _, event := range []domain.InboxEvent{junk, acceptedPeerJunk, good} {
		addTestInboxEvent(t, ctx, inbox, event)
	}

	if err := federation.ProcessPendingInbox(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("process: %v", err)
	}
	assertAllProcessed(t, inbox)
	if len(remoteObjects.puts) != 1 || remoteObjects.puts[0].ID != "https://peer.example/ap/objects/obj-1" {
		t.Fatalf("puts = %+v, want only the good article mirrored", remoteObjects.puts)
	}
}
