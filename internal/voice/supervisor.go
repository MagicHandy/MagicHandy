package voice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// WorkerState is the lifecycle state shown to the API and UI.
type WorkerState string

const (
	// StateDisabled means voice is switched off in settings.
	StateDisabled WorkerState = "disabled"
	// StateNotConfigured means voice is enabled but no worker command is set.
	StateNotConfigured WorkerState = "not_configured"
	// StateStopped means the worker is configured but not running.
	StateStopped WorkerState = "stopped"
	// StateStarting means the process is up but the hello handshake is pending.
	StateStarting WorkerState = "starting"
	// StateRunning means the handshake succeeded and requests are accepted.
	StateRunning WorkerState = "running"
	// StateCrashed means the process exited without being asked to stop.
	StateCrashed WorkerState = "crashed"
)

// WorkerConfig is the per-role launch configuration derived from settings.
type WorkerConfig struct {
	Enabled bool
	Command string
	Args    []string
}

func (c WorkerConfig) equal(other WorkerConfig) bool {
	return c.Enabled == other.Enabled &&
		c.Command == other.Command &&
		strings.Join(c.Args, "\x00") == strings.Join(other.Args, "\x00")
}

// WorkerStatus is the JSON status snapshot for one worker. It never contains
// secrets: command lines are local user settings, stderr is capped, and no
// request payloads are included.
type WorkerStatus struct {
	Role            Role        `json:"role"`
	State           WorkerState `json:"state"`
	Configured      bool        `json:"configured"`
	Command         string      `json:"command,omitempty"`
	Provider        string      `json:"provider,omitempty"`
	ProviderVersion string      `json:"provider_version,omitempty"`
	ProtocolVersion int         `json:"protocol_version,omitempty"`
	Capabilities    []string    `json:"capabilities,omitempty"`
	ModelState      string      `json:"model_state,omitempty"`
	WorkerQueue     int         `json:"worker_queue_depth"`
	QueueDepth      int         `json:"queue_depth"`
	ActiveRequestID string      `json:"active_request_id,omitempty"`
	StartedAt       string      `json:"started_at,omitempty"`
	LastError       string      `json:"last_error,omitempty"`
	StderrTail      string      `json:"stderr_tail,omitempty"`
}

const (
	helloTimeout      = 5 * time.Second
	controlTimeout    = 5 * time.Second
	defaultJobTimeout = 60 * time.Second
	shutdownGrace     = 2 * time.Second
	queueCapacity     = 8
	stderrTailBytes   = 4096
)

// Supervisor owns one worker process: lifecycle, handshake, the serialized
// request queue, crash detection, and status. All methods are safe for
// concurrent use.
type Supervisor struct {
	role Role

	mu         sync.Mutex
	config     WorkerConfig
	state      WorkerState
	process    *exec.Cmd
	conn       *conn
	stderr     *tailBuffer
	hello      Response
	lastHealth Response
	lastError  string
	startedAt  time.Time
	stopping   bool
	queue      chan *PendingRequest
	queued     int
	activeID   string
	nextID     uint64
	exited     chan struct{}
}

// NewSupervisor creates a stopped supervisor for one worker role.
func NewSupervisor(role Role) *Supervisor {
	s := &Supervisor{role: role}
	s.state = s.idleStateLocked()
	return s
}

// SetConfig applies settings. A config change stops a running worker so the
// next start uses the new command; it never starts one implicitly.
func (s *Supervisor) SetConfig(config WorkerConfig) {
	config.Command = strings.TrimSpace(config.Command)

	s.mu.Lock()
	if s.config.equal(config) {
		s.mu.Unlock()
		return
	}
	s.config = config
	running := s.process != nil
	s.mu.Unlock()

	if running {
		_ = s.Stop(context.Background())
	}
	s.mu.Lock()
	if s.process == nil {
		s.state = s.idleStateLocked()
	}
	s.mu.Unlock()
}

// idleStateLocked is the state for "no process": disabled, not configured,
// or plain stopped. Crash state is preserved until a start or config change.
func (s *Supervisor) idleStateLocked() WorkerState {
	switch {
	case !s.config.Enabled:
		return StateDisabled
	case s.config.Command == "":
		return StateNotConfigured
	default:
		return StateStopped
	}
}

