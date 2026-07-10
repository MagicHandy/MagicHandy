package voice

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strconv"
	"sync"
	"time"
)

// Audio retention bounds: completed speak audio is kept in memory for the
// lease-gated playback endpoint, capped per request and to the most recent
// few requests so retained audio cannot grow without bound.
const (
	maxRetainedAudioBytes = 2 << 20 // per request
	audioRetainCount      = 4       // most recent requests keeping audio
)

// Request lifecycle states shown to the API and UI.
const (
	RequestStateQueued   = "queued"
	RequestStateActive   = "active"
	RequestStateDone     = "done"
	RequestStateCanceled = "canceled"
	RequestStateFailed   = "failed"
)

// PendingRequest tracks one queued or completed work request. Snapshots and
// logs carry only metadata (chunk counts, byte counts, transcript text);
// retained audio is served exclusively through the lease-gated audio
// endpoint and is bounded by the retention constants above.
type PendingRequest struct {
	mu sync.Mutex

	ID        string
	Role      Role
	Type      string
	state     string
	createdAt time.Time

	request  Request
	canceled bool

	chunks         int
	audio          []byte
	audioFormat    string
	audioTruncated bool
	transcript     []TranscriptCandidate
	rejected       string
	failure        *WorkerError
}

// RequestSnapshot is the JSON view of one request's progress.
type RequestSnapshot struct {
	ID             string                `json:"id"`
	Role           Role                  `json:"role"`
	Type           string                `json:"type"`
	State          string                `json:"state"`
	CreatedAt      string                `json:"created_at"`
	AudioChunks    int                   `json:"audio_chunks,omitempty"`
	AudioBytes     int                   `json:"audio_bytes,omitempty"`
	AudioTruncated bool                  `json:"audio_truncated,omitempty"`
	Transcript     []TranscriptCandidate `json:"transcript,omitempty"`
	Rejected       string                `json:"rejected,omitempty"`
	Error          *WorkerError          `json:"error,omitempty"`
}

// Snapshot returns a copy safe to serialize.
func (p *PendingRequest) Snapshot() RequestSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	return RequestSnapshot{
		ID:             p.ID,
		Role:           p.Role,
		Type:           p.Type,
		State:          p.state,
		CreatedAt:      p.createdAt.Format(time.RFC3339),
		AudioChunks:    p.chunks,
		AudioBytes:     len(p.audio),
		AudioTruncated: p.audioTruncated,
		Transcript:     p.transcript,
		Rejected:       p.rejected,
		Error:          p.failure,
	}
}

// Text returns the submitted speak text (empty for other request types).
// The speak text is the same reply already visible in the chat log, so
// exposing it to in-process callers reveals nothing new; it still never
// enters status snapshots or logs.
func (p *PendingRequest) Text() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.request.Text
}

// Audio returns a copy of the retained audio and its format ("" when none
// is retained). Raw audio never appears in snapshots, traces, or logs.
func (p *PendingRequest) Audio() ([]byte, string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.audio) == 0 {
		return nil, ""
	}
	audio := make([]byte, len(p.audio))
	copy(audio, p.audio)
	if p.audioFormat == "pcm_s16le_24000" {
		return pcmS16LEToWAV(audio, 24000), "wav"
	}
	return audio, p.audioFormat
}

func pcmS16LEToWAV(pcm []byte, sampleRate uint32) []byte {
	wav := make([]byte, 44+len(pcm))
	copy(wav[0:4], "RIFF")
	// #nosec G115 -- retained request audio is capped at 2 MiB.
	binary.LittleEndian.PutUint32(wav[4:8], uint32(36+len(pcm)))
	copy(wav[8:12], "WAVE")
	copy(wav[12:16], "fmt ")
	binary.LittleEndian.PutUint32(wav[16:20], 16)
	binary.LittleEndian.PutUint16(wav[20:22], 1)
	binary.LittleEndian.PutUint16(wav[22:24], 1)
	binary.LittleEndian.PutUint32(wav[24:28], sampleRate)
	binary.LittleEndian.PutUint32(wav[28:32], sampleRate*2)
	binary.LittleEndian.PutUint16(wav[32:34], 2)
	binary.LittleEndian.PutUint16(wav[34:36], 16)
	copy(wav[36:40], "data")
	// #nosec G115 -- retained request audio is capped at 2 MiB.
	binary.LittleEndian.PutUint32(wav[40:44], uint32(len(pcm)))
	copy(wav[44:], pcm)
	return wav
}

