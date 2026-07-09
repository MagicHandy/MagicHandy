// Command voice-parakeet-worker is the ASR worker (ADR 0007) speaking the
// ADR 0003 protocol on stdio. Its managed mode owns a parakeet.cpp server;
// external mode accepts any compatible transcription server. Pure Go, no
// Python, no CGo; the model runtime lives in the server process.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mapledaemon/MagicHandy/internal/voice"
	"github.com/mapledaemon/MagicHandy/internal/voice/parakeetworker"
)

func main() {
	baseURL := flag.String("base-url", "", "externally managed OpenAI-compatible ASR server URL")
	model := flag.String("model", "", "server-side model name (empty uses the server's loaded model)")
	serverPath := flag.String("server-path", "", "parakeet-server executable to manage locally")
	serverModel := flag.String("server-model", "", "local GGUF model file for a managed parakeet-server")
	serverPort := flag.Int("server-port", parakeetworker.DefaultServerPort, "loopback port for a managed parakeet-server")
	flag.Parse()

	mode := "external server " + *baseURL
	if *serverPath != "" || *serverModel != "" {
		mode = "managed parakeet-server"
	}
	fmt.Fprintf(os.Stderr, "voice-parakeet-worker: started (protocol v%d, %s)\n", voice.ProtocolVersion, mode)
	if *baseURL == "" && *serverPath == "" && *serverModel == "" {
		fmt.Fprintln(os.Stderr, "voice-parakeet-worker: no ASR server is configured; load will explain the required flags")
	}

	err := parakeetworker.Run(os.Stdin, os.Stdout, parakeetworker.Options{
		BaseURL:     *baseURL,
		Model:       *model,
		ServerPath:  *serverPath,
		ServerModel: *serverModel,
		ServerPort:  *serverPort,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "voice-parakeet-worker: %v\n", err)
		os.Exit(1)
	}
}
