package voice

import (
	"bufio"
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
	writeMu sync.Mutex
	writer  io.Writer

	mu      sync.Mutex
	pending map[string]chan Response
	closed  bool
	readErr error

	done chan struct{}
}

func newConn(writer io.Writer, reader io.Reader) *conn {
	c := &conn{
		writer:  writer,
		pending: make(map[string]chan Response),
		done:    make(chan struct{}),
	}
	go c.readLoop(reader)
	return c
}

// send writes one request frame. Frames never carry secrets; payload fields
// are the caller's responsibility to keep small.
func (c *conn) send(request Request) error {
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode voice request: %w", err)
	}
	data = append(data, '\n')

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.isClosed() {
		return errConnClosed
	}
	if _, err := c.writer.Write(data); err != nil {
		return fmt.Errorf("write voice request: %w", err)
	}
	return nil
}

// register creates the response channel for a request ID. The returned
// release function must be called exactly once when the caller stops
// listening.
func (c *conn) register(id string) (<-chan Response, func(), error) {
	// Buffered so a slow consumer cannot stall the read loop; the supervisor
	// serializes work requests, so this bound is generous.
	ch := make(chan Response, 64)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, nil, errConnClosed
	}
	if _, exists := c.pending[id]; exists {
		return nil, nil, fmt.Errorf("duplicate voice request id %q", id)
	}
	c.pending[id] = ch

	release := func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}
	return ch, release, nil
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
			// A worker writing junk to stdout is a protocol failure worth
			// surfacing, but one bad line must not kill the session.
			continue
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
	ch := c.pending[response.RequestID]
	c.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- response:
	default:
		// The buffer bound protects the read loop; dropping here means the
		// consumer already abandoned the request.
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
	c.pending = make(map[string]chan Response)
	c.mu.Unlock()

	failure := Response{
		Type:  ResponseError,
		Error: &WorkerError{Code: ErrorCodeInternal, Message: err.Error()},
	}
	for id, ch := range pending {
		failure.RequestID = id
		select {
		case ch <- failure:
		default:
		}
	}
	close(c.done)
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
