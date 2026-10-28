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
- `image_path` (optional public URL) — the cover; mirrors the first uploaded
  image when the image list has one
- `images` (ordered list of uploaded images, at most 10)
- `busy` flag and optional `busy_until` timestamp
- optional `reserved_from` timestamp for future reservations
- optional category
- tags
- `created_at` and `updated_at`

The administrator can create, update, and delete objects. Updating an object can
replace or remove its photo. The administrator can also add images to an object,
remove individual images, and reorder them. Deleting an object deletes its
stored photos.

Only the administrator can mark an object busy (taken). A busy object may have a
`busy_until` timestamp and becomes available automatically after that time.
Releasing clears both the flag and the expiry immediately. Public reads expose
the effective busy state.

The administrator can also reserve an object for the future. A reserved object
stays available until `reserved_from` is reached, then behaves as busy until
`busy_until` (optional; without it the object stays taken once the reservation
starts). Releasing an object clears the reservation immediately.

### Photo uploads

- Photos are uploaded with object creation or update using
  `multipart/form-data`.
- Additional images are appended with `POST /api/objects/{id}/images` using
  `multipart/form-data`; the repeatable field `images` carries the files. An
  object holds at most 10 images, each up to 5 MiB.
- Images are addressed by id: `DELETE /api/objects/{id}/images/{imageId}`
  removes one image and renumbers the rest;
  `PATCH /api/objects/{id}/images/order` reorders the list with the complete
  ordered set of image ids.
- The first image of the ordered list is the cover: `image_path` is kept in
  sync with it. Reordering or removing the first image moves the cover to the
  new first entry; emptying the list clears `image_path`. Setting
  `remove_photo=true` on an update clears the cover and removes every image in
  the list.
- Storage root is `UPLOAD_DIR` (default `./static`); files are written to
  `UPLOAD_DIR/uploads/<random>.<ext>`.
- Public URL is `/static/uploads/<random>.<ext>`.
- Accepted content: JPEG, PNG, and WebP, up to 5 MiB.
- Validation is based on sniffed content and image decoding, not the client
  filename or declared content type.
- Filenames are generated with `crypto/rand`; client filenames are never used.
- Replacing a photo deletes the old file after a successful database update.
- Creating or updating an object cleans up a newly saved photo when the
  database operation fails. Multi-image uploads roll back the same way: when
  the list write fails, every newly saved file is deleted.
- Deleting an object deletes its cover and every file in its image list.
- Imported `photo_url` covers stay out of the image list; a remote or
  root-relative cover is replaced by the first uploaded image.
- Uploaded files are served with `X-Content-Type-Options: nosniff`; directory
  listings are disabled. SVG and HTML uploads are rejected.

### Spreadsheet import

The administrator can bulk import objects from an `.xlsx` workbook or a `.csv`
file.

- Upload uses `multipart/form-data` with the field `file`.
- Only `.xlsx` and `.csv` are accepted. Legacy binary `.xls` files are rejected
  with instructions to save as `.xlsx` or CSV.
- The first non-empty row must be a header row. Header names are
  case-insensitive.
- Supported columns:
  - `name` (required; aliases `object`, `object_name`, `title`);
  - `description` (`desc`, `notes`);
  - `category` (`category_name`);
  - `tags` (`tag`), comma-separated;
  - `photo_url` (`photo`, `photo_path`, `image_url`, `image`).
- Unknown columns are ignored. Each file is limited to 10 MiB and 5,000 data
  rows. For `.xlsx`, only the first worksheet is read.
- Rows are imported independently. Valid rows are committed even when other
  rows fail. Failures include the original spreadsheet row number.
- Existing objects are matched by normalized name, case-insensitively. A unique
  match is updated; otherwise a new object is created. If multiple existing
  objects share the name, the row fails rather than overwriting one silently.
- Missing category names are created automatically.
- Omitted optional columns leave the corresponding fields unchanged on update.
  Present-but-blank description, tags, and category clear those fields. A blank
  `photo_url` keeps the existing photo on update. The import never downloads
  photos; `photo_url` must be an `http(s)` URL or a root-relative path.
- Uploading a photo and setting `photo_url` are mutually exclusive.

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
- an object detail dialog opened by clicking a card (or `#object/{id}` deep
  link): full description, image gallery with main view, previous/next arrows,
  and selectable thumbnails, availability badges, tags, and added/updated dates;
  administrators additionally get an edit shortcut inside the dialog;
- administrator login and logout;
- object create/edit/delete with photo upload/replacement/removal;
- an image strip in the object dialog: add multiple files, remove individual
  images, and reorder them; card thumbnails keep using the cover;
