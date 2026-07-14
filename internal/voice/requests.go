package voice

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	cleanup  func()

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

func (p *PendingRequest) dropInlineAudio() {
	p.mu.Lock()
	p.request.AudioB64 = ""
	p.mu.Unlock()
}

func (p *PendingRequest) releaseInput() {
	p.mu.Lock()
	p.request.AudioB64 = ""
	p.request.AudioRef = ""
	cleanup := p.cleanup
	p.cleanup = nil
	p.mu.Unlock()
	if cleanup != nil {
		cleanup()
	}
}

func (p *PendingRequest) setState(state string) {
	p.mu.Lock()
	if p.canceled && state != RequestStateCanceled {
		p.mu.Unlock()
		return
	}
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
	return s.submit(request, nil)
}

func (s *Supervisor) submit(request Request, cleanup func()) (*PendingRequest, error) {
	expected := RequestSpeak
	if s.role == RoleASR {
		expected = RequestTranscribe
	}
	if request.Type != expected {
		if cleanup != nil {
			cleanup()
		}
		return nil, fmt.Errorf("%s worker cannot handle %q requests", s.role, request.Type)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.queue == nil || s.state != StateRunning {
		if cleanup != nil {
			cleanup()
		}
		return nil, fmt.Errorf("%s worker is not running", s.role)
	}
	if s.lastHealth.ModelState != ModelStateReady {
		if cleanup != nil {
			cleanup()
		}
		return nil, fmt.Errorf("%s worker model is not ready", s.role)
	}

	s.nextID++
	pending := &PendingRequest{
		ID:        fmt.Sprintf("%s-%d", s.role, s.nextID),
		Role:      s.role,
		Type:      request.Type,
		state:     RequestStateQueued,
		createdAt: time.Now().UTC(),
		request:   request,
		cleanup:   cleanup,
	}
	pending.request.ID = pending.ID

	select {
	case s.queue <- pending:
		s.queued++
		return pending, nil
	default:
		pending.releaseInput()
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
		} else {
			pending.releaseInput()
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
	defer pending.releaseInput()
	responses, release, err := workerConn.register(pending.ID)
	if err != nil {
		pending.fail(&WorkerError{Code: ErrorCodeInternal, Message: err.Error()})
		return
	}
	defer release()

	pending.mu.Lock()
	request := pending.request
	pending.mu.Unlock()
	err = workerConn.send(request)
	// send serializes synchronously, so inline test audio no longer needs to be
	// retained in the core once the worker frame has been written.
	pending.dropInlineAudio()
	if err != nil {
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
	if pending.isCanceled() {
		switch response.Type {
		case ResponseTranscript, ResponseDone, ResponseCanceled, ResponseError:
			return true
		default:
			return false
		}
	}
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
	mu         sync.Mutex
	workers    map[Role]*Supervisor
	requests   []*PendingRequest
	stagingDir string
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

// PrepareTranscriptionStaging creates this manager's process-session directory
// and reaps only sessions old enough that no bounded ASR request can own them.
func (m *Manager) PrepareTranscriptionStaging(dataDir string) error {
	parent := filepath.Join(dataDir, "voice", "inputs")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create voice input staging: %w", err)
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return fmt.Errorf("inspect voice input staging: %w", err)
	}
	staleBefore := time.Now().Add(-5 * time.Minute)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "session-") {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.ModTime().Before(staleBefore) {
			_ = os.RemoveAll(filepath.Join(parent, entry.Name()))
		}
	}
	dir, err := os.MkdirTemp(parent, fmt.Sprintf("session-%d-*", os.Getpid()))
	if err != nil {
		return fmt.Errorf("create voice input session: %w", err)
	}
	// #nosec G302 -- this is a directory; owner execute permission is required
	// to reach the private 0600 capture files inside it.
	if err := os.Chmod(dir, 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return fmt.Errorf("protect voice input session: %w", err)
	}
	m.stagingDir = dir
	return nil
}

// SubmitTranscription stages raw browser audio as a private short-lived file
// and sends only its reference over the worker protocol. This keeps real audio
// below the protocol's NDJSON frame bound and avoids a second base64 copy.
func (m *Manager) SubmitTranscription(audio []byte, format, dataDir string) (*PendingRequest, error) {
	if m.stagingDir == "" {
		if err := m.PrepareTranscriptionStaging(dataDir); err != nil {
			return nil, err
		}
	}
	// Another process can reap an old empty session; recreate ours before use.
	if err := os.MkdirAll(m.stagingDir, 0o700); err != nil {
		return nil, fmt.Errorf("create voice input session: %w", err)
	}
	format = strings.TrimSpace(format)
	file, err := os.CreateTemp(m.stagingDir, "capture-*."+format)
	if err != nil {
		return nil, fmt.Errorf("stage voice input: %w", err)
	}
	path := file.Name()
	cleanup := func() { _ = os.Remove(path) }
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		cleanup()
		return nil, fmt.Errorf("protect voice input: %w", err)
	}
	if _, err := file.Write(audio); err != nil {
		_ = file.Close()
		cleanup()
		return nil, fmt.Errorf("stage voice input: %w", err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return nil, fmt.Errorf("stage voice input: %w", err)
	}
	return m.submitAndTrack(RoleASR, Request{
		Type: RequestTranscribe, AudioRef: path, AudioFormat: format,
	}, cleanup)

}

// Submit queues and tracks non-staged work atomically with respect to Stop
// invalidation.
func (m *Manager) Submit(role Role, request Request) (*PendingRequest, error) {
	return m.submitAndTrack(role, request, nil)
}

func (m *Manager) submitAndTrack(role Role, request Request, cleanup func()) (*PendingRequest, error) {
	m.mu.Lock()
	pending, err := m.workers[role].submit(request, cleanup)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	m.trackLocked(pending)
	m.mu.Unlock()
	return pending, nil
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
	m.trackLocked(pending)
}

func (m *Manager) trackLocked(pending *PendingRequest) {
	m.requests = append(m.requests, pending)
	for len(m.requests) > requestLogLimit {
		evict := -1
		for i, request := range m.requests {
			state := request.Snapshot().State
			if state != RequestStateQueued && state != RequestStateActive {
				evict = i
				break
			}
		}
		if evict < 0 {
			break
		}
		m.requests = append(m.requests[:evict], m.requests[evict+1:]...)
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

// InvalidateAll marks every visible request for a role canceled immediately
// and returns work that still needs a worker-side cancel frame. Emergency Stop
// uses this local phase before touching the transport so stale ASR results can
// no longer be consumed even if the worker is slow to accept cancellation.
func (m *Manager) InvalidateAll(role Role) []*PendingRequest {
	m.mu.Lock()
	defer m.mu.Unlock()

	active := make([]*PendingRequest, 0, len(m.requests))
	for _, pending := range m.requests {
		if pending.Role != role {
			continue
		}
		snapshot := pending.Snapshot()
		if snapshot.State == RequestStateQueued || snapshot.State == RequestStateActive {
			active = append(active, pending)
		}
		if snapshot.State != RequestStateFailed && snapshot.State != RequestStateCanceled {
			pending.markCanceled()
			pending.dropAudio()
			pending.releaseInput()
		}
	}
	return active
}

// CancelInvalidated sends worker-side cancellation after the caller has
// completed its higher-priority safety work.
func (m *Manager) CancelInvalidated(role Role, requests []*PendingRequest) {
	worker := m.workers[role]
	for _, pending := range requests {
		worker.Cancel(pending)
	}
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
	if m.stagingDir != "" {
		_ = os.RemoveAll(m.stagingDir)
	}
}
