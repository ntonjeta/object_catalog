package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"biblio-oggetto/internal/domain"
	"biblio-oggetto/internal/usecase"
)

type ObjectRepo struct {
	db *sql.DB
}

func NewObjectRepo(db *sql.DB) *ObjectRepo {
	return &ObjectRepo{db: db}
}

func (r *ObjectRepo) Create(ctx context.Context, object domain.Object, entry domain.OutboxEntry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin object creation: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO objects (id, name, description, image_path, busy, busy_until, category_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		object.ID, object.Name, object.Description, object.ImagePath,
		busyValue(object.Busy), formatOptionalTime(object.BusyUntil), categoryID(object.Category),
		formatTime(object.CreatedAt), formatTime(object.UpdatedAt),
	)
	if err != nil {
		return mapError(err)
	}
	if err := replaceTagsIn(ctx, tx, objectTagsTable, object.ID, object.Tags); err != nil {
		return err
	}
	images := object.Images
	if len(images) == 0 && isBareFileName(object.ImagePath) {
		// A cover stored as an uploaded file name seeds row 0 in the same tx.
		id, err := domain.NewID()
		if err != nil {
			return fmt.Errorf("generate object image id: %w", err)
		}
		images = []domain.ObjectImage{{ID: id, FileName: object.ImagePath}}
	}
	if err := replaceImages(ctx, tx, object.ID, images); err != nil {
		return err
	}
	if err := appendOutboxTx(ctx, tx, entry); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit object creation: %w", err)
	}
	return nil
}

func (r *ObjectRepo) Update(ctx context.Context, object domain.Object, entry domain.OutboxEntry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin object update: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx,
		`UPDATE objects SET name = ?, description = ?, image_path = ?, busy = ?, busy_until = ?, category_id = ?, updated_at = ? WHERE id = ?`,
		object.Name, object.Description, object.ImagePath,
		busyValue(object.Busy), formatOptionalTime(object.BusyUntil), categoryID(object.Category),
		formatTime(object.UpdatedAt), object.ID,
	)
	if err != nil {
		return mapError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check object update: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	if err := replaceTagsIn(ctx, tx, objectTagsTable, object.ID, object.Tags); err != nil {
		return err
	}
	if err := appendOutboxTx(ctx, tx, entry); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit object update: %w", err)
	}
	return nil
}

func (r *ObjectRepo) Delete(ctx context.Context, id string, entry domain.OutboxEntry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin object deletion: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `DELETE FROM objects WHERE id = ?`, id)
	if err != nil {
		return mapError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check object deletion: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	if err := appendOutboxTx(ctx, tx, entry); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit object deletion: %w", err)
	}
	return nil
}

func (r *ObjectRepo) ByID(ctx context.Context, id string) (domain.Object, error) {
	row := r.db.QueryRowContext(ctx, objectSelect+` WHERE o.id = ?`, id)
	object, err := scanObject(row)
	if err != nil {
		return domain.Object{}, err
	}
	tags, err := r.tags(ctx, object.ID)
	if err != nil {
		return domain.Object{}, err
	}
	object.Tags = tags
	images, err := r.Images(ctx, object.ID)
	if err != nil {
		return domain.Object{}, err
	}
	object.Images = images
	return object, nil
}

func (r *ObjectRepo) ByName(ctx context.Context, name string) (domain.Object, error) {
	rows, err := r.db.QueryContext(ctx,
		objectSelect+` WHERE o.name = ? COLLATE NOCASE ORDER BY o.created_at ASC, o.id ASC LIMIT 2`,
		name,
	)
	if err != nil {
		return domain.Object{}, mapError(err)
	}
	defer rows.Close()

	var object domain.Object
	found := 0
	for rows.Next() {
		scanned, err := scanObject(rows)
		if err != nil {
			return domain.Object{}, err
		}
		if found == 0 {
			object = scanned
		}
		found++
	}
	if err := rows.Err(); err != nil {
		return domain.Object{}, mapError(err)
	}
	if err := rows.Close(); err != nil {
		return domain.Object{}, mapError(err)
	}
	switch found {
	case 0:
		return domain.Object{}, domain.ErrNotFound
	case 1:
		tags, err := r.tags(ctx, object.ID)
		if err != nil {
			return domain.Object{}, err
		}
		object.Tags = tags
		return object, nil
	default:
		return domain.Object{}, fmt.Errorf("%w: multiple objects named %q", domain.ErrConflict, name)
	}
}

