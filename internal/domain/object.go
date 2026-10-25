package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxObjectNameLength        = 200
	maxObjectDescriptionLength = 5000
	maxTags                    = 20
	maxTagLength               = 50
)

type Object struct {
	ID          string
	Name        string
	Description string
	PhotoPath   string
	Busy        bool
	BusyUntil   *time.Time
	Category    *Category
	Tags        []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// IsBusyAt reports whether the object is taken at the given time. An object
// without an expiry stays busy until it is explicitly released.
func (o Object) IsBusyAt(now time.Time) bool {
	if !o.Busy {
		return false
	}
	return o.BusyUntil == nil || o.BusyUntil.After(now)
}

func NormalizeObjectName(name string) string {
	return strings.Join(strings.Fields(name), " ")
}

func ValidateObjectName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: object name is required", ErrInvalid)
	}
	if utf8.RuneCountInString(name) > maxObjectNameLength {
		return fmt.Errorf("%w: object name must be at most %d characters", ErrInvalid, maxObjectNameLength)
	}
	return nil
}

func ValidateObjectDescription(description string) error {
	if utf8.RuneCountInString(description) > maxObjectDescriptionLength {
		return fmt.Errorf("%w: description must be at most %d characters", ErrInvalid, maxObjectDescriptionLength)
	}
	return nil
}

// NormalizeTags trims, lowercases, and deduplicates tags while preserving order.
func NormalizeTags(tags []string) ([]string, error) {
	normalized := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		if utf8.RuneCountInString(tag) > maxTagLength {
			return nil, fmt.Errorf("%w: tag must be at most %d characters", ErrInvalid, maxTagLength)
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		normalized = append(normalized, tag)
	}
	if len(normalized) > maxTags {
		return nil, fmt.Errorf("%w: at most %d tags are allowed", ErrInvalid, maxTags)
	}
	return normalized, nil
}
