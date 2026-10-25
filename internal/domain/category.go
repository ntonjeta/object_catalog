package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const maxCategoryNameLength = 100

type Category struct {
	ID        string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NormalizeCategoryName(name string) string {
	return strings.Join(strings.Fields(name), " ")
}

func ValidateCategoryName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: category name is required", ErrInvalid)
	}
	if utf8.RuneCountInString(name) > maxCategoryNameLength {
		return fmt.Errorf("%w: category name must be at most %d characters", ErrInvalid, maxCategoryNameLength)
	}
	return nil
}
