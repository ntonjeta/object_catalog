package sqlite

import (
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
