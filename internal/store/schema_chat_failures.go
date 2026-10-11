package store

var chatFailuresSchema = []string{
	`CREATE TABLE IF NOT EXISTS chat_failures (
		seq INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
		metadata TEXT NOT NULL CHECK (length(CAST(metadata AS BLOB)) <= 8192 AND json_valid(metadata)),
		details TEXT NOT NULL CHECK (length(CAST(details AS BLOB)) <= 262144 AND json_valid(details))
	)`,
}
