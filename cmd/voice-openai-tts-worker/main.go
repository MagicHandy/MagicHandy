// Command voice-openai-tts-worker adapts OpenAI-compatible speech servers to
// MagicHandy's ADR 0003 protocol. It can connect to an external server or own
// one explicitly configured local child process.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/voice"
	"github.com/mapledaemon/MagicHandy/internal/voice/openaittsworker"
)

type stringList []string

func (values *stringList) String() string {
	return strings.Join(*values, " ")
}

func (values *stringList) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func main() {
	baseURL := flag.String("base-url", "", "externally managed OpenAI-compatible TTS server URL")
	model := flag.String("model", "", "server-side model name")
	voiceName := flag.String("voice", "", "server-side voice name")
	responseFormat := flag.String("response-format", openaittsworker.DefaultResponseFormat, "audio response format")
	healthPath := flag.String("health-path", openaittsworker.DefaultHealthPath, "server health endpoint path")
	healthReadyField := flag.String("health-ready-field", "", "optional dot-separated boolean field required to be true")
	serverCommand := flag.String("server-command", "", "local TTS server executable to manage")
	serverDir := flag.String("server-dir", "", "working directory for the managed TTS server")
	serverPort := flag.Int("server-port", openaittsworker.DefaultServerPort, "loopback port for the managed TTS server")
	var serverArgs stringList
	flag.Var(&serverArgs, "server-arg", "one managed server argument; repeat for each argument")
	flag.Parse()

	mode := "external server"
	if *serverCommand != "" {
		mode = "managed server"
	}
	fmt.Fprintf(os.Stderr, "voice-openai-tts-worker: started (protocol v%d, %s)\n",
		voice.ProtocolVersion, mode)

	err := openaittsworker.Run(os.Stdin, os.Stdout, openaittsworker.Options{
		BaseURL:          *baseURL,
		APIKey:           os.Getenv("OPENAI_TTS_API_KEY"),
		Model:            *model,
		Voice:            *voiceName,
		ResponseFormat:   *responseFormat,
		HealthPath:       *healthPath,
		HealthReadyField: *healthReadyField,
		ServerCommand:    *serverCommand,
		ServerArgs:       serverArgs,
		ServerDir:        *serverDir,
		ServerPort:       *serverPort,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "voice-openai-tts-worker: %v\n", err)
		os.Exit(1)
	}
}
