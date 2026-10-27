package domain

import "time"

type ReservationStatus string

const (
	ReservationPending   ReservationStatus = "pending"
	ReservationConfirmed ReservationStatus = "confirmed"
	ReservationCancelled ReservationStatus = "cancelled"
	ReservationLapsed    ReservationStatus = "lapsed"
)

// Reservation is a future claim on an object. A pending reservation keeps the
// object lendable until the window starts; a confirmed reservation makes the
// object unavailable for the duration of [From, Until).
type Reservation struct {
	ID        string
	ObjectID  string
	From      time.Time
	Until     *time.Time
	Status    ReservationStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (r Reservation) isActive() bool {
	return r.Status == ReservationPending || r.Status == ReservationConfirmed
}

// IsActiveAt reports whether the reservation is still open for business at
// the given time: an active status and a window that has not ended.
func (r Reservation) IsActiveAt(now time.Time) bool {
	if !r.isActive() {
		return false
	}
	return r.Until == nil || now.Before(*r.Until)
}

// TakesAt reports whether the reservation blocks the object at now. Only a
// confirmed reservation inside [From, Until) takes effect.
func (r Reservation) TakesAt(now time.Time) bool {
	if r.Status != ReservationConfirmed || now.Before(r.From) {
		return false
	}
	return r.Until == nil || now.Before(*r.Until)
}

// Covers reports whether the reservation window includes the given instant.
func (r Reservation) Covers(at time.Time) bool {
	if at.Before(r.From) {
		return false
	}
	return r.Until == nil || at.Before(*r.Until)
}

// Overlaps reports whether two reservation windows share any instant. A nil
// end extends the window indefinitely.
func (r Reservation) Overlaps(other Reservation) bool {
	start := r.From
	if other.From.After(start) {
		start = other.From
	}
	switch {
	case r.Until == nil && other.Until == nil:
		return true
	case r.Until == nil:
		return start.Before(*other.Until)
	case other.Until == nil:
		return start.Before(*r.Until)
	default:
		end := *r.Until
		if other.Until.Before(end) {
			end = *other.Until
		}
		return start.Before(end)
	}
}
