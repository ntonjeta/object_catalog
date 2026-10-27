# ADR-0003: Federation adapter split

- Status: Accepted
- Date: 2026-10-27
- Amends: [ADR-0002](0002-federation.md) item 6

## Context

ADR-0002 placed the ActivityPub surface in `internal/adapter/http` with the
catalog API. After federation phases 1–3 that adapter held two protocols:
catalog JSON API, auth middleware, static file serving, plus the instance
actor, WebFinger, inbox/outbox/object endpoints, JSON-LD serialization,
remote actor/key fetches, HTTP-signature verification, and two background
workers. Seven ActivityPub files inside a package named for the catalog made
the hexagonal boundary murky and made it easy to mix catalog-only assumptions
into federation code.

## Decision

1. **Own adapter.** `internal/adapter/activitypub` is the sole ActivityPub
   adapter: endpoints, serialization, `SignatureVerifier`, `RemoteActors`,
   `Deliverer`, `InboxProcessor`. It depends on use cases, `domain`, and
   `httpx` only — never on the catalog adapter, never on repositories
   directly. `internal/adapter/http` keeps catalog API, auth, and static
   serving with no federation knowledge.
2. **Mounting in the composition root.** `internal/app` builds a root
   `ServeMux` that routes `/ap/` and `/.well-known/` to the federation
   handler and everything else to the catalog handler. Sub-handlers match
   full paths (no prefix stripping), so both adapters keep independent route
   tables.
3. **Shared wire helpers.** The error envelope (`{"error":{"code","message"}}`
   with the domain sentinel mapping) and panic recovery live in
   `internal/httpx`, a leaf package importing only `domain`. This replaces
   duplication between adapters with a third shared package instead of an
   adapter-to-adapter import.
4. **Worker ownership.** The delivery and inbox-processing goroutines are
   created and started by `app.New` (tick interval `Config.FederationPoll`,
   default 5 s) and stopped by `App.Close`. Adapters expose `Run(ctx)` loops;
   only the composition root manages lifecycles.

## Consequences

- The code-structure list names three transport packages: `adapter/http`,
  `adapter/activitypub`, and `httpx`.
- PR 4 (remote reservations) lands in the federation adapter and its use
  case, not in the catalog adapter.
- `httpx` may grow only in wire-format concerns; use-case logic and storage
  must not move there.
- Federation is always on: `BASE_URL` is required for every command, and the
  workers run even when there are no followers (pending entries are then
  marked delivered immediately).
