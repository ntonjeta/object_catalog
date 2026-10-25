package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"biblio-oggetto/internal/domain"
)

// ImportRow is one parsed spreadsheet row. Row is the original spreadsheet row
// number, including the header row, so errors can point back to the file.
type ImportRow struct {
	Row            int
	Name           string
	Description    string
	Category       string
	Tags           []string
	PhotoURL       string
	HasDescription bool
	HasCategory    bool
	HasTags        bool
	HasPhotoURL    bool
}

type ImportFailure struct {
	Row     int
	Message string
}

type ImportResult struct {
	Created  int
	Updated  int
	Failures []ImportFailure
}

// Import upserts objects by name. Rows are independent: a failed row does not
// prevent later rows from being imported.
func (uc *Objects) Import(ctx context.Context, rows []ImportRow) (ImportResult, error) {
	result := ImportResult{Failures: make([]ImportFailure, 0)}
	if len(rows) == 0 {
		return result, fmt.Errorf("%w: import file has no data rows", domain.ErrInvalid)
	}
	categoryIDs, err := uc.categoryIndex(ctx)
	if err != nil {
		return result, err
	}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if err := uc.importRow(ctx, row, categoryIDs, &result); err != nil {
			result.Failures = append(result.Failures, ImportFailure{Row: row.Row, Message: err.Error()})
		}
	}
	return result, nil
}

func (uc *Objects) importRow(ctx context.Context, row ImportRow, categoryIDs map[string]string, result *ImportResult) error {
	name := domain.NormalizeObjectName(row.Name)
	if err := domain.ValidateObjectName(name); err != nil {
		return err
	}
	description := strings.TrimSpace(row.Description)
	if err := domain.ValidateObjectDescription(description); err != nil {
		return err
	}
	tags, err := domain.NormalizeTags(row.Tags)
	if err != nil {
		return err
	}
	photoURL := strings.TrimSpace(row.PhotoURL)
	if err := domain.ValidateObjectPhotoPath(photoURL); err != nil {
		return err
	}
	existing, err := uc.objects.ByName(ctx, name)
	switch {
	case err == nil:
		categoryID := ""
		if row.HasCategory {
			categoryID, err = uc.importCategoryID(ctx, row.Category, categoryIDs)
			if err != nil {
				return err
			}
		}
		input := UpdateObjectInput{Name: &name}
		if row.HasDescription {
			input.Description = &description
		}
		if row.HasCategory {
			input.CategoryID = &categoryID
		}
		if row.HasTags {
			input.Tags = &tags
		}
		if row.HasPhotoURL && photoURL != "" {
			input.PhotoPath = &photoURL
		}
		if _, err := uc.Update(ctx, existing.ID, input); err != nil {
			return err
		}
		result.Updated++
		return nil
	case errors.Is(err, domain.ErrNotFound):
		categoryID := ""
		if row.HasCategory {
			categoryID, err = uc.importCategoryID(ctx, row.Category, categoryIDs)
			if err != nil {
				return err
			}
		}
		if _, err := uc.Create(ctx, CreateObjectInput{
			Name:        name,
			Description: description,
			CategoryID:  categoryID,
			Tags:        tags,
			PhotoPath:   photoURL,
		}); err != nil {
			return err
		}
		result.Created++
		return nil
	default:
		return err
	}
}

func (uc *Objects) categoryIndex(ctx context.Context) (map[string]string, error) {
	categories, err := uc.categories.List(ctx)
	if err != nil {
		return nil, err
	}
	index := make(map[string]string, len(categories))
	for _, category := range categories {
		index[strings.ToLower(domain.NormalizeCategoryName(category.Name))] = category.ID
	}
	return index, nil
}

func (uc *Objects) importCategoryID(ctx context.Context, name string, index map[string]string) (string, error) {
	name = domain.NormalizeCategoryName(name)
	if name == "" {
		return "", nil
	}
	key := strings.ToLower(name)
	if id, ok := index[key]; ok {
		return id, nil
	}
	category, err := NewCategories(uc.categories).Create(ctx, name)
	if err == nil {
		index[key] = category.ID
		return category.ID, nil
	}
	if !errors.Is(err, domain.ErrConflict) {
		return "", err
	}
	// Another request may have created the category between listing and
	// creating it. Refresh once and fall back to the existing record.
	categories, listErr := uc.categories.List(ctx)
	if listErr != nil {
		return "", listErr
	}
	for _, existing := range categories {
		if strings.EqualFold(domain.NormalizeCategoryName(existing.Name), name) {
			index[key] = existing.ID
			return existing.ID, nil
		}
	}
	return "", err
}
