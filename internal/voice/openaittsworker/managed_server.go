package openaittsworker

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const managedServerStderrBytes = 8 << 10
const managedServerStopTimeout = 3 * time.Second

type managedServer struct {
	path string
	args []string
	dir  string
	port int
	env  map[string]string

	mu      sync.Mutex
	command *exec.Cmd
	done    chan error
	output  *serverTail
}

func newManagedServer(path string, args []string, dir string, port int, env map[string]string) *managedServer {
	return &managedServer{
		path:   path,
		args:   append([]string(nil), args...),
		dir:    dir,
		port:   port,
		env:    cloneEnvironment(env),
		output: newServerTail(managedServerStderrBytes),
	}
}

func (s *managedServer) BaseURL() string {
	return "http://127.0.0.1:" + strconv.Itoa(s.port)
}

func (s *managedServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runningLocked() {
		return nil
	}
	info, err := os.Stat(s.path)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("TTS server command is unavailable: %s", s.path)
	}
	if s.dir != "" {
		info, err = os.Stat(s.dir)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("TTS server working directory is unavailable: %s", s.dir)
		}
	}
	if err := loopbackPortAvailable(s.port); err != nil {
		return fmt.Errorf("managed TTS port %d is unavailable: %w", s.port, err)
	}

	s.output = newServerTail(managedServerStderrBytes)
	// #nosec G204 -- command and argument paths are explicit local settings and
	// are passed directly. No shell parses or expands them.
	command := exec.Command(s.path, s.args...)
	if s.dir != "" {
		command.Dir = s.dir
	} else {
		command.Dir = filepath.Dir(s.path)
	}
	command.Env = appendEnvironment(os.Environ(), s.env)
	command.Stdout = s.output
	command.Stderr = s.output
	if err := command.Start(); err != nil {
		return fmt.Errorf("start TTS server: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- command.Wait()
	}()
	s.command = command
	s.done = done
	return nil
}

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
	var stopErr error
	if command.Process != nil {
		if err := command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			stopErr = err
		}
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(managedServerStopTimeout):
			stopErr = errors.Join(stopErr, errors.New("TTS server did not exit after termination"))
		}
	}
	return stopErr
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
			s.output.WriteString(err.Error())
		}
		return false
	default:
		return true
	}
}

func (s *managedServer) failureMessage(cause error) error {
	if output := s.output.String(); output != "" {
		return fmt.Errorf("%w: %s", cause, output)
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

func cloneEnvironment(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func appendEnvironment(base []string, values map[string]string) []string {
	if len(values) == 0 {
		return append([]string(nil), base...)
	}
	candidates := make([]string, 0, len(values))
	for key, value := range values {
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, '\x00') {
			continue
		}
		candidates = append(candidates, key)
	}
	sort.Strings(candidates)
	overrides := make(map[string]string, len(candidates))
	for _, key := range candidates {
		overrides[strings.ToUpper(key)] = key
	}
	keys := make([]string, 0, len(overrides))
	for _, key := range overrides {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make([]string, 0, len(base)+len(keys))
	for _, item := range base {
		key, _, found := strings.Cut(item, "=")
		if found {
			if _, replaced := overrides[strings.ToUpper(key)]; replaced {
				continue
			}
		}
		result = append(result, item)
	}
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}

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
