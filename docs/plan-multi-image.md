# Plan: multi-image objects

## 1. Goal + locked decision

Object gets ordered list of images via new `object_images` child table.
`objects.image_path` STAYS = denormalized cover = first image of list.
Non-breaking: JSON `image_path`, card thumbnails, import `photo_url`, all existing tests keep working. Add, never replace.

## 2. Open questions (recommended default each — proceed on defaults)

| # | Question | Recommendation |
|---|---|---|
| Q1 | Max images per object? | New `maxImagesPerObject = 10` in `internal/domain/object.go`; size cap stays `usecase.MaxPhotoBytes` (5 MiB) **per file**. |
| Q2 | Upload extras: new endpoint or extend existing PATCH multipart? | **New endpoint** `POST /api/objects/{id}/images`. Existing single-`photo` multipart path and its contract untouched. |
| Q3 | Ordering mechanism? | `position INTEGER` in child table + `PATCH /api/objects/{id}/images/order` with whole ordered id list (replace-whole-set, simplest, idempotent). Reorder arrows in UI issue that PATCH. |
| Q4 | Cover arbitrary or always `images[0]`? | Always `images[0]`. `image_path` synced by use case on every list mutation. |
| Q5 | Per-image caption/alt? | **Out of scope v1.** File + position only. `alt` stays `"Photo of <name>"`. |
| Q6 | Import extra images? | **Out of scope.** Single `photo_url` unchanged; imported URL never enters `object_images`. |
| Q7 | Interaction: object has legacy/remote URL `image_path`, admin uploads first extra image? | List becomes authoritative: URL cover replaced by `images[0]` file name. Nothing to delete (URL is not a stored file; `deletePhoto` already swallows that error). Clearing whole list leaves `image_path=''`. |

## 3. Schema + migration

DDL appended to `schema` slice in `internal/adapter/sqlite/schema.go` (all idempotent, runs every `Open`):

```sql
CREATE TABLE IF NOT EXISTS object_images (
  id TEXT PRIMARY KEY,
  object_id TEXT NOT NULL REFERENCES objects(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  file_name TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE (object_id, position)
)
CREATE INDEX IF NOT EXISTS object_images_object_id_idx ON object_images(object_id)
```

- FK cascade: object delete removes image rows (mirrors `object_tags`, `reservations`).
- `UNIQUE (object_id, position)` = slot-collision guard; repo rewrites positions in one tx.
- `id` random via `domain.NewID()`; API addresses images by `id`.

Backfill — new `backfillObjectImages(ctx, db)` called from `migrate` after `ensureObjectColumns`, following existing additive style (`db.go` rename/`found`-map idiom; no external tool):

```sql
INSERT OR IGNORE INTO object_images (id, object_id, position, file_name, created_at)
  SELECT lower(hex(randomblob(16))), o.id, 0, o.image_path, o.updated_at
  FROM objects o
  WHERE o.image_path <> ''
    AND o.image_path NOT LIKE '%://%'
    AND o.image_path NOT LIKE '/%'
    AND NOT EXISTS (SELECT 1 FROM object_images oi WHERE oi.object_id = o.id);
```

Uploaded files store bare file names (`filestore.Save` returns `<id>.<ext>`); URLs/root-relative paths are filtered out (Q6/Q7: remote covers never join the list). `INSERT OR IGNORE` + `NOT EXISTS` guard = rerun-safe. New sqlite unit test seeds old-shape DB, runs `migrate` twice, asserts one row per legacy object at position 0.

## 4. Domain layer (`internal/domain/`, no HTTP/SQL/fs deps)

- `object.go`: new type + field, `Object.ImagePath` STAYS:

```go
type ObjectImage struct {
    ID       string
    FileName string
}
// Object gains: Images []ObjectImage  // ordered; Images[0].FileName mirrors ImagePath when non-empty
```

- New const `maxImagesPerObject = 10`; new `ValidateObjectImageCount(n int) error` → `ErrInvalid`.
- `ValidateObjectPhotoPath` unchanged (cover still validated as today).

## 5. Use case layer

