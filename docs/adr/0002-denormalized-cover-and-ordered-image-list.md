# ADR-0002: Denormalized cover and ordered image list

- Status: Accepted
- Date: 2026-10-28

## Context

Objects carried exactly one photo in `objects.image_path`, which stored either
the bare name of an uploaded file or a remote/root-relative URL from spreadsheet
import. The single-photo contract is baked into the JSON API, the admin dialog,
the public card thumbnails, and the import semantics.

The product now needs an ordered list of up to ten uploaded images per object,
addressable individually so the administrator can remove or reorder single
images without resending the whole set.

## Decision

1. Add an ordered child table `object_images` (random `id`, `object_id`,
   `position`, `file_name`, `created_at`) with `ON DELETE CASCADE` to
   `objects` and `UNIQUE (object_id, position)`. The change is additive: no
   existing column, endpoint, or response field changes meaning.
2. `objects.image_path` stays, as a denormalized cover mirroring `images[0]`
   whenever the list holds an uploaded file. The use case resyncs the cover on
   every list mutation (add, remove, reorder, photo replace/remove) and deletes
   displaced files only after all database writes succeeded.
3. URL and root-relative covers never enter the list. On the first uploaded
   image the list becomes authoritative and the file takes over the cover;
   clearing the list leaves `image_path` empty.
4. The cap is 10 images per object; the per-file size cap stays `MaxPhotoBytes`
   (5 MiB). Import's single `photo_url` is unchanged.
5. On every `Open`, a rerun-safe backfill inserts one position-0 row per object
   whose `image_path` is a bare uploaded file name, so legacy databases gain
   the list without losing their existing cover.

## Consequences

Positive:

- No API break: existing clients, thumbnails, and tests keep reading
  `image_path`; `images` is an additive JSON field.
- Per-image ids make single-image `DELETE` and whole-list reorder endpoints
  trivial to address.
- FK cascade plus use-case file cleanup keeps rows and files consistent when
  objects are deleted.

Negative:

- Two sources of truth (`image_path` and `images[0]`) must be kept in sync by
  the use case; a future mutation path that forgets the resync diverges.
- Image lists are loaded per object in `ByID`/`List`, the same N+1 read pattern
  already accepted for tags; fine for a small catalog.
- Legacy rows and URL covers need the documented backfill/exclusion rules.

## Alternatives considered

- **Drop `image_path`, serve only the list** — rejected: breaking change to
  the object JSON and to every frontend and test that renders the cover.
- **One JSON array column on `objects`** — rejected: no per-image identity, so
  removing or reordering one image rewrites the whole array and no FK cascade
  protects the rows.
- **Keep one photo per object** — rejected: does not meet the requirement for
  ordered, individually managed images.

## References

- [Object Catalog Specification](../spec.md)
- [ADR-0001: Single-admin catalog with public reads](0001-single-admin-catalog-and-public-reads.md)
- [Plan: multi-image objects](../plan-multi-image.md)
