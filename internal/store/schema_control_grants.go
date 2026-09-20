package store

// Preserve every existing grant and its deadline. NULL represents a new,
// deliberately permanent permission; migration never promotes a timed grant.
var permanentControlSchema = []string{
	`CREATE TABLE user_control_grants_v26 (
		user_id TEXT PRIMARY KEY REFERENCES user_accounts(id) ON DELETE CASCADE,
		grant_id TEXT NOT NULL,
		issued_by TEXT NOT NULL REFERENCES user_accounts(id) ON DELETE CASCADE,
		created_at TEXT NOT NULL,
		expires_at TEXT
	)`,
	`INSERT INTO user_control_grants_v26 SELECT user_id, grant_id, issued_by, created_at, expires_at FROM user_control_grants`,
	`DROP TABLE user_control_grants`,
	`ALTER TABLE user_control_grants_v26 RENAME TO user_control_grants`,
}
