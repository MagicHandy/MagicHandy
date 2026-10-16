package workerclient

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/ble/workerproto"
	"github.com/mapledaemon/MagicHandy/internal/estim"
)

const hungWriteHelper = "MAGICHANDY_TEST_COYOTE_HUNG_WRITE"

// TestWorkerHungWriteHelper runs only as a test-owned child process. No radio
// adapter is constructed. Its write intentionally never completes.
func TestWorkerHungWriteHelper(_ *testing.T) {
	if os.Getenv(hungWriteHelper) != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request workerproto.Message
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			os.Exit(2)
		}
		reply := workerproto.Message{ID: request.ID}
		switch request.Type {
		case workerproto.TypeHello:
			reply.Type, reply.Protocol = workerproto.TypeHello, workerproto.Version
			reply.Profiles = []string{"coyote3", "handy4"}
		case workerproto.TypeConnect:
			reply.Type, reply.Name = workerproto.TypeConnected, estim.DeviceName
			reply.Profile, reply.MaxWriteBytes = request.Profile, 64
		case workerproto.TypeWrite:
			for {
				time.Sleep(time.Hour)
			}
		default:
			os.Exit(2)
		}
		if err := encoder.Encode(reply); err != nil {
			os.Exit(2)
		}
	}
	os.Exit(0)
}

func TestTimedOutWritesKillTheWorkerBeforeReturning(t *testing.T) {
	for _, ack := range []bool{false, true} {
		name := "without_gatt_response"
		if ack {
			name = "with_gatt_response"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv(hungWriteHelper, "1")
			connector := NewConnector(Options{Profile: "coyote3", Path: os.Args[0], Args: []string{"-test.run=^TestWorkerHungWriteHelper$"}})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			link, err := connector.Connect(ctx, estim.ConnectRequest{ConnectTimeout: time.Second, ScanTimeout: time.Second})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = link.Close() })
			writeCtx, writeCancel := context.WithTimeout(ctx, 100*time.Millisecond)
			defer writeCancel()
			if err := link.Write(writeCtx, strengthPacket(t, 0), ack); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("hung write result=%v; want deadline exceeded", err)
			}
			worker := link.(*workerLink)
			select {
			case <-worker.process.exited:
			default:
				t.Fatal("write returned while the old radio work could still run")
			}
			if err := link.Write(ctx, strengthPacket(t, 0), true); !errors.Is(err, estim.ErrLinkClosed) {
				t.Fatalf("retired worker accepted another write: %v", err)
			}
		})
	}
}
