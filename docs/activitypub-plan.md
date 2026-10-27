# ActivityPub Federation Plan

Status: PR 1–3 done, PR 4 deferred (to be tackled later)  
Decision records: [ADR-0002: ActivityPub federation with peer instances](adr/0002-federation.md),
[ADR-0003: Federation adapter split](adr/0003-federation-adapter-split.md)  
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

## Testing

Implemented:

- Unit tests: `Identity` (key generation/persistence/reuse), `Federation`
  (follow, undo, enqueue validation, dedup, processing, backoff), ActivityPub
  serialization, SQLite repos (outbox, followers, inbox events, key cache).
- E2E (`e2e/activitypub_test.go`): WebFinger, actor document, outbox
  announcements, signed follow → accept → delivery round-trip between two
  in-process instances, unsigned/tampered/mismatched-signer rejection,
  activity-id dedup.
- All four AGENTS.md verification commands pass as of this stopping point.

Outstanding (PR 4):

- Unit: reservation request parsing/validation; conflict with busy/loaned
  objects.
- E2E: follow + reserve round-trip, Accept/Reject back to requester.

## Current state at this stop

- PR 1–3 complete and race-tested; changes uncommitted in the
  `activityPub` worktree (`feat(federation)` commits pending).
- Known staged risk: inbox processing trusts the verified signer/actor pair;
  no rate limiting yet (see PR 4 notes).
