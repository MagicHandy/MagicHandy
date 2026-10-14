// Package workerclient starts the Coyote Bluetooth worker process and adapts
// it to estim.Connector and estim.Link. The worker owns the radio; this side
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
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/estim"
	"github.com/mapledaemon/MagicHandy/internal/estim/workerproto"
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
	Path   string
	Args   []string
	Logger *slog.Logger
}

// Connector starts one worker process per connection.
type Connector struct {
	options Options
}

var _ estim.Connector = (*Connector)(nil)

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
// and connect the Coyote. Any failure stops the worker again.
func (c *Connector) Connect(ctx context.Context, request estim.ConnectRequest) (estim.Link, error) {
	if strings.TrimSpace(c.options.Path) == "" {
		return nil, estim.ErrHelperMissing
	}
	proc, err := startProcess(c.options)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", estim.ErrHelperMissing, err)
	}
	link := &workerLink{session: proc.session, process: proc}
	device, err := link.open(ctx, request)
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
	device  estim.DeviceInfo
	once    sync.Once
}

func (l *workerLink) open(ctx context.Context, request estim.ConnectRequest) (estim.DeviceInfo, error) {
	hello, cancel := context.WithTimeout(ctx, helloTimeout)
	reply, err := l.session.request(hello, workerproto.Message{Type: workerproto.TypeHello, Protocol: workerproto.Version})
	cancel()
	if err != nil {
		return estim.DeviceInfo{}, fmt.Errorf("%w: %v", estim.ErrHelperMissing, err)
	}
	if reply.Protocol != workerproto.Version {
		return estim.DeviceInfo{}, fmt.Errorf("%w: protocol %d, want %d", estim.ErrHelperMissing, reply.Protocol, workerproto.Version)
	}
	budget := request.ConnectTimeout + request.ScanTimeout + connectSlack
	if request.RememberedAddress != "" {
		budget += request.ConnectTimeout
	}
	connect, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	reply, err = l.session.request(connect, workerproto.Message{
		Type:           workerproto.TypeConnect,
		Address:        request.RememberedAddress,
		Name:           estim.DeviceName,
		ConnectTimeout: int(request.ConnectTimeout / time.Millisecond),
		ScanTimeout:    int(request.ScanTimeout / time.Millisecond),
	})
	if err != nil {
		return estim.DeviceInfo{}, err
	}
	return estim.DeviceInfo{Name: reply.Name, Address: reply.Address}, nil
}

func (l *workerLink) Write(ctx context.Context, payload []byte, acknowledged bool) error {
	message := workerproto.Message{Type: workerproto.TypeWrite, Data: hex.EncodeToString(payload), Ack: acknowledged}
	// Even Write Without Response waits for the worker to finish its native
	// call. Otherwise a slow radio can accumulate old frames in the stdin pipe
	// ahead of Stop, beyond the core's generation and write serialization.
	_, err := l.session.request(ctx, message)
	if err != nil && (ctx.Err() != nil || errors.Is(err, estim.ErrLinkClosed)) {
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

func (l *workerLink) Events() <-chan estim.LinkEvent {
	return l.session.events
}

func (l *workerLink) Device() estim.DeviceInfo {
	return l.device
}

// Close disconnects and stops the worker. The caller zeroes strength first;
// Close itself sends nothing to the device.
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
			w.logger.Info("coyote worker", "message", line)
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
