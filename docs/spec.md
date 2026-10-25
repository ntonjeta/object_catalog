# Object Catalog Specification

Status: current  
Decision record: [ADR-0001: Single-admin catalog with public reads](adr/0001-single-admin-catalog-and-public-reads.md)

## Overview

The object catalog is a web application for browsing, searching, and managing
objects. The catalog is publicly readable. A single administrator account,
created out of band, manages objects, categories, and photos.

The backend is plain Go following Hexagonal architecture. The frontend is a
small vanilla HTML/CSS/JavaScript application embedded in the Go binary. SQLite
is the initial database adapter.

## Users and access

- There is no public signup.
- The initial and only account is an administrator, created with the
  `create-admin` command.
- The administrator password is read from the `ADMIN_PASSWORD` environment
  variable, validated, and stored as a bcrypt hash.
- Authentication uses opaque session tokens. Only a SHA-256 hash of each token
  is stored in SQLite; the raw token lives in an `HttpOnly`, `SameSite=Lax`
  cookie. `Secure` is configurable through `COOKIE_SECURE`.
- Public access:
  - `GET /api/objects`
  - `GET /api/objects/{id}`
  - `GET /api/categories`
  - `GET /api/categories/{id}`
- Write access, account access, and logout require an authenticated session.
  Until additional roles exist, every authenticated user is the administrator.
- The administrator may change their password or delete their account. If the
  account is deleted, a new administrator can be created with the CLI command.

## Features

### Object management

Objects have:

- `id`
- `name` (required)
- `description`
- `photo_path` (optional public URL)
- `busy` flag and optional `busy_until` timestamp
- optional category
- tags
- `created_at` and `updated_at`

The administrator can create, update, and delete objects. Updating an object can
replace or remove its photo. Deleting an object deletes its stored photo.

Only the administrator can mark an object busy (taken). A busy object may have a
`busy_until` timestamp and becomes available automatically after that time.
Releasing clears both the flag and the expiry immediately. Public reads expose
the effective busy state.

### Photo uploads

- Photos are uploaded with object creation or update using
  `multipart/form-data`.
- Storage root is `UPLOAD_DIR` (default `./static`); files are written to
  `UPLOAD_DIR/uploads/<random>.<ext>`.
- Public URL is `/static/uploads/<random>.<ext>`.
- Accepted content: JPEG, PNG, and WebP, up to 5 MiB.
- Validation is based on sniffed content and image decoding, not the client
  filename or declared content type.
- Filenames are generated with `crypto/rand`; client filenames are never used.
- Replacing a photo deletes the old file after a successful database update.
- Creating or updating an object cleans up a newly saved photo when the
  database operation fails.
- Uploaded files are served with `X-Content-Type-Options: nosniff`; directory
  listings are disabled. SVG and HTML uploads are rejected.

### Categorization

Categories have unique, case-insensitive names. The administrator can create,
rename, and delete categories. Deleting a category leaves its objects intact and
uncategorized.

### Search and pagination

`GET /api/objects` supports:

- `q` — case-insensitive match against object name, description, category name,
  and tags.
- `category` — exact category name, case-insensitive.
- `tag` — exact normalized tag.
- `limit` — default 20, maximum 100.
- `offset` — non-negative.

Results are ordered newest first.

### Frontend

The embedded frontend provides:

- public browsing, search, category filtering, tag filtering, and pagination;
- object thumbnails and photo previews;
- administrator login and logout;
- object create/edit/delete with photo upload/replacement/removal;
- administrator busy marking/release, with an availability badge for public
  readers;
- category create/rename/delete.

There is no frontend build step and no external CDN dependency.

## Code structure

Strict Hexagonal architecture:

```
cmd/server/             server and create-admin commands
internal/domain/        dependency-free models, validation, sentinel errors
internal/usecase/       business logic and outbound ports
internal/adapter/http/  HTTP adapter, JSON/multipart handling, auth middleware
internal/adapter/sqlite/ SQLite repositories and schema migrations
internal/adapter/filestore/ local photo storage
internal/app/           composition root
web/                    embedded HTML/CSS/JavaScript
e2e/                    end-to-end HTTP tests
```

- Interfaces are declared only where a use case consumes them (repository and
  photo-store ports). Interfaces are not created preemptively.
