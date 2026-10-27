# Federation Peers + Admin UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let an admin join peer ActivityPub instances from the UI, mirror peer announcements into the public catalog with a source badge, and debug federation (peers, followers, outbox) from an admin panel.

**Architecture:** Queue-native federation. Outbound `Follow`/`Undo` travel through the existing outbox/`Deliverer` with its backoff; `ProcessPendingInbox` gains branches for `Accept` and peer `Create`/`Update`/`Delete` into a `remote_objects` table; the public list merges local and remote rows with `UNION ALL`. Admin endpoints ride the existing `requireAuth` catalog API; the UI is one new vanilla-JS panel section.

**Tech Stack:** Go 1.24, `modernc.org/sqlite`, `github.com/go-fed/httpsig` (already present), vanilla HTML/CSS/JS embedded via `web/embed.go`.

**Spec:** `docs/superpowers/specs/2026-10-27-federation-peers-admin-ui-design.md`

## Global Constraints

- Pure Go — no CGO, no new third-party dependencies.
- No frontend build step, no framework, no CDN; edit `web/index.html`, `web/static/app.js`, `web/static/styles.css` only.
- Hexagonal: `internal/usecase` declares its own ports in `ports.go`; `internal/adapter/http` depends on use cases, never repositories; wiring in `internal/app/app.go`.
- API errors: `{"error":{"code","message"}}`, codes `invalid_input`, `unauthorized`, `not_found`, `conflict`, `internal`.
- Verify after every task: `gofmt -l . && go vet ./... && go test ./... && go test -race ./...` (all four; `gofmt -l .` prints nothing).
- Conventional Commits: `feat(scope): summary`, why in body.
- Update `docs/spec.md` when behavior/API changes.

## Review Focus

1. **Article JSON from a peer is malformed or hostile** (missing fields, wrong types, 10 MB name, article id pointing at a foreign origin) — `ProcessPendingInbox` must mark the event processed and keep draining, never error the batch; foreign-origin ids are dropped. Pinned in Task 5 tests `TestIngestArticleDefensiveParsing` / `TestIngestRejectsForeignOriginID` / `TestIngestBadArticleDoesNotFailBatch`.
2. **Duplicate delivery of the same activity id** (retry re-sends) — storing a peer article twice must upsert one row, not duplicate list entries. Pinned in Task 5 `TestIngestUpsertIdempotent` and Task 2 `TestRemoteObjectRepoPutUpserts`.
3. **Un-joined actor sends a validly-signed `Create`** — content must NOT appear in the catalog. Pinned in Task 5 `TestIngestIgnoresNonPeer` and Task 9 `TestPeerContentRequiresJoin`.
4. **Joining self or junk targets** — `AddPeer` rejects own actor id, non-URLs, non-http schemes with `domain.ErrInvalid`, and stores nothing. Pinned in Task 4 `TestAddPeerValidatesTarget`.
5. **Peer-supplied strings injected into the DOM** — card text/badge must render via `textContent`/`el()`, never `innerHTML`; links get `rel="noopener"`. Pinned in Task 10 by code inspection step (no JS test harness in repo).

---

### Task 1: Domain types + follow/undo serialization

**Files:**
- Modify: `internal/domain/federation.go`
- Modify: `internal/adapter/activitypub/serialize.go`
- Test: `internal/adapter/activitypub/serialize_test.go`

**Interfaces:**
- Produces: `domain.OutboxFollow`/`domain.OutboxUndo` (`OutboxKind = "follow"/"undo"`); `domain.PeerState`, `domain.Peer{ID, ActorID, InboxURL, FollowActivityID string; State PeerState; CreatedAt time.Time}`, constants `domain.PeerPending = "pending"`, `domain.PeerAccepted = "accepted"`; `domain.RemoteObject{ID, PeerActorID, Name, Description, ImageURL, CategoryName string; Busy bool; BusyUntil *time.Time; Tags []string; CreatedAt, UpdatedAt time.Time}`; `domain.Object` gains `Source string` (zero-value "" = local).
- `marshalActivity` renders `follow`/`undo` entries as `{"type":"Follow"|"Undo","actor":<our actor>,"object":"<entry.Target>"}` (object = bare string id).

- [ ] **Step 1: Write failing serialization tests**

