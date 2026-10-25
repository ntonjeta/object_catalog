# ADR-0001: Single-admin catalog with public reads

- Status: Accepted
- Date: 2026-10-25

## Context

The original specification assumed public signup with users managing their own
objects. During implementation the product scope was narrowed:

- the first stage is intended to run with a single administrator account;
- the catalog's read API must remain publicly accessible;
- the administrator is created operationally, not through the public API.

The system still needs secure authentication, password storage, and an
authorization boundary for write operations.

## Decision

1. Public signup is not implemented.
2. The administrator is created with `server create-admin -email ...`. The
   password comes from `ADMIN_PASSWORD`, is validated, and stored as a bcrypt
   hash. The CLI does not enforce a single-account limit: the `users` table may
   hold more than one account, and every authenticated account currently has
   administrator access.
3. Authentication uses opaque random session tokens. SQLite stores only their
   SHA-256 hashes. The raw token is sent in an `HttpOnly`, `SameSite=Lax`
   cookie, with `Secure` configurable for HTTPS deployments.
4. Public catalog endpoints are limited to reads of objects and categories.
   Object reads support search and pagination; category reads return the full
   list.
5. Object and category mutations, account operations, and logout require a valid
   session. Until a second role exists, an authenticated session is equivalent
   to administrator access.
6. No role column or role-based access-control layer is added preemptively.
7. Authentication is implemented with the Go standard library plus
   `golang.org/x/crypto/bcrypt`; no authentication framework is introduced.

## Consequences

Positive:

- Minimal attack surface: no registration endpoint or self-service account
  creation.
- Bootstrap is explicit and auditable.
- Opaque sessions can be revoked server-side by deleting stored token hashes.
- The public catalog can be consumed without credentials.

Negative:

- No self-service onboarding; an operator must run the CLI.
- Deleting the last administrator leaves no account until the CLI is run again.
- Public catalog data is exposed by design.
- If multiple roles are required later, schema and authorization changes will be
  necessary; multiple accounts already fit the current schema but all share
  administrator access.

## Alternatives considered

- **Public signup with the first user becoming administrator** — rejected:
  bootstrap depends on request ordering and creates a race/security risk.
- **Public signup with roles for every user** — deferred until multi-user
  requirements exist.
- **Third-party authentication framework** — rejected for the current scope; the
  standard library and bcrypt cover the requirements with less dependency
  surface.
- **JWT access tokens** — rejected: opaque sessions support immediate revocation
  and avoid token-verification complexity.

## References

- [Object Catalog Specification](../spec.md)
