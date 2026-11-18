# Security Assessment — 2026-11-17

Scope: main working tree, pre-deployment review for hosting behind nginx.
Method: manual review of auth, sessions,
cookies, CSRF, SQL, uploads, static/path handling, frontend DOM sinks, and
`govulncheck` (v1.10.0, `-mode` default/symbols).

## Verdict

The codebase is notably clean for its size. No injection, traversal, XSS,
CSRF, or secret-leak issues were found. The real risks are (1) known
reachable vulnerabilities in two dependencies and (2) configuration gaps
that must be closed at the nginx layer because the app does not enforce
them.

## What is solid (verified)

- **SQL injection: clean.** All queries are parameterized
  (`internal/adapter/sqlite/object.go:173-181`,
  `internal/adapter/sqlite/activity.go:35-50`). Dynamic WHERE fragments are
  constant strings only; user input reaches the DB exclusively as bind
  arguments. `LIKE` wildcards are escaped (`likePattern`,
  `internal/adapter/sqlite/helpers.go:48-55`).
- **Passwords and sessions.** bcrypt for password hashing
  (`internal/usecase/auth.go:38,66`). Session tokens are 32 bytes from
  `crypto/rand`; only the SHA-256 hash is stored
  (`internal/usecase/auth.go:157-168`). Expired sessions are deleted on
  login and on access; `ON DELETE CASCADE` removes a user's sessions when
  the account is deleted (`internal/adapter/sqlite/schema.go:12`).
- **Cookies.** `HttpOnly`, `SameSite=Lax`, host-only (no `Domain`),
  `Path=/` (`internal/adapter/http/auth.go:87-110`). All state-changing
  routes use POST/PUT/PATCH/DELETE only, so cross-site form submissions
  arrive without the session cookie under `Lax` — CSRF is covered without
  separate tokens.
- **No XSS sinks.** `web/static/app.js` builds the DOM exclusively with
  `textContent`, `createElement`, and literal `setAttribute` calls; zero
  `innerHTML`/`insertAdjacentHTML`/`document.write`/`eval`.
- **Uploads.** Content is validated by magic-byte sniffing plus
  `image.DecodeConfig` format matching; stored under generated random IDs
  with fixed extensions, `O_CREATE|O_EXCL`
  (`internal/adapter/filestore/store.go:55-95`). Path traversal is blocked
  at both layers: file name validation on delete
  (`internal/adapter/filestore/store.go:111-117`) and on serving
  (`internal/adapter/http/static.go:51-60`: rejects `/`, `fs.ValidPath`).
  Embedded assets go through `fs.ValidPath`
  (`internal/adapter/http/static.go:31`).
- **Request size limits everywhere.** 1 MB JSON bodies
  (`internal/adapter/http/json.go:15,97`); 5 MB per photo, max 10 per
  request (`internal/adapter/http/object_input.go:16-21,83-128`); 10 MB
  spreadsheets with a 16 MB unzip limit and 5 000-row cap
  (`internal/adapter/spreadsheet/reader.go:19-20,95`).
- **Error hygiene.** Internal errors are logged server-side and replaced by
  a generic message in the response (`internal/adapter/http/json.go:39-46`);
  panics are recovered per request (`internal/adapter/http/server.go:66-76`).

## Findings

### HIGH — vulnerable dependencies with reachable call sites

`govulncheck` reports 13 vulnerabilities called by this code, from two
modules:

- `github.com/xuri/excelize/v2@v2.10.1` — malformed XLSX inputs cause panics
  and unbounded memory allocation (GO-2026-6689, -6688, -6685, -6684, -6681,
  -6679, -6677, -6674, -6453, -6452). Reachable via spreadsheet import
  (`internal/adapter/spreadsheet/reader.go:93,106`).
- `golang.org/x/image@v0.31.0` — VP8L decoding memory exhaustion
  (GO-2026-6222), VP8 alpha size-mismatch panic (GO-2026-5061), large-WebP
  panic on 32-bit (GO-2026-4961). Reachable via `image.DecodeConfig` on every
  image upload (`internal/adapter/filestore/store.go:130`).

