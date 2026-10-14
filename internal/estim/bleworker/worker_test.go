package bleworker

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/estim"
	"github.com/mapledaemon/MagicHandy/internal/estim/workerproto"
)

type harness struct {
	t        *testing.T
	input    *io.PipeWriter
	replies  chan workerproto.Message
	finished chan struct{}
	err      error
	adapter  *SimulatedAdapter
}

func startHarness(t *testing.T) *harness {
	t.Helper()
	inputReader, inputWriter := io.Pipe()
	outputReader, outputWriter := io.Pipe()
	h := &harness{t: t, input: inputWriter, replies: make(chan workerproto.Message, 64), finished: make(chan struct{}), adapter: NewSimulatedAdapter()}
	go func() {
		h.err = Serve(context.Background(), inputReader, outputWriter, h.adapter, Info{Worker: "test", Platform: "test"})
		_ = outputWriter.Close()
		close(h.finished)
	}()
	go func() {
		scanner := bufio.NewScanner(outputReader)
		for scanner.Scan() {
			var message workerproto.Message
			if err := json.Unmarshal(scanner.Bytes(), &message); err == nil {
				h.replies <- message
			}
		}
		close(h.replies)
	}()
	t.Cleanup(func() {
		_ = inputWriter.Close()
		h.wait()
	})
	return h
}

func (h *harness) wait() {
	h.t.Helper()
	select {
	case <-h.finished:
		if h.err != nil {
			h.t.Error(h.err)
		}
	case <-time.After(5 * time.Second):
		h.t.Error("worker did not stop")
	}
}

func (h *harness) send(message workerproto.Message) {
	h.t.Helper()
	data, _ := json.Marshal(message)
	if _, err := h.input.Write(append(data, '\n')); err != nil {
		h.t.Fatal(err)
	}
}

// collect waits for one message of each wanted type, in any order.
func (h *harness) collect(wanted ...string) map[string]workerproto.Message {
	h.t.Helper()
	got := map[string]workerproto.Message{}
	deadline := time.After(5 * time.Second)
	for len(got) < len(wanted) {
		select {
		case message, ok := <-h.replies:
			if !ok {
				h.t.Fatalf("output ended waiting for %v", wanted)
			}
			for _, want := range wanted {
				if message.Type == want {
					got[want] = message
				}
			}
		case <-deadline:
			h.t.Fatalf("timed out waiting for %v, got %v", wanted, got)
		}
	}
	return got
}

func (h *harness) next(want string) workerproto.Message {
	h.t.Helper()
	for {
		select {
		case message, ok := <-h.replies:
			if !ok {
				h.t.Fatalf("output ended waiting for %q", want)
			}
			if message.Type == want {
				return message
			}
			if message.Type == workerproto.TypeError {
				h.t.Fatalf("unexpected error waiting for %q: %+v", want, message)
			}
		case <-time.After(5 * time.Second):
			h.t.Fatalf("timed out waiting for %q", want)
		}
	}
}

func (h *harness) connect() workerproto.Message {
	h.t.Helper()
	h.send(workerproto.Message{Type: workerproto.TypeConnect, ID: "c", Name: estim.DeviceName, ScanTimeout: 1000})
	return h.next(workerproto.TypeConnected)
}

func TestServeReportsCompletionWithoutGATTResponse(t *testing.T) {
	h := startHarness(t)
	h.connect()
	h.send(workerproto.Message{Type: workerproto.TypeWrite, ID: "tracked-stream", Data: hex.EncodeToString(zeroPacket(t, 1))})
	if reply := h.next(workerproto.TypeWritten); reply.ID != "tracked-stream" {
		t.Fatalf("untracked native write: %+v", reply)
	}
}

func zeroPacket(t *testing.T, sequence int) []byte {
	t.Helper()
	set := estim.StrengthChange{Mode: estim.StrengthSet}
	packet := estim.Packet{Sequence: sequence, A: set, B: set, Frames: [4]estim.Frame{estim.SilentFrame, estim.SilentFrame, estim.SilentFrame, estim.SilentFrame}}
	encoded, err := packet.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return encoded[:]
}

