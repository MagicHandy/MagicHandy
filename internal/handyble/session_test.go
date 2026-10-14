package handyble

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/ble"
)

func testSession(t *testing.T, limit int, writeOnly bool) (*Session, *SimulatedLink) {
	t.Helper()
	c := NewSimulatedConnector()
	c.WriteLimit = limit
	c.WriteOnly = writeOnly
	link, err := c.Connect(t.Context(), ble.ConnectRequest{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSession(link)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, c.LastLink()
}

func requestFields(t *testing.T, data []byte) (uint32, Method, wireFields) {
	t.Helper()
	outer, err := parseFields(data)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := parseFields(outer.bytes(2))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fields {
		if f.wire == 2 {
			body, err := parseFields(f.data)
			if err != nil {
				t.Fatal(err)
			}
			id := fields.uint(2)
			if id > math.MaxUint32 {
				t.Fatal("invalid request ID")
				return 0, 0, nil
			}
			return uint32(id), Method(f.number), body
		}
	}
	t.Fatal("missing request")
	return 0, 0, nil
}

func TestSessionUsesNegotiatedMTUAndAbsoluteChunkTails(t *testing.T) {
	s, link := testSession(t, 64, false)
	if err := s.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	before := len(link.Writes())
	var points []Point
	for i := range 73 {
		points = append(points, Point{Time: uint32(4000000000 + i*50), Position: uint32(i % 101)})
	}
	if _, err := s.Execute(t.Context(), Request{Method: Add, StreamID: 8, Points: points, Tail: 72}); err != nil {
		t.Fatal(err)
	}
	count, chunks := 0, 0
	for _, data := range link.Writes()[before:] {
		if len(data) > 64 {
			t.Fatalf("frame exceeds negotiated MTU: %d", len(data))
		}
		_, method, body := requestFields(t, data)
		if method != Add {
			continue
		}
		chunks++
		for _, f := range body {
			if f.number != 1 {
				continue
			}
			p, _ := parseFields(f.data)
			if p.uint(1) != uint64(points[count].Time) || p.uint(2) != uint64(points[count].Position) {
				t.Fatal("point changed during chunking")
			}
			count++
		}
		if count < 1 {
			t.Fatal("empty chunk")
			return
		}
		if body.uint(3) != uint64(count-1) {
			t.Fatalf("wrong chunk tail %d for %d points", body.uint(3), count)
		}
	}
	if count != 73 || chunks < 2 {
		t.Fatalf("count=%d chunks=%d", count, chunks)
	}
	if _, err := s.Execute(t.Context(), Request{Method: Play, StreamID: 8}); err != nil {
		t.Fatal(err)
	}
	last := link.Writes()
	_, _, body := requestFields(t, last[len(last)-1])
	if body.uint(4) != 0 || body.uint(5) != 1 {
		t.Fatal("play must disable looping and pause on starvation")
	}
}

func TestWriteOnlyFirmwareRemainsUsableWithoutClockTimestamp(t *testing.T) {
	s, link := testSession(t, 512, true)
	if err := s.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if status := s.Snapshot(); !status.Ready || status.ClockSynced {
		t.Fatalf("status %+v", status)
	}
	start := time.Now()
	if _, err := s.Execute(t.Context(), Request{Method: Play, StreamID: 4}); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("write-only playback waited for firmware responses")
	}
	writes := link.Writes()
	_, _, body := requestFields(t, writes[len(writes)-1])
	for _, f := range body {
		if f.number == 2 {
			t.Fatal("invented a synchronized server time")
		}
	}
}

type blockedLink struct {
	ble.Link
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (l *blockedLink) Write(ctx context.Context, payload []byte, ack bool) error {
	fields, _ := parseFields(payload)
	request, _ := parseFields(fields.bytes(2))
	if request.bytes(uint32(Add)) != nil {
		l.once.Do(func() {
			close(l.entered)
			select {
			case <-l.release:
			case <-ctx.Done():
			}
		})
	}
	return l.Link.Write(ctx, payload, ack)
}

func TestStopFencesRemainingChunksAndKeepsHealthyLink(t *testing.T) {
	for _, cancelOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "stop", true: "cancel"}[cancelOnly], func(t *testing.T) {
			c := NewSimulatedConnector()
			link, _ := c.Connect(t.Context(), ble.ConnectRequest{})
			blocked := &blockedLink{Link: link, entered: make(chan struct{}), release: make(chan struct{})}
			s, err := NewSession(blocked)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			points := make([]Point, 60)
			for i := range points {
				points[i] = Point{Time: uint32(i * 100), Position: 50}
			}
			result := make(chan error, 1)
			go func() {
				_, err := s.Execute(ctx, Request{Method: Add, StreamID: 1, Points: points, Tail: 59})
				result <- err
			}()
			select {
			case <-blocked.entered:
			case <-time.After(time.Second):
				t.Fatal("write never started")
			}
			if cancelOnly {
				cancel()
			} else {
				s.Interrupt()
			}
			close(blocked.release)
			if err := <-result; err == nil || (!cancelOnly && !errors.Is(err, ErrInterrupted)) {
				t.Fatalf("interrupted add: %v", err)
			}
			if _, err := s.Execute(t.Context(), Request{Method: Stop}); err != nil {
				t.Fatal(err)
			}
			adds := 0
			var last Method
			for _, data := range c.LastLink().Writes() {
				_, method, _ := requestFields(t, data)
				last = method
				if method == Add {
					adds++
				}
			}
			if adds != 1 || last != Stop || !s.Snapshot().Ready {
				t.Fatalf("after Stop: adds=%d last=%d status=%+v", adds, last, s.Snapshot())
			}
		})
	}
}

func TestLateCodeOnlyRejectionRetiresSession(t *testing.T) {
	s, link := testSession(t, 512, true)
	response, err := s.Execute(t.Context(), Request{Method: Stroke, Min: 0, Max: 1})
	if err != nil {
		t.Fatal(err)
	}
	errorReply := appendBytes(appendUint(nil, 1, uint64(response.ID)), 2, []byte{})
	link.Emit(appendBytes(appendUint(nil, 1, 3), 4, errorReply))
	select {
	case <-s.Done():
	case <-time.After(time.Second):
		t.Fatal("late rejection did not retire link")
	}
	if _, err := s.Execute(t.Context(), Request{Method: Play, StreamID: 1}); err == nil {
		t.Fatal("retired session accepted Play")
	}
}
