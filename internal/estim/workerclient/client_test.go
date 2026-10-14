package workerclient

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/estim"
)

var (
	workerOnce sync.Once
	workerPath string
	workerErr  error
)

// simulatedWorker builds cmd/estim-ble-worker once and returns its path. The
// tests run it with -simulate, so the real process boundary is exercised
// without a radio.
func simulatedWorker(t *testing.T) string {
	t.Helper()
	workerOnce.Do(func() {
		dir, err := os.MkdirTemp("", "estim-ble-worker")
		if err != nil {
			workerErr = err
			return
		}
		name := "estim-ble-worker"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		workerPath = filepath.Join(dir, name)
		build := exec.Command("go", "build", "-o", workerPath, "../../../cmd/estim-ble-worker") // #nosec G204 -- builds this repository's worker into a test temp dir.
		build.Env = append(os.Environ(), "CGO_ENABLED=0")
		if output, err := build.CombinedOutput(); err != nil {
			workerErr = errors.New(string(output))
		}
	})
	if workerErr != nil {
		t.Fatalf("build worker: %v", workerErr)
	}
	return workerPath
}

func connectSimulated(t *testing.T) estim.Link {
	t.Helper()
	connector := NewConnector(Options{Path: simulatedWorker(t), Args: []string{"-simulate"}})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	link, err := connector.Connect(ctx, estim.ConnectRequest{ConnectTimeout: time.Second, ScanTimeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = link.Close() })
	return link
}

func strengthPacket(t *testing.T, sequence int) []byte {
	t.Helper()
	set := estim.StrengthChange{Mode: estim.StrengthSet}
	frames := [4]estim.Frame{estim.SilentFrame, estim.SilentFrame, estim.SilentFrame, estim.SilentFrame}
	increase := estim.Packet{Sequence: sequence, A: estim.StrengthChange{Mode: estim.StrengthIncrease, Value: 3}, B: estim.StrengthChange{}, Frames: frames}
	if sequence == 0 {
		increase = estim.Packet{A: set, B: set, Frames: frames}
	}
	encoded, err := increase.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return encoded[:]
}

func waitEvent(t *testing.T, link estim.Link, kind estim.LinkEventKind) estim.LinkEvent {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case event, ok := <-link.Events():
			if !ok {
				t.Fatalf("events closed waiting for %s", kind)
			}
			if event.Kind == kind {
				return event
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s", kind)
		}
	}
}

func TestWorkerProcessConnectWriteAndReport(t *testing.T) {
	link := connectSimulated(t)
	if device := link.Device(); device.Name != estim.DeviceName || device.Address == "" {
		t.Fatalf("device = %+v", device)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := link.Write(ctx, strengthPacket(t, 4), true); err != nil {
		t.Fatal(err)
	}
	event := waitEvent(t, link, estim.LinkNotification)
	report, ok, err := estim.DecodeNotification(event.Payload)
	if err != nil || !ok || report.Sequence != 4 || report.A != 3 {
		t.Fatalf("report = %+v %v %v", report, ok, err)
	}
	if battery, err := link.ReadBattery(ctx); err != nil || battery <= 0 {
		t.Fatalf("battery = %d, %v", battery, err)
	}
	if err := link.Write(ctx, strengthPacket(t, 0), false); err != nil {
		t.Fatalf("write without GATT response must still complete at the worker: %v", err)
	}
}

func TestWorkerProcessCloseStopsTheProcess(t *testing.T) {
	link := connectSimulated(t)
	process := link.(*workerLink).process
	if err := link.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-process.exited:
	case <-time.After(5 * time.Second):
		t.Fatal("the worker process outlived Close")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := link.Write(ctx, strengthPacket(t, 0), true); !errors.Is(err, estim.ErrLinkClosed) {
		t.Fatalf("write after Close = %v, want ErrLinkClosed", err)
	}
}

func TestWorkerProcessExitIsReportedAsLinkLoss(t *testing.T) {
	link := connectSimulated(t)
	process := link.(*workerLink).process
	if err := process.command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	waitEvent(t, link, estim.LinkLost)
}

func TestConnectWithoutTheHelperReportsItMissing(t *testing.T) {
	connector := NewConnector(Options{})
	if _, err := connector.Connect(context.Background(), estim.ConnectRequest{}); !errors.Is(err, estim.ErrHelperMissing) {
		t.Fatalf("Connect without a worker path = %v", err)
	}
	connector = NewConnector(Options{Path: filepath.Join(t.TempDir(), "missing-worker.exe")})
	if _, err := connector.Connect(context.Background(), estim.ConnectRequest{}); !errors.Is(err, estim.ErrHelperMissing) {
		t.Fatalf("Connect with a missing worker = %v", err)
	}
}