func TestServeHelloConnectWriteAndNotify(t *testing.T) {
	h := startHarness(t)
	h.send(workerproto.Message{Type: workerproto.TypeHello, ID: "h", Protocol: workerproto.Version})
	if hello := h.next(workerproto.TypeHello); hello.Protocol != workerproto.Version || hello.ID != "h" {
		t.Fatalf("hello = %+v", hello)
	}
	if connected := h.connect(); connected.Name != estim.DeviceName || connected.ID != "c" {
		t.Fatalf("connected = %+v", connected)
	}
	h.adapter.Connector.LastLink().TurnSwitch(estim.ChannelA, 4)
	notify := h.next(workerproto.TypeNotify)
	payload, _ := hex.DecodeString(notify.Data)
	if report, ok, _ := estim.DecodeNotification(payload); !ok || report.A != 4 || report.Sequence != 0 {
		t.Fatalf("device switch report = %+v %v", report, ok)
	}
	h.send(workerproto.Message{Type: workerproto.TypeWrite, ID: "w", Data: hex.EncodeToString(zeroPacket(t, 3)), Ack: true})
	// The report and the acknowledgement travel on different goroutines, so
	// either may arrive first.
	got := h.collect(workerproto.TypeNotify, workerproto.TypeWritten)
	if written := got[workerproto.TypeWritten]; written.ID != "w" {
		t.Fatalf("written = %+v", written)
	}
	payload, _ = hex.DecodeString(got[workerproto.TypeNotify].Data)
	if report, _, _ := estim.DecodeNotification(payload); report.Sequence != 3 || report.A != 0 {
		t.Fatalf("zero report = %+v", report)
	}
	h.send(workerproto.Message{Type: workerproto.TypeBattery, ID: "b"})
	if battery := h.next(workerproto.TypeBattery); battery.Percent == nil || *battery.Percent <= 0 {
		t.Fatalf("battery = %+v", battery)
	}
}

func TestServeReportsMissingDeviceAndUnconnectedWrites(t *testing.T) {
	h := startHarness(t)
	h.send(workerproto.Message{Type: workerproto.TypeConnect, ID: "c", Name: "47L121000_O3", ScanTimeout: 50})
	if reply := h.next(workerproto.TypeError); reply.Code != workerproto.CodeNotFound || reply.ID != "c" {
		t.Fatalf("connect to a recovery-mode name = %+v", reply)
	}
	h.send(workerproto.Message{Type: workerproto.TypeWrite, Data: hex.EncodeToString(zeroPacket(t, 0))})
	if reply := h.next(workerproto.TypeError); reply.Code != workerproto.CodeNotConnected || reply.ID != "" {
		t.Fatalf("an unacknowledged write while disconnected must report an event: %+v", reply)
	}
	h.send(workerproto.Message{Type: workerproto.TypeWrite, ID: "bad", Data: "zz", Ack: true})
	if reply := h.next(workerproto.TypeError); reply.Code != workerproto.CodeBadRequest || reply.ID != "bad" {
		t.Fatalf("malformed payload = %+v", reply)
	}
}

// When the core disappears, stdin ends: the stored last will reaches the
// device before the worker disconnects.
func TestServeWritesLastWillWhenInputEnds(t *testing.T) {
	h := startHarness(t)
	h.connect()
	link := h.adapter.Connector.LastLink()
	link.TurnSwitch(estim.ChannelB, 9)
	h.next(workerproto.TypeNotify)
	h.send(workerproto.Message{Type: workerproto.TypeLastWill, Data: hex.EncodeToString(zeroPacket(t, 0))})
	h.send(workerproto.Message{Type: workerproto.TypeBattery, ID: "sync"})
	h.next(workerproto.TypeBattery)
	_ = h.input.Close()
	h.wait()
	if state := link.Snapshot(); state.StrengthB != 0 || !state.Closed {
		t.Fatalf("after input ended = %+v, want zero strength and a closed link", state)
	}
}

func TestServeShutdownSkipsLastWill(t *testing.T) {
	h := startHarness(t)
	h.connect()
	link := h.adapter.Connector.LastLink()
	link.TurnSwitch(estim.ChannelA, 5)
	h.next(workerproto.TypeNotify)
	h.send(workerproto.Message{Type: workerproto.TypeLastWill, Data: hex.EncodeToString(zeroPacket(t, 0))})
	h.send(workerproto.Message{Type: workerproto.TypeShutdown})
	h.wait()
	if state := link.Snapshot(); state.StrengthA != 5 || !state.Closed {
		t.Fatalf("a deliberate shutdown must not write the last will: %+v", state)
	}
}

func TestServeReportsLinkLoss(t *testing.T) {
	h := startHarness(t)
	h.connect()
	h.adapter.Connector.LastLink().Lose(errors.New("radio gone"))
	if reply := h.next(workerproto.TypeDisconnected); reply.ID != "" || reply.Reason != "radio gone" {
		t.Fatalf("link loss = %+v", reply)
	}
	h.send(workerproto.Message{Type: workerproto.TypeWrite, ID: "w", Data: hex.EncodeToString(zeroPacket(t, 1)), Ack: true})
	if reply := h.next(workerproto.TypeError); reply.Code != workerproto.CodeNotConnected {
		t.Fatalf("write after loss = %+v", reply)
	}
}

func TestServeRejectsASecondConnect(t *testing.T) {
	h := startHarness(t)
	h.connect()
	h.send(workerproto.Message{Type: workerproto.TypeConnect, ID: "again", Name: estim.DeviceName})
	if reply := h.next(workerproto.TypeError); reply.Code != workerproto.CodeBusy || reply.ID != "again" {
		t.Fatalf("second connect = %+v", reply)
	}
}
