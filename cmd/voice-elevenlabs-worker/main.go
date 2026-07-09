// Command voice-elevenlabs-worker is the ElevenLabs cloud TTS worker (ADR
// 0007) speaking the ADR 0003 protocol on stdio. Pure Go, no Python, no
// CGo. The API key is read exclusively from the ELEVENLABS_API_KEY
// environment variable (the core injects it from settings at spawn) — it
// must never be passed as a command-line argument, where other processes
// could read it.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mapledaemon/MagicHandy/internal/voice"
	"github.com/mapledaemon/MagicHandy/internal/voice/elevenlabsworker"
)

func main() {
	voiceID := flag.String("voice-id", elevenlabsworker.DefaultVoiceID, "ElevenLabs voice ID to speak with")
	modelID := flag.String("model-id", elevenlabsworker.DefaultModelID, "ElevenLabs model ID")
	format := flag.String("format", elevenlabsworker.DefaultOutputFormat, "ElevenLabs output format")
	baseURL := flag.String("base-url", elevenlabsworker.DefaultBaseURL, "API base URL (tests only)")
	flag.Parse()

	fmt.Fprintf(os.Stderr, "voice-elevenlabs-worker: started (protocol v%d, voice %s, model %s)\n",
		voice.ProtocolVersion, *voiceID, *modelID)
	if os.Getenv("ELEVENLABS_API_KEY") == "" {
		fmt.Fprintln(os.Stderr, "voice-elevenlabs-worker: ELEVENLABS_API_KEY is not set; load will fail until it is")
	}

	err := elevenlabsworker.Run(os.Stdin, os.Stdout, elevenlabsworker.Options{
		APIKey:       os.Getenv("ELEVENLABS_API_KEY"),
		VoiceID:      *voiceID,
		ModelID:      *modelID,
		BaseURL:      *baseURL,
		OutputFormat: *format,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "voice-elevenlabs-worker: %v\n", err)
		os.Exit(1)
	}
}