// Start launches the worker process and performs the hello handshake. It is
// an error if voice is disabled or no command is configured; a failed
// handshake tears the process back down.
func (s *Supervisor) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.process != nil {
		s.mu.Unlock()
		return nil
	}
	if !s.config.Enabled {
		s.mu.Unlock()
		return errors.New("voice is disabled in settings")
	}
	if s.config.Command == "" {
		s.mu.Unlock()
		return fmt.Errorf("no %s worker command is configured", s.role)
	}
	if info, err := os.Stat(s.config.Command); err != nil || info.IsDir() {
		s.mu.Unlock()
		return fmt.Errorf("%s worker command is unavailable: %s", s.role, s.config.Command)
	}

	stderr := newTailBuffer(stderrTailBytes)
	command, workerConn, err := s.spawn(stderr)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	exited := make(chan struct{})
	queue := make(chan *PendingRequest, queueCapacity)

	s.process = command
	s.conn = workerConn
	s.stderr = stderr
	s.state = StateStarting
	s.startedAt = time.Now().UTC()
	s.stopping = false
	s.lastError = ""
	s.hello = Response{}
	s.lastHealth = Response{}
	s.queue = queue
	s.queued = 0
	s.activeID = ""
	s.exited = exited
	s.mu.Unlock()

	go s.waitForExit(command, workerConn, exited)
	go s.dispatchLoop(workerConn, queue)

	hello, err := s.handshake(ctx, workerConn)
	if err != nil {
		// Tear down only a live process. If the worker is dying on its own
		// (the missing-dependency case), give the exit a moment to land so
		// waitForExit records the crashed state and stderr tail instead of
		// this path masking it as a deliberate stop.
		select {
		case <-exited:
		case <-time.After(500 * time.Millisecond):
			_ = s.Stop(context.Background())
		}
		s.mu.Lock()
		if s.lastError == "" {
			s.lastError = err.Error()
		}
		s.mu.Unlock()
		return err
	}

	s.mu.Lock()
	if s.conn == workerConn && s.process != nil {
		s.state = StateRunning
		s.hello = hello
	}
	s.mu.Unlock()
	return nil
}

// spawn launches the configured worker process with a protocol conn on its
// stdio. Called with s.mu held.
func (s *Supervisor) spawn(stderr *tailBuffer) (*exec.Cmd, *conn, error) {
	// #nosec G204 -- the worker command is an explicit local user setting and
	// is passed to exec without shell expansion (same trust model as the
	// managed llama.cpp runner path).
	command := exec.Command(s.config.Command, s.config.Args...)
	command.Stderr = stderr
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("open %s worker stdin: %w", s.role, err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("open %s worker stdout: %w", s.role, err)
	}
	if err := command.Start(); err != nil {
		return nil, nil, fmt.Errorf("start %s worker: %w", s.role, err)
	}
	return command, newConn(stdin, stdout), nil
}

func (s *Supervisor) handshake(ctx context.Context, workerConn *conn) (Response, error) {
	response, err := s.roundTrip(ctx, workerConn, Request{
		Type:            RequestHello,
		ProtocolVersion: ProtocolVersion,
	}, helloTimeout)
	if err != nil {
		return Response{}, fmt.Errorf("%s worker handshake: %w", s.role, err)
	}
	if response.Type == ResponseError {
		return Response{}, fmt.Errorf("%s worker handshake: %w", s.role, response.Error)
	}
	if response.ProtocolVersion != ProtocolVersion {
		return Response{}, fmt.Errorf("%s worker speaks protocol %d, core requires %d",
			s.role, response.ProtocolVersion, ProtocolVersion)
	}
	if response.Role != "" && response.Role != s.role {
		return Response{}, fmt.Errorf("worker reported role %q, expected %q", response.Role, s.role)
	}
	return response, nil
}

// Stop shuts the worker down gracefully: shutdown frame, a short grace
// window, then a hard kill. Stopping an idle supervisor is a no-op.
func (s *Supervisor) Stop(ctx context.Context) error {
	s.mu.Lock()
	process := s.process
	workerConn := s.conn
	exited := s.exited
	if process == nil {
		s.state = s.idleStateLocked()
		s.mu.Unlock()
		return nil
	}
	s.stopping = true
	s.mu.Unlock()

	_ = workerConn.send(Request{Type: RequestShutdown, ID: s.newRequestID()})

	select {
	case <-exited:
	case <-time.After(shutdownGrace):
		if process.Process != nil {
			_ = process.Process.Kill()
		}
		select {
		case <-exited:
		case <-ctx.Done():
			return ctx.Err()
		}
	case <-ctx.Done():
		if process.Process != nil {
			_ = process.Process.Kill()
		}
		<-exited
	}
	return nil
}

// Restart is stop-then-start with the current configuration.
func (s *Supervisor) Restart(ctx context.Context) error {
	if err := s.Stop(ctx); err != nil {
		return err
	}
	return s.Start(ctx)
}

