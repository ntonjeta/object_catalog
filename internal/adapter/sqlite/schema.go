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
}
