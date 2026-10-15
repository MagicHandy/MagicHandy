// Package workerclient starts the shared native Bluetooth worker process and adapts
// it to ble.Connector and ble.Link. The worker owns the radio; this side
// owns its lifetime, so a hung or crashed worker never outlives the link.
package workerclient

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/ble"
	"github.com/mapledaemon/MagicHandy/internal/ble/workerproto"
	"github.com/mapledaemon/MagicHandy/internal/processtree"
)

const (
	helloTimeout      = 5 * time.Second
	disconnectTimeout = time.Second
	exitTimeout       = 2 * time.Second
	connectSlack      = 3 * time.Second
	stderrLineBytes   = 4096
)

// Options locates the worker executable.
type Options struct {
	// Path is the worker executable. An empty path means the helper is
	// missing from this installation.
	Profile string
	Path    string
	Args    []string
	Logger  *slog.Logger
}

// Connector starts one worker process per connection.
type Connector struct {
	options Options
}

var _ ble.Connector = (*Connector)(nil)

// NewConnector returns a worker-backed connector.
func NewConnector(options Options) *Connector {
	if options.Logger == nil {
		options.Logger = slog.New(slog.DiscardHandler)
	}
	return &Connector{options: options}
}

// Simulated reports that links reach a real radio.
func (c *Connector) Simulated() bool {
	return false
}

// Connect starts the worker, checks its protocol version and asks it to find
// and connect the Bluetooth device. Any failure stops the worker again.
func (c *Connector) Connect(ctx context.Context, request ble.ConnectRequest) (ble.Link, error) {
	if strings.TrimSpace(c.options.Path) == "" {
		return nil, ble.ErrHelperMissing
	}
	proc, err := startProcess(c.options)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ble.ErrHelperMissing, err)
	}
	link := &workerLink{session: proc.session, process: proc}
	device, err := link.open(ctx, request, c.options.Profile)
	if err != nil {
		link.stop()
		return nil, err
	}
	link.device = device
	return link, nil
}

type workerLink struct {
	session *session
	process *process
	device  ble.DeviceInfo
	once    sync.Once
}

func (l *workerLink) hello(ctx context.Context, profile string) error {
	hello, cancel := context.WithTimeout(ctx, helloTimeout)
	reply, err := l.session.request(hello, workerproto.Message{Type: workerproto.TypeHello, Protocol: workerproto.Version})
	cancel()
	if err != nil {
		return fmt.Errorf("%w: %v", ble.ErrHelperMissing, err)
	}
	if reply.Protocol != workerproto.Version {
		return fmt.Errorf("%w: protocol %d, want %d", ble.ErrHelperMissing, reply.Protocol, workerproto.Version)
	}
	if !slices.Contains(reply.Profiles, profile) {
		return fmt.Errorf("%w: profile %s is unavailable", ble.ErrHelperMissing, profile)
	}
	return nil
}

func (l *workerLink) open(ctx context.Context, request ble.ConnectRequest, profile string) (ble.DeviceInfo, error) {
	if err := l.hello(ctx, profile); err != nil {
		return ble.DeviceInfo{}, err
	}
	if request.ConnectTimeout <= 0 {
		request.ConnectTimeout = 6 * time.Second
	}
	if request.ScanTimeout <= 0 {
		request.ScanTimeout = 12 * time.Second
	}
	budget := request.ConnectTimeout + request.ScanTimeout + connectSlack
	if request.RememberedAddress != "" {
		budget += request.ConnectTimeout
	}
	connect, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	reply, err := l.session.request(connect, workerproto.Message{
		Type:            workerproto.TypeConnect,
		Address:         request.RememberedAddress,
		SelectedAddress: request.SelectedAddress,
		Profile:         profile,
		ConnectTimeout:  int(request.ConnectTimeout / time.Millisecond),
		ScanTimeout:     int(request.ScanTimeout / time.Millisecond),
	})
	if err != nil {
		return ble.DeviceInfo{}, err
	}
	if reply.Type != workerproto.TypeConnected || reply.Profile != profile || reply.MaxWriteBytes <= 0 || reply.MaxWriteBytes > 512 {
		return ble.DeviceInfo{}, fmt.Errorf("%w: invalid Bluetooth connection capabilities", ble.ErrHelperMissing)
	}
	return ble.DeviceInfo{Name: reply.Name, Address: reply.Address, MaxWriteBytes: reply.MaxWriteBytes, UnacknowledgedWrites: reply.UnacknowledgedWrites}, nil
}

