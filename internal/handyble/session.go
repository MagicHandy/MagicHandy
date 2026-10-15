package handyble

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/ble"
)

// ErrInterrupted marks queued work invalidated by Stop.
var ErrInterrupted = errors.New("handy command interrupted by Stop")

const (
	writeTimeout      = time.Second
	responseTimeout   = 700 * time.Millisecond
	maxChunkPoints    = 20
	maxRecentRequests = 64
)

// Snapshot reports connection health and the latest accepted playback feedback.
type Snapshot struct {
	Ready         bool
	ClockSynced   bool
	PlaybackState string
	Error         string
}

type pendingRequest struct {
	generation uint64
	method     Method
	reply      chan Message
}

// Session owns RPC ordering and correlation on one immutable BLE link. The
// operation gate covers a whole point batch; Interrupt fences the next chunk
// without canceling a healthy native write halfway through Stop.
type Session struct {
	link          ble.Link
	limit         int
	gate          chan struct{}
	mu            sync.Mutex
	generation    uint64
	nextID        uint32
	recent        map[uint32]pendingRequest
	stream        uint32
	streamSet     bool
	clockSynced   bool
	playbackState string
	err           error
	quit          chan struct{}
	done          chan struct{}
	closed        chan struct{}
}

// NewSession attaches RPC correlation to an already-open immutable BLE link.
func NewSession(link ble.Link) (*Session, error) {
	limit := link.Device().MaxWriteBytes
	if limit < 48 {
		return nil, errors.New("bluetooth negotiated too small a write size for Handy HSP; reconnect with firmware 4 and an updated Bluetooth driver")
	}
	s := &Session{link: link, limit: min(limit, MaxFrameBytes), gate: make(chan struct{}, 1), recent: make(map[uint32]pendingRequest), quit: make(chan struct{}), done: make(chan struct{}), closed: make(chan struct{}), playbackState: "unknown"}
	go s.readEvents()
	return s, nil
}

// Initialize sends only Stop and optional clock commands. It never starts motion.
func (s *Session) Initialize(ctx context.Context) error {
	if _, err := s.Execute(ctx, Request{Method: StopCurrentMode}); err != nil {
		return err
	}
	lastWill, _ := Encode(Request{Method: StopCurrentMode, ID: math.MaxUint32})
	if err := s.link.SetLastWill(lastWill); err != nil {
		s.fail(err)
		return err
	}
	// Some firmware writes successfully but omits RPC responses. This probe is
	// optional, bounded, and never used as a readiness requirement.
	_ = s.syncClock(ctx)
	if err := ctx.Err(); err != nil {
		s.fail(err)
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Interrupt prevents further chunks or Play from an operation admitted before
// Stop. A native write already in progress has at most writeTimeout to finish.
func (s *Session) Interrupt() {
	s.mu.Lock()
	s.generation++
	s.streamSet = false
	s.playbackState = "unknown"
	s.mu.Unlock()
}

// Snapshot returns a copy of the current RPC session status.
func (s *Session) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := Snapshot{Ready: s.err == nil, ClockSynced: s.clockSynced, PlaybackState: s.playbackState}
	if s.err != nil {
		status.Error = s.err.Error()
	}
	return status
}

// Done closes when the event reader stops; Close also waits for link teardown.
func (s *Session) Done() <-chan struct{} { return s.done }

// Execute serializes one command, including all of an Add operation's chunks.
func (s *Session) Execute(ctx context.Context, request Request) (Message, error) {
	s.mu.Lock()
	generation := s.generation
	s.mu.Unlock()
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
	case <-ctx.Done():
		return Message{}, ctx.Err()
	case <-s.quit:
		return Message{}, ble.ErrLinkClosed
	}
	if err := s.valid(generation); err != nil {
		return Message{}, err
	}
	if request.Method == Add || request.Method == Play {
		s.mu.Lock()
		setup := !s.streamSet || s.stream != request.StreamID
		s.mu.Unlock()
		if setup {
			if _, err := s.rpc(ctx, generation, Request{Method: Setup, StreamID: request.StreamID}, false); err != nil {
				return Message{}, err
			}
			s.mu.Lock()
			s.stream, s.streamSet = request.StreamID, generation == s.generation
			s.mu.Unlock()
		}
	}
	if request.Method == Add {
		return s.add(ctx, generation, request)
	}
	if request.Method == Play {
		s.mu.Lock()
		synced := s.clockSynced
		s.playbackState = "play_requested"
		s.mu.Unlock()
		if synced {
			now := uint64(time.Now().UnixMilli())
			request.ServerTime = &now
		}
	}
	response, err := s.rpc(ctx, generation, request, request.Method == State || request.Method == ClockGet)
	if err == nil && (request.Method == Stop || request.Method == StopCurrentMode) {
		s.mu.Lock()
		s.streamSet = false
		s.playbackState = "stop_requested"
		s.mu.Unlock()
	}
	return response, err
}

