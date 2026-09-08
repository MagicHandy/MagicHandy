package voice

import (
	"errors"
	"time"
)

// AudioChunk is a bounded slice for progressive playback. The HTTP edge checks
// the live controller lease on every pull; cancellation is checked atomically
// with the copy so invalidated audio cannot be retrieved by a later request.
type AudioChunk struct {
	Data       []byte       `json:"data"`
	Format     string       `json:"format"`
	State      string       `json:"state"`
	Offset     int          `json:"offset"`
	NextOffset int          `json:"next_offset"`
	Done       bool         `json:"done"`
	Error      *WorkerError `json:"error,omitempty"`
}

// AudioChunk copies at most 32 KiB beginning at offset, with atomic lifecycle state.
func (p *PendingRequest) AudioChunk(offset int) (AudioChunk, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	chunk := AudioChunk{Format: p.audioFormat, State: p.state, Offset: offset, NextOffset: offset}
	if p.Type != RequestSpeak || p.Role != RoleTTS {
		return chunk, errors.New("request is not speech")
	}
	if p.canceled || p.state == RequestStateCanceled || p.state == RequestStateFailed {
		if p.failure != nil {
			failure := *p.failure
			chunk.Error = &failure
		}
		return chunk, nil
	}
	if offset < 0 || offset > len(p.audio) || (p.state == RequestStateDone && len(p.audio) == 0) {
		return chunk, errors.New("requested audio is no longer retained")
	}
	end := min(offset+32*1024, len(p.audio))
	chunk.Data = append([]byte(nil), p.audio[offset:end]...)
	chunk.NextOffset = end
	chunk.Done = p.state == RequestStateDone && end == len(p.audio)
	return chunk, nil
}

func elapsedMillis(start, end time.Time) int64 {
	if start.IsZero() || end.IsZero() {
		return 0
	}
	return max(0, end.Sub(start).Milliseconds())
}
