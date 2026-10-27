package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"

	"biblio-oggetto/internal/domain"
)

func (s *Server) inbox(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeError(w, errInternal)
		return
	}

	signer, err := s.verifier.Verify(r, body)
	if err != nil {
		log.Printf("federation: inbox rejected: %v", err)
		writeError(w, fmt.Errorf("%w: signature verification failed", domain.ErrUnauthorized))
		return
	}

	var activity struct {
		Actor string `json:"actor"`
	}
	if err := json.Unmarshal(body, &activity); err == nil && activity.Actor != "" && activity.Actor != signer {
		writeError(w, fmt.Errorf("%w: signer does not match activity actor", domain.ErrUnauthorized))
		return
	}

	if err := s.federation.EnqueueActivity(r.Context(), signer, body); err != nil {
		switch {
		case errors.Is(err, domain.ErrConflict):
			// Duplicate activity id: acknowledge without reprocessing.
			w.WriteHeader(http.StatusAccepted)
		case errors.Is(err, domain.ErrInvalid):
			writeError(w, err)
		default:
			log.Printf("federation: inbox enqueue: %v", err)
			writeError(w, errInternal)
		}
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) outbox(w http.ResponseWriter, r *http.Request) {
	actor, err := s.identity.Actor(r.Context())
	if err != nil {
		writeError(w, errInternal)
		return
	}
	limit, err := queryInt(r, "limit", 50)
	if err != nil {
		writeError(w, err)
		return
	}
	offset, err := queryInt(r, "offset", 0)
	if err != nil {
		writeError(w, err)
		return
	}
	entries, total, err := s.federation.Outbox(r.Context(), limit, offset)
	if err != nil {
		writeError(w, err)
		return
	}
	orderedItems := make([]json.RawMessage, 0, len(entries))
	for _, entry := range entries {
		body, err := marshalActivity(entry, actor.ID, s.baseURL)
		if err != nil {
			writeError(w, errInternal)
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
		writeError(w, errInternal)
		return
	}
	entry, err := s.federation.OutboxEntry(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, errNotFederationResource)
		} else {
			writeError(w, errInternal)
		}
		return
	}
	body, err := marshalActivity(entry, actor.ID, s.baseURL)
	if err != nil {
		writeError(w, errInternal)
		return
	}
	writeAPJSON(w, http.StatusOK, activityJSONContentType, json.RawMessage(body))
}

func (s *Server) catalogObject(w http.ResponseWriter, r *http.Request) {
	object, err := s.objects.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeAPJSON(w, http.StatusOK, activityJSONContentType, articleDoc(s.baseURL, object))
}