func (l *workerLink) Write(ctx context.Context, payload []byte, acknowledged bool) error {
	message := workerproto.Message{Type: workerproto.TypeWrite, Data: hex.EncodeToString(payload), Ack: acknowledged}
	// Even Write Without Response waits for the worker to finish its native
	// call. Otherwise a slow radio can accumulate old frames in the stdin pipe
	// ahead of Stop, beyond the core's generation and write serialization.
	_, err := l.session.request(ctx, message)
	if err != nil && (ctx.Err() != nil || errors.Is(err, ble.ErrLinkClosed)) {
		l.abort()
	}
	return err
}

// abort retires uncertain in-flight radio work before returning a timeout.
// A hung worker cannot process the old write after Stop has reported failure.
func (l *workerLink) abort() {
	l.once.Do(func() {
		l.session.retire()
		_ = processtree.Kill(l.process.command)
		l.process.stop(exitTimeout)
	})
}

func (l *workerLink) ReadBattery(ctx context.Context) (int, error) {
	reply, err := l.session.request(ctx, workerproto.Message{Type: workerproto.TypeBattery})
	if err != nil {
		return 0, err
	}
	if reply.Percent == nil {
		return 0, errors.New("battery reply carried no value")
	}
	return *reply.Percent, nil
}

func (l *workerLink) SetLastWill(payload []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return l.session.send(ctx, workerproto.Message{Type: workerproto.TypeLastWill, Data: hex.EncodeToString(payload)})
}

func (l *workerLink) Events() <-chan ble.LinkEvent {
	return l.session.events
}

func (l *workerLink) Device() ble.DeviceInfo {
	return l.device
}

// Close disconnects and stops the worker. Each driver first requests its own
// safe state; a profile may additionally require the stored Stop on disconnect.
func (l *workerLink) Close() error {
	l.stop()
	return nil
}

func (l *workerLink) stop() {
	l.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), disconnectTimeout)
		_, _ = l.session.request(ctx, workerproto.Message{Type: workerproto.TypeDisconnect})
		cancel()
		ctx, cancel = context.WithTimeout(context.Background(), disconnectTimeout)
		_ = l.session.send(ctx, workerproto.Message{Type: workerproto.TypeShutdown})
		cancel()
		l.session.retire()
		l.process.stop(exitTimeout)
	})
}

type process struct {
	command *exec.Cmd
	tree    *processtree.Handle
	session *session
	exited  chan struct{}
}

func startProcess(options Options) (*process, error) {
	command := exec.Command(options.Path, options.Args...) // #nosec G204 -- app-owned helper path resolved by the core.
	processtree.Configure(command)
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	command.Stderr = &logWriter{logger: options.Logger}
	if err := command.Start(); err != nil {
		return nil, err
	}
	tree, err := processtree.Attach(command)
	if err != nil {
		_ = processtree.Kill(command)
		_ = command.Wait()
		return nil, err
	}
	proc := &process{command: command, tree: tree, exited: make(chan struct{})}
	proc.session = newSession(stdin, stdout)
	go func() {
		<-proc.session.done
		_ = command.Wait()
		close(proc.exited)
	}()
	return proc, nil
}

func (p *process) stop(timeout time.Duration) {
	select {
	case <-p.exited:
	case <-time.After(timeout):
		_ = processtree.Kill(p.command)
		<-p.exited
	}
	_ = p.tree.Close()
}

// logWriter forwards worker stderr lines to the core log without echoing
// payload data: the worker logs only lifecycle text.
type logWriter struct {
	mu      sync.Mutex
	logger  *slog.Logger
	pending []byte
}

func (w *logWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = append(w.pending, data...)
	for {
		index := strings.IndexByte(string(w.pending), '\n')
		if index < 0 {
			if len(w.pending) > stderrLineBytes {
				w.pending = w.pending[:0]
			}
			return len(data), nil
		}
		line := strings.TrimSpace(string(w.pending[:index]))
		w.pending = w.pending[index+1:]
		if line != "" {
			w.logger.Info("Bluetooth worker", "message", line)
		}
	}
}

var _ io.Writer = (*logWriter)(nil)

// scannerFor bounds protocol lines from the worker.
func scannerFor(reader io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), workerproto.MaxLineBytes)
	return scanner
}
