package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"

	"biblio-oggetto/internal/domain"
)

func Open(ctx context.Context, path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// A single connection keeps the connection-scoped PRAGMAs in effect and
	// avoids SQLITE_BUSY under modest load. New adapters can relax this.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("apply %q: %w", pragma, err)
		}
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	for _, statement := range schema {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate database: %w", err)
		}
	}
	return ensureObjectColumns(ctx, db)
}

func ensureObjectColumns(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(objects)`)
	if err != nil {
		return fmt.Errorf("inspect objects table: %w", err)
	}
	found := make(map[string]bool)
	for rows.Next() {
		var (
			position     int
			name         string
			columnType   string
			notNull      int
			defaultValue sql.NullString
			primaryKey   int
		)
		if err := rows.Scan(&position, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return fmt.Errorf("inspect objects column: %w", err)
		}
		found[name] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("inspect objects table: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("inspect objects table: %w", err)
	}

	// Older databases stored the public photo path in photo_path.
	if found["photo_path"] && !found["image_path"] {
		if _, err := db.ExecContext(ctx, `ALTER TABLE objects RENAME COLUMN photo_path TO image_path`); err != nil {
			return fmt.Errorf("rename objects photo_path: %w", err)
		}
		found["image_path"] = true
	}

	for _, column := range []struct {
		name       string
		definition string
	}{
		{"image_path", `image_path TEXT NOT NULL DEFAULT ''`},
		{"busy", `busy INTEGER NOT NULL DEFAULT 0`},
		{"busy_until", `busy_until TEXT`},
	} {
		if found[column.name] {
			continue
		}
		if _, err := db.ExecContext(ctx, `ALTER TABLE objects ADD COLUMN `+column.definition); err != nil {
			return fmt.Errorf("add objects %s: %w", column.name, err)
		}
	}
	return nil
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	message := err.Error()
	switch {
	case strings.Contains(message, "UNIQUE constraint failed"):
		return fmt.Errorf("%w: record already exists", domain.ErrConflict)
	case strings.Contains(message, "FOREIGN KEY constraint failed"):
		return fmt.Errorf("%w: referenced record does not exist", domain.ErrInvalid)
	default:
		return err
	}
}
