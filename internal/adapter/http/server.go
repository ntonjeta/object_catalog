package httpapi

import (
	"net/http"

	"biblio-oggetto/internal/httpx"
	"biblio-oggetto/internal/usecase"
)

type Config struct {
	CookieSecure bool
	UploadDir    string
}

type Server struct {
	auth         *usecase.Auth
	objects      *usecase.Objects
	categories   *usecase.Categories
	federation   *usecase.Federation
	cookieSecure bool
	uploadDir    string
}

func New(auth *usecase.Auth, objects *usecase.Objects, categories *usecase.Categories, federation *usecase.Federation, cfg Config) *Server {
	return &Server{
		auth:         auth,
		objects:      objects,
		categories:   categories,
		federation:   federation,
		cookieSecure: cfg.CookieSecure,
		uploadDir:    cfg.UploadDir,
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

	mux.HandleFunc("GET /api/admin/federation", s.requireAuth(s.federationOverview))
	mux.HandleFunc("POST /api/admin/federation/peers", s.requireAuth(s.addPeer))
	mux.HandleFunc("DELETE /api/admin/federation/peers/{id}", s.requireAuth(s.removePeer))

	s.registerStatic(mux)
	mux.HandleFunc("/", s.notFound)

	return httpx.RecoverPanics(mux)
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, errNotFoundRoute)
}