In `serialize_test.go` add `TestMarshalActivityFollow` and `TestMarshalActivityUndo`: build `domain.OutboxEntry{ID:"e1", Kind: domain.OutboxFollow, Target:"https://peer.example/ap/actors/catalog", Published: fixedTime}`, call `marshalActivity(entry, "https://us.example/ap/actors/catalog", "https://us.example")`, decode into a map, assert `type == "Follow"`, `actor == "https://us.example/ap/actors/catalog"`, `object == entry.Target` (string), `id == "https://us.example/ap/outbox/e1"`. Undo case asserts `type == "Undo"` identically.

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/adapter/activitypub -run 'TestMarshalActivityFollow|TestMarshalActivityUndo' -v`
Expected: compile error (kinds missing), then shape mismatch.

- [ ] **Step 3: Add domain types + marshalActivity branches**

Add to `internal/domain/federation.go` the `Peer`, `PeerState`, `RemoteObject` types and kind constants exactly as in Interfaces; add `Source string` field to `domain.Object` with comment "peer actor id for mirrored remote objects; empty for local". Add the two switch branches in `marshalActivity` (set `activity.Type` and `activity.Object = entry.Target`).

- [ ] **Step 4: Run tests, verify pass; full suite**

Run: `go test ./internal/adapter/activitypub -v && gofmt -l . && go vet ./... && go test ./...`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/federation.go internal/domain/object.go internal/adapter/activitypub/serialize.go internal/adapter/activitypub/serialize_test.go
git commit -m "feat(federation): model peers and outbound follow/undo activities

Outbound follows ride the existing outbox queue; the deliverer needs
follow/undo shapes to post them to a single target inbox."
```

---

### Task 2: SQLite tables + PeerRepo + RemoteObjectRepo + ports

**Files:**
- Modify: `internal/adapter/sqlite/schema.go`
- Create: `internal/adapter/sqlite/peers.go`
- Create: `internal/adapter/sqlite/remote_objects.go`
- Modify: `internal/usecase/ports.go`
- Test: `internal/adapter/sqlite/db_test.go` (extend)

**Interfaces:**
- Consumes: Task 1 `domain.Peer`, `domain.PeerAccepted`, `domain.RemoteObject`.
- Produces ports (in `internal/usecase/ports.go`):

```go
type PeerRepository interface {
	Upsert(ctx context.Context, peer domain.Peer) error
	MarkAccepted(ctx context.Context, actorID string) error
	ByFollowActivityID(ctx context.Context, followActivityID string) (domain.Peer, error)
	ByID(ctx context.Context, id string) (domain.Peer, error)
	Remove(ctx context.Context, id string) error
	List(ctx context.Context) ([]domain.Peer, error)
}

type RemoteObjectRepository interface {
	Put(ctx context.Context, object domain.RemoteObject) error
	Delete(ctx context.Context, id string) error
	DeleteByPeer(ctx context.Context, peerActorID string) error
}
```

- Produces adapters: `sqlite.NewPeerRepo(db *sql.DB) *PeerRepo`, `sqlite.NewRemoteObjectRepo(db *sql.DB) *RemoteObjectRepo`.
- `List` sorts peers/followers oldest first; peer upsert on conflict(actor_id) refreshes `inbox_url`, `follow_activity_id`, `state` (re-join resets to pending) but keeps original `created_at`.

- [ ] **Step 1: Add the two interfaces to `ports.go`** (signatures above)

- [ ] **Step 2: Write failing repo tests** in `db_test.go`

Helpers `openDB(t)` already exist there — reuse. Tests:
- `TestPeerRepoUpsertAndStates`: insert pending peer; `MarkAccepted` flips to accepted; re-`Upsert` pending resets state to `pending` and refreshes `follow_activity_id`; `ByFollowActivityID("https://us/ap/outbox/x")` returns it; unknown id/activity → `domain.ErrNotFound`; `ByID`; `Remove` deletes and `ErrNotFound` after; `List` returns newest-joined-last by `created_at`.
- `TestRemoteObjectRepoPutUpserts`: `Put` same `ID` twice with different `Name` → one row, second name wins, tags replaced (`Put` also writes `Tags` into `remote_object_tags`, replacing).
- `TestRemoteObjectRepoDeleteByPeer`: two peers with objects; `DeleteByPeer(a)` removes only a's rows (use a raw `SELECT COUNT(*)` assert).

- [ ] **Step 3: Run tests, verify fail (missing tables/types)**

Run: `go test ./internal/adapter/sqlite -run 'TestPeerRepo|TestRemoteObjectRepo' -v`

- [ ] **Step 4: Implement schema + repos**

Append to `schema`:

