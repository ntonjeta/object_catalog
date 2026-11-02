// Package activitypub implements the ActivityPub federation adapter: the
// instance actor and discovery endpoints, the signed inbox/outbox HTTP
// surface, and the background delivery/processing workers. It depends on use
// cases only, never on repositories.
package activitypub

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"biblio-oggetto/internal/domain"
	"biblio-oggetto/internal/httpx"
	"biblio-oggetto/internal/usecase"
)

const (
	activityJSONContentType = "application/activity+json"
	jrdContentType          = "application/jrd+json"
	maxBodyBytes            = 1 << 20
)

var (
	errInternal = httpx.ErrInternal
)

var errNotFoundRoute = fmt.Errorf("%w: route not found", domain.ErrNotFound)

type Config struct {
	BaseURL string
}

type Server struct {
	identity       *usecase.Identity
	federation     *usecase.Federation
	objects        *usecase.Objects
	verifier       *SignatureVerifier
	baseURL        string
	federationHost string
}

func New(identity *usecase.Identity, federation *usecase.Federation, objects *usecase.Objects, verifier *SignatureVerifier, cfg Config) *Server {
	federationHost := ""
	if parsed, err := url.Parse(cfg.BaseURL); err == nil {
		federationHost = strings.ToLower(parsed.Host)
	}
	return &Server{
		identity:       identity,
		federation:     federation,
		objects:        objects,
		verifier:       verifier,
		baseURL:        strings.TrimRight(cfg.BaseURL, "/"),
		federationHost: federationHost,
	}
}

// Handler serves the federation endpoints. The composition root mounts it
// for "/ap/" and "/.well-known/" paths; the mux re-matches full paths.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/webfinger", s.webfinger)
	mux.HandleFunc("GET /ap/actors/{handle}", s.actor)
	mux.HandleFunc("POST /ap/inbox", s.inbox)
	mux.HandleFunc("GET /ap/outbox", s.outbox)
	mux.HandleFunc("GET /ap/outbox/{id}", s.outboxEntry)
	mux.HandleFunc("GET /ap/objects/{id}", s.catalogObject)
	mux.HandleFunc("/", s.notFound)
	return httpx.RecoverPanics(mux)
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, errNotFoundRoute)
}

func writeAPJSON(w http.ResponseWriter, status int, contentType string, value any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode %s response: %v", contentType, err)
	}
}
