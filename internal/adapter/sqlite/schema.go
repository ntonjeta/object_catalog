package sqlite

var schema = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		email TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS sessions (
		token_hash TEXT PRIMARY KEY,
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		expires_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS sessions_expires_at_idx ON sessions(expires_at)`,
	`CREATE TABLE IF NOT EXISTS categories (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL COLLATE NOCASE UNIQUE,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS objects (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		image_path TEXT NOT NULL DEFAULT '',
		busy INTEGER NOT NULL DEFAULT 0,
		busy_until TEXT,
		reserved_from TEXT,
		category_id TEXT REFERENCES categories(id) ON DELETE SET NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS objects_category_id_idx ON objects(category_id)`,
	`CREATE TABLE IF NOT EXISTS object_tags (
		object_id TEXT NOT NULL REFERENCES objects(id) ON DELETE CASCADE,
		tag TEXT NOT NULL,
		PRIMARY KEY (object_id, tag)
	)`,
	`CREATE INDEX IF NOT EXISTS object_tags_tag_idx ON object_tags(tag)`,
	`CREATE TABLE IF NOT EXISTS object_images (
		id TEXT PRIMARY KEY,
		object_id TEXT NOT NULL REFERENCES objects(id) ON DELETE CASCADE,
		position INTEGER NOT NULL,
		file_name TEXT NOT NULL,
		created_at TEXT NOT NULL,
		UNIQUE (object_id, position)
	)`,
	`CREATE INDEX IF NOT EXISTS object_images_object_id_idx ON object_images(object_id)`,
	`CREATE TABLE IF NOT EXISTS reservations (
		id TEXT PRIMARY KEY,
		object_id TEXT NOT NULL REFERENCES objects(id) ON DELETE CASCADE,
		status TEXT NOT NULL,
		starts_at TEXT NOT NULL,
		ends_at TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS reservations_object_id_idx ON reservations(object_id)`,
	`CREATE TABLE IF NOT EXISTS activities (
		id TEXT PRIMARY KEY,
		type TEXT NOT NULL,
		object_id TEXT NOT NULL,
		reservation_id TEXT NOT NULL DEFAULT '',
		actor_id TEXT NOT NULL DEFAULT '',
		detail TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS activities_object_id_idx ON activities(object_id)`,
	`CREATE TABLE IF NOT EXISTS instance_keys (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		private_pem TEXT NOT NULL,
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS outbox (
		id TEXT PRIMARY KEY,
		kind TEXT NOT NULL,
		object_json TEXT NOT NULL DEFAULT '',
		target TEXT NOT NULL DEFAULT '',
		follow_id TEXT NOT NULL DEFAULT '',
		published_at TEXT NOT NULL,
		delivered_at TEXT,
		attempts INTEGER NOT NULL DEFAULT 0,
		next_attempt_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS outbox_published_at_idx ON outbox(published_at)`,
	`CREATE INDEX IF NOT EXISTS outbox_pending_idx ON outbox(delivered_at, next_attempt_at)`,
	`CREATE TABLE IF NOT EXISTS followers (
		actor_id TEXT PRIMARY KEY,
		inbox_url TEXT NOT NULL,
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS inbox_events (
		id TEXT PRIMARY KEY,
		sender TEXT NOT NULL DEFAULT '',
		type TEXT NOT NULL,
		actor TEXT NOT NULL DEFAULT '',
		object_id TEXT NOT NULL DEFAULT '',
		payload TEXT NOT NULL,
		received_at TEXT NOT NULL,
		processed_at TEXT,
		attempts INTEGER NOT NULL DEFAULT 0,
		next_attempt_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS inbox_events_pending_idx ON inbox_events(processed_at, next_attempt_at)`,
	`CREATE TABLE IF NOT EXISTS remote_keys (
		key_id TEXT PRIMARY KEY,
		public_pem TEXT NOT NULL,
		fetched_at TEXT NOT NULL
	)`,
}