```sql
CREATE TABLE IF NOT EXISTS peers (
	id TEXT PRIMARY KEY,
	actor_id TEXT NOT NULL UNIQUE,
	inbox_url TEXT NOT NULL,
	follow_activity_id TEXT NOT NULL UNIQUE,
	state TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS remote_objects (
	id TEXT PRIMARY KEY,
	peer_actor_id TEXT NOT NULL,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	image_url TEXT NOT NULL DEFAULT '',
	busy INTEGER NOT NULL DEFAULT 0,
	busy_until TEXT,
	category_name TEXT NOT NULL DEFAULT '',
	published_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS remote_objects_peer_actor_id_idx ON remote_objects(peer_actor_id);
CREATE TABLE IF NOT EXISTS remote_object_tags (
	object_id TEXT NOT NULL REFERENCES remote_objects(id) ON DELETE CASCADE,
	tag TEXT NOT NULL,
	PRIMARY KEY (object_id, tag)
);
CREATE INDEX IF NOT EXISTS remote_object_tags_tag_idx ON remote_object_tags(tag);
```

`peers.go`/`remote_objects.go` follow the existing repo style (`mapError`, `formatTime`/`parseTime`, `checkUpdate` for delete-row-exists, `replaceTags` pattern adapted to `remote_object_tags` in `Put`). `RemoteObject.Tags` loads via a second query in reads; `Put` is `INSERT ... ON CONFLICT(id) DO UPDATE SET` all columns (no read method needed here — Task 3's merged list reads rows directly).

- [ ] **Step 5: Run tests pass + compile check on interface conformance**

Add at bottom of each new file: `var _ usecase.PeerRepository = (*PeerRepo)(nil)` etc. Run: `go test ./internal/adapter/sqlite -v`

- [ ] **Step 6: Commit** — `feat(sqlite): peers and remote_objects tables with repositories` (body: mirror storage for joined peers).

---

### Task 3: Actor resolution (acct handle or URL) in activitypub adapter

**Files:**
- Modify: `internal/adapter/activitypub/fetch.go`
- Modify: `internal/usecase/ports.go`
- Modify: `internal/usecase/fakes_test.go` (add `Resolve` to `fakeRemoteActors`, delegating to its existing `FetchActor` fixture, so usecase tests keep compiling)
- Test: `internal/adapter/activitypub/fetch_test.go` (create)

**Interfaces:**
- Consumes: existing `RemoteActors.FetchActor`.
- Produces: `(*activitypub.RemoteActors).Resolve(ctx context.Context, input string) (domain.RemoteActor, error)` — port addition to `RemoteActorRepository`:

```go
type RemoteActorRepository interface {
	FetchActor(ctx context.Context, actorID string) (domain.RemoteActor, error)
	Resolve(ctx context.Context, input string) (domain.RemoteActor, error)
}
```

- Semantics: trim input; if it matches `[user@host]` or `acct:user@host` → GET WebFinger at `scheme://host/.well-known/webfinger?resource=acct:user@host` where `scheme = "http"` when host starts with `localhost` or `127.`, else `"https"`; take `links[rel=="self"].href`, then `FetchActor(href)`. Else if absolute http(s) URL → `FetchActor(input)`. Else → `fmt.Errorf("%w: target must be an actor URL or handle@host", domain.ErrInvalid)`. Empty self href or non-200 WebFinger → error wrapping `domain.ErrNotFound`.

- [ ] **Step 1: Add `Resolve` to the port + write failing tests**

`fetch_test.go` uses two `httptest.Server`s:
- `TestResolveActorURL`: server serves actor doc `{"id":..., "inbox":...}` at `/ap/actors/catalog`; `Resolve(ctx, ts.URL+"/ap/actors/catalog")` returns it.
- `TestResolveAcctHandleViaWebFinger`: one server acts as host (register its `httptest` host via patching: test calls `Resolve(ctx, "acct:catalog@"+strings.TrimPrefix(ts.URL,"http://"))` with the localhost-`http` rule). WebFinger handler asserts `resource` query and returns `{links:[{rel:"self",href:actorID}]}`; actor doc served on same server. Assert `RemoteActor.ID/InboxURL`.
- `TestResolveJunk`: `"not a url"`, `"mailto:..."` → `domain.ErrInvalid`.
- `TestResolveUnknownHandle`: WebFinger 404 → `domain.ErrNotFound`.

- [ ] **Step 2: Verify fail** — `go test ./internal/adapter/activitypub -run TestResolve -v`
- [ ] **Step 3: Implement `Resolve` + helper `webfingerSelfURL(ctx, hostAndUser)`** in `fetch.go` (reuse `fetchDocument` for the actor step; WebFinger fetch is a plain GET with `Accept: application/jrd+json`, `LimitReader maxBodyBytes`).
- [ ] **Step 4: Verify pass + full suite** — `gofmt -l . && go vet ./... && go test ./...`
- [ ] **Step 5: Commit** — `feat(federation): resolve peer actor by handle or URL` (body: admin join accepts catalog@host; localhost http rule for local testing).

---

### Task 4: Federation use case — AddPeer / RemovePeer / Peers

**Files:**
- Modify: `internal/usecase/federation.go`
- Modify: `internal/usecase/fakes_test.go`
- Test: `internal/usecase/federation_test.go`
- Modify: `internal/app/app.go` (wire new repos + constructor change)

**Interfaces:**
- Consumes: Task 2 `PeerRepository`/`RemoteObjectRepository` ports, Task 3 `Resolve`.
- Produces:

```go
func NewFederation(outbox OutboxRepository, followers FollowerRepository, actors RemoteActorRepository, inbox InboxEventRepository, identity *Identity, peers PeerRepository, remoteObjects RemoteObjectRepository) *Federation

func (f *Federation) AddPeer(ctx context.Context, target string) (domain.Peer, error)
func (f *Federation) RemovePeer(ctx context.Context, id string) error
func (f *Federation) Peers(ctx context.Context) ([]domain.Peer, error)
```

- `AddPeer`: trim; refuse `target` resolving to our own `identity.Actor().ID` (`%w: cannot follow this instance`, `domain.ErrInvalid`); `actors.Resolve`; wrap every non-`ErrInvalid` resolve failure (WebFinger/actor fetch errors, non-200, id mismatch, missing inbox) as `fmt.Errorf("%w: could not resolve peer %q", domain.ErrNotFound, target)` — nothing is stored when resolution fails (owner-confirmed: join requires a live target; post-join downtime is handled by queue retries); `id := domain.NewID()`; `followActivityID := identity.BaseURL() + "/ap/outbox/" + entryID` (same `id`); `peers.Upsert(domain.Peer{ID: id, ActorID: r.ID, InboxURL: r.InboxURL, FollowActivityID: followActivityID, State: domain.PeerPending, CreatedAt: now})`; `outbox.Append(domain.OutboxEntry{ID: id, Kind: domain.OutboxFollow, Target: r.ID, FollowID: followActivityID, Published: now})`; return stored peer.
- `RemovePeer`: `peers.ByID` (not found → propagate `ErrNotFound`); append `OutboxEntry{ID: domain.NewID(), Kind: domain.OutboxUndo, Target: peer.ActorID, FollowID: peer.FollowActivityID, Published: now}`; `peers.Remove(id)`; `remoteObjects.DeleteByPeer(peer.ActorID)`.
- `Peers`: pass-through `peers.List`.

- [ ] **Step 1: Extend fakes** in `fakes_test.go`: `fakePeers` (map by id + actorID, records calls), `fakeRemoteObjects` (slice, records deletes), extend `fakeRemoteActors` with `Resolve` delegating to its existing fixture (add `resolved domain.RemoteActor` + `err` fields). Update `newTestFederation` to the 7-arg `NewFederation` and fix all existing call sites/tests.

- [ ] **Step 2: Write failing tests** in `federation_test.go`:
- `TestAddPeerStoresPendingAndQueuesFollow`: fake actor `https://peer/ap/actors/catalog`; assert stored peer pending with `FollowActivityID == identity.BaseURL()+"/ap/outbox/"+peer.ID`, outbox has one entry kind `follow` target peer actor.
- `TestAddPeerValidatesTarget`: empty target, resolver returning `domain.ErrInvalid` propagates; resolver returning our own actor id → `domain.ErrInvalid` and no peer/outbox writes.
- `TestAddPeerUnresolvableTarget`: fake resolver error (plain network-style error) → `errors.Is(err, domain.ErrNotFound)` true, message names target, `fakePeers` empty and `fakeOutbox` got no entry.
- `TestAddPeerRejoinResetsPending`: second AddPeer same actor → still one peer, state pending, new follow entry appended.
- `TestRemovePeerQueuesUndoAndCascades`: remove by id → undo entry appended (target = actor id), peer gone, `DeleteByPeer` called with actor id; unknown id → `domain.ErrNotFound`, no entries.
- `TestPeersLists`.

- [ ] **Step 3: Verify fail** — `go test ./internal/usecase -run 'Peer' -v`
- [ ] **Step 4: Implement** in `federation.go` (struct gains `peers`, `remoteObjects` fields).
- [ ] **Step 5: Wire app.go**: `peerRepo := sqlite.NewPeerRepo(db)`, `remoteObjectRepo := sqlite.NewRemoteObjectRepo(db)`, pass into `usecase.NewFederation(...)`. Run full suite + race: `gofmt -l . && go vet ./... && go test ./... && go test -race ./...`
- [ ] **Step 6: Commit** — `feat(federation): join and leave peer instances via outbox queue` (body: follows ride Deliverer backoff so restarts self-heal).

---### Task 5: Inbox ingest — Accept tracking + peer article mirror

**Files:**
- Modify: `internal/usecase/federation.go` (`ProcessPendingInbox`)
- Create: `internal/usecase/peer_ingest.go`
- Test: `internal/usecase/federation_test.go`
- Modify: `internal/usecase/fakes_test.go`

**Interfaces:**
- Consumes: Task 4 fields; `domain.InboxEvent.Payload`; `peers.ByFollowActivityID`; `peers.MarkAccepted`.
- Produces: `func decodePeerArticle(payload []byte, peerActorID string) (domain.RemoteObject, bool)` (second value = ingested; false on unparseable/invalid article); `func (f *Federation) processInboxEvent(ctx context.Context, event domain.InboxEvent, actor InstanceActor, now time.Time) error` extracted from the current switch.

- [ ] **Step 1: Write failing tests** (fakes: `fakePeers` gains `ByFollowActivityID` lookup data + `MarkAccepted` record):
- `TestInboxAcceptMarksPeerAccepted`: pending peer with `FollowActivityID = "https://us/ap/outbox/f1"`; event `{type:"Accept", actor: peerActor, objectID: "https://us/ap/outbox/f1"}` → `MarkAccepted` called with peer actor id. Accept from unknown activity → dropped, no error, event processed.
- `TestIngestCreateStoresRemoteObject`: accepted peer; event payload = a biblio-oggetto `Create` activity with embedded Article (name, content, image https URL, attachment Availability `on loan until 2026-12-01T00:00:00Z`, Category `Books`, three `Tag` attachments `#dune`,`#movie`) → `Put` called with parsed `RemoteObject{ID: article id, Name, Description, ImageURL, Busy:true, BusyUntil: 2026-12-01, CategoryName:"Books", Tags:["dune","movie"], CreatedAt/UpdatedAt from published/updated}`.
- `TestIngestUpdateAndDelete`: Update replaces (`Put` again, idempotent — `TestIngestUpsertIdempotent` covered at repo level Task 2; here assert second Put uses same ID); Delete event with `ObjectID = article id` → `remoteObjects.Delete(id)`.
- `TestIngestIgnoresNonPeer`: Create payload from actor not in peers (or pending only) → no `Put`, event processed without error.
- `TestIngestArticleDefensiveParsing`: payload variants — `object` as bare string id, missing name, 500-rune name (truncate to 200), non-http image (dropped, `ImageURL==""`), availability garbage → still stores with `Busy:true, BusyUntil:nil` for `on loan`, `false/nil` for unparseable; `ok=false` only when article id missing/non-http(s) or name empty after truncate.
- `TestIngestRejectsForeignOriginID`: accepted peer actor `https://peer.example/ap/actors/catalog`, Create article id `https://evil.example/ap/objects/x` (and same-suffix-but-different-origin) → no `Put`, event still processed; article id `https://peer.example/ap/objects/ok` → stored. Origin = scheme+host of `peerActorID` compared case-insensitively.
- `TestIngestBadArticleDoesNotFailBatch`: enqueue two inbox events, first undecodable junk (unknown peer) then good Create → both marked processed.

- [ ] **Step 2: Verify fail** — `go test ./internal/usecase -run 'Accept|Ingest' -v`
- [ ] **Step 3: Implement** switch branches in `ProcessPendingInbox` (keep Follow/Undo):

```go
case event.Type == "Accept":            err = f.processAccept(ctx, event)
case event.Type == "Create", event.Type == "Update":
    if f.isAcceptedPeer(ctx, event.Actor) { err = f.ingestArticle(ctx, event) }
    // else: fall through to drop (err stays nil)
case event.Type == "Delete":
    if f.isAcceptedPeer(ctx, event.Actor) { err = f.remoteObjects.Delete(ctx, event.ObjectID) }
```

`processAccept`: `peers.ByFollowActivityID(event.ObjectID)`; found and `peer.ActorID == event.Actor` → `MarkAccepted`; `ErrNotFound` → nil (drop). `ingestArticle`: `decodePeerArticle(event.Payload, event.Actor)`; `ok` → `remoteObjects.Put`; `!ok` → nil (drop, log). `decodePeerArticle` parses `{"actor","type","object":{id,name,content,image,published,updated,url?,attachment:[{type,name,content}]}}`; refuses non-`article actor` mismatch (`object` present but `activity.actor != peerActorID` → drop); enforces the origin rule (article `id` must share scheme+host with `peerActorID`, else drop); enforces limits per spec (truncate name/description, `domain.NormalizeTags` on `#`-stripped Tag names, image only absolute http(s), availability parse: prefix `"on loan until "` → RFC3339 best-effort).

- [ ] **Step 4: Verify pass + full suite + race.**
- [ ] **Step 5: Commit** — `feat(federation): mirror accepted-peer announcements into remote objects` (body: ingest gated on peers table so signed-but-unknown actors cannot inject content).

---

### Task 6: Deliverer routes follow/undo entries to the target only

**Files:**
- Modify: `internal/adapter/activitypub/deliver.go`
- Test: `e2e/federation_peers_test.go` (create — behavior needs two real instances)

**Interfaces:**
- Consumes: `federation.PendingDeliveries`, `actors.FetchActor` (via `federation` — add to usecase if needed: `func (f *Federation) FetchActor(ctx, actorID string) (domain.RemoteActor, error)`).
- Produces: entries of kind `follow`/`undo` delivered ONLY to freshly-fetched `entry.Target` inbox; zero-follower fast path unchanged for those kinds (a follow with no followers must NOT be silently marked delivered — skip that branch for follow/undo).

- [ ] **Step 1: Write failing e2e** `TestInstanceFollowsInstanceEndToEnd`: two `newEnv(t)` targets A/B. A joins B via use case: `A.app.Federation.AddPeer(ctx, B.baseURL+"/ap/actors/catalog")`. Eventually: B's `Followers` lists A's actor id; A's peer flips to `accepted` (A's inbox got B's Accept); create an object on B (`B.login(); B.createObject(...)`); eventually A's `remote_objects` mirror via `A.app.Federation` — assert with a new `Federation.RemoteObjects(ctx)`? No: assertion here uses the HTTP list added in Task 7/8. Instead assert A's sqlite: open handle unavailable in e2e — assert via `A.app.Federation.Outbox` delivered + follower presence only, defer content assert to Task 9. Keep this test to: follow delivered, B has follower, A peer accepted.

- [ ] **Step 2: Verify fail** (Accept never arrives — follow broadcast to zero followers got marked delivered or never sent): `go test ./e2e -run TestInstanceFollowsInstanceEndToEnd -v`
- [ ] **Step 3: Implement** in `Deliverer.process`: before follower fan-out, `if entry.Kind == domain.OutboxFollow || entry.Kind == domain.OutboxUndo { target, err := d.federation.FetchActor(ctx, entry.Target); on err → d.fail; single post to target.InboxURL; mark/fail per result; continue }`. Add thin `Federation.FetchActor` pass-through to the actors port.
- [ ] **Step 4: Verify pass; full suite + race.**
- [ ] **Step 5: Commit** — `feat(federation): deliver follow and undo to their target inbox only` (body: broadcasts go to followers; follow/undo address one peer).

---

### Task 7: Merge remote objects into the public list

**Files:**
- Modify: `internal/adapter/sqlite/object.go`
- Modify: `internal/adapter/http/objects.go`
- Test: `internal/adapter/sqlite/db_test.go`, `e2e/federation_test.go` later; e2e object list contract in Task 8

**Interfaces:**
- Consumes: `remote_objects`, `remote_object_tags` tables; `domain.Object.Source`.
- Produces: `ObjectRepo.List`/count over a `combined` subquery (below); `objectResponse` gains `Source string \`json:"source"\``; `Objects.List`/`Get` unchanged signatures; local-only behavior preserved for `ByID`/`ByName`.

Merged select (exact column set the scanner needs):

```sql
SELECT combined.id, combined.name, combined.description, combined.image_path,
       combined.busy, combined.busy_until, combined.category_id, combined.category_name,
       combined.source, combined.created_at, combined.updated_at
FROM (
  SELECT o.id, o.name, o.description, o.image_path, o.busy, o.busy_until,
         o.category_id, c.name AS category_name, '' AS source, o.created_at, o.updated_at
  FROM objects o LEFT JOIN categories c ON c.id = o.category_id
  UNION ALL
  SELECT r.id, r.name, r.description, r.image_url, r.busy, r.busy_until,
         NULL, NULLIF(r.category_name, ''), r.peer_actor_id, r.published_at, r.updated_at
  FROM remote_objects r
) combined
```

Keep `objectSelect`/`scanObject`/`ByID`/`ByName` untouched (they need `category_id` for the update flow). `List` uses the merged select with its own `scanCombined` helper: local rows build `Category` from id+name as today (empty when both null); remote rows (`source != ''`) build `Category = &domain.Category{Name: n}` when `category_name` non-null, ID left empty; every row gets `Source` (empty for local).

Where rules: `q` → name/description/category_name LIKE + `(EXISTS object_tags OR EXISTS remote_object_tags)`; `category` → `combined.source = '' AND combined.category_name = ? COLLATE NOCASE`; `tag` → either EXISTS. Order `created_at DESC, id DESC`. Scanner rules per the paragraph above. Tag loader per row: remote rows (`source != ""`) read `remote_object_tags`, else `object_tags`.

- [ ] **Step 1: Write failing repo tests**: seed 1 local (category `Books`, tag `dune`) + 2 remote rows (one shares tag `dune`); assert `List` total=3, ordering by created_at desc, `q="dune"` tag match finds both sources, `category="Books"` returns only local, remote row has `Source` set and `Category.ID == ""`. `TestObjectListMergeFiltersAndOrder` + `TestObjectListRemoteTags`.
- [ ] **Step 2: Verify fail**, [ ] **Step 3: Implement** query + scanner + tag loader + response field (`toObjectResponse`: `Source: object.Source`).
- [ ] **Step 4: Verify pass; full suite.** Existing e2e list tests must still pass unchanged (local-only DBs).
- [ ] **Step 5: Commit** — `feat(catalog): merge peer objects into the public list` (body: UNION over objects and mirrored remote_objects, source marker for badges).

---

### Task 8: Admin federation HTTP API

**Files:**
- Create: `internal/adapter/http/federation_admin.go`
- Modify: `internal/adapter/http/server.go` (routes + `New` signature), `internal/app/app.go`
- Test: `e2e/federation_admin_test.go`

**Interfaces:**
- Consumes: `Federation.AddPeer/RemovePeer/Peers/Followers/Outbox/ActorID`.
- Produces (all `requireAuth`):
  - `GET /api/admin/federation` → `{"actor":{"id":...,"handle":"catalog"},"peers":[{"id","actor_id","state","created_at"}],"followers":[{"actor_id","created_at"}],"outbox":[{"id","kind","target","published_at","delivered_at","attempts","next_attempt_at"}],"outbox_total":N}` (outbox limit 50 newest).
  - `POST /api/admin/federation/peers` `{"target":"..."}` → 201 `{"peer":{...}}`; invalid/self/resolve-failure-invalid → 400 `invalid_input`; resolve-not-found → 404 `not_found`.
  - `DELETE /api/admin/federation/peers/{id}` → 204; unknown → 404.
  - `httpapi.New(auth, objects, categories, federation *usecase.Federation, cfg)` — app.go passes the constructed federation (construct federation BEFORE both servers; note current order already does).

- [ ] **Step 1: Write failing e2e** `e2e/federation_admin_test.go`:
- `TestAdminFederationRequiresAuth`: anonymous GET/POST → 401 `unauthorized`.
- `TestAdminFederationOverview`: logged in, empty → actor id == `e.baseURL+"/ap/actors/catalog"`, handle `catalog`, empty arrays (assert JSON `[]`, not null), `outbox_total` 0.
- `TestAdminPeerJoinAndRemove`: second env B; POST join `target: B.baseURL+"/ap/actors/catalog"` → 201 pending peer; GET overview lists peer + followers; DELETE by peer id → 204, peer gone; DELETE unknown → 404; POST `""` → 400; POST target `http://127.0.0.1:1/ap/actors/catalog` (dead port) → 404 `not_found` and the join is not listed in overview (nothing stored on unresolvable target).

- [ ] **Step 2: Verify fail**, [ ] **Step 3: Implement** `federation_admin.go` (small structs + handlers; reuse `writeJSON`/`decodeJSON`/`writeError`; map `domain.ErrNotFound` via existing writeError; follower/outbox DTOs local to file).
- [ ] **Step 4: Verify pass; full suite + race.**
- [ ] **Step 5: Commit** — `feat(http): admin federation overview and peer join/remove endpoints` (body: panel needs actor/peers/followers/outbox from one GET for refresh).

---

### Task 9: Full two-instance content e2e

**Files:**
- Test: `e2e/federation_peers_test.go` (extend), `e2e/helpers_test.go` (add `source` to `objectDTO`)

**Interfaces:** consumes Tasks 4-8.

- [ ] **Step 1: Write** `TestPeerContentAppearsInCatalog`: A joins B via `A.app.Federation.AddPeer`; wait accepted (reuse Task 6 helper asserts); `B.login(); b1 := B.createObject({name:"Dune", description:"desert novel", tags:["scifi"]})` with category via `B.createCategory("Novels")`; eventually A's `GET /api/objects?q=Dune` (anonymous) contains row with `name=="Dune"`, `source == B actor id`, `id == B.baseURL+"/ap/objects/"+b1.ID`, description + tag mirrored. Then B update (`PATCH` name `Dune Messiah`) → eventually A row name updated. Then `B.deleteObject` → eventually absent. Assert counts in `A` list total include the remote row.
- [ ] **Step 2: Write** `TestPeerContentRequiresJoin`: B creates object; A (no join, but A is B's follower via direct `signedInboxPost` Follow like existing tests) — Create activity delivered to A's inbox; assert A's `/api/objects` does NOT contain it (follower ≠ peer; ingest gated on peers). Then A.AddPeer(B) → accepted; force re-push: B update object → now appears in A list (event ordering).
- [ ] **Step 3: Run, expect fail until any gap fixed; implement small fixes if e2e exposes them** (e.g. availability attachment mapping).
- [ ] **Step 4: Verify pass; full suite + race:** `go test -race ./e2e -run TestPeer -v && gofmt -l . && go vet ./... && go test ./...`
- [ ] **Step 5: Commit** — `test(e2e): two-instance peer mirror round trip` (body: proves join-gated ingest and unfollow cascade… include unfollow asserts: `A.app.Federation.RemovePeer` → B's `Followers` loses A and A's list drops B's row via eventually).

---

### Task 10: Admin UI — federation panel + peer badges

**Files:**
- Modify: `web/index.html`, `web/static/app.js`, `web/static/styles.css`

**Interfaces:** consumes Task 8 API and Task 7 `source` field. No JS test harness — verification is manual plus reading rules below.

- [ ] **Step 1: Markup** — new `<section class="federation-panel" id="federation-panel" aria-labelledby="federation-heading" data-admin-only hidden>` after the reservations panel: heading + Refresh button; `<p id="federation-actor" class="muted">`; join form (`<form id="peer-form">` with `#peer-target` input, submit button, `#peer-error` form-error `<p>`); `<div id="peers-list">`, `<div id="followers-list">`, `<div id="outbox-list">` (table). IDs also collected in `elements` at top of `app.js`.
- [ ] **Step 2: JS** — `loadFederation()` (GET overview; skip when no session), `renderFederation()` building rows with `el()` only (no `innerHTML`); peers row: actor id `<a href=actor_id>`, state badge `<span class="peer-badge peer-badge-accepted|pending">`, Remove button `data-action="remove-peer" data-id`; followers rows actor id + created date; outbox table: kind, target, `delivered ? "delivered" : attempts+" / next retry "+formatDate(next_attempt_at)`, published date. Submit `#peer-form` → POST; on error `showFormError(elements.peerError, message)`; on 201 reload. Click delegation on `#peers-list` for `remove-peer` → DELETE + reload + `showStatus("Peer removed")`. Call `loadFederation()` after login in `applyAdminVisibility`-adjacent path and from Refresh; keep `init()` calls: `if (state.user) loadFederation()` inside `loadSession` flow.
- [ ] **Step 3: Object cards** — in `renderObjects`, when `object.source`: add badge `el("span","source-badge","via "+sourceHost(object.source))` where `sourceHost` = `new URL(s).host`; make the `<h3>` name an `<a class="source-link" href=object.id target="_blank" rel="noopener">`; skip `card-actions` block entirely. Local cards unchanged.
- [ ] **Step 4: CSS** — reuse `.reservations-panel` shape for `.federation-panel` (copy its rule set), `.source-badge` like `.category-badge` with distinct background, `.peer-badge` variants, `.source-link { color: inherit; text-decoration: underline; }`.
- [ ] **Step 5: Manual verify** — `go run ./cmd/server` twice on two ports with distinct `DB_PATH`/`UPLOAD_DIR` and `BASE_URL` each (`127.0.0.1` ports), create admins, login A, open panel, join `catalog@127.0.0.1:<B-port>`, watch pending→accepted, create object on B, see badge card on A, click opens B article JSON, Remove peer clears rows. Record result in commit body.
- [ ] **Step 6: Full suite (embedded files) + commit** — `feat(web): federation admin panel and peer source badges` (body: join/leave UI for testing two-instance federation; peer cards link to canonical article).

---

### Task 11: Docs + final verification

**Files:**
- Modify: `docs/spec.md` (federation section: peers, admin endpoints, merged list), `docs/users/federation.md` (replace "no follower list in the admin UI" + describe join/peers/debugging), `docs/activitypub-plan.md` (PR 5 note), `internal/adapter/sqlite/schema.go` untouched
- Create: `docs/adr/0004-peer-join-and-mirror.md` (decision: outbound follow via outbox queue, peer-gated ingest, UNION list; consequences)

- [ ] **Step 1: Update the three docs + write ADR-0004** (normal prose; state why direct-send was rejected and why content requires join, not just follow).
- [ ] **Step 2: Full gate:** `gofmt -l . && go vet ./... && go test ./... && go test -race ./...` — all clean.
- [ ] **Step 3: Commit** — `docs(federation): record peer join design and update guides` (body: admin UI now exists; user guide lines on follower list rewritten).
