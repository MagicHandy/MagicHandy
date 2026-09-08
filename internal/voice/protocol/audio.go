package protocol

import (
	"encoding/binary"
	"time"
)

// MaxAudioBytes bounds a complete utterance across the workers, core and browser.
const MaxAudioBytes = 8 << 20

// SynthesisTimeout bounds OpenAI-compatible speech generation in both the core
// and HTTP adapter. Model loading has a separate, longer lifecycle deadline.
const SynthesisTimeout = 10 * time.Minute

// RepairWAVLengths replaces streaming sentinel lengths with the actual bounded
// response size. Faster Qwen's streaming WAV uses 0xffffffff because its final
// size is unknown when the first bytes are sent; browsers reject that header
// after MagicHandy has already retained the complete clip.
func RepairWAVLengths(audio []byte) []byte {
	if len(audio) < 12 || len(audio) > MaxAudioBytes ||
		string(audio[:4]) != "RIFF" || string(audio[8:12]) != "WAVE" {
		return audio
	}
	repaired := append([]byte(nil), audio...)
	// #nosec G115 -- audio is bounded to MaxAudioBytes above.
	binary.LittleEndian.PutUint32(repaired[4:8], uint32(len(repaired)-8))

	for offset := 12; offset+8 <= len(repaired); {
		size := binary.LittleEndian.Uint32(repaired[offset+4 : offset+8])
		remaining := len(repaired) - (offset + 8)
		// #nosec G115 -- remaining is bounded to MaxAudioBytes above.
		remainingSize := uint32(remaining)
		if string(repaired[offset:offset+4]) == "data" {
			if size == ^uint32(0) || size > remainingSize {
				binary.LittleEndian.PutUint32(repaired[offset+4:offset+8], remainingSize)
			}
			break
		}
		if size > remainingSize {
			break
		}
		next := offset + 8 + int(size)
		if size%2 != 0 {
			next++
		}
		offset = next
	}
	return repaired
}
