package activitypub

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"biblio-oggetto/internal/domain"
	"biblio-oggetto/internal/httpx"
)

type webfingerResponse struct {
	Subject string          `json:"subject"`
	Aliases []string        `json:"aliases"`
	Links   []webfingerLink `json:"links"`
}

type webfingerLink struct {
	Rel  string `json:"rel"`
	Type string `json:"type"`
	Href string `json:"href"`
}

type actorResponse struct {
	Context           []string `json:"@context"`
	ID                string   `json:"id"`
	Type              string   `json:"type"`
	Name              string   `json:"name"`
	PreferredUsername string   `json:"preferredUsername"`
	Inbox             string   `json:"inbox"`
	Outbox            string   `json:"outbox"`
	PublicKey         keyJSON  `json:"publicKey"`
}

type keyJSON struct {
	ID           string `json:"id"`
	Owner        string `json:"owner"`
	PublicKeyPEM string `json:"publicKeyPem"`
}

var (
	errNotFederationResource = fmt.Errorf("%w: federation resource not found", domain.ErrNotFound)
	errMissingResource       = fmt.Errorf("%w: resource parameter is required", domain.ErrInvalid)
)

func (s *Server) webfinger(w http.ResponseWriter, r *http.Request) {
	resource := r.URL.Query().Get("resource")
	if resource == "" {
		httpx.WriteError(w, errMissingResource)
		return
	}
	acct, ok := strings.CutPrefix(strings.ToLower(resource), "acct:")
	want := strings.ToLower(domain.InstanceActorHandle + "@" + s.federationHost)
	if !ok || acct != want {
		httpx.WriteError(w, errNotFederationResource)
		return
	}
	actorID := s.baseURL + "/ap/actors/" + domain.InstanceActorHandle
	writeAPJSON(w, http.StatusOK, jrdContentType, webfingerResponse{
		Subject: "acct:" + want,
		Aliases: []string{actorID},
		Links: []webfingerLink{{
			Rel:  "self",
			Type: activityJSONContentType,
			Href: actorID,
		}},
	})
}

func (s *Server) actor(w http.ResponseWriter, r *http.Request) {
	if !strings.EqualFold(r.PathValue("handle"), domain.InstanceActorHandle) {
		httpx.WriteError(w, errNotFederationResource)
		return
	}
	instanceActor, err := s.identity.Actor(r.Context())
	if err != nil {
		log.Printf("load instance actor: %v", err)
		httpx.WriteError(w, errInternal)
		return
	}
	writeAPJSON(w, http.StatusOK, activityJSONContentType, actorResponse{
		Context:           []string{asNamespace, "https://w3id.org/security/v1"},
		ID:                instanceActor.ID,
		Type:              "Service",
		Name:              instanceActor.Name,
		PreferredUsername: instanceActor.PreferredUsername,
		Inbox:             instanceActor.Inbox,
		Outbox:            instanceActor.Outbox,
		PublicKey: keyJSON{
			ID:           instanceActor.PublicKeyID,
			Owner:        instanceActor.ID,
			PublicKeyPEM: instanceActor.PublicKeyPEM,
		},
	})
}
