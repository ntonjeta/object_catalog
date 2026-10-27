# biblio-oggetto

Go 1.24 web app: public object catalog, single admin. Pure Go — no CGO
(SQLite via `modernc.org/sqlite`), no node/npm. Frontend in `web/` is vanilla
HTML/CSS/JS embedded into the binary via `web/embed.go`; there is **no build
step** — never add a framework, bundler, or CDN.

## Verify changes

No Makefile, no CI. Run all four; `gofmt -l .` must print nothing:

```bash
gofmt -l . && go vet ./... && go test ./... && go test -race ./...
```

Single test: `go test ./e2e -run TestName`.

## Architecture (strict hexagonal)

- `internal/domain/` — no deps on HTTP/SQL/adapters/filesystem.
- `internal/usecase/` — business logic; declares its own ports in `ports.go`.
  Don't add interfaces no use case needs.
- `internal/adapter/http/` — depends on use cases, **never** on repositories.
- `internal/app/app.go` — composition root; wire new adapters there.
- `cmd/server/main.go` — entrypoint; subcommands `serve` (default) and
  `create-admin` (needs `ADMIN_PASSWORD` env and `-email` flag).
- SQLite schema/migrations live in `internal/adapter/sqlite/schema.go`.

Config is env-only at process start: `ADDR`, `DB_PATH`, `UPLOAD_DIR`,
`SESSION_TTL`, `COOKIE_SECURE` (see README table).

## Testing layout

- Unit tests sit next to their package (e.g. `internal/usecase/*_test.go`).
  Use case tests use fakes in `internal/usecase/fakes_test.go`.
- `e2e/` spins the whole app: `httptest` server + temp SQLite + temp upload
  dir, no third-party services. Shared fixtures in `e2e/helpers_test.go`
  (`env` struct, admin `admin@example.com` / `password123`).
- New behavior needs tests: unit for logic, e2e for API contracts.

## Conventions

- Conventional Commits: `feat(scope): summary` + why in body.
- API errors are `{"error":{"code":"...","message":"..."}}` with codes
  `invalid_input`, `unauthorized`, `not_found`, `conflict`, `internal`.
- Update `docs/spec.md` when behavior or the API changes; decision records in
  `docs/adr/`.
