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
)

const managedLlamaLoadTimeout = 90 * time.Second

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

	mu      sync.Mutex
	process *exec.Cmd
	done    chan error
	stderr  *tailBuffer
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
	status := p.Load(ctx)
	if !status.Available {
		return "", errors.New(status.Message)
	}
	return p.client.StreamChat(ctx, request, onDelta)
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
		time.Sleep(250 * time.Millisecond)
	}
}

// Unload stops the managed llama-server process.
func (p *ManagedLlamaCPPProvider) Unload(context.Context) ProviderStatus {
	status := p.baseStatus()
	p.mu.Lock()
	process := p.process
	p.process = nil
	done := p.done
	p.done = nil
	p.mu.Unlock()

	if process == nil || process.Process == nil {
		status.Message = "llama.cpp runner is not loaded"
		return status
	}
	if err := process.Process.Kill(); err != nil {
		status.Message = err.Error()
		return status
	}
	if done != nil {
		<-done
	}
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
		return "managed llama.cpp requires a llama-server runner path"
	}
	if p.modelPath == "" {
		return "managed llama.cpp requires a GGUF model path"
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
		"-m", p.modelPath,
	}
	if alias := strings.TrimSpace(p.model); alias != "" {
		args = append(args, "-a", alias)
	}

	// #nosec G204 -- runner path/model path are explicit local user settings and
	// are passed directly to exec without shell expansion.
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
	return nil
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
		if err != nil {
			p.stderr.WriteString(err.Error())
		}
		return false
	default:
		return true
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
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
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
		b.data = b.data[len(b.data)-b.limit:]
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
