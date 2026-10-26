# Contributing to biblio-oggetto

Thanks for taking the time to contribute. This project is a small Go web
application: a public object catalog managed by a single administrator.

## Getting started

Requirements: Go 1.24 or newer. No CGO, no external services.

```bash
git clone <your-fork-url>
cd biblio-oggetto
go build ./...
go test ./...
```

Run the app locally:

```bash
ADMIN_PASSWORD='password123' go run ./cmd/server create-admin -email admin@example.com
go run ./cmd/server
```

## Before opening a pull request

Run the full check suite:

```bash
gofmt -l .
go vet ./...
go test ./...
go test -race ./...
```

`gofmt -l .` must print nothing. Add tests for new behavior — unit tests live
next to the package they cover, end-to-end tests live in `e2e/`.

## Commit messages

Use Conventional Commits:

```
<type>(<scope>): <short summary>

<why the change is needed>
```

Common types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`, `build`.

Examples:

```
feat(import): reject xls files with a clear message
fix(http): return not_found for unknown object ids
docs: document the busy_until expiry behaviour
```

## Architecture rules

The code follows strict hexagonal architecture. Please keep these boundaries:

- `internal/domain/` has no dependencies on HTTP, SQL, adapters, or the
  filesystem.
- `internal/usecase/` holds business logic and declares the ports it consumes.
  Do not add interfaces that no use case needs.
- `internal/adapter/http/` depends on use cases, never on repositories.
- The frontend in `web/` stays dependency-free: no build step, no CDN, no
  framework.

Adapters for other databases go behind the existing repository ports.

## Issues

Use the issue templates: one for bugs, one for feature requests. For a bug,
include the Go version, the operating system, the steps to reproduce, and the
actual versus expected behaviour. Please check for duplicates first.

## Pull requests

- One logical change per pull request.
- Reference the related issue in the description.
- Explain what changed and why, and how you verified it.
- Update `docs/spec.md` when behaviour or the API changes.

## License

By contributing you agree that your contributions are licensed under the
[MIT-0 License](LICENSE).
