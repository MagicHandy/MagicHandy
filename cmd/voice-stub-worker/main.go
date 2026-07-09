// Command voice-stub-worker is a model-free voice worker speaking the ADR
// 0003 protocol on stdio. It exists for protocol tests and manual lifecycle
// checks (start/stop, load/unload, cancellation, crash visibility) — it
// ships no ML models and produces silent audio and canned transcripts.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/mapledaemon/MagicHandy/internal/voice"
	"github.com/mapledaemon/MagicHandy/internal/voice/stubworker"
)

func main() {
	roleFlag := flag.String("role", "", "worker role: tts or asr")
	startLoaded := flag.Bool("start-loaded", false, "report the model as loaded at startup")
	failStart := flag.Bool("fail-start", false, "exit immediately with an error, simulating a missing dependency")
	advertiseProtocol := flag.Int("advertise-protocol", 0, "test knob: report this protocol version at hello")
	flag.Parse()

	role, err := voice.ParseRole(*roleFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "voice-stub-worker: %v\n", err)
		os.Exit(2)
	}
	if *failStart {
		fmt.Fprintln(os.Stderr, "voice-stub-worker: missing dependency: stub was asked to fail startup")
		os.Exit(3)
	}

	fmt.Fprintf(os.Stderr, "voice-stub-worker: %s worker started (protocol v%d)\n", role, voice.ProtocolVersion)

	err = stubworker.Run(os.Stdin, os.Stdout, stubworker.Options{
		Role:                     role,
		StartLoaded:              *startLoaded,
		AdvertiseProtocolVersion: *advertiseProtocol,
	})
	if errors.Is(err, stubworker.ErrCrashRequested) {
		fmt.Fprintln(os.Stderr, "voice-stub-worker: crashing on request")
		os.Exit(3)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "voice-stub-worker: %v\n", err)
		os.Exit(1)
	}
}
