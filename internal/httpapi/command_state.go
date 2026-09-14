package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

const (
	commandTicketLifetime  = 10 * time.Second
	commandReceiptLifetime = 5 * time.Minute
	maxCommandReceipts     = 256
	maxCommandReplyBytes   = 16 << 10
)

type commandScope struct {
	actor      controllerIdentity
	generation uint64
}

type commandReceiptKey struct{ session, client, id string }

// A receipt records request handling, not a physical-device guarantee. Only
// the originating authenticated session/tab may retrieve its bounded response.
type commandReceipt struct {
	ID            string          `json:"id"`
	Epoch         string          `json:"epoch"`
	Generation    uint64          `json:"generation"`
	Sequence      uint64          `json:"sequence"`
	State         string          `json:"state"`
	HTTPStatus    int             `json:"http_status,omitempty"`
	Replayable    bool            `json:"replayable"`
	Response      json.RawMessage `json:"response,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	CompletedAt   time.Time       `json:"completed_at,omitempty"`
	key           commandReceiptKey
	fingerprint   string
	bodyBytes     int64
	method, route string
}

type commandRuntime struct {
	mu             sync.Mutex
	serial         chan struct{}
	clock          func() time.Time
	scope          commandScope
	sequence       uint64
	motionSequence uint64
	receipts       map[commandReceiptKey]*commandReceipt
}

func newCommandRuntime() commandRuntime {
	return commandRuntime{serial: make(chan struct{}, 1), clock: time.Now, receipts: make(map[commandReceiptKey]*commandReceipt)}
}

func (c *commandRuntime) acquire(ctx context.Context) (func(), error) {
	timer := time.NewTimer(commandTicketLifetime)
	defer timer.Stop()
	select {
	case c.serial <- struct{}{}:
		return func() { <-c.serial }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, errors.New("control request expired while waiting; inspect the current state before retrying")
	}
}

func (c *commandRuntime) lastSequence(scope commandScope) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.scope != scope {
		return 0
	}
	return c.sequence
}

func (c *commandRuntime) lookup(key commandReceiptKey) *commandReceipt {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneLocked(false)
	if receipt := c.receipts[key]; receipt != nil {
		result := *receipt
		result.Response = append(json.RawMessage(nil), receipt.Response...)
		return &result
	}
	return nil
}

func (c *commandRuntime) begin(scope commandScope, receipt *commandReceipt) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneLocked(true)
	if c.receipts[receipt.key] != nil {
		return errors.New("command already received; inspect its receipt")
	}
	if c.scope == scope && receipt.Sequence <= c.sequence {
		return errors.New("command arrived out of order; refresh the current state before another change")
	}
	if len(c.receipts) >= maxCommandReceipts {
		return errors.New("command receipt capacity is busy; wait for an active request to finish")
	}
	if c.scope != scope {
		c.motionSequence = 0
	}
	c.scope, c.sequence = scope, receipt.Sequence
	if motionCommandRoute(receipt.route) {
		c.motionSequence = receipt.Sequence
	}
	receipt.CreatedAt, receipt.State = c.clock().UTC(), "pending"
	c.receipts[receipt.key] = receipt
	return nil
}

func (c *commandRuntime) finish(receipt *commandReceipt, capture *commandResponse, body *commandBody) {
	c.mu.Lock()
	defer c.mu.Unlock()
	receipt.State, receipt.HTTPStatus, receipt.CompletedAt = "complete", capture.status, c.clock().UTC()
	if receipt.HTTPStatus == 0 || capture.interrupted {
		receipt.State = "unknown"
		receipt.HTTPStatus = 0
		return
	}
	if body.complete {
		receipt.fingerprint, receipt.bodyBytes = body.fingerprint(), body.count
	}
	receipt.Replayable = body.complete && !capture.streamed && !capture.overflow &&
		(len(capture.data) == 0 || json.Valid(capture.data))
	if receipt.Replayable {
		receipt.Response = append(json.RawMessage(nil), capture.data...)
	}
}

func (c *commandRuntime) pruneLocked(needCapacity bool) {
	now := c.clock()
	var oldest *commandReceipt
	for key, receipt := range c.receipts {
		if receipt.State == "pending" {
			continue
		}
		if now.Sub(receipt.CompletedAt) >= commandReceiptLifetime {
			delete(c.receipts, key)
			continue
		}
		if oldest == nil || receipt.CompletedAt.Before(oldest.CompletedAt) {
			oldest = receipt
		}
	}
	if needCapacity && len(c.receipts) >= maxCommandReceipts && oldest != nil {
		delete(c.receipts, oldest.key)
	}
}

func (c *commandRuntime) prune() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneLocked(false)
}
