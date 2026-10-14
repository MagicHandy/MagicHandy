// Package bleworker is the radio side of the native Bluetooth worker. It
// speaks workerproto over stdin and stdout and drives an Adapter. The concrete
// Bluetooth adapter lives in cmd/bluetooth-worker, so the core binary never
// links a radio stack (AGENTS.md section 3).
package bleworker

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/ble"
	"github.com/mapledaemon/MagicHandy/internal/ble/workerproto"
)

// Adapter failures that map to protocol error codes.
var (
	ErrNotFound    = errors.New("device not found")
	ErrUnavailable = errors.New("bluetooth unavailable")
	ErrUnsupported = errors.New("device does not offer the required GATT service")
	ErrAmbiguous   = errors.New("more than one matching Bluetooth device was found; turn off the others and try again")
)

const (
	defaultConnectTimeout = 6 * time.Second
	defaultScanTimeout    = 12 * time.Second
)

// Handlers receive radio callbacks. They may run on adapter goroutines.
type Handlers struct {
	Notify  func([]byte)
	Battery func(int)
	Lost    func(error)
}

// Adapter finds and connects devices.
type Adapter interface {
	Enable() error
	ConnectAddress(ctx context.Context, address string, profile ble.Profile) (Peripheral, error)
	ScanName(ctx context.Context, profile ble.Profile) (string, error)
}

// Peripheral is one connected device. Open discovers the pulse service and
// subscribes to notifications; it returns ErrUnsupported when the service or
// either characteristic is missing.
type Peripheral interface {
	Open(handlers Handlers) error
	Write(payload []byte, withResponse bool) error
	ReadBattery() (int, error)
	Address() string
	Name() string
	MaxWriteBytes() int
	Close() error
}

// Info identifies the worker in its hello reply.
type Info struct {
	Worker   string
	Platform string
}

// Serve runs the protocol until shutdown or the end of input. When input
// ends without a shutdown, the core is gone: the worker writes the stored
// last-will packet, disconnects and returns.
func Serve(ctx context.Context, input io.Reader, output io.Writer, adapter Adapter, info Info) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	server := &server{adapter: adapter, out: &lineWriter{out: output}, info: info, cancel: cancel}
	defer server.wait()
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), workerproto.MaxLineBytes)
	for scanner.Scan() {
		var message workerproto.Message
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			server.out.send(workerproto.Message{Type: workerproto.TypeError, Code: workerproto.CodeBadRequest, Message: "malformed request"})
			continue
		}
		if done := server.handle(ctx, message); done {
			server.shutdown(false)
			return nil
		}
	}
	server.shutdown(true)
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read requests: %w", err)
	}
	return nil
}

type server struct {
	adapter Adapter
	out     *lineWriter
	info    Info
	cancel  context.CancelFunc

	mu         sync.Mutex
	enabled    bool
	connecting bool
	attempted  bool
	peripheral Peripheral
	lastWill   []byte
	profile    ble.Profile
	tasks      sync.WaitGroup
}

func (s *server) handle(ctx context.Context, message workerproto.Message) bool {
	switch message.Type {
	case workerproto.TypeHello:
		s.out.send(workerproto.Message{Type: workerproto.TypeHello, ID: message.ID, Protocol: workerproto.Version, Worker: s.info.Worker, Platform: s.info.Platform, Profiles: []string{ble.ProfileCoyote, ble.ProfileHandy}})
	case workerproto.TypeConnect:
		s.startConnect(ctx, message)
	case workerproto.TypeWrite:
		s.write(message)
	case workerproto.TypeBattery:
		s.battery(message)
	case workerproto.TypeLastWill:
		s.setLastWill(message)
	case workerproto.TypeDisconnect:
		s.disconnect()
		s.out.send(workerproto.Message{Type: workerproto.TypeDisconnected, ID: message.ID, Reason: "requested"})
	case workerproto.TypeShutdown:
		return true
	default:
		s.out.fail(message.ID, workerproto.CodeBadRequest, "unknown request type")
	}
	return false
}

