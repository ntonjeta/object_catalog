package httpapi

import (
	"log"
	"net/http"
	"net/url"
	"strings"

	"biblio-oggetto/internal/usecase"
)

type Config struct {
	CookieSecure bool
	UploadDir    string
	BaseURL      string
}

type Server struct {
	auth           *usecase.Auth
	objects        *usecase.Objects
	categories     *usecase.Categories
	identity       *usecase.Identity
	cookieSecure   bool
	uploadDir      string
	baseURL        string
	federationHost string
}

func New(auth *usecase.Auth, objects *usecase.Objects, categories *usecase.Categories, identity *usecase.Identity, cfg Config) *Server {
	federationHost := ""
	if parsed, err := url.Parse(cfg.BaseURL); err == nil {
		federationHost = strings.ToLower(parsed.Host)
	}
	return &Server{
		auth:           auth,
		objects:        objects,
		categories:     categories,
		identity:       identity,
		cookieSecure:   cfg.CookieSecure,
		uploadDir:      cfg.UploadDir,
		baseURL:        strings.TrimRight(cfg.BaseURL, "/"),
		federationHost: federationHost,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.requireAuth(s.logout))
	mux.HandleFunc("GET /api/auth/me", s.requireAuth(s.me))
	mux.HandleFunc("PUT /api/account/password", s.requireAuth(s.changePassword))
	mux.HandleFunc("DELETE /api/account", s.requireAuth(s.deleteAccount))

	mux.HandleFunc("GET /api/categories", s.listCategories)
	mux.HandleFunc("GET /api/categories/{id}", s.getCategory)
	mux.HandleFunc("POST /api/categories", s.requireAuth(s.createCategory))
	mux.HandleFunc("PATCH /api/categories/{id}", s.requireAuth(s.updateCategory))
	mux.HandleFunc("DELETE /api/categories/{id}", s.requireAuth(s.deleteCategory))

	mux.HandleFunc("GET /api/objects", s.listObjects)
	mux.HandleFunc("GET /api/objects/{id}", s.getObject)
	mux.HandleFunc("POST /api/objects", s.requireAuth(s.createObject))
	mux.HandleFunc("POST /api/objects/import", s.requireAuth(s.importObjects))
	mux.HandleFunc("PATCH /api/objects/{id}", s.requireAuth(s.updateObject))
	mux.HandleFunc("PATCH /api/objects/{id}/busy", s.requireAuth(s.setObjectBusy))
	mux.HandleFunc("POST /api/objects/{id}/images", s.requireAuth(s.addObjectImages))
	mux.HandleFunc("PATCH /api/objects/{id}/images/order", s.requireAuth(s.reorderObjectImages))
	mux.HandleFunc("DELETE /api/objects/{id}/images/{imageId}", s.requireAuth(s.deleteObjectImage))
	mux.HandleFunc("PATCH /api/objects/{id}/reservation", s.requireAuth(s.reserveObject))
	mux.HandleFunc("DELETE /api/objects/{id}", s.requireAuth(s.deleteObject))

	mux.HandleFunc("GET /.well-known/webfinger", s.webfinger)
	mux.HandleFunc("GET /ap/actors/{handle}", s.actor)

	s.registerStatic(mux)
	mux.HandleFunc("/", s.notFound)

	return s.recoverPanics(mux)
}

func (s *Server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("panic serving %s %s: %v", r.Method, r.URL.Path, recovered)
				writeError(w, errInternal)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, errNotFoundRoute)
}
