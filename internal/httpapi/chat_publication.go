package httpapi

import (
	"context"
	"sync"
)

// chatPublicationGate keeps a committed message and its optional speech ID
// observable together. Waiters leave on cancellation without a helper goroutine.
// The zero value is ready for use; it must not be copied after first use.
type chatPublicationGate struct {
	once  sync.Once
	token chan struct{}
}

func (g *chatPublicationGate) Lock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	g.once.Do(func() { g.token = make(chan struct{}, 1) })
	select {
	case g.token <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		g.Unlock()
		return err
	}
	return nil
}

func (g *chatPublicationGate) Unlock() { <-g.token }
