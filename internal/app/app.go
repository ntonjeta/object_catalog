package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"biblio-oggetto/internal/adapter/filestore"
	httpapi "biblio-oggetto/internal/adapter/http"
	"biblio-oggetto/internal/adapter/sqlite"
	"biblio-oggetto/internal/domain"
	"biblio-oggetto/internal/usecase"
)

const (
	defaultSessionTTL = 24 * time.Hour
	defaultUploadDir  = "./static"
)

type Config struct {
	DBPath       string
	SessionTTL   time.Duration
	CookieSecure bool
	UploadDir    string
	BaseURL      string
}

func validateBaseURL(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("BASE_URL is required: the instance origin must be configured for federation")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse BASE_URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("BASE_URL must use the http or https scheme, got %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", errors.New("BASE_URL must include a host")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("BASE_URL must not include a query or fragment")
	}
	return strings.TrimRight(raw, "/"), nil
}

type App struct {
	DB         *sql.DB
	Handler    http.Handler
	Auth       *usecase.Auth
	Objects    *usecase.Objects
	Categories *usecase.Categories
	Identity   *usecase.Identity
}

func New(ctx context.Context, cfg Config) (*App, error) {
	if cfg.DBPath == "" {
		return nil, fmt.Errorf("database path is required")
	}
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = defaultSessionTTL
	}
	if cfg.UploadDir == "" {
		cfg.UploadDir = defaultUploadDir
	}
	baseURL, err := validateBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}

	db, err := sqlite.Open(ctx, cfg.DBPath)
	if err != nil {
		return nil, err
	}
	photos, err := filestore.New(cfg.UploadDir)
	if err != nil {
		db.Close()
		return nil, err
	}

	users := sqlite.NewUserRepo(db)
	sessions := sqlite.NewSessionRepo(db)
	categoryRepo := sqlite.NewCategoryRepo(db)
	objectRepo := sqlite.NewObjectRepo(db)
	reservationRepo := sqlite.NewReservationRepo(db)
	activityRepo := sqlite.NewActivityRepo(db)
	instanceKeyRepo := sqlite.NewInstanceKeyRepo(db)

	auth := usecase.NewAuth(users, sessions, cfg.SessionTTL)
	objects := usecase.NewObjects(objectRepo, categoryRepo, photos, reservationRepo, activityRepo)
	categories := usecase.NewCategories(categoryRepo)
	identity := usecase.NewIdentity(instanceKeyRepo, baseURL)
	handler := httpapi.New(auth, objects, categories, identity, httpapi.Config{
		CookieSecure: cfg.CookieSecure,
		UploadDir:    photos.Dir(),
		BaseURL:      baseURL,
	}).Handler()

	return &App{
		DB:         db,
		Handler:    handler,
		Auth:       auth,
		Objects:    objects,
		Categories: categories,
		Identity:   identity,
	}, nil
}

func (a *App) Close() error {
	return a.DB.Close()
}

func (a *App) CreateAdmin(ctx context.Context, email, password string) (domain.User, error) {
	return a.Auth.CreateAdmin(ctx, email, password)
}