func (s *server) startConnect(ctx context.Context, message workerproto.Message) {
	s.mu.Lock()
	if s.attempted || s.connecting || s.peripheral != nil {
		s.mu.Unlock()
		s.out.fail(message.ID, workerproto.CodeBusy, "already connected or connecting")
		return
	}
	profile, ok := ble.LookupProfile(message.Profile)
	if !ok {
		s.mu.Unlock()
		s.out.fail(message.ID, workerproto.CodeBadRequest, "unknown Bluetooth profile")
		return
	}
	s.profile = profile
	s.attempted = true
	s.connecting = true
	s.mu.Unlock()
	s.tasks.Add(1)
	go func() {
		defer s.tasks.Done()
		peripheral, err := s.connect(ctx, message)
		s.mu.Lock()
		s.connecting = false
		if err == nil && ctx.Err() != nil {
			err = ctx.Err()
		} else if err == nil {
			s.peripheral = peripheral
		}
		s.mu.Unlock()
		if err != nil {
			if peripheral != nil {
				_ = peripheral.Close()
			}
			s.out.fail(message.ID, errorCode(err), err.Error())
			return
		}
		s.out.send(workerproto.Message{Type: workerproto.TypeConnected, ID: message.ID, Address: peripheral.Address(), Name: peripheral.Name(), Profile: message.Profile, MaxWriteBytes: peripheral.MaxWriteBytes()})
	}()
}

func (s *server) connect(ctx context.Context, message workerproto.Message) (Peripheral, error) {
	if err := s.enable(); err != nil {
		return nil, err
	}
	connectTimeout := durationOr(message.ConnectTimeout, defaultConnectTimeout)
	if address := strings.TrimSpace(message.Address); address != "" {
		if peripheral, err := s.open(ctx, address, connectTimeout); s.profile.RememberedOnly || err == nil || errors.Is(err, ErrUnavailable) || errors.Is(err, ErrUnsupported) {
			return peripheral, err
		}
	}
	scan, cancel := context.WithTimeout(ctx, durationOr(message.ScanTimeout, defaultScanTimeout))
	address, err := s.adapter.ScanName(scan, s.profile)
	cancel()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return s.open(ctx, address, connectTimeout)
}

func (s *server) open(ctx context.Context, address string, timeout time.Duration) (Peripheral, error) {
	attempt, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	peripheral, err := s.adapter.ConnectAddress(attempt, address, s.profile)
	if err != nil {
		return nil, err
	}
	if err := peripheral.Open(s.handlers(peripheral)); err != nil {
		_ = peripheral.Close()
		return nil, err
	}
	return peripheral, nil
}

func (s *server) handlers(peripheral Peripheral) Handlers {
	return Handlers{
		Notify: func(payload []byte) {
			s.out.send(workerproto.Message{Type: workerproto.TypeNotify, Data: hex.EncodeToString(payload)})
		},
		Battery: func(percent int) {
			s.out.send(workerproto.Message{Type: workerproto.TypeBattery, Percent: &percent})
		},
		Lost: func(err error) {
			s.mu.Lock()
			current := s.peripheral == peripheral
			if current {
				s.peripheral = nil
			}
			s.mu.Unlock()
			if current {
				reason := "link_lost"
				if err != nil {
					reason = err.Error()
				}
				s.out.send(workerproto.Message{Type: workerproto.TypeDisconnected, Reason: reason})
			}
		},
	}
}

func (s *server) enable() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.enabled {
		return nil
	}
	if err := s.adapter.Enable(); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	s.enabled = true
	return nil
}

func (s *server) write(message workerproto.Message) {
	payload, err := hex.DecodeString(message.Data)
	if err != nil || len(payload) == 0 || len(payload) > 512 {
		s.reportWrite(message, workerproto.CodeBadRequest, "invalid payload")
		return
	}
	s.mu.Lock()
	peripheral := s.peripheral
	s.mu.Unlock()
	if peripheral == nil {
		s.reportWrite(message, workerproto.CodeNotConnected, "not connected")
		return
	}
	if len(payload) > peripheral.MaxWriteBytes() {
		s.reportWrite(message, workerproto.CodeBadRequest, "payload exceeds profile limit")
		return
	}
	if err := peripheral.Write(payload, message.Ack); err != nil {
		s.reportWrite(message, workerproto.CodeWriteFailed, err.Error())
		return
	}
	if message.ID != "" {
		s.out.send(workerproto.Message{Type: workerproto.TypeWritten, ID: message.ID})
	}
}

