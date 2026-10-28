# ADR-0004: Peer join and mirrored catalog

- Status: Accepted
- Date: 2026-10-28
- Related: [ADR-0002](0002-federation.md), [ADR-0003](0003-federation-adapter-split.md), [design record](../superpowers/specs/2026-10-27-federation-peers-admin-ui-design.md)

## Context

Federation so far was publish-only. A remote account could follow
`catalog@host` and receive announcements, but this instance could not
follow anyone, no federation state was visible in the admin interface, and
announcements published by other instances were never read back. The
owner runs two biblio-oggetto instances and wants them to talk to each
other, driven from the admin interface: join a peer, watch the handshake
complete, see the peer's announcements in the catalog, and leave again.

Two questions had to be answered before that panel could be built: how an
outbound `Follow`/`Undo` reaches the peer, and which inbound actors may
write into the catalog.

## Decision

1. **Outbound follow rides the outbox queue.** Joining a peer appends a
   `follow` entry to the outbox, leaving appends an `undo` entry. Both are
   delivered by the existing `Deliverer` goroutine with the existing
   backoff (30 s base, 30 min cap) and delivery bookkeeping, and both
   address exactly one inbox — the target peer's, resolved from its actor
   document at delivery time. They are never fanned out to the follower
   broadcast, and the zero-follower fast path never consumes them.
2. **A join resolves before it stores.** `Federation.AddPeer` takes a
   handle (`catalog@host`, `acct:catalog@host`) or an absolute http(s)
   actor URL. Handles resolve through WebFinger, actor URLs are fetched
   directly, and the actor document must answer with the id that was
   asked for. When resolution fails nothing is stored: junk syntax and a
   self-follow are `invalid_input`, an unreachable or unknown peer is
   `not_found`. A peer that resolves is stored as `pending` together with
   the id of its follow activity (`<BASE_URL>/ap/outbox/<outbox-entry-id>`,
   the queued follow entry); the peer's `Accept` for exactly that activity
   moves the peer to `accepted`. Every join queues a new follow entry with
   a new activity id, while the peer row keeps the id it was created with;
   joining again resets the state to `pending`.
3. **Ingest is gated on the peers table, not on followership.** Only an
   actor that is a peer row in state `accepted` may write content into the
   catalog. A correctly signed activity from an unknown actor, or from an
   actor that merely follows this instance, is dropped after processing.
4. **The public list is a union.** `GET /api/objects` reads local objects
   and mirrored peer objects from one SQL `UNION ALL` query. Every row
   carries a `source` marker — empty for local rows, the peer actor id for
   mirrored ones — and the frontend turns that marker into a `via <host>`
   badge. Mirrored rows live in their own table and are deleted with the
   peer they came from.
5. **Peer content is clamped, never trusted.** An announced article id must
   be an absolute http(s) URL on the announcing peer's own origin; ids
   from another origin are dropped, so a mirrored card can never point at
   a third party's site. A `Delete` must name an id on the announcing
   peer's origin — one peer may not tombstone another peer's mirrors — and
   a delete for an id that was never mirrored is dropped instead of
   retried, because duplicated deliveries are normal ActivityPub traffic.
   Everything else is clamped rather than rejected: name to 200 runes,
   description to 5000, category name to 100, tags to 20 entries, image
   only when absolute http(s), and an unreadable loan expiry fails closed
   (busy without an expiry) rather than showing a lent object as free.

## Consequences

- Delivery needs no admin intervention. A peer that is down when the
  follow is sent is retried by the same backoff that already covers
  object announcements, and a restart picks the pending entries up again.
- The panel can show the whole handshake — peers with their state,
  followers, and outbox entries with attempts and next retry — because
  the state lives in the database instead of in the request that joined
  the peer.
- Content requires an explicit join, not a follow. Following this
  instance only ever grants delivery of our announcements; it never
  grants write access to our catalog.
- Inbound follow semantics are unchanged: any correctly signed `Follow` is
  still accepted automatically, and `Undo` still removes it. Joining a
  peer adds a second, independent relationship.
- Peer strings are untrusted input and are stored as such
  (`remote_objects`, `remote_object_tags`). The frontend renders every
  peer-supplied value through `textContent`, and peer links open with
  `rel="noopener"`.
- The origin rule ties a mirrored object to the peer that announced it.
  A peer that publishes objects under a different origin than its actor
  id is dropped silently — biblio-oggetto peers publish
  `<peer-base>/ap/objects/<id>` and announce from
  `<peer-base>/ap/actors/catalog`, so both share one origin.
- The merged list changes the public API: `source` is part of every
  object response, and the category filter applies to local objects only,
  because a mirrored object carries a free-form category name from its
  peer.
- Leaving a peer removes its mirrored objects from the catalog. Joining
  again starts from `pending` and mirrors only what the peer announces
  afterwards — nothing is backfilled.

## Alternatives considered

- **Direct send for `Follow`/`Undo`** — rejected: the join endpoint would
  sign and POST the activity synchronously, and a peer that was briefly
  unreachable would need a manual retry from the panel. The outbox queue
  already exists, already retries, and already survives restarts.
- **Ingest gated on followership** — rejected: a follower subscribes to
  our announcements, which does not make it a partner we trust to
  publish into our catalog. Keeping the two gates apart lets an instance
  be a follower of many accounts while mirroring only the peers an
  administrator joined.
- **Mirrored rows stored as ordinary objects** — rejected: local objects
  are editable and carry local ids, while peer rows are read-only copies
  that must vanish with the peer. A separate table joined by `UNION ALL`
  keeps the two apart.

## References

- [Object Catalog Specification](../spec.md)
- [Federation user guide](../users/federation.md)