**Ports (`internal/usecase/ports.go`)**: no new interface — `Objects` already owns `ObjectRepository`. Extend it minimally (only methods `Objects` calls):

```go
Images(ctx context.Context, objectID string) ([]domain.ObjectImage, error)
ReplaceImages(ctx context.Context, objectID string, images []domain.ObjectImage) error
```

`ReplaceImages` = DELETE rows + INSERT ordered in one tx. Sqlite repo (`object.go`) loads `Images` inside `ByID`/`List` exactly like `tags` (same per-object loop pattern, documented there as acceptable for small catalog).

New methods on `Objects` (`internal/usecase/object.go`), each mirroring existing save-then-rollback / delete-after-success discipline:

- `AddImages(ctx, id, uploads []PhotoUpload)`: enforce `ValidateObjectImageCount(len(current)+len(uploads))`; save each via `uc.savePhoto`; on any repo/db error `deletePhoto` every newly saved file (create-path parity, `object.go:74-120`); on success `ReplaceImages`, resync cover, delete nothing else. Cover sync: if old `image_path` was a list file now displaced… old list untouched → append doesn't change row 0, cover unchanged.
- `RemoveImage(ctx, id, imageID string)`: locate in list; `ReplaceImages` without it + renumbered positions; if it was row 0, new `image_path = images[0].FileName` (or `''`) written via `uc.objects.Update`, old cover file `deletePhoto` **after** success (exact `oldImagePath` parity, `object.go:187-189`); else cover unchanged, deleted file removed after success.
- `ReorderImages(ctx, id, orderedIDs []string)`: validate permutation of current ids (`ErrInvalid` otherwise); `ReplaceImages` + cover resync to new `images[0]`; no file deletes.
- Existing `Update` behavior: `input.Photo` still replaces the cover — when list non-empty, overwrite row 0 file (new upload → replace `images[0]`, delete old row-0 file after success); when empty, keep today's pure-`image_path` behavior (no list row invented). `RemovePhoto=true` → clear `image_path` **and** empty the list (delete every list file after success). `Delete` (`object.go:383-393`) additionally deletes all `Images` files — FK cascade removes rows; file cleanup loops the list in the use case before/after repo delete, same best-effort `_ =` style.
- `Create`: unchanged single-photo path; if `Photo` present, sqlite `Create` tx also inserts row 0 (`position=0`, `id` from domain). Keep one tx (`object.go:21-45` tx + `replaceTags` style).

## 6. HTTP adapter

Routes (`server.go`, all `requireAuth`):

```go
mux.HandleFunc("POST  /api/objects/{id}/images",      s.requireAuth(s.addObjectImages))
mux.HandleFunc("DELETE /api/objects/{id}/images/{imageId}", s.requireAuth(s.deleteObjectImage))
mux.HandleFunc("PATCH /api/objects/{id}/images/order", s.requireAuth(s.reorderObjectImages))
```

Response shape — `objectResponse` gains additive field (`image_path` semantics unchanged):

```json
{ "object": { "id": "…", "image_path": "/static/uploads/a.jpg",
  "images": [
    { "id": "img-1", "url": "/static/uploads/a.jpg", "position": 0 },
    { "id": "img-2", "url": "/static/uploads/b.png", "position": 1 }
  ] } }
```

File names rendered through existing `imageURL()` (`objects.go:252`); empty list → `"images": []`. Served by existing `static.go` `/static/uploads/<name>` path — no static change (new files land in same dir, same immutable cache headers).

Requests:

- `POST …/images` — multipart, field `images` (repeatable). New `multipartPhotos(r, cleanup) ([]usecase.PhotoUpload, …)` in `object_input.go`: `MaxBytesReader` bound `maxImagesPerObject*(usecase.MaxPhotoBytes+multipartOverhead)`, `ParseMultipartForm`, iterate `r.MultipartForm.File["images"]`. `filestore.Save` already enforces per-file 5 MiB + sniffing.
- `PATCH …/images/order` — `{ "image_ids": ["img-2","img-1"] }`.
- `DELETE …/images/{imageId}` — no body.

