package estim

import "github.com/mapledaemon/MagicHandy/internal/companion"

// Point and Timeline are the shared companion copy of the motion frame
// (internal/companion); the renderer samples it and never reshapes it.
type (
	Point    = companion.Point
	Timeline = companion.Timeline
)
