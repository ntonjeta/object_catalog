# ActivityPub Federation Plan

Status: in progress  
Decision record: [ADR-0002: ActivityPub federation with peer instances](adr/0002-federation.md)

## Goal

Make the catalog federatable with peer biblio-oggetto instances over
ActivityPub. Remote actors can discover the instance, follow its object
announcements, and (later) request reservations through the inbox.

## Phases

### PR 1 — ADR + discovery slice (this change)

- ADR-0002 records the federation decisions.
- `BASE_URL` env config, validated fail-fast in `app.New`.
- RSA signing key generated on first use, persisted in the `instance_keys`
  SQLite table (single row, id = 1).
- `internal/usecase/Identity`: ensures the instance key exists and returns the
  instance actor (id, name, inbox/outbox URLs, public key PEM).
- `GET /.well-known/webfinger` → `acct:catalog@<host>` resolves to the actor.
- `GET /ap/actors/{handle}` → ActivityStreams `Service` actor with `publicKey`,
  served as `application/activity+json`.

### PR 2 — Outbound publishing

- SQLite `outbox` table; `usecase.Objects` enqueues `Create`/`Update`/`Delete`
  activities transactionally (new port in `usecase/ports.go`).
- `GET /ap/outbox` and `GET /ap/outbox/{id}` (signed by the instance key).
- `ap_followers` table; `Follow` handled on the inbox read path with `Accept`.
- Delivery worker: goroutine started in `internal/app`, signed `POST` to
  follower inboxes, retries with backoff.

### PR 3 — Inbox security

- `POST /ap/inbox`: HTTP signature verification (`github.com/go-fed/httpsig`)
  against fetched remote keys, activity-ID dedupe table, async queue,
  unknown activity types answered 202 and dropped.

### PR 4 — Reservation over federation

- Remote `Create`/`Propose` note mapped onto `domain.Reservation` plus the
  existing admin approval flow; `Accept`/`Reject` sent to the requester inbox.
- E2E: two in-process instances, follow + reserve round-trip.

## Testing

- Unit tests for `Identity` (key generation, persistence, reuse) with fakes in
  `internal/usecase/fakes_test.go`.
- E2E tests for WebFinger and the actor document in `e2e/activitypub_test.go`.
- All four verification commands from AGENTS.md must pass.