Errors per convention: no/oversized files, bad permutation, missing imageID → `invalid_input` 400; unknown object/image id → `not_found` 404; over-cap → `invalid_input` 400 (could argue `conflict`; convention maps domain `ErrInvalid`→`invalid_input`, keep). Not-logged-in → `unauthorized` 401.

## 7. Frontend (`web/`, vanilla only)

`index.html` (object dialog, current photo block ~:140-151):

- Keep `#object-photo` single input = cover control, unchanged.
- New block after it: `<input id="object-images" type="file" accept="image/jpeg,image/png,image/webp" multiple>` + hint "Up to 10, each ≤5 MB." + `<div id="object-images-strip"></div>` (thumbnail grid) + `<input id="object-images-remove" type="hidden">` no — removal via per-thumb buttons.
- Import help text (:214) untouched (Q6).

`app.js`:

- Element refs (~:39-42 block): `objectImages`, `objectImagesStrip`.
- Replace single `photoObjectURL` (:92) with `let pendingImageURLs = []`; helper `revokePendingImageURLs()` loops `URL.revokeObjectURL`; `hidePhotoImage`/`showPhotoPreview` keep cover URL slot separate — cover keeps its own `photoObjectURL`, strip previews use the array.
- State: `editingObjectImages` (server list) + `pendingUploads` (new `File[]`).
- `openNewObject`/`openEditObject` (:626-691): reset both arrays; edit populates strip from `object.images` (list response must carry `images` — confirmed in §6).
- Card render (:398-406): unchanged (uses `image_path` cover — locked). Admin strip thumb: `<img>` + ✕ button (`DELETE …/images/{id}` then `api GET` refresh or use envelope response) + ← → buttons issuing full `PATCH …/images/order` from current DOM order.
- `#object-images` change handler (pattern of :1094-1110): per file size guard `5*1024*1024`, reject over-cap (`existing+pending > 10`, showFormError), push `URL.createObjectURL` previews into strip.
- Save flow (:907-928): after PATCH create/update succeeds on an existing object, sequentially `POST /api/objects/{id}/images` with `FormData` appending every pending file under field `images`; on error `showStatus`, keep dialog open. New objects: create first, then same extra-images POST using returned `object.id`.

## 8. Tests

New unit:

- `internal/domain/domain_test.go`: `TestValidateObjectImageCount`.
- `internal/usecase/object_test.go`: `TestObjectsAddImages`, `TestObjectsRemoveImageCoverSync`, `TestObjectsReorderImages`, `TestObjectsRemovePhotoClearsImages`, `TestObjectsAddImagesRollback`.
- `internal/adapter/sqlite/db_test.go`: `TestOpenBackfillsObjectImages` (runs `migrate` twice). New `internal/adapter/sqlite/object_images_test.go`: `TestObjectRepoImagesRoundTrip` (create/list/replace/cascade delete).

Edits to existing:

- `internal/usecase/fakes_test.go`: `fakeObjects` gains `images map[string][]domain.ObjectImage` + `Images`/`ReplaceImages`; existing photo-lifecycle tests assert new row-0 behavior of `Update`+`RemovePhoto` (`TestObjectsPhotoLifecycle` :274).
- `e2e/helpers_test.go`: `objectDTO` gains `Images []objectImageDTO` (`{id,url,position}`).

New e2e (`e2e/images_test.go`, admin `admin@example.com`/`password123`):

- `TestObjectImagesUploadServeReorderRemove` — full lifecycle, asserts `image_path` cover tracks `images[0]`.
- `TestObjectImagesCapAndValidation` — 11th file → `invalid_input`; bad permutation → `invalid_input`; unknown image id → `not_found`; unauthenticated POST → 401.
- `TestObjectDeleteRemovesAllImageFiles` — files gone from upload dir.
- `e2e/photos_test.go`: extend `TestObjectPhotoCreateServeReplaceRemove` asserting single-photo contract unchanged (row-0 replace).
- `e2e/import_test.go`: no edits (URL cover stays out of list — add one assert in `TestImportCSVUpsertAndPartialFailures` that `images == []`).

