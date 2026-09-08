package voice

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func TestLateWorkerResponsesCannotUndoCancellation(t *testing.T) {
	for _, terminal := range []string{RequestStateCanceled, RequestStateFailed, RequestStateDone} {
		t.Run(terminal, func(t *testing.T) {
			p := &PendingRequest{Type: RequestSpeak, state: terminal}
			if err := p.appendAudio(Response{AudioFormat: "mp3", AudioB64: "AQI="}); err != nil {
				t.Fatal(err)
			}
			if err := p.completeAudio(); err != nil {
				t.Fatal(err)
			}
			p.fail(&WorkerError{Code: ErrorCodeInternal, Message: "late error"})
			p.timeOut(&WorkerError{Code: ErrorCodeTimeout, Message: "late timeout"})
			if snap := p.Snapshot(); snap.State != terminal || snap.AudioBytes != 0 || snap.Error != nil {
				t.Fatalf("terminal audio changed: %+v", snap)
			}
			p = &PendingRequest{Type: RequestTranscribe, state: RequestStateActive}
			// The response passed its initial check before Stop invalidated it.
			p.invalidate()
			if err := p.completeTranscript(Response{Candidates: []TranscriptCandidate{{Text: "stale", Confidence: 1}}}); err != nil {
				t.Fatal(err)
			}
			if snap := p.Snapshot(); snap.State != RequestStateCanceled || len(snap.Transcript) != 0 {
				t.Fatalf("canceled transcript changed: %+v", snap)
			}
		})
	}
}

func TestControlDeadlineClosesBlockedStdin(t *testing.T) {
	input, writer := io.Pipe()
	output, reader := io.Pipe()
	defer func() { _ = input.Close() }()
	defer func() { _ = reader.Close() }()
	defer func() { _ = output.Close() }()
	c := newConn(writer, output)
	s := NewSupervisor(RoleTTS)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, err := s.roundTrip(ctx, c, Request{Type: RequestHealth}, time.Second); finished <- err }()
	select {
	case err := <-finished:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("blocked stdin exceeded the control deadline")
	}
	if !c.isClosed() {
		t.Fatal("partially written session remains usable")
	}
}

func TestConnectionTeardownDoesNotWaitForFullResponseQueue(t *testing.T) {
	c := &conn{pending: make(map[string]*responseSink), done: make(chan struct{})}
	_, release, err := c.register("slow")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	for i := 0; i < 64; i++ {
		c.dispatch(Response{RequestID: "slow"})
	}
	finished := make(chan struct{})
	go func() { c.closeWithError(io.EOF); close(finished) }()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("teardown blocked behind response consumer")
	}
}

type partialVoiceWriter struct{ err error }

func (w partialVoiceWriter) Write([]byte) (int, error) { return 1, w.err }
func (partialVoiceWriter) Close() error                { return nil }

func TestPartialFrameRetiresConnection(t *testing.T) {
	for _, writeErr := range []error{nil, io.ErrUnexpectedEOF} {
		c := &conn{writer: partialVoiceWriter{err: writeErr}, writeGate: make(chan struct{}, 1), done: make(chan struct{})}
		if err := c.sendContext(context.Background(), Request{Type: RequestHealth}); err == nil || !c.isClosed() {
			t.Fatalf("partial write left session usable: err=%v closed=%v", err, c.isClosed())
		}
		if err := c.sendContext(context.Background(), Request{Type: RequestHealth}); !errors.Is(err, errConnClosed) {
			t.Fatalf("next write did not reject a corrupted session: %v", err)
		}
	}
}
