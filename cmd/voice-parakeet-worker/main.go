// Command voice-parakeet-worker is the ASR worker (ADR 0007) speaking the
// ADR 0003 protocol on stdio. It proxies transcription to an external
// OpenAI-compatible server — Parakeet-TDT via achetronic/parakeet is the
// recommended engine; any server exposing POST /v1/audio/transcriptions
// works. Pure Go, no Python, no CGo; the model runtime lives in the server.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mapledaemon/MagicHandy/internal/voice"
	"github.com/mapledaemon/MagicHandy/internal/voice/parakeetworker"
)

func main() {
	baseURL := flag.String("base-url", "", "OpenAI-compatible ASR server URL (required), e.g. http://127.0.0.1:8765")
	model := flag.String("model", "", "server-side model name (empty uses the server's loaded model)")
	flag.Parse()

	fmt.Fprintf(os.Stderr, "voice-parakeet-worker: started (protocol v%d, server %s)\n",
		voice.ProtocolVersion, *baseURL)
	if *baseURL == "" {
		fmt.Fprintln(os.Stderr, "voice-parakeet-worker: -base-url is not set; load will fail until it is")
	}

	err := parakeetworker.Run(os.Stdin, os.Stdout, parakeetworker.Options{
		BaseURL: *baseURL,
		Model:   *model,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "voice-parakeet-worker: %v\n", err)
		os.Exit(1)
	}
}
