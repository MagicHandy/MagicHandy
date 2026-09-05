package chat

import (
	"math/rand"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// FreshLayeredScore gives a new conversation its own irregular realization.
// The chosen seed is retained with the score for faithful plots and replay.
func FreshLayeredScore(speed int) motion.FlowSpec {
	score := DefaultLayeredScore(speed)
	score.Seed = freshLayeredSeed(score.Seed)
	return score
}

func freshLayeredSeed(previous uint32) uint32 {
	seed := previous
	for seed == 0 || seed == previous {
		// #nosec G404 -- Non-security motion variation; hard bounds and the shared engine remain authoritative.
		seed = rand.Uint32()
	}
	return seed
}
