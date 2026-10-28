package e2e

import (
	"net/http"
	"strings"
	"testing"
)

type federationPeerDTO struct {
	ID        string `json:"id"`
	ActorID   string `json:"actor_id"`
	State     string `json:"state"`
	CreatedAt string `json:"created_at"`
}

type federationFollowerDTO struct {
	ActorID   string `json:"actor_id"`
	CreatedAt string `json:"created_at"`
}

type federationOutboxDTO struct {
	ID            string  `json:"id"`
	Kind          string  `json:"kind"`
	Target        string  `json:"target"`
	PublishedAt   string  `json:"published_at"`
	DeliveredAt   *string `json:"delivered_at"`
	Attempts      int     `json:"attempts"`
	NextAttemptAt string  `json:"next_attempt_at"`
}

type federationOverviewDTO struct {
	Actor struct {
		ID     string `json:"id"`
		Handle string `json:"handle"`
	} `json:"actor"`
	Peers       []federationPeerDTO     `json:"peers"`
	Followers   []federationFollowerDTO `json:"followers"`
	Outbox      []federationOutboxDTO   `json:"outbox"`
	OutboxTotal int                     `json:"outbox_total"`
}

func decodeOverview(t *testing.T, body []byte) federationOverviewDTO {
	t.Helper()
	return decode[federationOverviewDTO](t, body)
}

func TestAdminFederationRequiresAuth(t *testing.T) {
	e := newEnv(t)

	response, body := e.do(e.anonymous, http.MethodGet, "/api/admin/federation", nil)
	assertStatus(t, response, http.StatusUnauthorized, body)
	assertErrorCode(t, body, "unauthorized")

	response, body = e.do(e.anonymous, http.MethodPost, "/api/admin/federation/peers",
		map[string]string{"target": "https://peer.example/ap/actors/catalog"})
	assertStatus(t, response, http.StatusUnauthorized, body)
	assertErrorCode(t, body, "unauthorized")

	response, body = e.do(e.anonymous, http.MethodDelete, "/api/admin/federation/peers/whatever", nil)
	assertStatus(t, response, http.StatusUnauthorized, body)
	assertErrorCode(t, body, "unauthorized")
}

func TestAdminFederationOverview(t *testing.T) {
	e := newEnv(t)
	e.login()

	response, body := e.do(e.admin, http.MethodGet, "/api/admin/federation", nil)
	assertStatus(t, response, http.StatusOK, body)
	overview := decodeOverview(t, body)

	if overview.Actor.ID != e.baseURL+"/ap/actors/catalog" {
		t.Fatalf("actor id = %q, want %q", overview.Actor.ID, e.baseURL+"/ap/actors/catalog")
	}
	if overview.Actor.Handle != "catalog" {
		t.Fatalf("actor handle = %q, want catalog", overview.Actor.Handle)
	}
	if overview.OutboxTotal != 0 || len(overview.Peers) != 0 || len(overview.Followers) != 0 || len(overview.Outbox) != 0 {
		t.Fatalf("empty overview = %+v, want empty arrays", overview)
	}
	// The panel renders arrays directly; null would break it.
	if !strings.Contains(string(body), `"peers":[]`) || !strings.Contains(string(body), `"followers":[]`) || !strings.Contains(string(body), `"outbox":[]`) {
		t.Fatalf("overview body = %s, want empty JSON arrays", string(body))
	}
}

func TestAdminPeerJoinAndRemove(t *testing.T) {
	a := newEnv(t)
	b := newEnv(t)
	a.login()
	b.login()

	response, body := a.do(a.admin, http.MethodPost, "/api/admin/federation/peers",
		map[string]string{"target": b.baseURL + "/ap/actors/catalog"})
	assertStatus(t, response, http.StatusCreated, body)
	peer := decode[struct {
		Peer federationPeerDTO `json:"peer"`
	}](t, body).Peer

	if peer.ID == "" || peer.ActorID != b.baseURL+"/ap/actors/catalog" || peer.State != "pending" {
		t.Fatalf("joined peer = %+v, want pending row with peer actor id", peer)
	}

	// A's overview lists the pending peer.
	response, body = a.do(a.admin, http.MethodGet, "/api/admin/federation", nil)
	assertStatus(t, response, http.StatusOK, body)
	overview := decodeOverview(t, body)
	if len(overview.Peers) != 1 || overview.Peers[0].ID != peer.ID {
		t.Fatalf("A overview peers = %+v, want the joined peer", overview.Peers)
	}

	// The follow travels the outbox: B eventually lists A as a follower.
	a.eventually("B to record A as a follower", func() bool {
		response, body := b.do(b.admin, http.MethodGet, "/api/admin/federation", nil)
		if response.StatusCode != http.StatusOK {
			return false
		}
		bOverview := decodeOverview(t, body)
		for _, follower := range bOverview.Followers {
			if follower.ActorID == a.baseURL+"/ap/actors/catalog" {
				return true
			}
		}
		return false
	})

	// A eventually sees B's Accept and flips the peer to accepted.
	a.eventually("peer to flip to accepted", func() bool {
		response, body := a.do(a.admin, http.MethodGet, "/api/admin/federation", nil)
		if response.StatusCode != http.StatusOK {
			return false
		}
		overview := decodeOverview(t, body)
		return len(overview.Peers) == 1 && overview.Peers[0].State == "accepted"
	})

	response, body = a.do(a.admin, http.MethodDelete, "/api/admin/federation/peers/"+peer.ID, nil)
	assertStatus(t, response, http.StatusNoContent, body)

	response, body = a.do(a.admin, http.MethodGet, "/api/admin/federation", nil)
	assertStatus(t, response, http.StatusOK, body)
	overview = decodeOverview(t, body)
	if len(overview.Peers) != 0 {
		t.Fatalf("peers after remove = %+v, want none", overview.Peers)
	}

	response, body = a.do(a.admin, http.MethodDelete, "/api/admin/federation/peers/unknown-id", nil)
	assertStatus(t, response, http.StatusNotFound, body)
	assertErrorCode(t, body, "not_found")

	response, body = a.do(a.admin, http.MethodPost, "/api/admin/federation/peers",
		map[string]string{"target": ""})
	assertStatus(t, response, http.StatusBadRequest, body)
	assertErrorCode(t, body, "invalid_input")

	response, body = a.do(a.admin, http.MethodPost, "/api/admin/federation/peers",
		map[string]string{"target": "http://127.0.0.1:1/ap/actors/catalog"})
	assertStatus(t, response, http.StatusNotFound, body)
	assertErrorCode(t, body, "not_found")

	response, body = a.do(a.admin, http.MethodGet, "/api/admin/federation", nil)
	assertStatus(t, response, http.StatusOK, body)
	overview = decodeOverview(t, body)
	if len(overview.Peers) != 0 {
		t.Fatalf("peers after dead target = %+v, want nothing stored", overview.Peers)
	}
}
