package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"biblio-oggetto/internal/app"
	"biblio-oggetto/internal/domain"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return runServe()
	}
	switch args[0] {
	case "serve":
		return runServe()
	case "create-admin":
		return runCreateAdmin(args[1:])
	default:
		return fmt.Errorf("unknown command %q (expected serve or create-admin)", args[0])
	}
}

func runCreateAdmin(args []string) error {
	flags := flag.NewFlagSet("create-admin", flag.ContinueOnError)
	email := flags.String("email", "", "administrator email address")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *email == "" {
		return errors.New("create-admin: -email is required")
	}
	password := os.Getenv("ADMIN_PASSWORD")
	if password == "" {
		return errors.New("create-admin: ADMIN_PASSWORD environment variable is required")
	}

	cfg, err := configFromEnv()
	if err != nil {
		return err
	}
	ctx := context.Background()
	application, err := app.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer application.Close()

	user, err := application.CreateAdmin(ctx, *email, password)
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return fmt.Errorf("admin %s already exists", *email)
		}
		return err
	}
	fmt.Printf("admin created: %s\n", user.Email)
	return nil
}

func runServe() error {
	cfg, err := configFromEnv()
	if err != nil {
		return err
	}
	addr := envOrDefault("ADDR", ":8080")

	ctx := context.Background()
	application, err := app.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer application.Close()

	server := &http.Server{
		Addr:              addr,
		Handler:           application.Handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(shutdown)

	go func() {
		<-shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	log.Printf("listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func configFromEnv() (app.Config, error) {
	cfg := app.Config{
		DBPath:       envOrDefault("DB_PATH", "catalog.db"),
		SessionTTL:   24 * time.Hour,
		CookieSecure: false,
		UploadDir:    envOrDefault("UPLOAD_DIR", "./static"),
	}
	if raw := os.Getenv("SESSION_TTL"); raw != "" {
		ttl, err := time.ParseDuration(raw)
		if err != nil {
			return app.Config{}, fmt.Errorf("parse SESSION_TTL: %w", err)
		}
		if ttl <= 0 {
			return app.Config{}, errors.New("SESSION_TTL must be positive")
		}
		cfg.SessionTTL = ttl
	}
	if raw := os.Getenv("COOKIE_SECURE"); raw != "" {
		secure, err := strconv.ParseBool(raw)
		if err != nil {
			return app.Config{}, fmt.Errorf("parse COOKIE_SECURE: %w", err)
		}
		cfg.CookieSecure = secure
	}
	return cfg, nil
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