## 9. Docs

`docs/spec.md` rewrites (by anchor): :40-53 (images list in object fields), :65-80 (photo uploads: extras endpoint, 10×5 MiB rule, cover sync), :131-142 (frontend strip add/remove/reorder), :170-187 (data model gains `object_images` row + backfill migration note), :191-215 (three new routes + multipart field `images`), :225-227 (response includes `images`), :266-278 (testing coverage line).
**ADR warranted**: yes — `docs/adr/0002-denormalized-cover-and-ordered-image-list.md` (why `image_path` kept, `images[0]` sync rule, URL covers excluded from list).

## 10. Ordered task checklist

Each step ends green on `gofmt -l . && go vet ./... && go test ./... && go test -race ./...`.

- [ ] 1. Schema+backfill — `internal/adapter/sqlite/schema.go`, `db.go`; test `db_test.go`. Prove: `go test ./internal/adapter/sqlite -run 'TestOpen'`.
- [ ] 2. Repo images round-trip — `internal/adapter/sqlite/object.go` (+create row 0), new `object_images_test.go`. Prove: `go test ./internal/adapter/sqlite`.
- [ ] 3. Domain types — `internal/domain/object.go`, `domain_test.go`. Prove: `go test ./internal/domain`. *(parallel with 1–2: no cross-deps until compile boundary at 4)*
- [ ] 4. Ports+fakes — `internal/usecase/ports.go`, `fakes_test.go`. Prove: `go test ./internal/usecase` (existing green, fakes updated).
- [ ] 5. Use case methods+RemovePhoto/Delete parity — `internal/usecase/object.go`, `object_test.go`. Prove: `go test ./internal/usecase`.
- [ ] 6. HTTP routes+parsing+response — `server.go`, `objects.go`, `object_input.go`, `internal/app/app.go` (no wiring change needed — same repo/store), unit tests in `internal/adapter/http`. Prove: `go test ./internal/adapter/http ./e2e -run 'Object|Image'`. *(6b docs/spec.md + ADR — parallel with 7)*
- [ ] 7. E2E — new `e2e/images_test.go`, `helpers_test.go`, `photos_test.go`, `import_test.go` edits. Prove: `go test ./e2e`.
- [ ] 8. Frontend — `web/index.html`, `web/static/app.js`, `web/static/style.css` (strip grid). Prove: `go test ./e2e -run TestFrontendIsServed` + manual `go run ./cmd/server`.
- [ ] 9. Full gate + spec/ADR merge. Prove: `gofmt -l . && go vet ./... && go test ./... && go test -race ./...` (empty gofmt).
- [ ] Commits (Conventional): `feat(sqlite): add object_images table and backfill`, `feat(domain): ordered image type and count cap`, `feat(usecase): multi-image add/remove/reorder with cover sync`, `feat(http): object image endpoints`, `feat(web): image strip in admin dialog`, `docs(spec): multi-image behavior`, `docs(adr): cover denormalization`.

## 11. Blast radius

Create: `e2e/images_test.go`, `internal/adapter/sqlite/object_images_test.go`, `docs/adr/0002-…md`, this plan file.
Edit (10): `internal/domain/object.go`+`domain_test.go`, `internal/usecase/ports.go`+`object.go`+`object_test.go`+`fakes_test.go`, `internal/adapter/sqlite/schema.go`+`db.go`+`object.go`+`db_test.go`, `internal/adapter/http/server.go`+`objects.go`+`object_input.go`(+`*_test.go`), `web/index.html`, `web/static/app.js`, `web/static/style.css`, `e2e/helpers_test.go`+`photos_test.go`+`import_test.go`, `docs/spec.md`.

Will NOT touch: `internal/usecase/photo.go`, `internal/adapter/filestore/`, `internal/adapter/http/import.go`, `internal/adapter/http/static.go`, `internal/adapter/spreadsheet/`, `internal/usecase/import.go`, `internal/app/app.go`, `cmd/server/`, `web/embed.go`, `README.md`, any reservation/category/auth code.

STATUS: ready-to-execute
