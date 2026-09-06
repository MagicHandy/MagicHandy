package chat

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRequestReadsCancelWhileDatabasePoolIsBusy(t *testing.T) {
	log, err := OpenMessageLog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()
	log.db.SQL().SetMaxOpenConns(1)
	connection, err := log.db.SQL().Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()
	for name, read := range map[string]func(context.Context) error{
		"active":   func(ctx context.Context) error { _, err := log.ActiveSessionIDContext(ctx); return err },
		"sessions": func(ctx context.Context) error { _, err := log.SessionsContext(ctx); return err },
		"session":  func(ctx context.Context) error { _, err := log.SessionContext(ctx, "session"); return err },
		"messages": func(ctx context.Context) error { _, err := log.AfterSessionContext(ctx, "session", 0, 20); return err },
		"recent":   func(ctx context.Context) error { _, err := log.RecentSessionContext(ctx, "session", 20); return err },
		"prompt":   func(ctx context.Context) error { _, err := log.ReadPromptContext(ctx, "session"); return err },
		"head":     func(ctx context.Context) error { _, err := log.LatestSeqSessionContext(ctx, "session"); return err },
		"cursor": func(ctx context.Context) error {
			_, err := log.CursorSessionContext(ctx, "client", "session")
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
			defer cancel()
			if err := read(ctx); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("read error = %v", err)
			}
		})
	}
}
