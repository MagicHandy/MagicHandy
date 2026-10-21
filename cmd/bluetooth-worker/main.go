// Command bluetooth-worker is the shared native Bluetooth helper. The core starts
// it per connection and speaks internal/ble/workerproto over stdin/stdout.
// It keeps the radio stack out of the core binary (AGENTS.md section 3) and
// never decides what to send: it writes the packets the core composed.
//
// When stdin closes without a shutdown request, the core is gone: the worker
// writes the stored last-will packet, disconnects and exits. If the worker is
// killed instead, last-will delivery is not guaranteed. Buffered Handy motion
// can remain until its finite buffer is exhausted; see ADR 0046.
package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/mapledaemon/MagicHandy/internal/ble"
	"github.com/mapledaemon/MagicHandy/internal/estim"
	"github.com/mapledaemon/MagicHandy/internal/handyble"
	"os"
	"os/signal"
	"runtime"

	"github.com/mapledaemon/MagicHandy/internal/ble/bleworker"
)

func main() {
	simulate := flag.Bool("simulate", false, "use simulated device profiles without accessing Bluetooth")
	flag.Parse()

	adapter := newAdapter()
	if *simulate {
		adapter = bleworker.NewSimulatedAdapter(map[string]ble.Connector{ble.ProfileCoyote: estim.NewSimulatedConnector(), ble.ProfileHandy: handyble.NewSimulatedConnector()})
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	fmt.Fprintf(os.Stderr, "bluetooth-worker: started (simulate=%t)\n", *simulate)
	info := bleworker.Info{Worker: "bluetooth-worker", Platform: runtime.GOOS + "/" + runtime.GOARCH}
	if err := bleworker.Serve(ctx, os.Stdin, os.Stdout, adapter, info); err != nil {
		fmt.Fprintf(os.Stderr, "bluetooth-worker: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "bluetooth-worker: stopped")
}