func (r *ObjectRepo) List(ctx context.Context, filter usecase.ObjectFilter) ([]domain.Object, int, error) {
	where, args := combinedWhere(filter)

	var total int
	if err := r.db.QueryRowContext(ctx, combinedCount+where, args...).Scan(&total); err != nil {
		return nil, 0, mapError(err)
	}

	queryArgs := append(append([]any{}, args...), filter.Limit, filter.Offset)
	rows, err := r.db.QueryContext(ctx,
		combinedSelect+where+` ORDER BY combined.created_at DESC, combined.id DESC LIMIT ? OFFSET ?`,
		queryArgs...,
	)
	if err != nil {
		return nil, 0, mapError(err)
	}
	defer rows.Close()

	objects := make([]domain.Object, 0)
	for rows.Next() {
		object, err := scanCombined(rows)
		if err != nil {
			return nil, 0, err
		}
		objects = append(objects, object)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, mapError(err)
	}

	// Tags and images are loaded per object; the public catalog is small
	// enough that this keeps the query simple. A batched loader can replace it
	// if needed.
	for i := range objects {
		tags, err := r.tagsFor(ctx, objects[i])
		if err != nil {
			return nil, 0, err
		}
		objects[i].Tags = tags
		images, err := r.Images(ctx, objects[i].ID)
		if err != nil {
			return nil, 0, err
		}
		objects[i].Images = images
	}
	return objects, total, nil
}

// tagsFor reads the tags of a catalog row. Mirrored rows live in their own
// tag table, keyed by the peer article id.
func (r *ObjectRepo) tagsFor(ctx context.Context, object domain.Object) ([]string, error) {
	table := objectTagsTable
	if object.Source != "" {
		table = remoteObjectTagsTable
	}
	return r.tagsFrom(ctx, table, object.ID)
}

func (r *ObjectRepo) tags(ctx context.Context, objectID string) ([]string, error) {
	return r.tagsFrom(ctx, objectTagsTable, objectID)
}

func (r *ObjectRepo) tagsFrom(ctx context.Context, table, objectID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT tag FROM `+table+` WHERE object_id = ? ORDER BY tag`, objectID,
	)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()

	tags := make([]string, 0)
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, mapError(err)
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return nil, mapError(err)
	}
	return tags, nil
}

func (r *ObjectRepo) Images(ctx context.Context, objectID string) ([]domain.ObjectImage, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, file_name FROM object_images WHERE object_id = ? ORDER BY position`, objectID,
	)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()

	images := make([]domain.ObjectImage, 0)
	for rows.Next() {
		var image domain.ObjectImage
		if err := rows.Scan(&image.ID, &image.FileName); err != nil {
			return nil, mapError(err)
		}
		images = append(images, image)
	}
	if err := rows.Err(); err != nil {
		return nil, mapError(err)
	}
	return images, nil
}

// ReplaceImages rewrites the whole image list for one object: positions are
// renumbered from zero in slice order, inside a single transaction.
func (r *ObjectRepo) ReplaceImages(ctx context.Context, objectID string, images []domain.ObjectImage) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin object images replacement: %w", err)
	}
	defer tx.Rollback()

	if err := replaceImages(ctx, tx, objectID, images); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit object images replacement: %w", err)
	}
	return nil
}

func replaceImages(ctx context.Context, tx *sql.Tx, objectID string, images []domain.ObjectImage) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM object_images WHERE object_id = ?`, objectID); err != nil {
		return mapError(err)
	}
	createdAt := formatTime(time.Now().UTC())
	for position, image := range images {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO object_images (id, object_id, position, file_name, created_at) VALUES (?, ?, ?, ?, ?)`,
			image.ID, objectID, position, image.FileName, createdAt,
		); err != nil {
			return mapError(err)
		}
	}
	return nil
}

// isBareFileName reports whether an image_path refers to a stored upload
// rather than a remote URL or a root-relative public path.
func isBareFileName(imagePath string) bool {
	return imagePath != "" && !strings.HasPrefix(imagePath, "/") && !strings.Contains(imagePath, "://")
}

const objectSelect = `SELECT
	o.id, o.name, o.description, o.image_path, o.busy, o.busy_until, o.created_at, o.updated_at,
	c.id, c.name, c.created_at, c.updated_at
	FROM objects o
	LEFT JOIN categories c ON c.id = o.category_id`

// combinedSelect merges the local catalog with the objects mirrored from
// joined peers. The source column carries the peer actor id for mirrored rows
// and stays empty for local ones.
const combinedSelect = `SELECT
	combined.id, combined.name, combined.description, combined.image_path,
	combined.busy, combined.busy_until, combined.category_id, combined.category_name,
	combined.source, combined.created_at, combined.updated_at
	FROM (
		SELECT o.id, o.name, o.description, o.image_path, o.busy, o.busy_until,
			o.category_id, c.name AS category_name, '' AS source, o.created_at, o.updated_at
		FROM objects o LEFT JOIN categories c ON c.id = o.category_id
		UNION ALL
		SELECT r.id, r.name, r.description, r.image_url, r.busy, r.busy_until,
			NULL, NULLIF(r.category_name, ''), r.peer_actor_id, r.published_at, r.updated_at
		FROM remote_objects r
	) combined`

