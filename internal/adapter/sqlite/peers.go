package sqlite

import (
	"context"
	"database/sql"

	"biblio-oggetto/internal/domain"
	"biblio-oggetto/internal/usecase"
)

type PeerRepo struct {
	db *sql.DB
}

func NewPeerRepo(db *sql.DB) *PeerRepo {
	return &PeerRepo{db: db}
}

// Upsert stores a peer by actor id. A re-join refreshes the inbox URL, the
// follow activity id and the state (back to pending) but keeps the original
// created_at.
func (r *PeerRepo) Upsert(ctx context.Context, peer domain.Peer) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO peers (id, actor_id, inbox_url, follow_activity_id, state, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(actor_id) DO UPDATE SET
			 inbox_url = excluded.inbox_url,
			 follow_activity_id = excluded.follow_activity_id,
			 state = excluded.state`,
		peer.ID, peer.ActorID, peer.InboxURL, peer.FollowActivityID,
		string(peer.State), formatTime(peer.CreatedAt),
	)
	return mapError(err)
}

// MarkAccepted flips the peer with the given actor id to the accepted state.
func (r *PeerRepo) MarkAccepted(ctx context.Context, actorID string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE peers SET state = ? WHERE actor_id = ?`, string(domain.PeerAccepted), actorID,
	)
	return checkUpdate(result, err, "peer accept mark")
}

func (r *PeerRepo) ByFollowActivityID(ctx context.Context, followActivityID string) (domain.Peer, error) {
	row := r.db.QueryRowContext(ctx, peerSelect+` WHERE follow_activity_id = ?`, followActivityID)
	return scanPeer(row)
}

func (r *PeerRepo) ByID(ctx context.Context, id string) (domain.Peer, error) {
	row := r.db.QueryRowContext(ctx, peerSelect+` WHERE id = ?`, id)
	return scanPeer(row)
}

func (r *PeerRepo) Remove(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM peers WHERE id = ?`, id)
	return checkUpdate(result, err, "peer removal")
}

// List returns peers oldest first, matching the follower list order.
func (r *PeerRepo) List(ctx context.Context) ([]domain.Peer, error) {
	rows, err := r.db.QueryContext(ctx, peerSelect+` ORDER BY created_at, id`)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()

	peers := make([]domain.Peer, 0)
	for rows.Next() {
		peer, err := scanPeer(rows)
		if err != nil {
			return nil, err
		}
		peers = append(peers, peer)
	}
	if err := rows.Err(); err != nil {
		return nil, mapError(err)
	}
	return peers, nil
}

const peerSelect = `SELECT id, actor_id, inbox_url, follow_activity_id, state, created_at FROM peers`

func scanPeer(row scanner) (domain.Peer, error) {
	var (
		peer      domain.Peer
		state     string
		createdAt string
	)
	if err := row.Scan(
		&peer.ID, &peer.ActorID, &peer.InboxURL, &peer.FollowActivityID, &state, &createdAt,
	); err != nil {
		return domain.Peer{}, mapError(err)
	}
	peer.State = domain.PeerState(state)
	var err error
	if peer.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Peer{}, err
	}
	return peer, nil
}

var _ usecase.PeerRepository = (*PeerRepo)(nil)