// reportWrite answers a tracked write; legacy untracked writes report failures
// as events. The core tracks every write to bound outstanding radio work.
func (s *server) reportWrite(message workerproto.Message, code, text string) {
	if message.ID != "" {
		s.out.fail(message.ID, code, text)
		return
	}
	s.out.send(workerproto.Message{Type: workerproto.TypeError, Code: code, Message: text})
}

func (s *server) battery(message workerproto.Message) {
	s.mu.Lock()
	peripheral := s.peripheral
	s.mu.Unlock()
	if peripheral == nil {
		s.out.fail(message.ID, workerproto.CodeNotConnected, "not connected")
		return
	}
	percent, err := peripheral.ReadBattery()
	if err != nil {
		s.out.fail(message.ID, workerproto.CodeWriteFailed, err.Error())
		return
	}
	s.out.send(workerproto.Message{Type: workerproto.TypeBattery, ID: message.ID, Percent: &percent})
}

func (s *server) setLastWill(message workerproto.Message) {
	payload, err := hex.DecodeString(message.Data)
	if err != nil || len(payload) > 512 {
		s.out.fail(message.ID, workerproto.CodeBadRequest, "invalid payload")
		return
	}
	s.mu.Lock()
	s.lastWill = payload
	s.mu.Unlock()
}

func (s *server) disconnect() {
	// One connection attempt per process. Retire an unfinished discovery too.
	s.cancel()
	s.mu.Lock()
	peripheral, lastWill, stop := s.peripheral, s.lastWill, s.profile.StopOnDisconnect
	s.peripheral = nil
	s.lastWill = nil
	s.mu.Unlock()
	if peripheral != nil {
		if stop && len(lastWill) > 0 {
			_ = peripheral.Write(lastWill, true)
		}
		_ = peripheral.Close()
	}
}

// shutdown ends any connect attempt and disconnects. writeLastWill is true
// only when the core vanished without asking for a shutdown.
func (s *server) shutdown(writeLastWill bool) {
	s.cancel()
	s.mu.Lock()
	peripheral, lastWill := s.peripheral, s.lastWill
	s.peripheral, s.lastWill = nil, nil
	s.mu.Unlock()
	if peripheral == nil {
		return
	}
	if (writeLastWill || s.profile.StopOnDisconnect) && len(lastWill) > 0 {
		_ = peripheral.Write(lastWill, true)
	}
	_ = peripheral.Close()
}

func (s *server) wait() {
	s.tasks.Wait()
	// A connect that finished after shutdown has already closed itself.
	s.mu.Lock()
	peripheral := s.peripheral
	s.peripheral = nil
	s.mu.Unlock()
	if peripheral != nil {
		_ = peripheral.Close()
	}
}

func errorCode(err error) string {
	switch {
	case errors.Is(err, ErrNotFound):
		return workerproto.CodeNotFound
	case errors.Is(err, ErrUnavailable):
		return workerproto.CodeUnavailable
	case errors.Is(err, ErrUnsupported):
		return workerproto.CodeUnsupported
	case errors.Is(err, ErrAmbiguous):
		return workerproto.CodeAmbiguous
	default:
		return workerproto.CodeConnect
	}
}

func durationOr(millis int, fallback time.Duration) time.Duration {
	if millis <= 0 {
		return fallback
	}
	return time.Duration(millis) * time.Millisecond
}

type lineWriter struct {
	mu  sync.Mutex
	out io.Writer
}

func (w *lineWriter) send(message workerproto.Message) {
	data, err := json.Marshal(message)
	if err != nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = w.out.Write(append(data, '\n'))
}

func (w *lineWriter) fail(id, code, text string) {
	w.send(workerproto.Message{Type: workerproto.TypeError, ID: id, Code: code, Message: text})
}
