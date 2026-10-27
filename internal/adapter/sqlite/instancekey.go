package sqlite

import (
	"context"
	"database/sql"

	"biblio-oggetto/internal/domain"
)

type InstanceKeyRepo struct {
	db *sql.DB
}

func NewInstanceKeyRepo(db *sql.DB) *InstanceKeyRepo {
	return &InstanceKeyRepo{db: db}
}

func (r *InstanceKeyRepo) Create(ctx context.Context, key domain.InstanceKey) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO instance_keys (id, private_pem, created_at) VALUES (1, ?, ?)`,
		key.PrivatePEM, formatTime(key.CreatedAt),
	)
	return mapError(err)
}

func (r *InstanceKeyRepo) Get(ctx context.Context) (domain.InstanceKey, error) {
	var (
		instanceKey domain.InstanceKey
		createdAt   string
	)
	row := r.db.QueryRowContext(ctx, `SELECT private_pem, created_at FROM instance_keys WHERE id = 1`)
	if err := row.Scan(&instanceKey.PrivatePEM, &createdAt); err != nil {
		return domain.InstanceKey{}, mapError(err)
	}
	var err error
	instanceKey.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.InstanceKey{}, err
	}
	return instanceKey, nil
}
