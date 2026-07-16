package llm

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	managedLlamaLoadTimeout   = 30 * time.Second
	managedLlamaUnloadTimeout = 10 * time.Second
)

// ManagedLlamaCPPOptions configures a managed llama-server process.
type ManagedLlamaCPPOptions struct {
	HTTPProviderOptions
	RunnerPath string
	ModelPath  string
}

// ManagedLlamaCPPProvider starts and owns one configured llama-server process.
type ManagedLlamaCPPProvider struct {
	baseURL    string
	model      string
	runnerPath string
	modelPath  string
	client     *LlamaCPPProvider

	mu       sync.Mutex
	process  *exec.Cmd
	done     chan error
	stderr   *tailBuffer
	ready    bool
	stopping bool
}

// NewManagedLlamaCPPProvider creates a managed llama.cpp provider.
func NewManagedLlamaCPPProvider(options ManagedLlamaCPPOptions) (*ManagedLlamaCPPProvider, error) {
	httpOptions, err := normalizeHTTPOptions(options.HTTPProviderOptions)
	if err != nil {
		return nil, err
	}
	client, err := NewLlamaCPPProvider(httpOptions)
	if err != nil {
		return nil, err
	}
	return &ManagedLlamaCPPProvider{
		baseURL:    httpOptions.BaseURL,
		model:      httpOptions.Model,
		runnerPath: strings.TrimSpace(options.RunnerPath),
		modelPath:  strings.TrimSpace(options.ModelPath),
		client:     client,
		stderr:     newTailBuffer(4096),
	}, nil
}

// StreamChat loads the configured runner if needed and streams chat text.
func (p *ManagedLlamaCPPProvider) StreamChat(ctx context.Context, request ChatRequest, onDelta func(string) error) (string, error) {
	// Load performs health and model-list probes. Reuse that successful state on
	// warm calls; the chat request itself will still report a crashed server.
	if !p.readyToServe() {
		status := p.Load(ctx)
		if !status.Available {
			return "", errors.New(status.Message)
		}
	}
	raw, err := p.client.StreamChat(ctx, request, onDelta)
	if err != nil && !errors.Is(err, ErrOutputTruncated) && ctx.Err() == nil {
		p.setReady(false)
	}
	return raw, err
}

// Status returns managed runner state without starting a process.
func (p *ManagedLlamaCPPProvider) Status(ctx context.Context) ProviderStatus {
	status := p.baseStatus()
	if message := p.setupMessage(); message != "" {
		status.Message = message
		return status
	}
	if !p.running() {
		status.Message = "llama.cpp runner is not loaded"
		return status
	}
	providerStatus := p.client.Status(ctx)
	providerStatus.Managed = true
	providerStatus.Loaded = true
	if !providerStatus.Available && providerStatus.Message == "" {
		providerStatus.Message = p.stderrMessage("llama.cpp runner is not ready")
	}
	return providerStatus
}

// Load starts the configured llama-server process and waits for readiness.
func (p *ManagedLlamaCPPProvider) Load(ctx context.Context) ProviderStatus {
	status := p.baseStatus()
	if message := p.setupMessage(); message != "" {
		status.Message = message
		return status
	}
	if err := p.ensureStarted(); err != nil {
		status.Message = err.Error()
		return status
	}

	deadline := time.Now().Add(managedLlamaLoadTimeout)
	for {
		providerStatus := p.client.Status(ctx)
		providerStatus.Managed = true
		providerStatus.Loaded = p.running()
		if providerStatus.Available {
			p.setReady(true)
			return providerStatus
		}
		if time.Now().After(deadline) {
			if providerStatus.Message == "" {
				providerStatus.Message = p.stderrMessage("llama.cpp runner did not become ready")
			}
			return providerStatus
		}
		if err := ctx.Err(); err != nil {
			providerStatus.Message = err.Error()
			return providerStatus
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			providerStatus.Message = ctx.Err().Error()
			return providerStatus
		case <-timer.C:
		}
	}
}

// Unload stops the managed llama-server process.
func (p *ManagedLlamaCPPProvider) Unload(ctx context.Context) ProviderStatus {
	status := p.baseStatus()
	if ctx == nil {
		ctx = context.Background()
	}
	waitCtx, cancel := context.WithTimeout(ctx, managedLlamaUnloadTimeout)
	defer cancel()

	p.mu.Lock()
	defer p.mu.Unlock()
	process := p.process
	done := p.done
	p.ready = false

	if process == nil || process.Process == nil {
		status.Message = "llama.cpp runner is not loaded"
		status.Loaded = false
		return status
	}
	p.stopping = true
	if err := process.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		p.stopping = false
		status.Message = err.Error()
		return status
	}
	if done != nil {
		select {
		case <-done:
			p.process = nil
			p.done = nil
			p.stopping = false
		case <-waitCtx.Done():
			status.Loaded = false
			status.Message = "llama.cpp runner termination timed out: " + waitCtx.Err().Error()
			return status
		}
	} else {
		p.process = nil
		p.stopping = false
	}
	status.Loaded = false
	status.Message = "unloaded"
	return status
}

