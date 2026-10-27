# ActivityPub Federation Plan

Status: in progress  
Decision record: [ADR-0002: ActivityPub federation with peer instances](adr/0002-federation.md)

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
- `followers` table; `POST /ap/inbox` resolves the remote actor document,
  handles `Follow`/`Undo` and queues `Accept` (verification comes in PR 3).
- Delivery: `httpapi.Deliverer` goroutine started in `internal/app`
  (`FederationPoll`, default 5 s), POSTs signed by `github.com/go-fed/httpsig`
  (rsa-sha256 over `(request-target) host date digest`), exponential backoff
  30 s → 30 min. `GET /ap/outbox` responses themselves are unsigned: peers
  fetch them publicly; signing applies to outbound requests only.

### PR 3 — Inbox security (done)

- `httpapi.SignatureVerifier` validates draft-cavage signatures on
  `POST /ap/inbox`: rsa-sha256, signature must cover
  `(request-target) host date digest`, Date skew ≤ 12 h, `Digest` re-checked
  against the body; the remote public key is resolved from the key id
  document (actor or standalone key) and cached in `remote_keys`. Cache hits
  are force-refetched once on verification failure (key rotation).
- Signer must equal the activity `actor` field; verification failure is
  `unauthorized` (401), unparseable bodies `invalid_input` (400).
- Verified payloads are queued in `inbox_events` (activity id = primary key;
  duplicates acknowledged and dropped) and drained asynchronously by
  `httpapi.InboxProcessor` on the same poll interval. Follow/Undo reuse the
  Federation use case, unknown types are dropped after processing, failures
  retry with `usecase.RetryDelay`.

### PR 4 — Reservation over federation

- Remote `Create`/`Propose` note mapped onto `domain.Reservation` plus the
  existing admin approval flow; `Accept`/`Reject` sent to the requester inbox.
- E2E: two in-process instances, follow + reserve round-trip.

## Testing

- Unit tests for `Identity` (key generation, persistence, reuse) with fakes in
  `internal/usecase/fakes_test.go`.
- E2E tests for WebFinger and the actor document in `e2e/activitypub_test.go`.
- All four verification commands from AGENTS.md must pass.
