// Command estim-ble-worker is the Coyote 3.0 Bluetooth helper. The core starts
// it per connection and speaks internal/estim/workerproto over stdin/stdout.
// It keeps the radio stack out of the core binary (AGENTS.md section 3) and
// never decides what to send: it writes the packets the core composed.
//
// When stdin closes without a shutdown request, the core is gone: the worker
// writes the stored last-will packet, disconnects and exits. If the worker is
// killed instead, the Coyote still stops on its own after the last 100 ms
// packet it received.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"

	"github.com/mapledaemon/MagicHandy/internal/estim/bleworker"
)

func main() {
	simulate := flag.Bool("simulate", false, "answer as a simulated Coyote instead of using Bluetooth")
	flag.Parse()

	adapter := newAdapter()
	if *simulate {
		adapter = bleworker.NewSimulatedAdapter()
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	fmt.Fprintf(os.Stderr, "estim-ble-worker: started (simulate=%t)\n", *simulate)
	info := bleworker.Info{Worker: "estim-ble-worker", Platform: runtime.GOOS + "/" + runtime.GOARCH}
	if err := bleworker.Serve(ctx, os.Stdin, os.Stdout, adapter, info); err != nil {
		fmt.Fprintf(os.Stderr, "estim-ble-worker: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "estim-ble-worker: stopped")
}