Both entry points are admin-gated, but imported spreadsheets typically come
from untrusted archives, and memory exhaustion takes down the whole process —
the per-request `recoverPanics` cannot contain it.

Remediation: `go get github.com/xuri/excelize/v2@latest` (≥ v2.11.x) and
`go get golang.org/x/image@v0.45.0` (or newer), then re-run `govulncheck`.

### MEDIUM — session cookie `Secure` flag off by default

`cmd/server/main.go:123` hardcodes `CookieSecure: false`; the README table
(documenting `COOKIE_SECURE`, default `false`) confirms it. Under nginx with
TLS, forgetting `COOKIE_SECURE=true` sends the session cookie over plain HTTP.
Set `COOKIE_SECURE=true` in the deployment environment.

### MEDIUM — no login rate limiting

Nothing in the Go code throttles authentication attempts
(`internal/adapter/http/auth.go:30-42`); bcrypt cost does not stop sustained
brute force. Must be enforced at nginx (`limit_req`, see checklist).

### MEDIUM — no security headers

The app sets no CSP, no `X-Frame-Options`/`frame-ancestors`, and no HSTS
anywhere. With a single-admin UI and no frame protection, clickjacking is a
live risk if the app is reachable through a browser session. Must be added at
nginx (or in Go later).

### LOW — email enumeration by timing

`Auth.Login` returns before running bcrypt when the email is unknown
(`internal/usecase/auth.go:58-68`), producing a measurably faster failure.
A constant-time design would hash a dummy value on the not-found path.

### LOW — partial server timeouts

`cmd/server/main.go:93-97` sets only `ReadHeaderTimeout: 5s`; there is no
`ReadTimeout`, `WriteTimeout`, or `IdleTimeout`, leaving the body phase open
to slow-client attacks. Mitigate with nginx `client_body_timeout` and friends.

### LOW — relative default paths for DB and uploads

`DB_PATH` defaults to `catalog.db` and `UPLOAD_DIR` to `./static`
(`cmd/server/main.go:121-124`). If nginx is ever configured with a `root`/
`alias` pointing into the app directory instead of a pure reverse proxy, the
SQLite database and all uploads are served directly. Keep the app directory
out of nginx's filesystem namespace.

### INFO — flagged procedure reviewed and found sound

`internal/usecase/auth.go:120` carries a TODO ("don't trust the LLM in
security as is") over `ChangePassword`. Reviewed: it verifies the current
password, updates the hash, deletes all the user's sessions, and issues a
fresh token (`internal/usecase/auth.go:121-151`). Behavior is correct.

### INFO — federation code lives in a worktree

`worktree/activityPub` adds ActivityPub federation
(`internal/usecase/federation.go`), introducing outbound HTTP fetches (SSRF
surface) that do not exist in the main tree. Re-run this assessment before
merging it.

## nginx deployment checklist

- Reverse-proxy only (`proxy_pass` to the Go listener); no `root`/`alias`
  into the application directory.
- Environment: `COOKIE_SECURE=true` (plus explicit `DB_PATH`, `UPLOAD_DIR`
  outside any web root).
- `limit_req` zone on `POST /api/auth/login` (e.g. `burst=5 nodelay`).
- Response headers: a `Content-Security-Policy` (at minimum
  `default-src 'self'`, `img-src 'self' data:`), `X-Frame-Options: DENY`
  (or CSP `frame-ancestors 'none'`), and `Strict-Transport-Security`.
- `client_max_body_size` ≥ 60 MB if the 10-image batch upload is kept
  (10 × (5 MB photo + 1 MB overhead), `internal/adapter/http/object_input.go:16-21`);
  lower it by reducing the batch size instead.
- `proxy_redirect off;` and `proxy_set_header X-Forwarded-Proto $scheme;`.
  The app ignores `X-Forwarded-For` (it logs no client IPs), which is fine.

## Suggested order of work

1. Bump `excelize` and `golang.org/x/image`, re-run `govulncheck`.
2. Deploy nginx config per the checklist above.
3. Set `COOKIE_SECURE=true` in the service environment.
4. Optional hardening: dummy-hash login path, server read/write timeouts,
   in-app security headers.
