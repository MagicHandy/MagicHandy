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
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/voice"
	"github.com/mapledaemon/MagicHandy/internal/voice/elevenlabs"
	"github.com/mapledaemon/MagicHandy/internal/voice/elevenlabsworker"
)

func main() {
	voiceID := flag.String("voice-id", elevenlabsworker.DefaultVoiceID, "ElevenLabs voice ID to speak with")
	modelID := flag.String("model-id", elevenlabsworker.DefaultModelID, "ElevenLabs model ID")
	format := flag.String("format", elevenlabsworker.DefaultOutputFormat, "output format: auto, pcm_24000, or an mp3_/opus_ format")
	language := flag.String("language", "", "ISO 639 language code for models that accept one (optional)")
	stability := flag.Float64("stability", -1, "voice stability 0..1 (negative keeps the voice's setting)")
	similarity := flag.Float64("similarity", -1, "similarity boost 0..1 (negative keeps the voice's setting)")
	style := flag.Float64("style", -1, "style exaggeration 0..1 (negative keeps the voice's setting)")
	speed := flag.Float64("speed", -1, "speaking speed 0.7..1.2 (negative keeps the voice's setting)")
	speakerBoost := flag.String("speaker-boost", "", "on or off (empty keeps the voice's setting)")
	baseURL := flag.String("base-url", elevenlabsworker.DefaultBaseURL, "API base URL (tests only)")
	flag.Parse()

	fmt.Fprintf(os.Stderr, "voice-elevenlabs-worker: started (protocol v%d, voice %s, model %s, format %s)\n",
		voice.ProtocolVersion, *voiceID, *modelID, *format)
	if os.Getenv("ELEVENLABS_API_KEY") == "" {
		fmt.Fprintln(os.Stderr, "voice-elevenlabs-worker: ELEVENLABS_API_KEY is not set; load will fail until it is")
	}

	err := elevenlabsworker.Run(os.Stdin, os.Stdout, elevenlabsworker.Options{
		APIKey:        os.Getenv("ELEVENLABS_API_KEY"),
		VoiceID:       *voiceID,
		ModelID:       *modelID,
		BaseURL:       *baseURL,
		OutputFormat:  *format,
		LanguageCode:  strings.ToLower(strings.TrimSpace(*language)),
		VoiceSettings: voiceSettings(*stability, *similarity, *style, *speed, *speakerBoost),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "voice-elevenlabs-worker: %v\n", err)
		os.Exit(1)
	}
}

func voiceSettings(stability, similarity, style, speed float64, speakerBoost string) *elevenlabs.VoiceSettings {
	settings := &elevenlabs.VoiceSettings{}
	if stability >= 0 {
		settings.Stability = &stability
	}
	if similarity >= 0 {
		settings.SimilarityBoost = &similarity
	}
	if style >= 0 {
		settings.Style = &style
	}
	if speed > 0 {
		settings.Speed = &speed
	}
	switch strings.ToLower(strings.TrimSpace(speakerBoost)) {
	case "on", "true", "1":
		enabled := true
		settings.UseSpeakerBoost = &enabled
	case "off", "false", "0":
		disabled := false
		settings.UseSpeakerBoost = &disabled
	}
	if settings.Empty() {
		return nil
	}
	return settings
}
