package store

var chatEdgeSchema = []string{
	`CREATE TABLE IF NOT EXISTS chat_edge_holds (
		session_id TEXT NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
		pressed_ns INTEGER NOT NULL,
		after_seq INTEGER NOT NULL,
		kind TEXT NOT NULL CHECK(kind IN ('moving','motor_tempo','still')),
		holding_pace INTEGER NOT NULL CHECK(holding_pace BETWEEN 0 AND 100),
		planned_seconds INTEGER NOT NULL DEFAULT 0 CHECK(planned_seconds=0 OR planned_seconds BETWEEN 8 AND 180),
		return_pace INTEGER NOT NULL DEFAULT 0 CHECK(return_pace BETWEEN 0 AND 100),
		return_style TEXT NOT NULL DEFAULT '' CHECK(return_style IN ('','previous','gentle')),
		PRIMARY KEY (session_id, pressed_ns)
	)`,
	`CREATE TRIGGER IF NOT EXISTS trim_chat_edge_holds AFTER INSERT ON chat_edge_holds BEGIN
		DELETE FROM chat_edge_holds WHERE session_id = NEW.session_id AND pressed_ns IN
		  (SELECT pressed_ns FROM chat_edge_holds WHERE session_id = NEW.session_id ORDER BY pressed_ns DESC LIMIT -1 OFFSET 2);
	END`,
}

// Pausing does not consume a request intended for the next moving hold.
// Retain the last still observation for read-back and two moving observations
// to delimit the interval of user feedback. Earlier v32 rows remain usable.
var chatEdgeRetentionByKind = []string{
	`DROP TRIGGER IF EXISTS trim_chat_edge_holds`,
	`DELETE FROM chat_edge_holds AS older WHERE older.kind='still' AND EXISTS (
		SELECT 1 FROM chat_edge_holds AS newer WHERE newer.session_id=older.session_id
		AND newer.kind='still' AND newer.pressed_ns>older.pressed_ns)`,
	`CREATE TRIGGER trim_chat_edge_holds AFTER INSERT ON chat_edge_holds BEGIN
		DELETE FROM chat_edge_holds WHERE session_id=NEW.session_id AND pressed_ns IN (
			SELECT pressed_ns FROM chat_edge_holds WHERE session_id=NEW.session_id
			AND (kind='still')=(NEW.kind='still') ORDER BY pressed_ns DESC
			LIMIT -1 OFFSET CASE WHEN NEW.kind='still' THEN 1 ELSE 2 END);
	END`,
}
