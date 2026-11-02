package e2e

import (
	"context"
	"net/http"
	"testing"

	"biblio-oggetto/internal/domain"
)

func containsObject(objects []objectDTO, id string) (objectDTO, bool) {
	for _, object := range objects {
		if object.ID == id {
			return object, true
		}
	}
	return objectDTO{}, false
}

// listObjects fetches the public catalog anonymously and decodes it.
func listObjects(e *env, query string) objectsEnvelope {
	e.t.Helper()
	response, data := e.do(e.anonymous, http.MethodGet, "/api/objects?"+query, nil)
	assertStatus(e.t, response, http.StatusOK, data)
	return decode[objectsEnvelope](e.t, data)
}

// waitForAcceptedPeer joins target and waits until the Accept round trip flips
// the peer to accepted, returning the stored peer.
func waitForAcceptedPeer(joiner, target *env, peerActor string) domain.Peer {
	joiner.t.Helper()
	ctx := context.Background()
	peer, err := joiner.app.Federation.AddPeer(ctx, peerActor)
	if err != nil {
		joiner.t.Fatalf("add peer: %v", err)
	}
	joiner.eventually("peer flips to accepted", func() bool {
		peers, err := joiner.app.Federation.Peers(ctx)
		if err != nil {
			return false
		}
		for _, stored := range peers {
			if stored.ID == peer.ID && stored.State == domain.PeerAccepted {
				return true
			}
		}
		return false
	})
	return peer
}

// TestPeerContentAppearsInCatalog proves the full mirror round trip: a joined
// peer's Create/Update/Delete announcements flow into the public catalog, and
// unfollowing cascades the mirrors away.
func TestPeerContentAppearsInCatalog(t *testing.T) {
	a := newEnv(t) // joins b
	b := newEnv(t) // host
	ctx := context.Background()
	bActor := b.baseURL + "/ap/actors/catalog"

	peer := waitForAcceptedPeer(a, b, bActor)

	b.login()
	novels := b.createCategory("Novels")
	remote := b.createObject(map[string]any{
		"name":        "Dune",
		"description": "desert novel",
		"category_id": novels.ID,
		"tags":        []string{"scifi"},
	})
	wantID := b.baseURL + "/ap/objects/" + remote.ID

	a.eventually("peer article appears in the catalog", func() bool {
		envelope := listObjects(a, "q=Dune")
		_, ok := containsObject(envelope.Objects, wantID)
		return ok
	})

	envelope := listObjects(a, "q=Dune")
	row, ok := containsObject(envelope.Objects, wantID)
	if !ok {
		t.Fatalf("remote row %q missing from catalog", wantID)
	}
	if row.Source != bActor {
		t.Fatalf("row source = %q, want %q", row.Source, bActor)
	}
	if row.Description != "desert novel" {
		t.Fatalf("row description = %q, want mirrored", row.Description)
	}
	if len(row.Tags) != 1 || row.Tags[0] != "scifi" {
		t.Fatalf("row tags = %v, want [scifi]", row.Tags)
	}
	if envelope.Total < 1 {
		t.Fatalf("catalog total = %d, want the remote row counted", envelope.Total)
	}

	// The peer update replaces the mirrored name.
	response, data := b.do(b.admin, http.MethodPatch, "/api/objects/"+remote.ID,
		map[string]any{"name": "Dune Messiah"})
	assertStatus(t, response, http.StatusOK, data)
	a.eventually("peer update renames the mirrored row", func() bool {
		row, ok := containsObject(listObjects(a, "q=Dune").Objects, wantID)
		return ok && row.Name == "Dune Messiah"
	})

	// The peer delete removes the mirrored row.
	response, data = b.do(b.admin, http.MethodDelete, "/api/objects/"+remote.ID, nil)
	assertStatus(t, response, http.StatusNoContent, data)
	a.eventually("peer delete drops the mirrored row", func() bool {
		_, ok := containsObject(listObjects(a, "").Objects, wantID)
		return !ok
	})

	// Unfollowing cascades: any later mirror of the same article is gone and
	// B loses A as a follower.
	second := b.createObject(map[string]any{"name": "Dune's Son"})
	secondID := b.baseURL + "/ap/objects/" + second.ID
	a.eventually("second article mirrored", func() bool {
		_, ok := containsObject(listObjects(a, "q=Dune%27s%20Son").Objects, secondID)
		return ok
	})

	if err := a.app.Federation.RemovePeer(ctx, peer.ID); err != nil {
		t.Fatalf("remove peer: %v", err)
	}
	a.eventually("unfollow drops the mirrored rows", func() bool {
		_, ok := containsObject(listObjects(a, "").Objects, secondID)
		return !ok
	})
	a.eventually("host loses the unfollowed follower", func() bool {
		followers, err := b.app.Federation.Followers(ctx)
		if err != nil {
			return false
		}
		for _, follower := range followers {
			if follower.ActorID == a.baseURL+"/ap/actors/catalog" {
				return false
			}
		}
		return true
	})
}

