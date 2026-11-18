package httpapi

import (
	"context"
	"sync"

	"github.com/mapledaemon/MagicHandy/internal/chatapp"
	"github.com/mapledaemon/MagicHandy/internal/media"
	"github.com/mapledaemon/MagicHandy/internal/memory"
	"github.com/mapledaemon/MagicHandy/internal/patterns"
	"github.com/mapledaemon/MagicHandy/internal/store"
)

type stateSummaries struct {
	library            summaryCache[patterns.Summary]
	media              summaryCache[media.Summary]
	memory             summaryCache[memory.Summary]
	chat, chatWithMood summaryCache[chatapp.Observation]
}

// Writes to these observed domains use WithTx. Never cache a failed read
// or publish a result under a revision committed while that read was in flight.
type summaryCache[T any] struct {
	mu       sync.Mutex
	loading  chan struct{}
	revision uint64
	valid    bool
	value    T
}

func (c *summaryCache[T]) read(ctx context.Context, db *store.DB, read func(context.Context) (T, error)) (T, error) {
	for {
		if err := ctx.Err(); err != nil {
			var empty T
			return empty, err
		}
		c.mu.Lock()
		revision := db.Revision()
		if c.valid && c.revision == revision {
			value := c.value
			c.mu.Unlock()
			return value, nil
		}
		if pending := c.loading; pending != nil {
			c.mu.Unlock()
			select {
			case <-pending:
				continue
			case <-ctx.Done():
				var empty T
				return empty, ctx.Err()
			}
		}
		c.loading = make(chan struct{})
		c.mu.Unlock()
		value, err := read(ctx)
		c.mu.Lock()
		if err == nil && db.Revision() == revision {
			c.value, c.revision, c.valid = value, revision, true
		}
		close(c.loading)
		c.loading = nil
		c.mu.Unlock()
		return value, err
	}
}
