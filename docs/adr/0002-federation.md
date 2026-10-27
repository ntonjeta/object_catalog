# ADR-0002: ActivityPub federation with peer instances

- Status: Accepted
- Date: 2026-10-27

## Context

The catalog should be federatable so peer instances (and their users) can
discover objects and eventually request reservations. The app must stay pure
Go: no CGO, no build step, SQLite via `modernc.org/sqlite`.

## Decision

1. **Peer-instance scope.** The federation target is other instances of this
   application. Remote actors follow the instance and receive object
   announcements; reservation requests arrive over the inbox in a later
   phase.
2. **One instance-level actor.** The admin `User` is not a fediverse identity.
   The instance exposes a single ActivityStreams `Service` actor at
   `/ap/actors/catalog` (handle `catalog`, `domain.InstanceActorHandle`).
   Catalog objects are not actors; they are published as `Create`/`Announce`
   activities wrapping `Article` documents with schema.org fields.
3. **Canonical origin.** `BASE_URL` is a required environment variable
   (absolute `http(s)` origin). Actor ids, WebFinger `acct` matching, and all
   federated URLs derive from it. `app.New` fails fast on an invalid value.
4. **Hand-rolled minimal AP layer.** `go-fed/activity` is archived and
   unmaintained, so ActivityPub types are serialized by hand with
   `encoding/json` plus a fixed `@context`. The only future federation
   dependency is `github.com/go-fed/httpsig` (pure Go) for HTTP signatures,
   introduced with the delivery/inbox phases. Keys use stdlib
   `crypto/rsa` (2048-bit, PKCS#8 private PEM, PKIX public PEM).
5. **Key persistence.** The signing key lives in the `instance_keys` table
   (single row, id = 1), created on first use, never regenerated implicitly.
6. **Hexagonal placement.** Key storage is an `InstanceKeyRepository` port in
   `usecase/ports.go`; the `Identity` use case owns generation/derivation.
   AP JSON-LD serialization is transport detail and lives in
   `internal/adapter/http`.
7. **Deferred.** Inbox signature verification, outbox delivery, followers,
   and remote reservations. The actor advertises `/ap/inbox` and `/ap/outbox`
   now; those endpoints return 404 until their phases land.

## Consequences

- Existing deployments must set `BASE_URL` before starting.
- WebFinger and the actor document are publicly readable, like the catalog
  reads.
- Federation behavior is testable in-process: two `httptest` apps can act as
  peer instances without third-party services.
