// Command voice-openai-tts-worker adapts OpenAI-compatible speech servers to
// MagicHandy's ADR 0003 protocol. It can connect to an external server or own
// one explicitly configured local child process.
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/voice"
	"github.com/mapledaemon/MagicHandy/internal/voice/openaittsworker"
)

type stringList []string

type optionalUint32 struct {
	value uint32
	set   bool
}

func (value *optionalUint32) String() string {
	if !value.set {
		return ""
	}
	return strconv.FormatUint(uint64(value.value), 10)
}

func (value *optionalUint32) Set(raw string) error {
	parsed, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return fmt.Errorf("seed must be an unsigned 32-bit integer: %w", err)
	}
	value.value = uint32(parsed)
	value.set = true
	return nil
}

func (value *optionalUint32) Pointer() *uint32 {
	if !value.set {
		return nil
	}
	seed := value.value
	return &seed
}

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
	instruct := flag.String("instruct", "", "optional provider-specific voice style instruction")
	responseFormat := flag.String("response-format", openaittsworker.DefaultResponseFormat, "audio response format")
	healthPath := flag.String("health-path", openaittsworker.DefaultHealthPath, "server health endpoint path")
	healthReadyField := flag.String("health-ready-field", "", "optional dot-separated boolean field required to be true")
	var seed optionalUint32
	flag.Var(&seed, "seed", "optional unsigned 32-bit generation seed")
	randomizeSeed := flag.Bool("randomize-seed", false, "choose a fresh generation seed for each request")
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
		Instruct:         *instruct,
		ResponseFormat:   *responseFormat,
		HealthPath:       *healthPath,
		HealthReadyField: *healthReadyField,
		Seed:             seed.Pointer(),
		RandomizeSeed:    *randomizeSeed,
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