const combinedCount = `SELECT COUNT(*)
	FROM (
		SELECT o.id, o.name, o.description, o.image_path, o.busy, o.busy_until,
			o.category_id, c.name AS category_name, '' AS source, o.created_at, o.updated_at
		FROM objects o LEFT JOIN categories c ON c.id = o.category_id
		UNION ALL
		SELECT r.id, r.name, r.description, r.image_url, r.busy, r.busy_until,
			NULL, NULLIF(r.category_name, ''), r.peer_actor_id, r.published_at, r.updated_at
		FROM remote_objects r
	) combined`

func combinedWhere(filter usecase.ObjectFilter) (string, []any) {
	clauses := make([]string, 0, 3)
	args := make([]any, 0, 9)
	if filter.Query != "" {
		pattern := likePattern(filter.Query)
		clauses = append(clauses, `(combined.name LIKE ? ESCAPE '\'
			OR combined.description LIKE ? ESCAPE '\'
			OR combined.category_name LIKE ? ESCAPE '\'
			OR EXISTS (
				SELECT 1 FROM object_tags ot
				WHERE ot.object_id = combined.id AND ot.tag LIKE ? ESCAPE '\'
			)
			OR EXISTS (
				SELECT 1 FROM remote_object_tags rot
				WHERE rot.object_id = combined.id AND rot.tag LIKE ? ESCAPE '\'
			))`)
		args = append(args, pattern, pattern, pattern, pattern, pattern)
	}
	if filter.Category != "" {
		// The category filter applies to the local catalog only; mirrored
		// objects carry a free-form category name from the peer.
		clauses = append(clauses, `combined.source = '' AND combined.category_name = ? COLLATE NOCASE`)
		args = append(args, filter.Category)
	}
	if filter.Tag != "" {
		clauses = append(clauses, `(EXISTS (
				SELECT 1 FROM object_tags ot
				WHERE ot.object_id = combined.id AND ot.tag = ?
			)
			OR EXISTS (
				SELECT 1 FROM remote_object_tags rot
				WHERE rot.object_id = combined.id AND rot.tag = ?
			))`)
		args = append(args, filter.Tag, filter.Tag)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// scanCombined reads one row of the merged catalog. Local rows keep the
// category id; mirrored rows only carry the peer's category name.
func scanCombined(row scanner) (domain.Object, error) {
	var (
		object               domain.Object
		busy                 int
		busyUntil            sql.NullString
		createdAt, updatedAt string
		categoryID           sql.NullString
		categoryName         sql.NullString
	)
	if err := row.Scan(
		&object.ID, &object.Name, &object.Description, &object.ImagePath, &busy, &busyUntil,
		&categoryID, &categoryName, &object.Source, &createdAt, &updatedAt,
	); err != nil {
		return domain.Object{}, mapError(err)
	}
	object.Busy = busy != 0
	var err error
	if busyUntil.Valid {
		until, err := parseTime(busyUntil.String)
		if err != nil {
			return domain.Object{}, err
		}
		object.BusyUntil = &until
	}
	if object.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Object{}, err
	}
	if object.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return domain.Object{}, err
	}
	switch {
	case categoryID.Valid:
		object.Category = &domain.Category{ID: categoryID.String, Name: categoryName.String}
	case categoryName.Valid:
		object.Category = &domain.Category{Name: categoryName.String}
	}
	object.Tags = []string{}
	return object, nil
}

func scanObject(row scanner) (domain.Object, error) {
	var (
		object               domain.Object
		busy                 int
		busyUntil            sql.NullString
		createdAt, updatedAt string
		categoryID           sql.NullString
		categoryName         sql.NullString
		categoryCreatedAt    sql.NullString
		categoryUpdatedAt    sql.NullString
	)
	if err := row.Scan(
		&object.ID, &object.Name, &object.Description, &object.ImagePath, &busy, &busyUntil,
		&createdAt, &updatedAt,
		&categoryID, &categoryName, &categoryCreatedAt, &categoryUpdatedAt,
	); err != nil {
		return domain.Object{}, mapError(err)
	}
	object.Busy = busy != 0
	var err error
	if busyUntil.Valid {
		until, err := parseTime(busyUntil.String)
		if err != nil {
			return domain.Object{}, err
		}
		object.BusyUntil = &until
	}
	if object.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Object{}, err
	}
	if object.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return domain.Object{}, err
	}
	if categoryID.Valid {
		category := &domain.Category{
			ID:   categoryID.String,
			Name: categoryName.String,
		}
		if category.CreatedAt, err = parseOptionalTime(categoryCreatedAt.String); err != nil {
			return domain.Object{}, err
		}
		if category.UpdatedAt, err = parseOptionalTime(categoryUpdatedAt.String); err != nil {
			return domain.Object{}, err
		}
		object.Category = category
	}
	object.Tags = []string{}
	object.Images = []domain.ObjectImage{}
	return object, nil
}