func (p *PendingRequest) dropAudio() {
	p.mu.Lock()
	p.audio = nil
	p.mu.Unlock()
}

func (p *PendingRequest) setState(state string) {
	p.mu.Lock()
	p.state = state
	p.mu.Unlock()
}

func (p *PendingRequest) markCanceled() {
	p.mu.Lock()
	p.canceled = true
	p.state = RequestStateCanceled
	p.mu.Unlock()
}

func (p *PendingRequest) isCanceled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.canceled
}

func (p *PendingRequest) fail(err *WorkerError) {
	p.mu.Lock()
	p.state = RequestStateFailed
	p.failure = err
	p.mu.Unlock()
}

// Submit queues one speak/transcribe request. The queue is bounded and
// rejects new work when full (catch-up flooding protection; a configurable
// drop-oldest policy arrives with real audio playback).
func (s *Supervisor) Submit(request Request) (*PendingRequest, error) {
	expected := RequestSpeak
	if s.role == RoleASR {
		expected = RequestTranscribe
	}
	if request.Type != expected {
		return nil, fmt.Errorf("%s worker cannot handle %q requests", s.role, request.Type)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.queue == nil || s.state != StateRunning {
		return nil, fmt.Errorf("%s worker is not running", s.role)
	}

	s.nextID++
	pending := &PendingRequest{
		ID:        strconv.FormatUint(s.nextID, 10),
		Role:      s.role,
		Type:      request.Type,
		state:     RequestStateQueued,
		createdAt: time.Now().UTC(),
		request:   request,
	}
	pending.request.ID = pending.ID

	select {
	case s.queue <- pending:
		s.queued++
		return pending, nil
	default:
		return nil, fmt.Errorf("%s worker queue is full", s.role)
	}
}

// Cancel cancels a queued or active request by ID and tells the worker.
func (s *Supervisor) Cancel(pending *PendingRequest) {
	pending.markCanceled()

	s.mu.Lock()
	workerConn := s.conn
	s.mu.Unlock()
	if workerConn != nil {
		_ = workerConn.send(Request{
			Type:     RequestCancel,
			ID:       s.newRequestID(),
			TargetID: pending.ID,
		})
	}
}

// dispatchLoop serializes work requests to the worker. It exits when the
// queue closes (process exit or stop), failing whatever was still queued.
func (s *Supervisor) dispatchLoop(workerConn *conn, queue chan *PendingRequest) {
	for pending := range queue {
		s.mu.Lock()
		s.queued--
		if !pending.isCanceled() {
			s.activeID = pending.ID
		}
		s.mu.Unlock()

		if !pending.isCanceled() {
			s.execute(workerConn, pending)
		}

		s.mu.Lock()
		s.activeID = ""
		s.mu.Unlock()
	}
}

// execute runs one request to its terminal frame, applying the job timeout.
// Model errors stay in the request record — they never propagate into chat
// history, TTS playback, or motion (ADR 0003).
func (s *Supervisor) execute(workerConn *conn, pending *PendingRequest) {
	responses, release, err := workerConn.register(pending.ID)
	if err != nil {
		pending.fail(&WorkerError{Code: ErrorCodeInternal, Message: err.Error()})
		return
	}
	defer release()

	if err := workerConn.send(pending.request); err != nil {
		pending.fail(&WorkerError{Code: ErrorCodeInternal, Message: err.Error()})
		return
	}
	pending.setState(RequestStateActive)

	timer := time.NewTimer(defaultJobTimeout)
	defer timer.Stop()
	for {
		select {
		case response := <-responses:
			if s.applyResponse(pending, response) {
				return
			}
		case <-timer.C:
			s.Cancel(pending)
			pending.fail(&WorkerError{
				Code:    ErrorCodeTimeout,
				Message: fmt.Sprintf("%s request timed out after %s", pending.Type, defaultJobTimeout),
			})
			return
		}
	}
}

// applyResponse folds one frame into the request record; true means terminal.
func (s *Supervisor) applyResponse(pending *PendingRequest, response Response) bool {
	switch response.Type {
	case ResponseAudioChunk:
		pending.mu.Lock()
		pending.chunks++
		if data, err := base64.StdEncoding.DecodeString(response.AudioB64); err == nil && len(data) > 0 {
			if pending.audioFormat == "" {
				pending.audioFormat = response.AudioFormat
			}
			if len(pending.audio)+len(data) <= maxRetainedAudioBytes {
				pending.audio = append(pending.audio, data...)
			} else {
				pending.audioTruncated = true
			}
		}
		pending.mu.Unlock()
		return false
	case ResponseTranscript:
		pending.mu.Lock()
		pending.transcript = response.Candidates
		pending.rejected = response.Rejected
		pending.state = RequestStateDone
		pending.mu.Unlock()
		return true
	case ResponseDone:
		pending.setState(RequestStateDone)
		return true
	case ResponseCanceled:
		pending.markCanceled()
		return true
	case ResponseError:
		workerErr := response.Error
		if workerErr == nil {
			workerErr = &WorkerError{Code: ErrorCodeInternal, Message: "worker reported an error"}
		}
		pending.fail(workerErr)
		return true
	default:
		return false
	}
}

// Manager owns the per-role supervisors and a bounded log of recent
// requests. It is the single voice entry point for the HTTP edge.
type Manager struct {
	mu       sync.Mutex
	workers  map[Role]*Supervisor
	requests []*PendingRequest
}

// requestLogLimit bounds the recent-request log used by the status API.
const requestLogLimit = 32

// NewManager creates supervisors for every role, all idle.
func NewManager() *Manager {
	workers := make(map[Role]*Supervisor, len(Roles()))
	for _, role := range Roles() {
		workers[role] = NewSupervisor(role)
	}
	return &Manager{workers: workers}
}

// Config is the manager-level configuration derived from settings.
type Config struct {
	TTS WorkerConfig
	ASR WorkerConfig
}

// Configure applies settings to both supervisors.
func (m *Manager) Configure(config Config) {
	m.workers[RoleTTS].SetConfig(config.TTS)
	m.workers[RoleASR].SetConfig(config.ASR)
}

// Worker returns the supervisor for a role.
func (m *Manager) Worker(role Role) *Supervisor {
	return m.workers[role]
}

// Status returns both workers' snapshots keyed by role.
func (m *Manager) Status() map[Role]WorkerStatus {
	status := make(map[Role]WorkerStatus, len(m.workers))
	for role, worker := range m.workers {
		status[role] = worker.Status()
	}
	return status
}

// Track adds a request to the recent-request log, evicting the oldest and
// dropping retained audio beyond the newest few (metadata stays visible).
func (m *Manager) Track(pending *PendingRequest) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, pending)
	if len(m.requests) > requestLogLimit {
		m.requests = m.requests[len(m.requests)-requestLogLimit:]
	}
	for i := 0; i < len(m.requests)-audioRetainCount; i++ {
		m.requests[i].dropAudio()
	}
}

// Request finds a tracked request by ID.
func (m *Manager) Request(id string) (*PendingRequest, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, pending := range m.requests {
		if pending.ID == id {
			return pending, true
		}
	}
	return nil, false
}

// Requests lists tracked request snapshots, newest first.
func (m *Manager) Requests() []RequestSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	snapshots := make([]RequestSnapshot, 0, len(m.requests))
	for i := len(m.requests) - 1; i >= 0; i-- {
		snapshots = append(snapshots, m.requests[i].Snapshot())
	}
	return snapshots
}

// Shutdown stops every worker; called at app close.
func (m *Manager) Shutdown() {
	for _, worker := range m.workers {
		worker.Shutdown()
	}
}
