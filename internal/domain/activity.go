package domain

import "time"

type ActivityType string

const (
	ActivityObjectBusy           ActivityType = "object_busy"
	ActivityObjectReleased       ActivityType = "object_released"
	ActivityReservationMade      ActivityType = "reservation_made"
	ActivityReservationConfirmed ActivityType = "reservation_confirmed"
	ActivityReservationCancelled ActivityType = "reservation_cancelled"
	ActivityReservationLapsed    ActivityType = "reservation_lapsed"
)

// Activity is an append-only audit record of a state change. ActorID is the
// authenticated user who triggered it; an empty actor means the system.
type Activity struct {
	ID            string
	Type          ActivityType
	ObjectID      string
	ReservationID string
	ActorID       string
	Detail        string
	CreatedAt     time.Time
}
