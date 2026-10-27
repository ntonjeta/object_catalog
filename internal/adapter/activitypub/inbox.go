package activitypub

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"

	"biblio-oggetto/internal/domain"
	"biblio-oggetto/internal/httpx"
)

func queryInt(r *http.Request, name string, fallback int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%w: %s must be a non-negative integer", domain.ErrInvalid, name)
	}
	return int(value), nil
}

func (s *Server) inbox(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		httpx.WriteError(w, errInternal)
		return
	}

	signer, err := s.verifier.Verify(r, body)
	if err != nil {
		log.Printf("federation: inbox rejected: %v", err)
		httpx.WriteError(w, fmt.Errorf("%w: signature verification failed", domain.ErrUnauthorized))
		return
	}

	var activity struct {
		Actor string `json:"actor"`
	}
	if err := json.Unmarshal(body, &activity); err == nil && activity.Actor != "" && activity.Actor != signer {
		httpx.WriteError(w, fmt.Errorf("%w: signer does not match activity actor", domain.ErrUnauthorized))
		return
	}

	if err := s.federation.EnqueueActivity(r.Context(), signer, body); err != nil {
		switch {
		case errors.Is(err, domain.ErrConflict):
			// Duplicate activity id: acknowledge without reprocessing.
			w.WriteHeader(http.StatusAccepted)
		case errors.Is(err, domain.ErrInvalid):
			httpx.WriteError(w, err)
		default:
			log.Printf("federation: inbox enqueue: %v", err)
			httpx.WriteError(w, errInternal)
		}
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) outbox(w http.ResponseWriter, r *http.Request) {
	actor, err := s.identity.Actor(r.Context())
	if err != nil {
		httpx.WriteError(w, errInternal)
		return
	}
	limit, err := queryInt(r, "limit", 50)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	offset, err := queryInt(r, "offset", 0)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	entries, total, err := s.federation.Outbox(r.Context(), limit, offset)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	orderedItems := make([]json.RawMessage, 0, len(entries))
	for _, entry := range entries {
		body, err := marshalActivity(entry, actor.ID, s.baseURL)
		if err != nil {
			httpx.WriteError(w, errInternal)
			return
		}
		orderedItems = append(orderedItems, body)
	}
	writeAPJSON(w, http.StatusOK, activityJSONContentType, asCollection{
		Context:      asContextJSON,
		ID:           s.baseURL + "/ap/outbox",
		Type:         "OrderedCollection",
		TotalItems:   total,
		OrderedItems: orderedItems,
	})
}

func (s *Server) outboxEntry(w http.ResponseWriter, r *http.Request) {
	actor, err := s.identity.Actor(r.Context())
	if err != nil {
		httpx.WriteError(w, errInternal)
		return
	}
	entry, err := s.federation.OutboxEntry(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			httpx.WriteError(w, errNotFederationResource)
		} else {
			httpx.WriteError(w, errInternal)
		}
		return
	}
	body, err := marshalActivity(entry, actor.ID, s.baseURL)
	if err != nil {
		httpx.WriteError(w, errInternal)
		return
	}
	writeAPJSON(w, http.StatusOK, activityJSONContentType, json.RawMessage(body))
}

func (s *Server) catalogObject(w http.ResponseWriter, r *http.Request) {
	object, err := s.objects.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	writeAPJSON(w, http.StatusOK, activityJSONContentType, articleDoc(s.baseURL, object))
}