// waitForExit is the crash detector: it reaps the process, fails the
// session, and records why. A deliberate Stop lands in the idle state; any
// other exit is visible as crashed with the stderr tail attached.
func (s *Supervisor) waitForExit(command *exec.Cmd, workerConn *conn, exited chan struct{}) {
	err := command.Wait()
	workerConn.closeWithError(errors.New("voice worker process exited"))

	s.mu.Lock()
	if s.conn == workerConn {
		queue := s.queue
		s.process = nil
		s.conn = nil
		s.queue = nil
		s.activeID = ""
		s.queued = 0
		if s.stopping {
			s.state = s.idleStateLocked()
		} else {
			s.state = StateCrashed
			reason := "voice worker exited unexpectedly"
			if err != nil {
				reason = fmt.Sprintf("voice worker exited unexpectedly: %v", err)
			}
			if tail := s.stderr.String(); tail != "" {
				reason += " — stderr: " + tail
			}
			s.lastError = reason
		}
		s.stopping = false
		if queue != nil {
			close(queue)
		}
	}
	s.mu.Unlock()
	close(exited)
}

// newRequestID allocates a process-unique request ID.
func (s *Supervisor) newRequestID() string {
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.mu.Unlock()
	return strconv.FormatUint(id, 10)
}

// roundTrip sends one unary request and waits for its terminal response.
func (s *Supervisor) roundTrip(ctx context.Context, workerConn *conn, request Request, timeout time.Duration) (Response, error) {
	if request.ID == "" {
		request.ID = s.newRequestID()
	}
	responses, release, err := workerConn.register(request.ID)
	if err != nil {
		return Response{}, err
	}
	defer release()

	if err := workerConn.send(request); err != nil {
		return Response{}, err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case response := <-responses:
			if response.Terminal() {
				return response, nil
			}
		case <-timer.C:
			return Response{}, fmt.Errorf("%s request timed out after %s", request.Type, timeout)
		case <-ctx.Done():
			return Response{}, ctx.Err()
		}
	}
}

// Health performs a live health round-trip and caches the result for status
// snapshots. It fails fast when the worker is not running.
func (s *Supervisor) Health(ctx context.Context) (Response, error) {
	s.mu.Lock()
	workerConn := s.conn
	s.mu.Unlock()
	if workerConn == nil {
		return Response{}, fmt.Errorf("%s worker is not running", s.role)
	}
	response, err := s.roundTrip(ctx, workerConn, Request{Type: RequestHealth}, controlTimeout)
	if err != nil {
		return Response{}, err
	}
	if response.Type == ResponseError {
		return Response{}, response.Error
	}
	s.mu.Lock()
	s.lastHealth = response
	s.mu.Unlock()
	return response, nil
}

// SetModelLoaded asks the worker to load or unload its model.
func (s *Supervisor) SetModelLoaded(ctx context.Context, loaded bool) (Response, error) {
	s.mu.Lock()
	workerConn := s.conn
	s.mu.Unlock()
	if workerConn == nil {
		return Response{}, fmt.Errorf("%s worker is not running", s.role)
	}
	requestType := RequestUnload
	if loaded {
		requestType = RequestLoad
	}
	response, err := s.roundTrip(ctx, workerConn, Request{Type: requestType}, controlTimeout)
	if err != nil {
		return Response{}, err
	}
	if response.Type == ResponseError {
		return Response{}, response.Error
	}
	s.mu.Lock()
	s.lastHealth = response
	s.mu.Unlock()
	return response, nil
}

// Status reports the current lifecycle snapshot without touching the worker.
func (s *Supervisor) Status() WorkerStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	status := WorkerStatus{
		Role:        s.role,
		State:       s.state,
		Configured:  s.config.Command != "",
		Command:     s.config.Command,
		QueueDepth:  s.queued,
		WorkerQueue: s.lastHealth.QueueDepth,
		ModelState:  s.lastHealth.ModelState,
		LastError:   s.lastError,
	}
	if s.activeID != "" {
		status.ActiveRequestID = s.activeID
		status.QueueDepth++
	}
	if s.hello.Provider != "" {
		status.Provider = s.hello.Provider
		status.ProviderVersion = s.hello.ProviderVersion
		status.ProtocolVersion = s.hello.ProtocolVersion
		status.Capabilities = s.hello.Capabilities
	}
	if !s.startedAt.IsZero() && s.process != nil {
		status.StartedAt = s.startedAt.Format(time.RFC3339)
	}
	if s.stderr != nil && s.state == StateCrashed {
		status.StderrTail = s.stderr.String()
	}
	return status
}

// Shutdown stops the worker without reporting an error; used at app close.
func (s *Supervisor) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace+time.Second)
	defer cancel()
	_ = s.Stop(ctx)
}

// tailBuffer keeps the last N bytes of worker stderr for diagnostics.
type tailBuffer struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func newTailBuffer(limit int) *tailBuffer {
	return &tailBuffer{limit: limit}
}

func (b *tailBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, data...)
	if len(b.data) > b.limit {
		b.data = b.data[len(b.data)-b.limit:]
	}
	return len(data), nil
}

func (b *tailBuffer) String() string {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(string(b.data))
}