func (s *Session) add(ctx context.Context, generation uint64, request Request) (Message, error) {
	if len(request.Points) == 0 || uint64(len(request.Points)) > uint64(request.Tail)+1 {
		return Message{}, errors.New("invalid Handy point tail")
	}
	first := uint64(request.Tail) + 1 - uint64(len(request.Points))
	var response Message
	for offset := 0; offset < len(request.Points); {
		count := min(maxChunkPoints, len(request.Points)-offset)
		chunk := request
		for {
			chunk.Points, chunk.Tail = request.Points[offset:offset+count], uint32(first+uint64(offset+count)-1) // #nosec G115 -- slice bounds and Tail validate 0 <= offset+count <= len(Points) <= Tail+1.
			chunk.Flush = request.Flush && offset == 0
			chunk.ID = math.MaxUint32 // fit even the largest real request ID
			encoded, err := Encode(chunk)
			if err != nil {
				return Message{}, err
			}
			if len(encoded) <= s.limit {
				break
			}
			count--
			if count == 0 {
				return Message{}, errors.New("handy point cannot fit the negotiated Bluetooth write size")
			}
		}
		var err error
		response, err = s.rpc(ctx, generation, chunk, false)
		if err != nil {
			return Message{}, err
		}
		offset += count
	}
	return response, nil
}

func (s *Session) valid(generation uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	if generation != s.generation {
		return ErrInterrupted
	}
	return nil
}

func (s *Session) fail(err error) {
	s.mu.Lock()
	if s.err != nil {
		s.mu.Unlock()
		return
	}
	s.err = err
	s.generation++
	close(s.quit)
	s.mu.Unlock()
	go func() { _ = s.link.Close(); close(s.closed) }()
}

// Close retires the session and waits for the worker and event reader.
func (s *Session) Close() error {
	s.fail(ble.ErrLinkClosed)
	<-s.closed
	<-s.done
	return nil
}

func (s *Session) readEvents() {
	defer close(s.done)
	for {
		select {
		case <-s.quit:
			return
		case event, ok := <-s.link.Events():
			if !ok {
				s.fail(ble.ErrLinkClosed)
				return
			}
			switch event.Kind {
			case ble.LinkLost, ble.LinkWriteFailed:
				s.fail(errors.New("the Handy Bluetooth link was lost"))
				return
			case ble.LinkNotification:
				message, err := Decode(event.Payload)
				if err != nil {
					continue
				} // bounded, possibly unrelated firmware notifications
				s.receive(message)
			}
		}
	}
}

func (s *Session) receive(message Message) {
	s.mu.Lock()
	request, known := s.recent[message.ID]
	current := known && request.generation == s.generation
	if !message.Notification && known && message.ErrorCode != nil && request.method != ClockGet {
		s.mu.Unlock()
		s.fail(fmt.Errorf("handy rejected Bluetooth RPC %d (code %d)", request.method, *message.ErrorCode))
		return
	}
	if !message.Notification && known {
		select {
		case request.reply <- message:
		default:
		}
	}
	if state := message.State; state != nil && s.streamSet && state.StreamID == s.stream && (message.Notification || current) {
		s.playbackState = state.PlayState
	}
	s.mu.Unlock()
}
