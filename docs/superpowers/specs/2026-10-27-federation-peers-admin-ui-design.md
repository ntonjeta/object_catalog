# Federation Peers + Admin UI — Design

Date: 2026-10-27
Status: approved by owner in conversation
Related: [ADR-0002](../../adr/0002-federation.md), [ADR-0003](../../adr/0003-federation-adapter-split.md), [federation plan](../../activitypub-plan.md), [user guide](../../users/federation.md)

## Intent and why

The owner runs two biblio-oggetto instances and wants them to communicate
with each other, driven from the admin interface. Today federation is
publish-only: a remote account can follow `catalog@host`, and there is no
way for an administrator to make this instance follow anyone, and no
interface at all for federation state. This design adds an admin
federation panel (join peers, see peers/followers/outbox) and the
outbound-follow backend that makes two instances exchange object
announcements.

## Goals

- An admin can join a peer instance by handle (`catalog@other-host`) or
  actor URL; this instance sends a signed `Follow`, tracks state
  (`pending` → `accepted`), and can leave again (sends `Undo`).
- Announcements (`Create`/`Update`/`Delete`) published by a peer we follow
  are stored as remote objects and appear in the public catalog list.
- Peer objects show inline in the object list with a source badge; the
  object card links to the peer instance (canonical article URL).
- Admin federation panel shows: our actor handle, join form, peers with
  state, our followers, and outbox entries with delivery state (pending,
  delivered, attempts, next retry).
- Two in-process instances can be tested end-to-end: join, announce,
  update, delete, unfollow.

## Non-goals

- Reservation-over-federation (PR 4 in the activitypub plan).
- Likes, boosts, comments, reposts; approval queues; block lists; rate
  limiting.
- Editing or deleting peer content from this instance.
- Human-readable object pages on the peer side (the article URL serves
  ActivityPub JSON; that is the jump target).
- Any change to public discovery/webfinger behavior.

## Anti-goals

- A peer actor that has not been joined cannot inject content into our
  catalog, even with a valid signature.
- The panel must not become a moderation tool; it is a testing/status UI.

## Constraints

- Pure Go, no CGO, no new third-party dependencies (`go-fed/httpsig`
  stays the only federation lib).
- Frontend stays vanilla HTML/CSS/JS embedded via `web/embed.go`; no
  build step.
- Strict hexagonal layout: `usecase` declares ports, `http` adapter
  depends on use cases only, wiring in `internal/app/app.go`.
- API error envelope stays `{"error":{"code","message"}}` with the five
  existing codes.
- Unit tests next to packages with fakes in
  `internal/usecase/fakes_test.go`; API contracts proven in `e2e/`.

## Approaches considered

**A. Queue-native (chosen).** Outbound follow/undo are outbox entries
(new kinds) delivered by the existing `Deliverer` goroutine with the
existing backoff; inbound peer activities extend `ProcessPendingInbox`;
remote objects merge into the list with a SQL `UNION ALL`. Reuses the
retry machinery the project already trusts; peer downtime between restarts
self-heals. Cost: more domain churn (new table, kinds, use-case
methods).

**B. Direct call.** The join endpoint signs and POSTs the `Follow`
synchronously; failures surface in the UI for manual retry. Less code;
delivery stops until an admin clicks again — poor fit for a federation
test tool where both servers restart often. Rejected.

**C. Read-only panel.** Rejected: does not satisfy "instances communicate
each other".

## Decided shape

### Domain

- New `OutboxKind`s: `follow`, `undo`. `Follow` activity object is the
  peer actor id; `Undo` likewise (matches what our own inbox accepts).
- `Peer{ID, ActorID, InboxURL, FollowActivityID, State, CreatedAt}` with
  states `pending`/`accepted`. `FollowActivityID` is the URL
  `<base>/ap/outbox/<outbox-entry-id>` that the peer's `Accept` will
  reference.
- `RemoteObject{ID, PeerActorID, Name, Description, ImageURL, Busy,
  BusyUntil, CategoryName, Tags, CreatedAt, UpdatedAt}`. `ID` is the
  peer's canonical article URL (`<peer-base>/ap/objects/<id>`), which is
  also the jump target from the card.
- `domain.Object` gains `Source string`: empty for local rows, the peer
  actor id for mirrored rows.

### Join / leave

