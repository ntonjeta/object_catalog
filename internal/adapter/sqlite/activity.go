package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"biblio-oggetto/internal/domain"
	"biblio-oggetto/internal/usecase"
)

type ActivityRepo struct {
	db *sql.DB
}

func NewActivityRepo(db *sql.DB) *ActivityRepo {
	return &ActivityRepo{db: db}
}

func (r *ActivityRepo) Create(ctx context.Context, activity domain.Activity) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO activities (id, type, object_id, reservation_id, actor_id, detail, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		activity.ID, string(activity.Type), activity.ObjectID, activity.ReservationID,
		activity.ActorID, activity.Detail, formatTime(activity.CreatedAt),
	)
	return mapError(err)
}

func (r *ActivityRepo) List(ctx context.Context, filter usecase.ActivityFilter) ([]domain.Activity, int, error) {
	const base = `FROM activities a`
	clause := ""
	args := make([]any, 0, 1)
	if filter.ObjectID != "" {
		clause = ` WHERE a.object_id = ?`
		args = append(args, filter.ObjectID)
	}

	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) `+base+clause, args...).Scan(&total); err != nil {
		return nil, 0, mapError(err)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	query := `SELECT a.id, a.type, a.object_id, a.reservation_id, a.actor_id, a.detail, a.created_at ` +
		base + clause + ` ORDER BY a.created_at DESC LIMIT ? OFFSET ?`
	rows, err := r.db.QueryContext(ctx, query, append(args, limit, filter.Offset)...)
	if err != nil {
		return nil, 0, mapError(err)
	}
	defer rows.Close()

	var activities []domain.Activity
	for rows.Next() {
		var (
			activity     domain.Activity
			activityType string
			createdAt    string
		)
		if err := rows.Scan(
			&activity.ID, &activityType, &activity.ObjectID, &activity.ReservationID,
			&activity.ActorID, &activity.Detail, &createdAt,
		); err != nil {
			return nil, 0, mapError(err)
		}
		activity.Type = domain.ActivityType(activityType)
		if activity.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, 0, err
		}
		activities = append(activities, activity)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate activities: %w", err)
	}
	return activities, total, nil
}
