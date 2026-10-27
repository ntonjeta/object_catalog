package e2e

import (
	"context"
	"testing"

	"biblio-oggetto/internal/domain"
)

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
