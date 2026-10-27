package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"biblio-oggetto/internal/domain"
)

const timeLayout = "2006-01-02T15:04:05.000000000Z"

func formatTime(t time.Time) string {
	return t.UTC().Format(timeLayout)
}

func parseTime(value string) (time.Time, error) {
	t, err := time.Parse(timeLayout, value)
	if err == nil {
		return t, nil
	}
	// Accept timestamps written by earlier versions that used RFC3339Nano.
	fallback, fallbackErr := time.Parse(time.RFC3339Nano, value)
	if fallbackErr != nil {
		return time.Time{}, fmt.Errorf("parse time %q: %w", value, err)
	}
	return fallback, nil
}

func parseOptionalTime(value string) (time.Time, error) {
	return parseTime(value)
}

func formatOptionalTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTime(*value)
}

func busyValue(value bool) int {
	if value {
		return 1
	}
	return 0
}

func likePattern(value string) string {
	escaped := strings.NewReplacer(
		`\`, `\\`,
		`%`, `\%`,
		`_`, `\_`,
	).Replace(value)
	return "%" + escaped + "%"
}

func categoryID(category *domain.Category) any {
	if category == nil {
		return nil
	}
	return category.ID
}

const (
	objectTagsTable       = "object_tags"
	remoteObjectTagsTable = "remote_object_tags"
)

// replaceTagsIn swaps the tags of objectID inside the caller's transaction.
// The table argument selects the local or the mirrored tag table and must be
// one of the package constants; it is interpolated into SQL, never bound.
func replaceTagsIn(ctx context.Context, tx *sql.Tx, table, objectID string, tags []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE object_id = ?`, objectID); err != nil {
		return mapError(err)
	}
	for _, tag := range tags {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO `+table+` (object_id, tag) VALUES (?, ?)`, objectID, tag,
		); err != nil {
			return mapError(err)
		}
	}
	return nil
}
