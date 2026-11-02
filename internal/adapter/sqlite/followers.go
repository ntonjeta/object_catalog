package sqlite

import (
	"context"
	"database/sql"

	"biblio-oggetto/internal/domain"
)

type FollowerRepo struct {
	db *sql.DB
}

func NewFollowerRepo(db *sql.DB) *FollowerRepo {
	return &FollowerRepo{db: db}
}

// Add upserts the follower, refreshing the inbox URL on re-follow.
func (r *FollowerRepo) Add(ctx context.Context, follower domain.Follower) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO followers (actor_id, inbox_url, created_at) VALUES (?, ?, ?)
		 ON CONFLICT(actor_id) DO UPDATE SET inbox_url = excluded.inbox_url`,
		follower.ActorID, follower.InboxURL, formatTime(follower.CreatedAt),
	)
	return mapError(err)
}

func (r *FollowerRepo) Remove(ctx context.Context, actorID string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM followers WHERE actor_id = ?`, actorID)
	return checkUpdate(result, err, "follower removal")
}

func (r *FollowerRepo) List(ctx context.Context) ([]domain.Follower, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT actor_id, inbox_url, created_at FROM followers ORDER BY created_at, actor_id`,
	)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()

	followers := make([]domain.Follower, 0)
	for rows.Next() {
		var (
			follower  domain.Follower
			createdAt string
		)
		if err := rows.Scan(&follower.ActorID, &follower.InboxURL, &createdAt); err != nil {
			return nil, mapError(err)
		}
		if follower.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, err
		}
		followers = append(followers, follower)
	}
	if err := rows.Err(); err != nil {
		return nil, mapError(err)
	}
	return followers, nil
}
