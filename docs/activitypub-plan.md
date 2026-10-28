# ActivityPub Federation Plan

Status: PR 1–3 and PR 5 done, PR 4 deferred (to be tackled later)  
Decision records: [ADR-0002: ActivityPub federation with peer instances](adr/0002-federation.md),
[ADR-0003: Federation adapter split](adr/0003-federation-adapter-split.md),
[ADR-0004: Peer join and mirrored catalog](adr/0004-peer-join-and-mirror.md)  
User guide: [docs/users/federation.md](users/federation.md)

## Goal

Make the catalog federatable with peer biblio-oggetto instances over
ActivityPub. Remote actors can discover the instance, follow its object
announcements, and (later) request reservations through the inbox.

## Phases

### PR 1 — ADR + discovery slice (done)

- ADR-0002 records the federation decisions.
- `BASE_URL` env config, validated fail-fast in `app.New`.
- RSA signing key generated on first use, persisted in the `instance_keys`
  SQLite table (single row, id = 1).
- `internal/usecase/Identity`: ensures the instance key exists and returns the
  instance actor (id, name, inbox/outbox URLs, public key PEM).
- `GET /.well-known/webfinger` → `acct:catalog@<host>` resolves to the actor.
- `GET /ap/actors/{handle}` → ActivityStreams `Service` actor with `publicKey`,
  served as `application/activity+json`.

### PR 2 — Outbound publishing (done)

- SQLite `outbox` table; `usecase.Objects` embeds `Create`/`Update`/`Delete`
  activity entries in the same SQLite transaction as the object write
  (`ObjectRepository` methods now take a `domain.OutboxEntry`).
- `GET /ap/outbox` (OrderedCollection, `limit`/`offset`, default 50) and
  `GET /ap/outbox/{id}`; `GET /ap/objects/{id}` serves the live `Article`.
- `followers` table; `POST /ap/inbox` handles `Follow`/`Undo` and queues
  `Accept`.
- Delivery: `activitypub.Deliverer` goroutine started in `internal/app`
  (`FederationPoll`, default 5 s), POSTs signed by `github.com/go-fed/httpsig`
  (rsa-sha256 over `(request-target) host date digest`), exponential backoff
  30 s → 30 min (`usecase.RetryDelay`). `GET /ap/outbox` responses themselves
  are unsigned: peers fetch them publicly; signing applies to outbound
  requests only.

### PR 3 — Inbox security (done)

- `activitypub.SignatureVerifier` validates draft-cavage signatures on
  `POST /ap/inbox`: rsa-sha256, signature must cover
  `(request-target) host date digest`, Date skew ≤ 12 h, `Digest` re-checked
  against the body; the remote public key is resolved from the key id
  document (actor or standalone key) and cached in `remote_keys`. Cache hits
  are force-refetched once on verification failure (key rotation).
- Signer must equal the activity `actor` field; verification failure is
  `unauthorized` (401), unparseable bodies `invalid_input` (400).
- Verified payloads are queued in `inbox_events` (activity id = primary key;
  duplicates acknowledged and dropped) and drained asynchronously by
  `activitypub.InboxProcessor` on the same poll interval. Follow/Undo reuse the
  Federation use case, unknown types are dropped after processing, failures
  retry with `usecase.RetryDelay`.

### PR 4 — Reservation over federation (deferred)

- Remote `Create`/`Propose` note mapped onto `domain.Reservation` plus the
  existing admin approval flow; `Accept`/`Reject` sent to the requester inbox.
- E2E: two in-process instances, follow + reserve round-trip.

Implementation notes for when PR 4 is picked up:

- The inbox pipeline already parses `type`/`actor`/`object` into
  `inbox_events` columns (`decodeInboxEvent`); `Federation.ProcessPendingInbox`
  currently drops non-Follow/Undo types — add the reservation branch there.
- The reservation object must be dereferenced: a `Create`/`Propose` carries
  an object whose id is a local `Article` (`/ap/objects/{id}`); map the id
  back to the catalog object.
