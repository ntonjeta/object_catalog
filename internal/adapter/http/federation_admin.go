package httpapi

import (
	"net/http"
	"time"

	"biblio-oggetto/internal/domain"
)

// federationOutboxLimit bounds the outbox rows the admin panel receives;
// the panel is a debugging view, not a paginator.
const federationOutboxLimit = 50

type federationPeerResponse struct {
	ID        string    `json:"id"`
	ActorID   string    `json:"actor_id"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
}

type federationFollowerResponse struct {
	ActorID   string    `json:"actor_id"`
	CreatedAt time.Time `json:"created_at"`
}

type federationOutboxResponse struct {
	ID            string     `json:"id"`
	Kind          string     `json:"kind"`
	Target        string     `json:"target"`
	PublishedAt   time.Time  `json:"published_at"`
	DeliveredAt   *time.Time `json:"delivered_at"`
	Attempts      int        `json:"attempts"`
	NextAttemptAt time.Time  `json:"next_attempt_at"`
}

type federationOverviewResponse struct {
	Actor struct {
		ID     string `json:"id"`
		Handle string `json:"handle"`
	} `json:"actor"`
	Peers       []federationPeerResponse     `json:"peers"`
	Followers   []federationFollowerResponse `json:"followers"`
	Outbox      []federationOutboxResponse   `json:"outbox"`
	OutboxTotal int                          `json:"outbox_total"`
}

type addPeerRequest struct {
	Target string `json:"target"`
}

type peerEnvelope struct {
	Peer federationPeerResponse `json:"peer"`
}

func (s *Server) federationOverview(w http.ResponseWriter, r *http.Request) {
	actorID, err := s.federation.ActorID(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	peers, err := s.federation.Peers(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	followers, err := s.federation.Followers(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	entries, total, err := s.federation.Outbox(r.Context(), federationOutboxLimit, 0)
	if err != nil {
		writeError(w, err)
		return
	}

	overview := federationOverviewResponse{
		Peers:     make([]federationPeerResponse, 0, len(peers)),
		Outbox:    make([]federationOutboxResponse, 0, len(entries)),
		Followers: make([]federationFollowerResponse, 0, len(followers)),
	}
	overview.Actor.ID = actorID
	overview.Actor.Handle = domain.InstanceActorHandle
	for _, peer := range peers {
		overview.Peers = append(overview.Peers, federationPeerResponse{
			ID:        peer.ID,
			ActorID:   peer.ActorID,
			State:     string(peer.State),
			CreatedAt: peer.CreatedAt,
		})
	}
	for _, follower := range followers {
		overview.Followers = append(overview.Followers, federationFollowerResponse{
			ActorID:   follower.ActorID,
			CreatedAt: follower.CreatedAt,
		})
	}
	for _, entry := range entries {
		overview.Outbox = append(overview.Outbox, federationOutboxResponse{
			ID:            entry.ID,
			Kind:          string(entry.Kind),
			Target:        entry.Target,
			PublishedAt:   entry.Published,
			DeliveredAt:   entry.DeliveredAt,
			Attempts:      entry.Attempts,
			NextAttemptAt: entry.NextAttemptAt,
		})
	}
	overview.OutboxTotal = total
	writeJSON(w, http.StatusOK, overview)
}

func (s *Server) addPeer(w http.ResponseWriter, r *http.Request) {
	var request addPeerRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	peer, err := s.federation.AddPeer(r.Context(), request.Target)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, peerEnvelope{Peer: federationPeerResponse{
		ID:        peer.ID,
		ActorID:   peer.ActorID,
		State:     string(peer.State),
		CreatedAt: peer.CreatedAt,
	}})
}

func (s *Server) removePeer(w http.ResponseWriter, r *http.Request) {
	if err := s.federation.RemovePeer(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
