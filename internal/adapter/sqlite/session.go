package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"biblio-oggetto/internal/domain"
)

type SessionRepo struct {
	db *sql.DB
}

func NewSessionRepo(db *sql.DB) *SessionRepo {
	return &SessionRepo{db: db}
}

func (r *SessionRepo) Create(ctx context.Context, tokenHash, userID string, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?, ?, ?)`,
		tokenHash, userID, formatTime(expiresAt),
	)
	return mapError(err)
}

func (r *SessionRepo) ByTokenHash(ctx context.Context, tokenHash string) (domain.Session, error) {
	var (
		session   domain.Session
		expiresAt string
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT user_id, expires_at FROM sessions WHERE token_hash = ?`, tokenHash,
	).Scan(&session.UserID, &expiresAt)
	if err != nil {
		return domain.Session{}, mapError(err)
	}
	session.ExpiresAt, err = parseTime(expiresAt)
	if err != nil {
		return domain.Session{}, err
	}
	return session, nil
}

func (r *SessionRepo) Delete(ctx context.Context, tokenHash string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	if err != nil {
		return mapError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check session deletion: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *SessionRepo) DeleteByUser(ctx context.Context, userID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return mapError(err)
}

func (r *SessionRepo) DeleteExpired(ctx context.Context, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, formatTime(now))
	return mapError(err)
}
