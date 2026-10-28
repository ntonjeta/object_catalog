package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"biblio-oggetto/internal/domain"
	"biblio-oggetto/internal/usecase"
)

func TestOpenMigratesExistingObjectsWithoutImagePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	if _, err := legacy.Exec(`CREATE TABLE objects (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		category_id TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if _, err := legacy.Exec(`INSERT INTO objects (id, name, created_at, updated_at)
		VALUES ('legacy', 'Legacy', '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z')`); err != nil {
		t.Fatalf("insert legacy object: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	var imagePath string
	if err := db.QueryRow(`SELECT image_path FROM objects WHERE id = 'legacy'`).Scan(&imagePath); err != nil {
		t.Fatalf("query migrated image_path: %v", err)
	}
	if imagePath != "" {
		t.Fatalf("image_path = %q, want empty default", imagePath)
	}
}

func TestOpenRenamesLegacyPhotoPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	if _, err := legacy.Exec(`CREATE TABLE objects (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		photo_path TEXT NOT NULL DEFAULT '',
		category_id TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if _, err := legacy.Exec(`INSERT INTO objects (id, name, photo_path, created_at, updated_at)
		VALUES ('legacy', 'Legacy', '/static/uploads/old.png', '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z')`); err != nil {
		t.Fatalf("insert legacy object: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	var imagePath string
	var busy int
	var busyUntil sql.NullString
	var reservedFrom sql.NullString
	if err := db.QueryRow(`SELECT image_path, busy, busy_until, reserved_from FROM objects WHERE id = 'legacy'`).
		Scan(&imagePath, &busy, &busyUntil, &reservedFrom); err != nil {
		t.Fatalf("query migrated image_path: %v", err)
	}
	if imagePath != "/static/uploads/old.png" {
		t.Fatalf("image_path = %q, want legacy value", imagePath)
	}
	if busy != 0 {
		t.Fatalf("busy = %d, want 0 default", busy)
	}
	if busyUntil.Valid {
		t.Fatalf("busy_until = %q, want NULL default", busyUntil.String)
	}
	if reservedFrom.Valid {
		t.Fatalf("reserved_from = %q, want NULL default", reservedFrom.String)
	}
}

func TestOpenBackfillsObjectImages(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	if _, err := legacy.Exec(`CREATE TABLE objects (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		image_path TEXT NOT NULL DEFAULT '',
		category_id TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	seed := []struct {
		id        string
		imagePath string
	}{
		{"uploaded", "cover.png"},
		{"remote", "https://example.com/photo.jpg"},
		{"root-relative", "/static/uploads/old.png"},
		{"empty", ""},
	}
	for _, item := range seed {
		if _, err := legacy.Exec(`INSERT INTO objects (id, name, image_path, created_at, updated_at)
			VALUES (?, ?, ?, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z')`,
			item.id, item.id, item.imagePath); err != nil {
			t.Fatalf("insert legacy object %q: %v", item.id, err)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	// Rerun-safe: migrating a second time must not duplicate or add rows.
	if err := migrate(ctx, db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	rows, err := db.QueryContext(ctx, `SELECT object_id, position, file_name FROM object_images ORDER BY object_id`)
	if err != nil {
		t.Fatalf("query object_images: %v", err)
	}
	defer rows.Close()
	type imageRow struct {
		objectID string
		position int
		fileName string
	}
	var found []imageRow
	for rows.Next() {
		var row imageRow
		if err := rows.Scan(&row.objectID, &row.position, &row.fileName); err != nil {
			t.Fatalf("scan object_images row: %v", err)
		}
		found = append(found, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read object_images: %v", err)
	}
	want := []imageRow{{objectID: "uploaded", position: 0, fileName: "cover.png"}}
	if len(found) != len(want) {
		t.Fatalf("object_images rows = %+v, want exactly %+v (one per legacy cover, position 0, no URL or root-relative rows)", found, want)
	}
	for i := range want {
		if found[i] != want[i] {
			t.Fatalf("object_images row %d = %+v, want %+v", i, found[i], want[i])
		}
	}
	var imageID string
	if err := db.QueryRowContext(ctx, `SELECT id FROM object_images WHERE object_id = 'uploaded'`).Scan(&imageID); err != nil {
		t.Fatalf("query backfilled image id: %v", err)
	}
	if len(imageID) != 32 {
		t.Fatalf("backfilled image id = %q, want 32 hex characters", imageID)
	}
}

func TestObjectRepoByName(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	repo := NewObjectRepo(db)

	now := time.Now().UTC()
	object := domain.Object{
		ID:          "one",
		Name:        "Dune",
		Description: "desert",
		Tags:        []string{"scifi"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := repo.Create(ctx, object, domain.OutboxEntry{}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	found, err := repo.ByName(ctx, "dune")
	if err != nil {
		t.Fatalf("ByName: %v", err)
	}
	if found.ID != object.ID || len(found.Tags) != 1 || found.Tags[0] != "scifi" {
		t.Fatalf("found = %+v", found)
	}
	if _, err := repo.ByName(ctx, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing error = %v, want ErrNotFound", err)
	}

	second := object
	second.ID = "two"
	if err := repo.Create(ctx, second, domain.OutboxEntry{}); err != nil {
		t.Fatalf("create duplicate: %v", err)
	}
	if _, err := repo.ByName(ctx, "Dune"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate error = %v, want ErrConflict", err)
	}
}

func TestObjectRepoCreateEmbedsOutboxEntry(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	repo := NewObjectRepo(db)
	outbox := NewOutboxRepo(db)

	now := time.Now().UTC()
	entry := domain.OutboxEntry{
		ID:        "entry-1",
		Kind:      domain.OutboxCreate,
		Object:    domain.Object{ID: "one", Name: "Dune", Tags: []string{"scifi"}},
		Published: now,
	}
	if err := repo.Create(ctx, entry.Object, entry); err != nil {
		t.Fatalf("create with entry: %v", err)
	}
	stored, err := outbox.ByID(ctx, "entry-1")
	if err != nil {
		t.Fatalf("outbox ByID: %v", err)
	}
	if stored.Kind != domain.OutboxCreate || stored.Object.Name != "Dune" || len(stored.Object.Tags) != 1 {
		t.Fatalf("stored = %+v", stored)
	}

	pending, err := outbox.Pending(ctx, now, 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending = (%v, %v), want one", pending, err)
	}
	if err := outbox.MarkAttempt(ctx, "entry-1", 1, now.Add(time.Hour)); err != nil {
		t.Fatalf("MarkAttempt: %v", err)
	}
	pending, err = outbox.Pending(ctx, now, 10)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending after backoff = (%v, %v), want none", pending, err)
	}
	if err := outbox.MarkDelivered(ctx, "entry-1", now); err != nil {
		t.Fatalf("MarkDelivered: %v", err)
	}
	if err := outbox.MarkDelivered(ctx, "missing", now); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing mark error = %v, want ErrNotFound", err)
	}
}

func TestFollowerRepoUpsertAndRemove(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	repo := NewFollowerRepo(db)

	if err := repo.Add(ctx, domain.Follower{ActorID: "alice", InboxURL: "https://remote.example/inbox/old", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := repo.Add(ctx, domain.Follower{ActorID: "alice", InboxURL: "https://remote.example/inbox/new", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	followers, err := repo.List(ctx)
	if err != nil || len(followers) != 1 {
		t.Fatalf("list = (%v, %v), want one follower", followers, err)
	}
	if followers[0].InboxURL != "https://remote.example/inbox/new" {
		t.Fatalf("inbox = %q, want refreshed", followers[0].InboxURL)
	}
	if err := repo.Remove(ctx, "alice"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := repo.Remove(ctx, "alice"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second remove error = %v, want ErrNotFound", err)
	}
}

func TestInboxRepoPendingAndMarks(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	repo := NewInboxRepo(db)

	now := time.Now().UTC()
	event := domain.InboxEvent{
		ID: "https://remote.example/f/1", Sender: "alice", Type: "Follow",
		Actor: "alice", ObjectID: "self", Payload: "{}", ReceivedAt: now,
	}
	if err := repo.Add(ctx, event); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := repo.Add(ctx, event); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate add error = %v, want ErrConflict", err)
	}
	pending, err := repo.Pending(ctx, now, 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending = (%v, %v), want one", pending, err)
	}
	if pending[0].Payload != "{}" || pending[0].Type != "Follow" {
		t.Fatalf("pending entry = %+v", pending[0])
	}

	if err := repo.MarkAttempt(ctx, event.ID, 1, now.Add(time.Hour)); err != nil {
		t.Fatalf("mark attempt: %v", err)
	}
	pending, err = repo.Pending(ctx, now, 10)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending after backoff = (%v, %v), want none", pending, err)
	}
	pending, err = repo.Pending(ctx, now.Add(time.Hour), 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending after window = (%v, %v), want one", pending, err)
	}

	if err := repo.MarkProcessed(ctx, event.ID, now); err != nil {
		t.Fatalf("mark processed: %v", err)
	}
	pending, err = repo.Pending(ctx, now.Add(time.Hour), 10)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending after processed = (%v, %v), want none", pending, err)
	}
	if err := repo.MarkProcessed(ctx, "missing", now); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing processed error = %v, want ErrNotFound", err)
	}
}

func TestRemoteKeyRepoGetPut(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	repo := NewRemoteKeyRepo(db)

	if _, err := repo.Get(ctx, "key-1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("get missing error = %v, want ErrNotFound", err)
	}
	now := time.Now().UTC()
	if err := repo.Put(ctx, "key-1", "pem-one", now); err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := repo.Put(ctx, "key-1", "pem-two", now); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	pem, err := repo.Get(ctx, "key-1")
	if err != nil || pem != "pem-two" {
		t.Fatalf("get = (%q, %v), want pem-two", pem, err)
	}
}

// openDB opens a migrated catalog database in the test's temporary directory
// and closes it when the test finishes.
func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})
	return db
}

func TestPeerRepoUpsertAndStates(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	repo := NewPeerRepo(db)

	joined := time.Now().UTC()
	peer := domain.Peer{
		ID:               "peer-1",
		ActorID:          "https://remote.example/ap/actor",
		InboxURL:         "https://remote.example/ap/inbox/one",
		FollowActivityID: "https://us/ap/outbox/one",
		State:            domain.PeerPending,
		CreatedAt:        joined,
	}
	if err := repo.Upsert(ctx, peer); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	stored, err := repo.ByID(ctx, "peer-1")
	if err != nil || stored.State != domain.PeerPending {
		t.Fatalf("ByID after insert = (%+v, %v), want pending", stored, err)
	}

	if err := repo.MarkAccepted(ctx, peer.ActorID); err != nil {
		t.Fatalf("MarkAccepted: %v", err)
	}
	stored, err = repo.ByID(ctx, "peer-1")
	if err != nil || stored.State != domain.PeerAccepted {
		t.Fatalf("ByID after accept = (%+v, %v), want accepted", stored, err)
	}

	if err := repo.MarkAccepted(ctx, "https://nowhere.example/ap/actor"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("MarkAccepted unknown error = %v, want ErrNotFound", err)
	}

	// Re-join resets the state to pending and refreshes inbox and follow
	// activity id while keeping the original created_at.
	rejoined := peer
	rejoined.InboxURL = "https://remote.example/ap/inbox/two"
	rejoined.FollowActivityID = "https://us/ap/outbox/x"
	rejoined.CreatedAt = joined.Add(time.Hour)
	if err := repo.Upsert(ctx, rejoined); err != nil {
		t.Fatalf("re-join upsert: %v", err)
	}
	stored, err = repo.ByID(ctx, "peer-1")
	if err != nil {
		t.Fatalf("ByID after re-join: %v", err)
	}
	if stored.State != domain.PeerPending {
		t.Fatalf("state = %q, want pending reset", stored.State)
	}
	if stored.FollowActivityID != "https://us/ap/outbox/x" || stored.InboxURL != "https://remote.example/ap/inbox/two" {
		t.Fatalf("refreshed = %+v, want new follow id and inbox", stored)
	}
	if !stored.CreatedAt.Equal(joined) {
		t.Fatalf("created_at = %v, want original %v", stored.CreatedAt, joined)
	}

	found, err := repo.ByFollowActivityID(ctx, "https://us/ap/outbox/x")
	if err != nil || found.ID != "peer-1" {
		t.Fatalf("ByFollowActivityID = (%+v, %v), want peer-1", found, err)
	}
	if _, err := repo.ByFollowActivityID(ctx, "https://us/ap/outbox/missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown activity error = %v, want ErrNotFound", err)
	}
	if _, err := repo.ByID(ctx, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown id error = %v, want ErrNotFound", err)
	}

	later := domain.Peer{
		ID:               "peer-2",
		ActorID:          "https://other.example/ap/actor",
		InboxURL:         "https://other.example/ap/inbox",
		FollowActivityID: "https://us/ap/outbox/two",
		State:            domain.PeerAccepted,
		CreatedAt:        joined.Add(2 * time.Hour),
	}
	if err := repo.Upsert(ctx, later); err != nil {
		t.Fatalf("upsert second peer: %v", err)
	}
	peers, err := repo.List(ctx)
	if err != nil || len(peers) != 2 {
		t.Fatalf("List = (%v, %v), want two peers", peers, err)
	}
	if peers[0].ID != "peer-1" || peers[1].ID != "peer-2" {
		t.Fatalf("List order = (%q, %q), want oldest first", peers[0].ID, peers[1].ID)
	}

	if err := repo.Remove(ctx, "peer-1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := repo.Remove(ctx, "peer-1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second Remove error = %v, want ErrNotFound", err)
	}
	if _, err := repo.ByID(ctx, "peer-1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ByID after remove error = %v, want ErrNotFound", err)
	}
	peers, err = repo.List(ctx)
	if err != nil || len(peers) != 1 {
		t.Fatalf("List after remove = (%v, %v), want one peer", peers, err)
	}
}

func TestRemoteObjectRepoPutUpserts(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	repo := NewRemoteObjectRepo(db)

	now := time.Now().UTC()
	until := now.Add(time.Hour)
	object := domain.RemoteObject{
		ID:           "https://remote.example/ap/objects/1",
		PeerActorID:  "https://remote.example/ap/actor",
		Name:         "Dune",
		Description:  "desert planet",
		ImageURL:     "https://remote.example/img/dune.png",
		CategoryName: "Sci-Fi",
		Busy:         true,
		BusyUntil:    &until,
		Tags:         []string{"scifi", "classic"},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := repo.Put(ctx, object); err != nil {
		t.Fatalf("put: %v", err)
	}

	updated := object
	updated.Name = "Dune Messiah"
	updated.Description = ""
	updated.ImageURL = ""
	updated.CategoryName = ""
	updated.Busy = false
	updated.BusyUntil = nil
	updated.Tags = []string{"scifi"}
	updated.CreatedAt = now.Add(-time.Hour)
	updated.UpdatedAt = now.Add(time.Hour)
	if err := repo.Put(ctx, updated); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM remote_objects`).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("row count = %d, want one row after upsert", count)
	}

	var (
		name         string
		description  string
		imageURL     string
		categoryName string
		busy         int
		busyUntil    sql.NullString
		publishedAt  string
		updatedAt    string
	)
	if err := db.QueryRowContext(ctx,
		`SELECT name, description, image_url, category_name, busy, busy_until, published_at, updated_at
		 FROM remote_objects WHERE id = ?`, object.ID,
	).Scan(&name, &description, &imageURL, &categoryName, &busy, &busyUntil, &publishedAt, &updatedAt); err != nil {
		t.Fatalf("query row: %v", err)
	}
	if name != "Dune Messiah" {
		t.Fatalf("name = %q, want second put to win", name)
	}
	if busy != 0 || busyUntil.Valid {
		t.Fatalf("busy = (%d, %v), want reset to false/NULL", busy, busyUntil)
	}
	if publishedAt != formatTime(updated.CreatedAt) || updatedAt != formatTime(updated.UpdatedAt) {
		t.Fatalf("timestamps = (%q, %q), want (%q, %q)", publishedAt, updatedAt,
			formatTime(updated.CreatedAt), formatTime(updated.UpdatedAt))
	}

	rows, err := db.QueryContext(ctx,
		`SELECT tag FROM remote_object_tags WHERE object_id = ? ORDER BY tag`, object.ID)
	if err != nil {
		t.Fatalf("query tags: %v", err)
	}
	defer rows.Close()
	tags := make([]string, 0)
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			t.Fatalf("scan tag: %v", err)
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tags: %v", err)
	}
	if len(tags) != 1 || tags[0] != "scifi" {
		t.Fatalf("tags = %v, want replaced by second put", tags)
	}
}

func TestObjectListMergeFiltersAndOrder(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	objects := NewObjectRepo(db)
	categories := NewCategoryRepo(db)
	remotes := NewRemoteObjectRepo(db)

	now := time.Now().UTC()
	if err := categories.Create(ctx, domain.Category{
		ID: "cat-1", Name: "Books", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create category: %v", err)
	}
	local := domain.Object{
		ID:          "local-1",
		Name:        "Local Book",
		Description: "printed copy",
		Category:    &domain.Category{ID: "cat-1", Name: "Books"},
		Tags:        []string{"dune"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := objects.Create(ctx, local, domain.OutboxEntry{}); err != nil {
		t.Fatalf("create local object: %v", err)
	}

	first := domain.RemoteObject{
		ID:           "https://peer.example/ap/objects/1",
		PeerActorID:  "https://peer.example/ap/actor",
		Name:         "Remote Book",
		Description:  "mirrored copy",
		ImageURL:     "https://peer.example/img/1.png",
		CategoryName: "Novels",
		Tags:         []string{"dune"},
		CreatedAt:    now.Add(time.Hour),
		UpdatedAt:    now.Add(time.Hour),
	}
	second := domain.RemoteObject{
		ID:          "https://other.example/ap/objects/1",
		PeerActorID: "https://other.example/ap/actor",
		Name:        "Other Book",
		Tags:        []string{"scifi"},
		CreatedAt:   now.Add(2 * time.Hour),
		UpdatedAt:   now.Add(2 * time.Hour),
	}
	for _, object := range []domain.RemoteObject{first, second} {
		if err := remotes.Put(ctx, object); err != nil {
			t.Fatalf("put remote %s: %v", object.ID, err)
		}
	}

	listed, total, err := objects.List(ctx, usecase.ObjectFilter{Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 3 || len(listed) != 3 {
		t.Fatalf("total = %d, rows = %d, want 3 and 3", total, len(listed))
	}
	wantOrder := []string{second.ID, first.ID, local.ID}
	for i, want := range wantOrder {
		if listed[i].ID != want {
			t.Fatalf("row %d = %q, want %q (created_at desc)", i, listed[i].ID, want)
		}
	}
	if listed[1].Source != first.PeerActorID {
		t.Fatalf("remote source = %q, want %q", listed[1].Source, first.PeerActorID)
	}
	if listed[2].Source != "" {
		t.Fatalf("local source = %q, want empty", listed[2].Source)
	}
	if listed[1].Category == nil || listed[1].Category.Name != "Novels" {
		t.Fatalf("remote category = %+v, want name Novels", listed[1].Category)
	}
	if listed[1].Category.ID != "" {
		t.Fatalf("remote category id = %q, want empty", listed[1].Category.ID)
	}
	if listed[2].Category == nil || listed[2].Category.ID != "cat-1" {
		t.Fatalf("local category = %+v, want id cat-1", listed[2].Category)
	}

	matched, matchedTotal, err := objects.List(ctx, usecase.ObjectFilter{Query: "dune", Limit: 20})
	if err != nil {
		t.Fatalf("List q=dune: %v", err)
	}
	if matchedTotal != 2 || len(matched) != 2 {
		t.Fatalf("q=dune total = %d, rows = %d, want 2", matchedTotal, len(matched))
	}
	sources := map[string]bool{}
	for _, object := range matched {
		sources[object.Source] = true
	}
	if !sources[""] || !sources[first.PeerActorID] {
		t.Fatalf("q=dune sources = %v, want local and peer row", sources)
	}

	byCategory, categoryTotal, err := objects.List(ctx, usecase.ObjectFilter{Category: "Books", Limit: 20})
	if err != nil {
		t.Fatalf("List category=Books: %v", err)
	}
	if categoryTotal != 1 || len(byCategory) != 1 {
		t.Fatalf("category=Books total = %d, rows = %d, want 1", categoryTotal, len(byCategory))
	}
	if byCategory[0].ID != local.ID {
		t.Fatalf("category=Books row = %q, want local object", byCategory[0].ID)
	}

	byTag, tagTotal, err := objects.List(ctx, usecase.ObjectFilter{Tag: "scifi", Limit: 20})
	if err != nil {
		t.Fatalf("List tag=scifi: %v", err)
	}
	if tagTotal != 1 || len(byTag) != 1 || byTag[0].ID != second.ID {
		t.Fatalf("tag=scifi = (%d, %+v), want only the other peer row", tagTotal, byTag)
	}
}

func TestObjectListRemoteTags(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	objects := NewObjectRepo(db)
	remotes := NewRemoteObjectRepo(db)

	now := time.Now().UTC()
	remote := domain.RemoteObject{
		ID:          "https://peer.example/ap/objects/1",
		PeerActorID: "https://peer.example/ap/actor",
		Name:        "Remote Book",
		Tags:        []string{"scifi", "classic"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := remotes.Put(ctx, remote); err != nil {
		t.Fatalf("put remote: %v", err)
	}

	listed, _, err := objects.List(ctx, usecase.ObjectFilter{Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("rows = %d, want 1", len(listed))
	}
	if len(listed[0].Tags) != 2 || listed[0].Tags[0] != "classic" || listed[0].Tags[1] != "scifi" {
		t.Fatalf("tags = %v, want [classic scifi] from remote_object_tags", listed[0].Tags)
	}
}

func TestRemoteObjectRepoDeleteByPeer(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	repo := NewRemoteObjectRepo(db)

	now := time.Now().UTC()
	first := domain.RemoteObject{
		ID:          "https://a.example/ap/objects/1",
		PeerActorID: "https://a.example/ap/actor",
		Name:        "A One",
		Tags:        []string{"a"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	second := first
	second.ID = "https://a.example/ap/objects/2"
	second.Name = "A Two"
	third := domain.RemoteObject{
		ID:          "https://b.example/ap/objects/1",
		PeerActorID: "https://b.example/ap/actor",
		Name:        "B One",
		Tags:        []string{"b"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	for _, object := range []domain.RemoteObject{first, second, third} {
		if err := repo.Put(ctx, object); err != nil {
			t.Fatalf("put %s: %v", object.ID, err)
		}
	}

	if err := repo.DeleteByPeer(ctx, "https://a.example/ap/actor"); err != nil {
		t.Fatalf("DeleteByPeer: %v", err)
	}

	var remaining int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM remote_objects WHERE peer_actor_id = ?`, "https://a.example/ap/actor").
		Scan(&remaining); err != nil {
		t.Fatalf("count a rows: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("peer a rows = %d, want 0", remaining)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM remote_objects WHERE peer_actor_id = ?`, "https://b.example/ap/actor").
		Scan(&remaining); err != nil {
		t.Fatalf("count b rows: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("peer b rows = %d, want 1 untouched", remaining)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM remote_object_tags`).Scan(&remaining); err != nil {
		t.Fatalf("count tags: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("tag rows = %d, want cascade deleted a's tags", remaining)
	}

	// Deleting an unknown peer removes nothing but is not an error.
	if err := repo.DeleteByPeer(ctx, "https://c.example/ap/actor"); err != nil {
		t.Fatalf("DeleteByPeer unknown = %v, want nil", err)
	}

	if err := repo.Delete(ctx, third.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := repo.Delete(ctx, third.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second Delete error = %v, want ErrNotFound", err)
	}
}
