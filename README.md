# biblio-oggetto

Web application for browsing, searching, and managing a catalog of objects.
The catalog is publicly readable; a single administrator account manages
objects, categories, and photos.

Backend is plain Go (hexagonal architecture, no CGO). Frontend is vanilla
HTML/CSS/JavaScript embedded in the binary — no build step, no CDN.

## Features

- Public browsing with search, category and tag filters, and pagination
- Object management: create, edit, delete, photo upload/replace/remove
- Busy ("taken") marking with optional expiry, plus public availability badge
- Categories with unique, case-insensitive names
- Bulk import from `.xlsx` or `.csv` with per-row results
- Session authentication (bcrypt passwords, opaque server-side sessions)

## Requirements

- Go 1.24 or newer

Dependencies are pure Go; CGO is not required.

## Quick start

```bash
# 1. create the administrator account
ADMIN_PASSWORD='password123' go run ./cmd/server create-admin -email admin@example.com

# 2. run the server
go run ./cmd/server
```

The server listens on `:8080` by default. Open <http://localhost:8080>,
log in with the administrator account, and start adding objects.

To build a standalone binary:

```bash
go build -o server ./cmd/server
./server
```

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `ADDR` | `:8080` | HTTP listen address |
| `DB_PATH` | `catalog.db` | SQLite database path |
| `UPLOAD_DIR` | `./static` | Root for uploaded photos |
| `SESSION_TTL` | `24h` | Session lifetime |
| `COOKIE_SECURE` | `false` | Set the session cookie `Secure` flag |
| `ADMIN_PASSWORD` | — | Required only by `create-admin` |

Set `COOKIE_SECURE=true` when serving over HTTPS.

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
| `DELETE` | `/api/objects/{id}` | admin |

`GET /api/objects` accepts `q`, `category`, `tag`, `limit` (max 100), and
`offset`. Errors use `{"error":{"code":"...","message":"..."}}` with codes
`invalid_input`, `unauthorized`, `not_found`, `conflict`, `internal`.

## Project layout

```
cmd/server/                 server and create-admin commands
internal/domain/            models, validation, sentinel errors
internal/usecase/           business logic and outbound ports
internal/adapter/http/      HTTP adapter, JSON/multipart, auth middleware
internal/adapter/sqlite/    SQLite repositories and schema migrations
internal/adapter/spreadsheet/ XLSX and CSV import parsing
internal/adapter/filestore/ local photo storage
internal/app/               composition root
web/                        embedded HTML/CSS/JavaScript
e2e/                        end-to-end HTTP tests
docs/                       specification and decision records
```

## Testing

```bash
gofmt -l .
go vet ./...
go test ./...
go test -race ./...
```

End-to-end tests use `httptest`, a temporary SQLite database, and a temporary
upload directory. No third-party services are required.

## Documentation

- [Specification](docs/spec.md)
- [ADR-0001: Single-admin catalog with public reads](docs/adr/0001-single-admin-catalog-and-public-reads.md)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Participation is governed by the

## License

[MIT-0](LICENSE) — use, change, and redistribute freely, no attribution required.
