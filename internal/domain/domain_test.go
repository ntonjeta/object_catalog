package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestEmailValidation(t *testing.T) {
	if got := NormalizeEmail("  Admin@Example.COM "); got != "admin@example.com" {
		t.Fatalf("NormalizeEmail = %q, want admin@example.com", got)
	}
	valid := []string{"admin@example.com", "a.b+tag@sub.example.org"}
	for _, email := range valid {
		if err := ValidateEmail(email); err != nil {
			t.Errorf("ValidateEmail(%q) = %v, want nil", email, err)
		}
	}
	invalid := []string{"", "not-an-email", "Name <admin@example.com>", "admin@example", strings.Repeat("a", 250) + "@example.com"}
	for _, email := range invalid {
		if err := ValidateEmail(email); !errors.Is(err, ErrInvalid) {
			t.Errorf("ValidateEmail(%q) = %v, want ErrInvalid", email, err)
		}
	}
}

func TestPasswordValidation(t *testing.T) {
	if err := ValidatePassword("password123"); err != nil {
		t.Fatalf("valid password: %v", err)
	}
	if err := ValidatePassword("short"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("short password error = %v, want ErrInvalid", err)
	}
	if err := ValidatePassword(strings.Repeat("a", 73)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("long password error = %v, want ErrInvalid", err)
	}
}

func TestTagNormalization(t *testing.T) {
	tags, err := NormalizeTags([]string{" SciFi ", "scifi", "Classic", "", "  "})
	if err != nil {
		t.Fatalf("NormalizeTags: %v", err)
	}
	want := []string{"scifi", "classic"}
	if len(tags) != len(want) {
		t.Fatalf("tags = %v, want %v", tags, want)
	}
	for i := range want {
		if tags[i] != want[i] {
			t.Fatalf("tags = %v, want %v", tags, want)
		}
	}
	if _, err := NormalizeTags([]string{strings.Repeat("a", 51)}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("long tag error = %v, want ErrInvalid", err)
	}
	tooMany := make([]string, 21)
	for i := range tooMany {
		tooMany[i] = string(rune('a' + i))
	}
	if _, err := NormalizeTags(tooMany); !errors.Is(err, ErrInvalid) {
		t.Fatalf("too many tags error = %v, want ErrInvalid", err)
	}
}

func TestObjectAndCategoryValidation(t *testing.T) {
	if got := NormalizeObjectName("  Dune   Messiah "); got != "Dune Messiah" {
		t.Fatalf("NormalizeObjectName = %q, want Dune Messiah", got)
	}
	if err := ValidateObjectName(""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty object name error = %v, want ErrInvalid", err)
	}
	if err := ValidateObjectName(strings.Repeat("a", 201)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("long object name error = %v, want ErrInvalid", err)
	}
	if err := ValidateObjectDescription(strings.Repeat("a", 5001)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("long description error = %v, want ErrInvalid", err)
	}
	if got := NormalizeCategoryName("  Science   Fiction "); got != "Science Fiction" {
		t.Fatalf("NormalizeCategoryName = %q, want Science Fiction", got)
	}
	if err := ValidateCategoryName("   "); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty category name error = %v, want ErrInvalid", err)
	}
}

func TestObjectIsBusyAt(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	if (Object{}).IsBusyAt(now) {
		t.Fatal("available object is busy")
	}
	if !(Object{Busy: true}).IsBusyAt(now) {
		t.Fatal("busy object without expiry is not busy")
	}
	until := now.Add(time.Hour)
	if !(Object{Busy: true, BusyUntil: &until}).IsBusyAt(now) {
		t.Fatal("object before expiry is not busy")
	}
	past := now.Add(-time.Hour)
	if (Object{Busy: true, BusyUntil: &past}).IsBusyAt(now) {
		t.Fatal("object after expiry is busy")
	}
	start := now.Add(time.Hour)
	if (Object{Busy: true, ReservedFrom: &start, BusyUntil: &until}).IsBusyAt(now) {
		t.Fatal("object before reservation start is busy")
	}
	if !(Object{Busy: true, ReservedFrom: &past, BusyUntil: &until}).IsBusyAt(now) {
		t.Fatal("object during reservation window is not busy")
	}
}

func TestObjectPhotoPathValidation(t *testing.T) {
	for _, value := range []string{"", "/static/uploads/photo.png", "https://example.com/photo.jpg", "http://example.com/photo.jpg"} {
		if err := ValidateObjectPhotoPath(value); err != nil {
			t.Errorf("ValidateObjectPhotoPath(%q) = %v, want nil", value, err)
		}
	}
	for _, value := range []string{"//example.com/photo.jpg", "ftp://example.com/photo.jpg", "photo.jpg", "https://", "/static/photo name.jpg", strings.Repeat("a", 2049)} {
		if err := ValidateObjectPhotoPath(value); !errors.Is(err, ErrInvalid) {
			t.Errorf("ValidateObjectPhotoPath(%q) = %v, want ErrInvalid", value, err)
		}
	}
}

func TestNewID(t *testing.T) {
	first, err := NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	second, err := NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	if len(first) != 32 || len(second) != 32 {
		t.Fatalf("id lengths = %d, %d, want 32", len(first), len(second))
	}
	if first == second {
		t.Fatalf("NewID returned duplicate ids")
	}
}
