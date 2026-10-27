package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"biblio-oggetto/internal/domain"
)

type outboxExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// appendOutboxTx inserts a federation outbox entry within the given
// transaction. A zero entry (empty ID) is skipped.
func appendOutboxTx(ctx context.Context, execer outboxExecer, entry domain.OutboxEntry) error {
	if entry.ID == "" {
		return nil
	}
	objectJSON := ""
	if entry.Kind != domain.OutboxAccept {
		encoded, err := json.Marshal(entry.Object)
		if err != nil {
			return fmt.Errorf("encode outbox object snapshot: %w", err)
		}
		objectJSON = string(encoded)
	}
	_, err := execer.ExecContext(ctx,
		`INSERT INTO outbox (id, kind, object_json, target, follow_id, published_at, attempts, next_attempt_at)
		 VALUES (?, ?, ?, ?, ?, ?, 0, ?)`,
		entry.ID, string(entry.Kind), objectJSON, entry.Target, entry.FollowID,
		formatTime(entry.Published), formatTime(entry.NextAttemptAt),
	)
	return mapError(err)
}

type OutboxRepo struct {
	db *sql.DB
}

func NewOutboxRepo(db *sql.DB) *OutboxRepo {
	return &OutboxRepo{db: db}
}

func (r *OutboxRepo) Append(ctx context.Context, entry domain.OutboxEntry) error {
	return appendOutboxTx(ctx, r.db, entry)
}

func (r *OutboxRepo) ByID(ctx context.Context, id string) (domain.OutboxEntry, error) {
	row := r.db.QueryRowContext(ctx, outboxSelect+` WHERE id = ?`, id)
	return scanOutboxEntry(row)
}

func (r *OutboxRepo) List(ctx context.Context, limit, offset int) ([]domain.OutboxEntry, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM outbox`).Scan(&total); err != nil {
		return nil, 0, mapError(err)
	}
	rows, err := r.db.QueryContext(ctx,
		outboxSelect+` ORDER BY published_at DESC, id DESC LIMIT ? OFFSET ?`, limit, offset,
	)
	if err != nil {
		return nil, 0, mapError(err)
	}
	defer rows.Close()

	entries := make([]domain.OutboxEntry, 0)
	for rows.Next() {
		entry, err := scanOutboxEntry(rows)
		if err != nil {
			return nil, 0, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, mapError(err)
	}
	return entries, total, nil
}

func (r *OutboxRepo) Pending(ctx context.Context, now time.Time, limit int) ([]domain.OutboxEntry, error) {
	rows, err := r.db.QueryContext(ctx,
		outboxSelect+` WHERE delivered_at IS NULL AND next_attempt_at <= ?
		 ORDER BY published_at ASC, id ASC LIMIT ?`,
		formatTime(now), limit,
	)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()

	entries := make([]domain.OutboxEntry, 0)
	for rows.Next() {
		entry, err := scanOutboxEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, mapError(err)
	}
	return entries, nil
}

func (r *OutboxRepo) MarkDelivered(ctx context.Context, id string, at time.Time) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE outbox SET delivered_at = ? WHERE id = ?`, formatTime(at), id,
	)
	return checkUpdate(result, err, "outbox delivery mark")
}

func (r *OutboxRepo) MarkAttempt(ctx context.Context, id string, attempts int, nextAttemptAt time.Time) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE outbox SET attempts = ?, next_attempt_at = ? WHERE id = ?`,
		attempts, formatTime(nextAttemptAt), id,
	)
	return checkUpdate(result, err, "outbox attempt mark")
}

const outboxSelect = `SELECT id, kind, object_json, target, follow_id, published_at, delivered_at, attempts, next_attempt_at FROM outbox`

func checkUpdate(result sql.Result, err error, what string) error {
	if err != nil {
		return mapError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check %s: %w", what, err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func scanOutboxEntry(row scanner) (domain.OutboxEntry, error) {
	var (
		entry       domain.OutboxEntry
		objectJSON  string
		deliveredAt sql.NullString
		publishedAt string
		nextAttempt string
		kind        string
	)
	if err := row.Scan(
		&entry.ID, &kind, &objectJSON, &entry.Target, &entry.FollowID,
		&publishedAt, &deliveredAt, &entry.Attempts, &nextAttempt,
	); err != nil {
		return domain.OutboxEntry{}, mapError(err)
	}
	entry.Kind = domain.OutboxKind(kind)
	var err error
	if entry.Published, err = parseTime(publishedAt); err != nil {
		return domain.OutboxEntry{}, err
	}
	if deliveredAt.Valid {
		delivered, parseErr := parseTime(deliveredAt.String)
		if parseErr != nil {
			return domain.OutboxEntry{}, parseErr
		}
		entry.DeliveredAt = &delivered
	}
	if nextAttempt != "" {
		at, parseErr := parseTime(nextAttempt)
		if parseErr != nil {
			return domain.OutboxEntry{}, parseErr
		}
		entry.NextAttemptAt = at
	}
	if objectJSON != "" {
		if err = json.Unmarshal([]byte(objectJSON), &entry.Object); err != nil {
			return domain.OutboxEntry{}, fmt.Errorf("decode outbox object snapshot: %w", err)
		}
	}
	return entry, nil
}
