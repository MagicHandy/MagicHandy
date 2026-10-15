package workerclient

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/ble"
	"github.com/mapledaemon/MagicHandy/internal/ble/workerproto"
)

const (
	eventBuffer       = 64
	writeStallTimeout = 2 * time.Second
)

// session is the core side of one worker's stdio protocol. Replies route to
// the waiting request by ID; unsolicited lines become link events.
type session struct {
	writer    io.WriteCloser
	writeGate chan struct{}

	mu      sync.Mutex
	pending map[string]chan workerproto.Message
	nextID  uint64
	retired bool

	events chan ble.LinkEvent
	quit   chan struct{}
	done   chan struct{}
}

func newSession(writer io.WriteCloser, reader io.Reader) *session {
	s := &session{
		writer:    writer,
		writeGate: make(chan struct{}, 1),
		pending:   make(map[string]chan workerproto.Message),
		events:    make(chan ble.LinkEvent, eventBuffer),
		quit:      make(chan struct{}),
		done:      make(chan struct{}),
	}
	go s.readLoop(reader)
	return s
}

// send writes one line. A stalled write retires the session: a partial line
// cannot be retried, and closing stdin ends the worker.
func (s *session) send(ctx context.Context, message workerproto.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	select {
	case s.writeGate <- struct{}{}:
		defer func() { <-s.writeGate }()
	case <-ctx.Done():
		return ctx.Err()
	case <-s.quit:
		return ble.ErrLinkClosed
	}
	if s.isRetired() {
		return ble.ErrLinkClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// The write itself ignores ctx: a caller giving up must not tear down a
	// healthy session. Only a worker that stops reading stdin retires it.
	stall := time.AfterFunc(writeStallTimeout, s.retire)
	_, err = s.writer.Write(data)
	stall.Stop()
	if err != nil {
		s.retire()
		return fmt.Errorf("%w: %v", ble.ErrLinkClosed, err)
	}
	return nil
}

func (s *session) request(ctx context.Context, message workerproto.Message) (workerproto.Message, error) {
	replies := make(chan workerproto.Message, 1)
	s.mu.Lock()
	if s.retired {
		s.mu.Unlock()
		return workerproto.Message{}, ble.ErrLinkClosed
	}
	s.nextID++
	message.ID = strconv.FormatUint(s.nextID, 10)
	s.pending[message.ID] = replies
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.pending, message.ID)
		s.mu.Unlock()
	}()
	if err := s.send(ctx, message); err != nil {
		return workerproto.Message{}, err
	}
	select {
	case reply := <-replies:
		if reply.Type == workerproto.TypeError {
			return reply, replyError(reply)
		}
		return reply, nil
	case <-ctx.Done():
		return workerproto.Message{}, ctx.Err()
	case <-s.done:
		return workerproto.Message{}, ble.ErrLinkClosed
	}
}

func (s *session) readLoop(reader io.Reader) {
	defer close(s.done)
	defer close(s.events)
	scanner := scannerFor(reader)
	for scanner.Scan() {
		var message workerproto.Message
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			continue
		}
		if message.ID != "" {
			// A reply nobody waits for any more (its request timed out) is
			// dropped; it must never be mistaken for a stream event.
			s.deliver(message)
			continue
		}
		if event, ok := linkEvent(message); ok {
			s.emit(event)
		}
	}
	if !s.isRetired() {
		s.emit(ble.LinkEvent{Kind: ble.LinkLost, Err: errors.New("the Bluetooth device Bluetooth helper stopped")})
	}
}

func (s *session) deliver(message workerproto.Message) {
	s.mu.Lock()
	replies, ok := s.pending[message.ID]
	s.mu.Unlock()
	if ok {
		select {
		case replies <- message:
		default:
		}
	}
}

func (s *session) emit(event ble.LinkEvent) {
	select {
	case s.events <- event:
	case <-s.quit:
	}
}

func linkEvent(message workerproto.Message) (ble.LinkEvent, bool) {
	switch message.Type {
	case workerproto.TypeNotify:
		payload, err := hex.DecodeString(message.Data)
		if err != nil {
			return ble.LinkEvent{}, false
		}
		return ble.LinkEvent{Kind: ble.LinkNotification, Payload: payload}, true
	case workerproto.TypeBattery:
		if message.Percent == nil {
			return ble.LinkEvent{}, false
		}
		return ble.LinkEvent{Kind: ble.LinkBattery, Battery: *message.Percent}, true
	case workerproto.TypeDisconnected:
		return ble.LinkEvent{Kind: ble.LinkLost, Err: fmt.Errorf("the Bluetooth device disconnected: %s", message.Reason)}, true
	case workerproto.TypeError:
		if message.Code == workerproto.CodeNotConnected {
			return ble.LinkEvent{Kind: ble.LinkLost, Err: ble.ErrLinkClosed}, true
		}
		return ble.LinkEvent{Kind: ble.LinkWriteFailed, Err: replyError(message)}, true
	default:
		return ble.LinkEvent{}, false
	}
}

func replyError(reply workerproto.Message) error {
	switch reply.Code {
	case workerproto.CodeDiscovery:
		return ble.ErrDiscoveryFailed
	case workerproto.CodeNotFound:
		return ble.ErrNotFound
	case workerproto.CodeUnavailable:
		return ble.ErrBluetoothUnavailable
	case workerproto.CodeUnsupported:
		return ble.ErrUnsupportedDevice
	case workerproto.CodeNotConnected:
		return ble.ErrLinkClosed
	case workerproto.CodeAmbiguous:
		return errors.New("more than one matching Bluetooth device was found; turn off the others and try again")
	default:
		return fmt.Errorf("the Bluetooth device reported %s: %s", reply.Code, reply.Message)
	}
}

func (s *session) isRetired() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.retired
}

// retire stops all further writes and closes the worker's stdin.
func (s *session) retire() {
	s.mu.Lock()
	if s.retired {
		s.mu.Unlock()
		return
	}
	s.retired = true
	s.mu.Unlock()
	close(s.quit)
	_ = s.writer.Close()
}
