CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	email TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS media (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	owner_user_id INTEGER NOT NULL,
	original_filename TEXT NOT NULL,
	stored_filename TEXT NOT NULL,
	relative_path TEXT NOT NULL UNIQUE,
	media_type TEXT NOT NULL CHECK (media_type IN ('music', 'video', 'photo')),
	mime_type TEXT NOT NULL,
	file_size INTEGER NOT NULL,
	title TEXT,
	artist TEXT,
	album TEXT,
	duration_seconds INTEGER,
	width INTEGER,
	height INTEGER,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY(owner_user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_media_owner_created ON media(owner_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_media_owner_type_created ON media(owner_user_id, media_type, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_media_owner_filename ON media(owner_user_id, original_filename);
