package app

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
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
}

type App struct {
	DB         *sql.DB
	Handler    http.Handler
	Auth       *usecase.Auth
	Objects    *usecase.Objects
	Categories *usecase.Categories
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

	auth := usecase.NewAuth(users, sessions, cfg.SessionTTL)
	objects := usecase.NewObjects(objectRepo, categoryRepo, photos, reservationRepo, activityRepo)
	categories := usecase.NewCategories(categoryRepo)
	handler := httpapi.New(auth, objects, categories, httpapi.Config{
		CookieSecure: cfg.CookieSecure,
		UploadDir:    photos.Dir(),
	}).Handler()

	return &App{
		DB:         db,
		Handler:    handler,
		Auth:       auth,
		Objects:    objects,
		Categories: categories,
	}, nil
}

func (a *App) Close() error {
	return a.DB.Close()
}

func (a *App) CreateAdmin(ctx context.Context, email, password string) (domain.User, error) {
	return a.Auth.CreateAdmin(ctx, email, password)
}
