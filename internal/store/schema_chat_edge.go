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