// Close releases the managed process.
func (p *ManagedLlamaCPPProvider) Close() error {
	status := p.Unload(context.Background())
	if status.Message != "" && status.Message != "unloaded" && status.Message != "llama.cpp runner is not loaded" {
		return errors.New(status.Message)
	}
	return nil
}

func (p *ManagedLlamaCPPProvider) baseStatus() ProviderStatus {
	return ProviderStatus{
		Provider: llamaCPPProviderName,
		BaseURL:  p.baseURL,
		Model:    p.model,
		Managed:  true,
		Loaded:   p.running(),
	}
}

func (p *ManagedLlamaCPPProvider) setupMessage() string {
	if p.runnerPath == "" {
		return "managed llama.cpp runtime is not installed"
	}
	if p.modelPath == "" {
		return "managed llama.cpp requires a selected managed model"
	}
	if info, err := os.Stat(p.runnerPath); err != nil || info.IsDir() {
		return fmt.Sprintf("llama-server runner is unavailable: %s", p.runnerPath)
	}
	if info, err := os.Stat(p.modelPath); err != nil || info.IsDir() {
		return fmt.Sprintf("GGUF model is unavailable: %s", p.modelPath)
	}
	return ""
}

func (p *ManagedLlamaCPPProvider) ensureStarted() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.runningLocked() {
		return nil
	}
	if p.process != nil {
		return errors.New("llama.cpp runner is still stopping")
	}
	return p.startLocked()
}

func (p *ManagedLlamaCPPProvider) startLocked() error {
	host, port, err := llamaHostPort(p.baseURL)
	if err != nil {
		return err
	}
	args := []string{
		"--host", host,
		"--port", strconv.Itoa(port),
		"--alias", p.model,
		"--offline",
		"--no-ui",
		"-m", p.modelPath,
	}

	// #nosec G204 -- runner/model paths were validated beneath app-owned stores
	// and are passed directly to exec without shell expansion.
	command := exec.Command(p.runnerPath, args...)
	command.Dir = filepath.Dir(p.runnerPath)
	command.Stderr = p.stderr
	command.Stdout = p.stderr
	if err := command.Start(); err != nil {
		return fmt.Errorf("start llama.cpp runner: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- command.Wait()
	}()

	p.process = command
	p.done = done
	p.ready = false
	p.stopping = false
	return nil
}

func (p *ManagedLlamaCPPProvider) readyToServe() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.runningLocked() && p.ready
}

func (p *ManagedLlamaCPPProvider) setReady(ready bool) {
	p.mu.Lock()
	p.ready = ready
	p.mu.Unlock()
}

func (p *ManagedLlamaCPPProvider) running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.runningLocked()
}

func (p *ManagedLlamaCPPProvider) runningLocked() bool {
	if p.process == nil {
		return false
	}
	select {
	case err := <-p.done:
		p.process = nil
		p.done = nil
		p.ready = false
		if err != nil && !p.stopping {
			p.stderr.WriteString(err.Error())
		}
		p.stopping = false
		return false
	default:
		return !p.stopping
	}
}

func (p *ManagedLlamaCPPProvider) stderrMessage(fallback string) string {
	if stderr := p.stderr.String(); stderr != "" {
		return fallback + ": " + stderr
	}
	return fallback
}

func llamaHostPort(baseURL string) (string, int, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", 0, fmt.Errorf("parse llama.cpp base URL: %w", err)
	}
	host := parsed.Hostname()
	portText := parsed.Port()
	if host == "" {
		return "", 0, fmt.Errorf("llama.cpp base URL %q must include a host", baseURL)
	}
	if parsed.Scheme != "http" {
		return "", 0, errors.New("managed llama.cpp requires an http base URL")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", 0, errors.New("managed llama.cpp base URL must not include a path")
	}
	ip := net.ParseIP(host)
	if !strings.EqualFold(host, "localhost") && (ip == nil || !ip.IsLoopback()) && (ip == nil || !ip.IsUnspecified()) {
		return "", 0, errors.New("managed llama.cpp must bind to a loopback address")
	}
	if portText == "" {
		switch parsed.Scheme {
		case "http":
			portText = "80"
		case "https":
			portText = "443"
		default:
			return "", 0, fmt.Errorf("llama.cpp base URL %q must include a port", baseURL)
		}
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("llama.cpp base URL %q has invalid port", baseURL)
	}
	if ip != nil && ip.IsUnspecified() {
		host = "127.0.0.1"
	}
	return host, port, nil
}

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
		start := len(b.data) - b.limit
		for start < len(b.data) && !utf8.RuneStart(b.data[start]) {
			start++
		}
		b.data = b.data[start:]
	}
	return len(data), nil
}

func (b *tailBuffer) WriteString(value string) {
	_, _ = b.Write([]byte(value))
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(string(b.data))
}
