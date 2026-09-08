package voice

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// maxFrameBytes bounds one protocol line. Stub payloads are tiny; real
// providers pass audio by reference, so a line this large is a protocol bug.
const maxFrameBytes = 1 << 20

// errConnClosed reports that the worker connection is gone (process exited
// or the supervisor tore the pipe down).
var errConnClosed = errors.New("voice worker connection is closed")

// conn is the core side of one worker's stdio protocol session. It writes
// request frames to the worker's stdin and dispatches response frames from
// its stdout to the goroutine waiting on that request ID.
type conn struct {
	writeGate chan struct{}
	writer    io.WriteCloser

	mu      sync.Mutex
	pending map[string]*responseSink
	closed  bool
	readErr error

	done chan struct{}
}

type responseSink struct {
	responses chan Response
	done      chan struct{}
	stopOnce  sync.Once
}

func newResponseSink() *responseSink {
	return &responseSink{
		responses: make(chan Response, 64),
		done:      make(chan struct{}),
	}
}

func (s *responseSink) stop() {
	s.stopOnce.Do(func() { close(s.done) })
}

func newConn(writer io.WriteCloser, reader io.Reader) *conn {
	c := &conn{
		writeGate: make(chan struct{}, 1),
		writer:    writer,
		pending:   make(map[string]*responseSink),
		done:      make(chan struct{}),
	}
	go c.readLoop(reader)
	return c
}

// send writes one request frame. Frames never carry secrets; payload fields
// are the caller's responsibility to keep small.
func (c *conn) send(request Request) error {
	ctx, cancel := context.WithTimeout(context.Background(), controlTimeout)
	defer cancel()
	return c.sendContext(ctx, request)
}

// A timed-out write retires the entire protocol session: a partial JSON frame
// cannot be retried safely. Closing the owned stdin pipe releases the write;
// the supervisor observes done and terminates the unresponsive child.
func (c *conn) sendContext(ctx context.Context, request Request) error {
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode voice request: %w", err)
	}
	data = append(data, '\n')

	if len(data) > maxFrameBytes {
		return errors.New("voice request exceeds protocol frame limit")
	}
	select {
	case c.writeGate <- struct{}{}:
		defer func() { <-c.writeGate }()
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return errConnClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.isClosed() {
		return errConnClosed
	}
	stopCancellation := context.AfterFunc(ctx, func() { c.closeWithError(ctx.Err()) })
	defer stopCancellation()
	n, err := c.writer.Write(data)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		c.closeWithError(err)
		return fmt.Errorf("write voice request: %w", err)
	}
	if n != len(data) {
		c.closeWithError(io.ErrShortWrite)
		return io.ErrShortWrite
	}
	return nil
}

// register creates the response channel for a request ID. The returned
// release function must be called exactly once when the caller stops
// listening.
func (c *conn) register(id string) (<-chan Response, func(), error) {
	// The buffer absorbs short bursts. dispatch applies backpressure after it
	// fills: dropping an audio frame would silently corrupt speech playback.
	sink := newResponseSink()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, nil, errConnClosed
	}
	if _, exists := c.pending[id]; exists {
		return nil, nil, fmt.Errorf("duplicate voice request id %q", id)
	}
	c.pending[id] = sink

	release := func() {
		c.mu.Lock()
		if c.pending[id] == sink {
			delete(c.pending, id)
		}
		c.mu.Unlock()
		sink.stop()
	}
	return sink.responses, release, nil
}

func (c *conn) readLoop(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxFrameBytes)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var response Response
		if err := json.Unmarshal(line, &response); err != nil {
			c.closeWithError(fmt.Errorf("decode voice worker response: %w", err))
			return
		}
		c.dispatch(response)
	}

	err := scanner.Err()
	if err == nil {
		err = io.EOF
	}
	c.closeWithError(fmt.Errorf("voice worker stream ended: %w", err))
}

func (c *conn) dispatch(response Response) {
	c.mu.Lock()
	sink := c.pending[response.RequestID]
	c.mu.Unlock()
	if sink == nil {
		return
	}
	select {
	case sink.responses <- response:
	case <-sink.done:
	case <-c.done:
	}
}

// closeWithError fails every pending request and marks the session closed.
func (c *conn) closeWithError(err error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.readErr = err
	pending := c.pending
	c.pending = make(map[string]*responseSink)
	c.mu.Unlock()
	close(c.done)
	if c.writer != nil {
		_ = c.writer.Close()
	}

	failure := Response{
		Type:  ResponseError,
		Error: &WorkerError{Code: ErrorCodeInternal, Message: err.Error()},
	}
	for id, sink := range pending {
		failure.RequestID = id
		select {
		case sink.responses <- failure:
		case <-sink.done:
		default:
			// Callers also select on done; teardown must not wait for a slow
			// consumer to drain its full response buffer.
		}
	}
}

func (c *conn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// closedChan lets the supervisor observe stream teardown.
func (c *conn) closedChan() <-chan struct{} {
	return c.done
}

func (c *conn) failure() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.readErr
}