- The HTTP adapter depends on use cases, not repositories.
- Domain code has no adapter, HTTP, SQL, or filesystem dependencies.
- Follow Effective Go practices:
  <https://go.dev/doc/effective_go>.

## Data model

- `users` — administrator identity, unique email, bcrypt password hash.
- `sessions` — hashed session token, user reference, expiry.
- `categories` — unique case-insensitive name.
- `objects` — name, description, `photo_path`, `busy`, optional `busy_until`,
  optional category reference.
- `object_tags` — composite primary key `(object_id, tag)`.

Constraints:

- Foreign keys are enforced.
- Deleting a user cascades sessions.
- Deleting a category sets `objects.category_id` to null.
- Deleting an object cascades tags.
- Schema creation is idempotent. Existing databases are migrated by adding
  `objects.photo_path`, `objects.busy`, and `objects.busy_until` when absent.

## HTTP API

| Method | Path | Access |
| --- | --- | --- |
| `POST` | `/api/auth/login` | public |
| `POST` | `/api/auth/logout` | admin |
| `GET` | `/api/auth/me` | admin |
| `PUT` | `/api/account/password` | admin |
| `DELETE` | `/api/account` | admin |
| `GET` | `/api/categories` | public |
| `GET` | `/api/categories/{id}` | public |
| `POST` | `/api/categories` | admin |
| `PATCH` | `/api/categories/{id}` | admin |
| `DELETE` | `/api/categories/{id}` | admin |
| `GET` | `/api/objects` | public |
| `GET` | `/api/objects/{id}` | public |
| `POST` | `/api/objects` | admin |
| `PATCH` | `/api/objects/{id}` | admin |
| `PATCH` | `/api/objects/{id}/busy` | admin |
| `DELETE` | `/api/objects/{id}` | admin |

Object create/update accept both JSON and `multipart/form-data`. Multipart
fields are `name`, `description`, `category_id`, `tags` (comma-separated), and
`photo`; updates additionally accept `remove_photo=true`. JSON updates accept
`remove_photo` as a boolean but cannot upload a photo.

Busy state is changed with `PATCH /api/objects/{id}/busy`. To mark an object
taken, send `{"busy":true,"busy_until":"<RFC3339 timestamp>"}`; `busy_until` is
required and must be in the future. To release it, send `{"busy":false}`.

Successful object responses include `photo_path` (empty when absent), `busy`,
and `busy_until` (null when available). Errors use:

```json
{"error":{"code":"invalid_input","message":"..."}}
```

Codes: `invalid_input` (400), `unauthorized` (401), `not_found` (404),
`conflict` (409), `internal` (500).

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `ADDR` | `:8080` | HTTP listen address |
| `DB_PATH` | `catalog.db` | SQLite database path |
| `UPLOAD_DIR` | `./static` | Root for uploaded photos |
| `SESSION_TTL` | `24h` | Session lifetime |
| `COOKIE_SECURE` | `false` | Set the session cookie `Secure` flag |
| `ADMIN_PASSWORD` | — | Required only by `create-admin` |

Commands:

```bash
ADMIN_PASSWORD='password123' go run ./cmd/server create-admin -email admin@example.com
go run ./cmd/server
```

## Testing

- Unit tests use the `testing` package.
- End-to-end tests exercise the real HTTP handler with `httptest`, a temporary
  SQLite database, and a temporary upload directory.
- Every endpoint is covered, including validation, duplicate/conflict cases,
  unauthorized writes, missing records, pagination, search combinations, photo
  upload/serve/replace/remove/delete, busy marking/release/expiry, and legacy
  schema migration.
- Third-party services are not used; none need mocks.
- Frontend static serving is covered by E2E tests. A headless Chromium smoke test
  verifies login, category CRUD, photo upload, thumbnails, search, edit/remove,
  delete, and logout.

Commands:

```bash
gofmt -l .
go vet ./...
go test ./...
go test -race ./...
```

## Dependencies

- Go 1.24.
- `modernc.org/sqlite` — pure-Go SQLite driver, no CGO.
- `golang.org/x/crypto/bcrypt` — password hashing.
- `golang.org/x/image/webp` — WebP validation.

New database adapters can be added behind the use-case repository ports.

## Out of scope

- Public registration and multi-user roles.
- Account settings UI.
- External object storage or CDN delivery.
- Versioned migration tooling beyond additive schema changes.