- `Federation.AddPeer(target)`: trim; if `acct:handle@host` (or
  `handle@host`) resolve via WebFinger on the host (HTTPS; plain HTTP only
  for `localhost`/`127.x` hosts), if an http(s) URL use it directly; fetch
  the actor document; refuse to follow our own actor id; upsert the peer
  as `pending`; append a `follow` outbox entry.
- Join error mapping (owner-confirmed): resolve happens synchronously
  before any row is stored — joining an offline peer fails outright and
  stores nothing. Junk syntax, self-follow, non-http schemes →
  `invalid_input` (400). Everything that means "could not reach/validate
  the actor" (WebFinger or actor fetch error, non-200, unknown handle,
  actor-document id mismatch, missing inbox) → `not_found` (404) with a
  message naming the target. The 10 s HTTP client timeout already bounds
  the wait. Peer downtime *after* a successful join stays handled by the
  queue backoff.
- `Federation.RemovePeer(id)`: append an `undo` outbox entry to the peer's
  actor id, delete the peer row, cascade-delete its remote objects.
- `Deliverer`: `follow`/`undo` entries are posted only to `entry.Target`
  (inbox freshly resolved via `RemoteActorRepository.FetchActor`), not
  fanned out to followers.

### Ingest (ProcessPendingInbox additions)

- `Accept` whose object id equals a pending peer's `FollowActivityID` and
  whose actor matches → peer becomes `accepted`.
- `Create`/`Update`/`Delete` from an actor in state `accepted` → parse the
  article from the stored payload and upsert/delete `RemoteObject`.
  Activities from anyone else keep being dropped after processing.
- Article parsing is defensive: missing/blank fields become empty, the
  name is truncated to 200 runes, description to 5000, tags normalized
  through `domain.NormalizeTags` (cap 20), image kept only if absolute
  http(s), availability attachment (`available` / `on loan [until <t>]`)
  mapped to `Busy`/`BusyUntil`, category attachment name to
  `CategoryName`. An article without a valid id or name is dropped
  silently. The article id must share the scheme+host origin of the
  announcing peer's actor id (owner-confirmed: peers are biblio-oggetto
  instances publishing `<peer-base>/ap/objects/<id>`; a foreign-origin id
  would make the card's jump target phishing-capable); ids from any other
  origin are dropped. Parsing never fails the whole inbox batch.

### Public list merge

- `ObjectRepository.List` becomes a `UNION ALL` over `objects` and
  `remote_objects` in one query: search (`q`) matches name/description/
  category-name/tags on both sides; `tag` matches both tag tables;
  `category` filter applies to local objects only (`source = ''` guard);
  ordering `created_at DESC, id DESC`; `total` counts both sides.
- List rows for remote objects carry `Category = &Category{Name: ...}`
  with empty id; `GET /api/objects/{id}` stays local-only (remote ids
  contain slashes and never reach it from the UI).
- `objectResponse` gains `"source"` (string, `""` for local).

### Admin HTTP API (all `requireAuth`)

- `GET /api/admin/federation` → `{actor:{id, handle}, peers:[{id,
  actor_id, state, created_at}], followers:[{actor_id, created_at}],
  outbox:[{id, kind, target, published_at, delivered_at, attempts,
  next_attempt_at}], outbox_total}` (outbox newest first, limit 50).
- `POST /api/admin/federation/peers` body `{"target": "..."}` → 201 peer
  object; bad/self/relative targets → 400 `invalid_input`.
- `DELETE /api/admin/federation/peers/{id}` → 204; unknown id → 404.

### Web UI

- New `federation-panel` section on the page, `data-admin-only hidden`
  like the reservations panel. Sections in order: this instance's handle
  (copyable text), join form (input + Join button + inline error), peers
  list (actor id, state badge, Remove button), followers list, outbox
  table (kind, target, state, attempts). Refresh button reloads.
- Object cards: when `source` is non-empty, the name is a plain link to
  the peer article URL (new tab, `rel="noopener"`) and a `peer badge`
  shows the source host; admin action buttons are not rendered for peer
  rows. All peer-supplied strings render via `textContent` only.

### Inbound follows

Unchanged: any correctly signed `Follow` from anyone is auto-accepted;
`Undo` removes. The peers feature does not alter follower semantics.

## Left to the builder

- Exact SQL text and column names beyond the listed fields.
- Fake names in `fakes_test.go`; e2e helper additions.
- CSS class naming consistent with existing panels.
- Small log lines for undeliverable follow entries.
