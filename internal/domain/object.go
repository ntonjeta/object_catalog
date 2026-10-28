package domain

import (
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxObjectNameLength        = 200
	maxObjectDescriptionLength = 5000
	maxObjectPhotoPathLength   = 2048
	maxTags                    = 20
	maxTagLength               = 50
	maxImagesPerObject         = 10
)

// ObjectImage is one ordered image attached to an object. FileName is the bare
// name of a file in the photo store, never a URL or root-relative path.
type ObjectImage struct {
	ID       string
	FileName string
}

type Object struct {
	ID          string
	Name        string
	Description string
	ImagePath   string
	// Images is the ordered image list. When non-empty, Images[0].FileName
	// mirrors ImagePath (the denormalized cover).
	Images      []ObjectImage
	Busy        bool
	BusyUntil   *time.Time
	Category    *Category
	Tags        []string
	Reservation *Reservation
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// IsBusyAt reports whether the object is on loan at the given time. An object
// without an expiry stays busy until it is explicitly released. Reservation
// windows are evaluated separately; see Object.EffectiveBusyAt.
func (o Object) IsBusyAt(now time.Time) bool {
	if !o.Busy {
		return false
	}
	return o.BusyUntil == nil || o.BusyUntil.After(now)
}

// EffectiveBusyAt reports whether the object is unavailable at the given
// time, either on loan or inside a confirmed reservation window.
func (o Object) EffectiveBusyAt(now time.Time) bool {
	if o.IsBusyAt(now) {
		return true
	}
	return o.Reservation != nil && o.Reservation.TakesAt(now)
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

// ValidateObjectPhotoPath accepts an empty path, an absolute HTTP(S) URL, or a
// root-relative public path such as /static/uploads/photo.png.
func ValidateObjectPhotoPath(photoPath string) error {
	if photoPath == "" {
		return nil
	}
	if utf8.RuneCountInString(photoPath) > maxObjectPhotoPathLength {
		return fmt.Errorf("%w: photo path must be at most %d characters", ErrInvalid, maxObjectPhotoPathLength)
	}
	if strings.ContainsAny(photoPath, " \t\r\n") {
		return fmt.Errorf("%w: photo path must not contain whitespace", ErrInvalid)
	}
	if strings.HasPrefix(photoPath, "/") {
		if strings.HasPrefix(photoPath, "//") {
			return fmt.Errorf("%w: photo path must not be protocol-relative", ErrInvalid)
		}
		return nil
	}
	parsed, err := url.Parse(photoPath)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%w: photo path must be an http(s) URL or start with /", ErrInvalid)
	}
	return nil
}

// ValidateObjectImageCount enforces the per-object image cap for a prospective
// total count.
func ValidateObjectImageCount(n int) error {
	if n > maxImagesPerObject {
		return fmt.Errorf("%w: an object may have at most %d images", ErrInvalid, maxImagesPerObject)
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
