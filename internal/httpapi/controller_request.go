package httpapi

import (
	"context"
	"sync"
)

type controllerRequestBindingKey struct{}

// Synchronous cancellation closes the admission-to-apply gap left by an
// AfterFunc on the owner context. Stop cancels registered requests before it
// advances engine work; its own request detaches before rotating ownership.
type controllerRequestBinding struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	detach func() bool
}

func (b *controllerRequestBinding) bind(controller *controllerRuntime, actor controllerIdentity) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.detach != nil {
		return true
	}
	b.detach = controller.bindRequest(actor, b.cancel)
	return b.detach != nil
}

func (b *controllerRequestBinding) release() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.detach == nil {
		return false
	}
	detach := b.detach
	b.detach = nil
	return detach()
}

func (c *controllerRuntime) bindRequest(actor controllerIdentity, cancel context.CancelFunc) func() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.snapshotLocked(actor).Active {
		return nil
	}
	if c.requests == nil {
		c.requests = make(map[uint64]context.CancelFunc)
	}
	c.requestID++
	id := c.requestID
	c.requests[id] = cancel
	return func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		_, present := c.requests[id]
		delete(c.requests, id)
		return present
	}
}
