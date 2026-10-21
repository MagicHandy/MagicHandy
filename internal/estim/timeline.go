package estim

import "github.com/mapledaemon/MagicHandy/internal/companion"

// Point is one timed semantic position of the shared companion frame copy
// (internal/companion).
type Point = companion.Point

// Timeline is the shared companion copy of the motion frame; the renderer
// samples it and never reshapes it.
type Timeline = companion.Timeline
