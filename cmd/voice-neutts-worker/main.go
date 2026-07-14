// Command voice-neutts-worker adapts the non-Python neutts-rs stream_pcm
// runner to MagicHandy's voice worker protocol.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mapledaemon/MagicHandy/internal/voice"
	"github.com/mapledaemon/MagicHandy/internal/voice/neuttsworker"
)

func main() {
	runner := flag.String("runner", "", "neutts-rs stream_pcm executable")
	refAudio := flag.String("ref-audio", "", "reference WAV path (provenance; pre-encoded codes are still required)")
	refCodes := flag.String("ref-codes", "", "pre-encoded NeuCodec .npy reference codes")
	refText := flag.String("ref-text", "", "verbatim reference recording transcript")
	backbone := flag.String("backbone", "neuphonic/neutts-air-q4-gguf", "preinstalled NeuTTS GGUF repository identifier")
	ggufFile := flag.String("gguf-file", "", "exact GGUF filename in the preinstalled backbone cache")
	chunk := flag.Int("chunk", 25, "NeuCodec token chunk size")
	flag.Parse()

	fmt.Fprintf(os.Stderr, "voice-neutts-worker: started (protocol v%d, local runner)\n", voice.ProtocolVersion)
	if err := neuttsworker.Run(os.Stdin, os.Stdout, neuttsworker.Options{
		RunnerPath: *runner, ReferenceWAV: *refAudio, ReferenceCodes: *refCodes,
		ReferenceText: *refText, Backbone: *backbone, GGUFFile: *ggufFile, ChunkTokens: *chunk,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "voice-neutts-worker: %v\n", err)
		os.Exit(1)
	}
}
