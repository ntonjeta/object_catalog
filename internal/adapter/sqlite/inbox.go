package sqlite

import (
	"context"
	"database/sql"
	"time"

	"biblio-oggetto/internal/domain"
)

type InboxRepo struct {
	db *sql.DB
}

func NewInboxRepo(db *sql.DB) *InboxRepo {
	return &InboxRepo{db: db}
}

func (r *InboxRepo) Add(ctx context.Context, event domain.InboxEvent) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO inbox_events (id, sender, type, actor, object_id, payload, received_at, attempts, next_attempt_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?)`,
		event.ID, event.Sender, event.Type, event.Actor, event.ObjectID,
		event.Payload, formatTime(event.ReceivedAt), formatTime(event.NextAttemptAt),
	)
	return mapError(err)
}

func (r *InboxRepo) Pending(ctx context.Context, now time.Time, limit int) ([]domain.InboxEvent, error) {
	rows, err := r.db.QueryContext(ctx,
		inboxSelect+` WHERE processed_at IS NULL AND next_attempt_at <= ?
		 ORDER BY received_at ASC, id ASC LIMIT ?`,
		formatTime(now), limit,
	)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()

	events := make([]domain.InboxEvent, 0)
	for rows.Next() {
		event, err := scanInboxEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, mapError(err)
	}
	return events, nil
}

func (r *InboxRepo) MarkProcessed(ctx context.Context, id string, at time.Time) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE inbox_events SET processed_at = ? WHERE id = ?`, formatTime(at), id,
	)
	return checkUpdate(result, err, "inbox processed mark")
}

func (r *InboxRepo) MarkAttempt(ctx context.Context, id string, attempts int, nextAttemptAt time.Time) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE inbox_events SET attempts = ?, next_attempt_at = ? WHERE id = ?`,
		attempts, formatTime(nextAttemptAt), id,
	)
	return checkUpdate(result, err, "inbox attempt mark")
}

const inboxSelect = `SELECT id, sender, type, actor, object_id, payload, received_at, processed_at, attempts, next_attempt_at FROM inbox_events`

func scanInboxEvent(row scanner) (domain.InboxEvent, error) {
	var (
		event       domain.InboxEvent
		receivedAt  string
		processedAt sql.NullString
		nextAttempt string
		err         error
	)
	if err := row.Scan(
		&event.ID, &event.Sender, &event.Type, &event.Actor, &event.ObjectID,
		&event.Payload, &receivedAt, &processedAt, &event.Attempts, &nextAttempt,
	); err != nil {
		return domain.InboxEvent{}, mapError(err)
	}
	if event.ReceivedAt, err = parseTime(receivedAt); err != nil {
		return domain.InboxEvent{}, err
	}
	if processedAt.Valid {
		processed, parseErr := parseTime(processedAt.String)
		if parseErr != nil {
			return domain.InboxEvent{}, parseErr
		}
		event.ProcessedAt = &processed
	}
	if nextAttempt != "" {
		at, parseErr := parseTime(nextAttempt)
		if parseErr != nil {
			return domain.InboxEvent{}, parseErr
		}
		event.NextAttemptAt = at
	}
	return event, nil
}

type RemoteKeyRepo struct {
	db *sql.DB
}

func NewRemoteKeyRepo(db *sql.DB) *RemoteKeyRepo {
	return &RemoteKeyRepo{db: db}
}

func (r *RemoteKeyRepo) Get(ctx context.Context, keyID string) (string, error) {
	var pem string
	row := r.db.QueryRowContext(ctx, `SELECT public_pem FROM remote_keys WHERE key_id = ?`, keyID)
	if err := row.Scan(&pem); err != nil {
		return "", mapError(err)
	}
	return pem, nil
}

func (r *RemoteKeyRepo) Put(ctx context.Context, keyID, publicPEM string, at time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO remote_keys (key_id, public_pem, fetched_at) VALUES (?, ?, ?)
		 ON CONFLICT(key_id) DO UPDATE SET public_pem = excluded.public_pem, fetched_at = excluded.fetched_at`,
		keyID, publicPEM, formatTime(at),
	)
	return mapError(err)
}
