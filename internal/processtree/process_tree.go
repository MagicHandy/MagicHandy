// Package processtree keeps app-owned worker descendants within one lifecycle.
package processtree

import "sync"

// Handle owns the platform resource used to terminate a process tree.
type Handle struct {
	once     sync.Once
	close    func() error
	closeErr error
}

func newHandle(closeTree func() error) *Handle {
	return &Handle{close: closeTree}
}

// Close terminates the owned process tree. Repeated calls are safe.
func (h *Handle) Close() error {
	if h == nil {
		return nil
	}
	h.once.Do(func() {
		if h.close != nil {
			h.closeErr = h.close()
		}
	})
	return h.closeErr
}