// TestPeerContentRequiresJoin proves the anti-goal: a validly signed Create
// from an actor we merely follow (not joined) never enters the catalog, and
// joining is what unlocks the mirror.
func TestPeerContentRequiresJoin(t *testing.T) {
	a := newEnv(t) // follower first, peer later
	b := newEnv(t) // host
	ctx := context.Background()
	aActor := a.baseURL + "/ap/actors/catalog"
	bActor := b.baseURL + "/ap/actors/catalog"

	// A becomes B's follower by a direct signed Follow, without joining B.
	response, data := a.signedInboxPost(b, map[string]any{
		"type":   "Follow",
		"id":     a.baseURL + "/ap/outbox/follow-1",
		"actor":  aActor,
		"object": bActor,
	})
	assertStatus(t, response, http.StatusAccepted, data)
	b.eventually("B records A as a follower", func() bool {
		followers, err := b.app.Federation.Followers(ctx)
		return err == nil && len(followers) == 1
	})

	b.login()
	object := b.createObject(map[string]any{
		"name":        "Lonely Object",
		"description": "unjoined",
	})
	wantID := b.baseURL + "/ap/objects/" + object.ID

	b.eventually("B queues the Create for its followers", func() bool {
		return len(b.deliveredCreateEntries(ctx, object.ID)) > 0
	})

	// A has the delivery in its inbox but rejects the content: a follower is
	// not a peer.
	response, data = a.do(a.anonymous, http.MethodGet, "/api/objects?q=Lonely", nil)
	assertStatus(t, response, http.StatusOK, data)
	if _, ok := containsObject(decode[objectsEnvelope](t, data).Objects, wantID); ok {
		t.Fatalf("unjoined peer content leaked into the catalog")
	}

	// Joining flips the gate; a re-push of the article now mirrors.
	waitForAcceptedPeer(a, b, bActor)
	response, data = b.do(b.admin, http.MethodPatch, "/api/objects/"+object.ID,
		map[string]any{"description": "now mirrored"})
	assertStatus(t, response, http.StatusOK, data)

	a.eventually("joined peer content mirrors into the catalog", func() bool {
		row, ok := containsObject(listObjects(a, "q=Lonely").Objects, wantID)
		return ok && row.Source == bActor && row.Description == "now mirrored"
	})
}

// deliveredCreateEntries returns the outbox entries for a local object id that
// have been marked delivered to at least one inbox.
func (e *env) deliveredCreateEntries(ctx context.Context, objectID string) []domain.OutboxEntry {
	e.t.Helper()
	entries, _, err := e.app.Federation.Outbox(ctx, 0, 0)
	if err != nil {
		return nil
	}
	matched := make([]domain.OutboxEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Kind == domain.OutboxCreate && entry.Object.ID == objectID && entry.DeliveredAt != nil {
			matched = append(matched, entry)
		}
	}
	return matched
}

// TestInstanceFollowsInstanceEndToEnd proves the first real two-instance
// handshake: A joins B over the federation use case, A's Deliverer posts the
// Follow to B's inbox, B records A as a follower and answers with an Accept,
// and A's inbox processor flips the peer to accepted.
func TestInstanceFollowsInstanceEndToEnd(t *testing.T) {
	a := newEnv(t) // joins b
	b := newEnv(t) // host
	ctx := context.Background()

	aActor := a.baseURL + "/ap/actors/catalog"
	bActor := b.baseURL + "/ap/actors/catalog"

	peer, err := a.app.Federation.AddPeer(ctx, bActor)
	if err != nil {
		t.Fatalf("add peer: %v", err)
	}

	b.eventually("host records the follower", func() bool {
		followers, err := b.app.Federation.Followers(ctx)
		if err != nil {
			return false
		}
		for _, follower := range followers {
			if follower.ActorID == aActor && follower.InboxURL == a.baseURL+"/ap/inbox" {
				return true
			}
		}
		return false
	})

	a.eventually("peer flips to accepted", func() bool {
		peers, err := a.app.Federation.Peers(ctx)
		if err != nil {
			return false
		}
		for _, stored := range peers {
			if stored.ID == peer.ID && stored.State == domain.PeerAccepted {
				return true
			}
		}
		return false
	})

	// The follow entry the Deliverer posted carried the peer id: it must be
	// marked delivered, not silently consumed by the zero-follower path.
	entry, err := a.app.Federation.OutboxEntry(ctx, peer.ID)
	if err != nil {
		t.Fatalf("follow outbox entry: %v", err)
	}
	if entry.Kind != domain.OutboxFollow {
		t.Fatalf("entry kind = %q, want follow", entry.Kind)
	}
	if entry.DeliveredAt == nil {
		t.Fatalf("follow entry %s was not delivered: %+v", entry.ID, entry)
	}
}