- administrator busy marking/release and future reservations, with an availability badge for public
  readers;
- object import from `.xlsx` or CSV with a per-row result summary;
- category create/rename/delete.

There is no frontend build step and no external CDN dependency.

## Code structure

Strict Hexagonal architecture:

```
cmd/server/             server and create-admin commands
internal/domain/        dependency-free models, validation, sentinel errors
internal/usecase/       business logic and outbound ports
internal/adapter/http/  HTTP adapter, JSON/multipart handling, auth middleware
internal/adapter/spreadsheet/ XLSX and CSV import parsing
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
- `objects` — name, description, `image_path`, `busy`, optional `busy_until`,
  optional `reserved_from`, optional category reference.
- `object_tags` — composite primary key `(object_id, tag)`.
- `object_images` — ordered images per object: random `id`, `object_id`,
  `position`, `file_name` (the bare stored file name, never a URL).

Constraints:

- Foreign keys are enforced.
- Deleting a user cascades sessions.
- Deleting a category sets `objects.category_id` to null.
- Deleting an object cascades tags and images.
- `object_images` enforces `UNIQUE (object_id, position)`.
- Schema creation is idempotent. Existing databases are migrated by renaming
  `objects.photo_path` to `objects.image_path`, by adding
  `objects.image_path`, `objects.busy`, and `objects.busy_until` when absent,
  and by backfilling one `object_images` row at position 0 for each object
  whose `image_path` is a bare uploaded file name. URLs and root-relative
  paths are excluded; the backfill is rerun-safe.

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
| `POST` | `/api/objects/import` | admin |
| `PATCH` | `/api/objects/{id}` | admin |
| `PATCH` | `/api/objects/{id}/busy` | admin |
| `PATCH` | `/api/objects/{id}/reservation` | admin |
| `POST` | `/api/objects/{id}/images` | admin |
| `PATCH` | `/api/objects/{id}/images/order` | admin |
| `DELETE` | `/api/objects/{id}/images/{imageId}` | admin |
| `DELETE` | `/api/objects/{id}` | admin |

Object create/update accept both JSON and `multipart/form-data`. Multipart
fields are `name`, `description`, `category_id`, `tags` (comma-separated), and
`photo`; updates additionally accept `remove_photo=true`. JSON updates accept
`remove_photo` as a boolean but cannot upload a photo. Extra images upload
through `POST /api/objects/{id}/images` with `multipart/form-data`, repeating
the field `images` per file (at most 10 images per object, 5 MiB each).
Reordering sends `PATCH /api/objects/{id}/images/order` with
`{"image_ids":["<id>", …]}` — every current image id exactly once, in the
desired order. Over-cap uploads and invalid permutations fail with
`invalid_input`; unknown object or image ids fail with `not_found`.

Busy state is changed with `PATCH /api/objects/{id}/busy`. To mark an object
taken, send `{"busy":true,"busy_until":"<RFC3339 timestamp>"}`; `busy_until` is
required and must be in the future. To release it, send `{"busy":false}`.
Reservations use `PATCH /api/objects/{id}/reservation` with
`{"from":"<RFC3339 timestamp>"}` and an optional `until`. `from` must be in the
future and `until` must be after `from`; reserving a currently busy object
fails with `conflict`.

Successful object responses include `image_path` (empty when absent), `images`
(the ordered list as `{"id", "url", "position"}` entries, `[]` when the object
has none; the first uploaded image is mirrored in `image_path`), `busy`
(the effective state at read time), `busy_until`, and `reserved_from` (null
when the object carries no reservation). A create or update that stores its
cover reports that cover in `image_path` immediately, but its `images` list is
authoritative only on subsequent reads: the response of `POST /api/objects` and
of `PATCH /api/objects/{id}` may carry `images: []` even though the stored file
is already row 0, so clients refresh the list from `GET`. The import endpoint
returns per-row outcomes:

```json
{
  "created": 2,
  "updated": 1,
  "failed": 1,
  "failures": [{"row": 4, "message": "object name is required"}]
}
```

Import file parsing errors use `invalid_input`. Errors use:

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
upload/serve/replace/remove/delete, multi-image upload/serve/reorder/remove
  with the per-object cap and cascade file cleanup on object delete, busy
marking/release/expiry, reservations, CSV/XLSX
  import, partial import failures, and legacy schema migration.
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
- `github.com/xuri/excelize/v2` — XLSX import parsing.

New database adapters can be added behind the use-case repository ports.

## Out of scope

- Public registration and multi-user roles.
- Account settings UI.
- External object storage or CDN delivery.
- Versioned migration tooling beyond additive schema changes.
