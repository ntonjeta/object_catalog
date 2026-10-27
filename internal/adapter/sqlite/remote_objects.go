package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"biblio-oggetto/internal/domain"
	"biblio-oggetto/internal/usecase"
)

// RemoteObjectRepo mirrors objects announced by joined peers. The merged
// catalog (Task 3) reads the rows directly, so only writes live here.
type RemoteObjectRepo struct {
	db *sql.DB
}

func NewRemoteObjectRepo(db *sql.DB) *RemoteObjectRepo {
	return &RemoteObjectRepo{db: db}
}

// Put stores the mirrored object, refreshing every column and replacing its
// tags on re-announce. domain.CreatedAt maps to the published_at column.
func (r *RemoteObjectRepo) Put(ctx context.Context, object domain.RemoteObject) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin remote object store: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO remote_objects (
			id, peer_actor_id, name, description, image_url, busy, busy_until,
			category_name, published_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
			 peer_actor_id = excluded.peer_actor_id,
			 name = excluded.name,
			 description = excluded.description,
			 image_url = excluded.image_url,
			 busy = excluded.busy,
			 busy_until = excluded.busy_until,
			 category_name = excluded.category_name,
			 published_at = excluded.published_at,
			 updated_at = excluded.updated_at`,
		object.ID, object.PeerActorID, object.Name, object.Description, object.ImageURL,
		busyValue(object.Busy), formatOptionalTime(object.BusyUntil), object.CategoryName,
		formatTime(object.CreatedAt), formatTime(object.UpdatedAt),
	)
	if err != nil {
		return mapError(err)
	}
	if err := replaceTagsIn(ctx, tx, remoteObjectTagsTable, object.ID, object.Tags); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit remote object store: %w", err)
	}
	return nil
}

func (r *RemoteObjectRepo) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM remote_objects WHERE id = ?`, id)
	return checkUpdate(result, err, "remote object deletion")
}

// DeleteByPeer drops every mirrored object of one peer. Mirroring nothing is
// not an error, so the row count is not checked.
func (r *RemoteObjectRepo) DeleteByPeer(ctx context.Context, peerActorID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM remote_objects WHERE peer_actor_id = ?`, peerActorID)
	return mapError(err)
}

var _ usecase.RemoteObjectRepository = (*RemoteObjectRepo)(nil)
