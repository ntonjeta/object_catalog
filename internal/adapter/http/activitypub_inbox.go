package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"biblio-oggetto/internal/domain"
	"biblio-oggetto/internal/usecase"
)

func (s *Server) inbox(w http.ResponseWriter, r *http.Request) {
	var activity struct {
		ID     string          `json:"id"`
		Type   string          `json:"type"`
		Actor  string          `json:"actor"`
		Object json.RawMessage `json:"object"`
	}
	if !decodeJSON(w, r, &activity) {
		return
	}
	objectID := activityObjectID(activity.Object)

	switch activity.Type {
	case "Follow":
		targeted, err := s.targetsSelf(r, objectID)
		if err != nil {
			writeError(w, errInternal)
			return
		}
		if !targeted {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if err := s.federation.Follow(r.Context(), usecase.FollowRequest{
			ActivityID: activity.ID,
			ActorID:    activity.Actor,
			ObjectID:   objectID,
		}); err != nil {
			if errors.Is(err, domain.ErrInvalid) {
				writeError(w, err)
			} else {
				writeError(w, errInternal)
			}
			return
		}
		w.WriteHeader(http.StatusAccepted)
	case "Undo":
		targeted, err := s.targetsSelf(r, objectID)
		if err != nil {
			writeError(w, errInternal)
			return
		}
		if !targeted {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if err := s.federation.Unfollow(r.Context(), usecase.FollowRequest{
			ActivityID: activity.ID,
			ActorID:    activity.Actor,
			ObjectID:   objectID,
		}); err != nil {
			if errors.Is(err, domain.ErrInvalid) {
				writeError(w, err)
			} else {
				writeError(w, errInternal)
			}
			return
		}
		w.WriteHeader(http.StatusAccepted)
	default:
		// Unhandled activity types are acknowledged and dropped.
		w.WriteHeader(http.StatusAccepted)
	}
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

func (s *Server) targetsSelf(r *http.Request, objectID string) (bool, error) {
	actorID, err := s.federation.ActorID(r.Context())
	if err != nil {
		return false, err
	}
	return objectID != "" && objectID == actorID, nil
}

func activityObjectID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString
	}
	var asObject struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &asObject); err == nil {
		return asObject.ID
	}
	return ""
}
