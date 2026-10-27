package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"biblio-oggetto/internal/domain"
)

type ReservationRepo struct {
	db *sql.DB
}

func NewReservationRepo(db *sql.DB) *ReservationRepo {
	return &ReservationRepo{db: db}
}

const reservationSelect = `SELECT id, object_id, status, starts_at, ends_at, created_at, updated_at FROM reservations`

func scanReservation(row scanner) (domain.Reservation, error) {
	var (
		reservation domain.Reservation
		status      string
		startsAt    string
		endsAt      sql.NullString
		createdAt   string
		updatedAt   string
	)
	if err := row.Scan(
		&reservation.ID, &reservation.ObjectID, &status, &startsAt, &endsAt, &createdAt, &updatedAt,
	); err != nil {
		return domain.Reservation{}, mapError(err)
	}
	reservation.Status = domain.ReservationStatus(status)
	var err error
	if reservation.From, err = parseTime(startsAt); err != nil {
		return domain.Reservation{}, err
	}
	if endsAt.Valid {
		until, err := parseTime(endsAt.String)
		if err != nil {
			return domain.Reservation{}, err
		}
		reservation.Until = &until
	}
	if reservation.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Reservation{}, err
	}
	if reservation.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return domain.Reservation{}, err
	}
	return reservation, nil
}

func (r *ReservationRepo) Create(ctx context.Context, reservation domain.Reservation) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO reservations (id, object_id, status, starts_at, ends_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		reservation.ID, reservation.ObjectID, string(reservation.Status), formatTime(reservation.From),
		formatOptionalTime(reservation.Until), formatTime(reservation.CreatedAt), formatTime(reservation.UpdatedAt),
	)
	return mapError(err)
}

func (r *ReservationRepo) Update(ctx context.Context, reservation domain.Reservation) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE reservations SET status = ?, starts_at = ?, ends_at = ?, updated_at = ? WHERE id = ?`,
		string(reservation.Status), formatTime(reservation.From), formatOptionalTime(reservation.Until),
		formatTime(reservation.UpdatedAt), reservation.ID,
	)
	if err != nil {
		return mapError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check reservation update: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *ReservationRepo) ByID(ctx context.Context, id string) (domain.Reservation, error) {
	row := r.db.QueryRowContext(ctx, reservationSelect+` WHERE id = ?`, id)
	return scanReservation(row)
}

func (r *ReservationRepo) ActiveByObject(ctx context.Context, objectID string) ([]domain.Reservation, error) {
	rows, err := r.db.QueryContext(ctx,
		reservationSelect+` WHERE object_id = ? AND status IN (?, ?) ORDER BY starts_at ASC`,
		objectID, string(domain.ReservationPending), string(domain.ReservationConfirmed))
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()

	var reservations []domain.Reservation
	for rows.Next() {
		reservation, err := scanReservation(rows)
		if err != nil {
			return nil, err
		}
		reservations = append(reservations, reservation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reservations: %w", err)
	}
	return reservations, nil
}
