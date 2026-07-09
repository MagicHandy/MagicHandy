package parakeetworker

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// DefaultServerPort is the loopback port used by a managed parakeet-server
// when callers do not choose one explicitly.
const DefaultServerPort = 8990

const managedServerStderrBytes = 4096

// managedServer owns only the parakeet-server process it starts. It never
// reaches into an existing server, which keeps external and managed modes
// distinct and prevents unload from stopping a user-managed service.
type managedServer struct {
	path  string
	model string
	port  int

	mu      sync.Mutex
	command *exec.Cmd
	done    chan error
	stderr  *serverTail
}

func newManagedServer(path, model string, port int) *managedServer {
	if port == 0 {
		port = DefaultServerPort
	}
	return &managedServer{
		path:   strings.TrimSpace(path),
		model:  strings.TrimSpace(model),
		port:   port,
		stderr: newServerTail(managedServerStderrBytes),
	}
}

func (s *managedServer) BaseURL() string {
	return "http://127.0.0.1:" + strconv.Itoa(s.port)
}

// Start launches parakeet-server once. The worker then waits for its /health
// endpoint; command.Start succeeding alone does not mean model load succeeded.
func (s *managedServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runningLocked() {
		return nil
	}
	if info, err := os.Stat(s.path); err != nil || info.IsDir() {
		return fmt.Errorf("parakeet-server is unavailable: %s", s.path)
	}
	if info, err := os.Stat(s.model); err != nil || info.IsDir() {
		return fmt.Errorf("parakeet GGUF model is unavailable: %s", s.model)
	}
	if s.port < 1 || s.port > 65535 {
		return fmt.Errorf("managed ASR port %d is invalid", s.port)
	}
	if err := loopbackPortAvailable(s.port); err != nil {
		return fmt.Errorf("managed ASR port %d is unavailable: %w", s.port, err)
	}
	// A new process gets a fresh diagnostic tail so a retry cannot report stale
	// output from an earlier failed launch.
	s.stderr = newServerTail(managedServerStderrBytes)

	args := []string{
		"--model", s.model,
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(s.port),
	}
	// #nosec G204 -- runner and model paths are explicit local user settings.
	// Arguments are passed directly, never through a shell.
	command := exec.Command(s.path, args...)
	command.Dir = filepath.Dir(s.path)
	command.Stdout = s.stderr
	command.Stderr = s.stderr
	if err := command.Start(); err != nil {
		return fmt.Errorf("start parakeet-server: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- command.Wait()
	}()
	s.command = command
	s.done = done
	return nil
}

// Stop terminates the child process this instance owns. parakeet-server has no
// authenticated shutdown endpoint, so a direct process stop is both local and
// bounded. A pre-existing external server is never represented here.
func (s *managedServer) Stop() error {
	s.mu.Lock()
	command := s.command
	done := s.done
	s.command = nil
	s.done = nil
	s.mu.Unlock()

	if command == nil {
		return nil
	}
	if command.Process != nil {
		if err := command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
	}
	if done != nil {
		<-done
	}
	return nil
}

func (s *managedServer) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runningLocked()
}

func (s *managedServer) runningLocked() bool {
	if s.command == nil {
		return false
	}
	select {
	case err := <-s.done:
		s.command = nil
		s.done = nil
		if err != nil {
			s.stderr.WriteString(err.Error())
		}
		return false
	default:
		return true
	}
}

func (s *managedServer) failureMessage(cause error) error {
	if stderr := s.stderr.String(); stderr != "" {
		return fmt.Errorf("%w: %s", cause, stderr)
	}
	return cause
}

func loopbackPortAvailable(port int) error {
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return err
	}
	return listener.Close()
}

// serverTail is a bounded stderr/stdout buffer used only to make a managed-load
// error actionable. The runner command carries no credentials, and the buffer
// is never added to traces or ordinary diagnostics.
type serverTail struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func newServerTail(limit int) *serverTail {
	return &serverTail{limit: limit}
}

func (b *serverTail) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, data...)
	if len(b.data) > b.limit {
		b.data = b.data[len(b.data)-b.limit:]
	}
	return len(data), nil
}

func (b *serverTail) WriteString(value string) {
	_, _ = b.Write([]byte(value))
}

func (b *serverTail) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(string(b.data))
}
