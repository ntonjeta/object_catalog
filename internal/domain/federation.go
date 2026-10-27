package domain

import "time"

// OutboxKind classifies a pending federated activity.
type OutboxKind string

const (
	OutboxCreate OutboxKind = "create"
	OutboxUpdate OutboxKind = "update"
	OutboxDelete OutboxKind = "delete"
	OutboxAccept OutboxKind = "accept"
)

// OutboxEntry is one entry of the federation outbox. Object holds the
// snapshot at activity time for create/update/delete kinds; Target and
// FollowID describe the remote Follow for the accept kind.
type OutboxEntry struct {
	ID            string
	Kind          OutboxKind
	Object        Object
	Target        string
	FollowID      string
	Published     time.Time
	DeliveredAt   *time.Time
	Attempts      int
	NextAttemptAt time.Time
}

// Follower is a remote actor that follows the instance actor.
type Follower struct {
	ActorID   string
	InboxURL  string
	CreatedAt time.Time
}

// RemoteActor is the minimal view of a remote ActivityPub actor document.
type RemoteActor struct {
	ID       string
	InboxURL string
}