- Reservations arrive from a non-follower actor: the `Accept`/`Reject` reply
  must address the requester's inbox specifically, not the follower broadcast
  fan-out. Options: a per-entry `recipient` column on `outbox` (deliverer
  already reads followers per entry; branch when set) or a direct signed POST
  reusing the Deliverer's signer plumbing.
- Pending reservations need admin visibility in `web/` (existing approval
  flow covers confirm/cancel; show the remote requester).
- Trust boundary: PR 4 should re-validate the signer against the actor the
  `Reserve` targets and rate-limit pending reservations per remote actor to
  avoid queue spam.

### PR 5 — Peer join, mirroring and admin panel (done)

- `peers` and `remote_objects` SQLite tables with their repositories;
  `AddPeer` resolves a handle (`catalog@host`) or actor URL (WebFinger for
  handles), stores the peer as `pending` and queues a `follow` outbox entry.
  The peer's `Accept` for that follow activity moves it to `accepted`;
  joining again refreshes the follow and resets the state to `pending`.
- `Follow`/`Undo` ride the existing outbox and `Deliverer`: they are posted
  to the target peer's inbox only (never to the follower fan-out) and use
  the same backoff as object announcements.
- `ProcessPendingInbox` handles `Accept`, and for peers in state `accepted`
  mirrors `Create`/`Update`/`Delete` into `remote_objects`. Ingest is gated
  on the peers table, so a signed-but-unknown actor — or one that merely
  follows this instance — cannot inject content. Article ids from a foreign
  origin are dropped, and a delete for an id that was never mirrored is
  dropped instead of retried.
- `GET /api/objects` reads local objects and mirrored peer objects from one
  SQL `UNION ALL` query; every row carries a `source` marker (the peer
  actor id for mirrored rows).
- Admin API: `GET /api/admin/federation` (overview: actor, peers,
  followers, outbox), `POST /api/admin/federation/peers`, `DELETE
  /api/admin/federation/peers/{id}`.
- Admin UI: a federation panel (this instance's handle, join form, peers
  with their state, followers, outbox with delivery state) and a
  `via <host>` badge on peer cards, whose name links to the peer's
  canonical article URL.
- Decision record: [ADR-0004](adr/0004-peer-join-and-mirror.md).

## Testing

Implemented:

- Unit tests: `Identity` (key generation/persistence/reuse), `Federation`
  (follow, undo, enqueue validation, dedup, processing, backoff), ActivityPub
  serialization, SQLite repos (outbox, followers, inbox events, key cache,
  peers, remote objects).
- Peer unit tests: actor resolution by handle and URL, join and leave, the
  `pending` → `accepted` handshake, and article ingest (defensive parsing,
  foreign-origin ids, unknown-id deletes, upsert idempotence).
- E2E (`e2e/activitypub_test.go`): WebFinger, actor document, outbox
  announcements, signed follow → accept → delivery round-trip between two
  in-process instances, unsigned/tampered/mismatched-signer rejection,
  activity-id dedup.
- E2E (`e2e/federation_peers_test.go`, `e2e/federation_admin_test.go`): the
  two-instance handshake, mirrored announcements appearing in the public
  list, the join gate that keeps unjoined actors out, the admin overview
  and join/remove endpoints.
- All four AGENTS.md verification commands pass as of this stopping point.

Outstanding (PR 4):

- Unit: reservation request parsing/validation; conflict with busy/loaned
  objects.
- E2E: follow + reserve round-trip, Accept/Reject back to requester.

## Current state at this stop

- PR 1–3 and PR 5 complete and race-tested, committed on the `activityPub`
  branch.
- PR 4 (remote reservations) stays deferred; the implementation notes above
  still apply.
- Known staged risk: inbox processing trusts the verified signer/actor pair;
  no rate limiting yet (see PR 4 notes). Peer ingest adds a second gate —
  only joined, accepted peers may write into the catalog.
