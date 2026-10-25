package domain

import "time"

type Session struct {
	UserID    string
	ExpiresAt time.Time
}
