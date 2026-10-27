package domain

import "time"

// OutboxKind classifies a pending federated activity.
type OutboxKind string

const (
	OutboxCreate OutboxKind = "create"
	OutboxUpdate OutboxKind = "update"
	OutboxDelete OutboxKind = "delete"
	OutboxAccept OutboxKind = "accept"
	OutboxFollow OutboxKind = "follow"
	OutboxUndo   OutboxKind = "undo"
)

// PeerState tracks the state of an outbound follow to a peer instance.
type PeerState string

const (
	PeerPending  PeerState = "pending"
	PeerAccepted PeerState = "accepted"
)

// Peer is a remote catalog instance this instance follows.
type Peer struct {
	ID               string
	ActorID          string
	InboxURL         string
	FollowActivityID string
	State            PeerState
	CreatedAt        time.Time
}

// RemoteObject is an object mirrored from a peer instance.
type RemoteObject struct {
	ID           string
	PeerActorID  string
	Name         string
	Description  string
	ImageURL     string
	CategoryName string
	Busy         bool
	BusyUntil    *time.Time
	Tags         []string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// OutboxEntry is one entry of the federation outbox. Object holds the
// snapshot at activity time for create/update/delete kinds; Target and
// FollowID describe the remote Follow for the accept kind. Target is the
// peer actor id for follow and undo kinds.
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

// InboxEvent is a signature-verified incoming activity awaiting processing.
// The raw payload is kept for audit; the parsed fields drive processing.
type InboxEvent struct {
	ID            string
	Sender        string
	Type          string
	Actor         string
	ObjectID      string
	Payload       string
	ReceivedAt    time.Time
	ProcessedAt   *time.Time
	Attempts      int
	NextAttemptAt time.Time
}
