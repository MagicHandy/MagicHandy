package chat

import (
	"fmt"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// These are descriptions of active semantics, not a second motion model.
// Inactive local widths and layer phases are omitted so speech cannot mistake
// them for the current stroke. The engine still owns all generated movement.
func writeAutopilotSpeechFacts(b *strings.Builder, c AutopilotContext) {
	b.WriteString("CURRENT MOTION FACTS override motion described in older check-ins. Describe this active phrase, not an instantaneous position or phase. No sensed feedback is supplied.\n")
	if c.CurrentSpeed <= 0 {
		b.WriteString("Motion has not started yet.\n")
		return
	}
	fmt.Fprintf(b, "Requested overall pace: %d%%.\n", c.CurrentSpeed)
	if c.CurrentFlow != nil {
		f := c.CurrentFlow
		if f.Gesture != nil {
			writeGestureSpeechFacts(b, *f)
		} else {
			ceiling := f.RangeCeilingPercent
			if ceiling == 0 {
				ceiling = f.MaxPercent - f.MinPercent
			}
			if ceiling == f.RangeFloorPercent {
				fmt.Fprintf(b, "Fixed stroke width: %d percentage points.\n", ceiling)
			} else {
				fmt.Fprintf(b, "Stroke width varies within %d–%d percentage points.\n", f.RangeFloorPercent, ceiling)
			}
			writeLayerSpeechFacts(b, *f)
		}
	} else if c.MotionMode == MotionModeDynamic {
		fmt.Fprintf(b, "Creative strokes have a widest span of %d percentage points with %s length variation down toward %d, and center/rhythm variation.\n", c.CurrentSpan, c.CurrentSpanProfile, c.CurrentSpanMin)
		if c.CurrentSectionCount > 0 {
			fmt.Fprintf(b, "The phrase contains %d evolving sections.\n", c.CurrentSectionCount)
		}
	} else {
		fmt.Fprintf(b, "A catalog pattern is playing in the %s region.\n", c.CurrentArea)
	}
	if c.CommandedPeakSpeed > 0 {
		fmt.Fprintf(b, "The compiled phrase travels within approximately %d–%d%% of the slider; these are its outer bounds, not its current position.\n", c.CommandedPositionMin, c.CommandedPositionMax)
	}
}

func writeGestureSpeechFacts(b *strings.Builder, f motion.FlowSpec) {
	g := f.Gesture
	canRoam := f.MaxPercent-f.MinPercent > 10 && (g.FocusWidthPercent < f.MaxPercent-f.MinPercent || g.VariationPercent > 0 || g.ReboundCount > 0)
	position := f.MinPercent + (f.MaxPercent-f.MinPercent)*g.FocusPercent/100
	region := "middle region"
	if position <= 33 {
		region = "lower region"
	} else if position >= 67 {
		region = "upper region"
	}
	if g.FocusRoamPercent > 0 && !canRoam {
		region = "active band with no remaining space to change location"
	} else if g.FocusRoamPercent == 100 {
		region = "freely changing regions within the active band"
	} else if g.FocusRoamPercent > 0 {
		region = "changing locations biased toward the " + region
	}
	switch g.FocusMixPercent {
	case 0:
		fmt.Fprintf(b, "Only broad strokes across the active %d–%d%% band. Local work and rebounds are off; ignore old local positions or widths.\n", f.MinPercent, f.MaxPercent)
	case 100:
		fmt.Fprintf(b, "Only localized strokes in the %s, with a nominal width of %d percentage points. No broad groups are mixed in.\n", region, g.FocusWidthPercent)
	default:
		fmt.Fprintf(b, "Reach varies through widths between the broad %d–%d%% band and local work in the %s, whose nominal width is %d percentage points. The mix biases this changing reach; it does not schedule separate groups or guarantee full-band strokes.\n", f.MinPercent, f.MaxPercent, region, g.FocusWidthPercent)
	}
	if g.FocusMixPercent > 0 && g.ReboundCount > 0 {
		b.WriteString("Shrinking returns and gradual recovery are enabled as local reach contracts, where their width fits.\n")
	}
	if g.FocusMixPercent > 0 && g.FocusRoamPercent > 0 && canRoam {
		b.WriteString("The local working location roams gradually; both endpoints can move. Do not describe a fixed anchor.\n")
	}
	if g.FasterDirection != "even" && g.ContrastPercent > 0 {
		fmt.Fprintf(b, "Travel toward the %s is faster than the return direction.\n", g.FasterDirection)
	}
	if g.VariationPercent > 0 {
		b.WriteString("Pace varies inside the phrase; local widths also vary when local work is active.\n")
	}
}

func writeLayerSpeechFacts(b *strings.Builder, f motion.FlowSpec) {
	movingCenter := false
	for _, layer := range f.Layers {
		if layer.AmountPercent == 0 {
			continue
		}
		switch layer.Axis {
		case "center":
			movingCenter = true
			fmt.Fprintf(b, "The working region moves with a %s center layer.\n", layer.Shape)
		case "range":
			if f.RangeCeilingPercent != f.RangeFloorPercent {
				fmt.Fprintf(b, "Stroke length follows a %s range layer.\n", layer.Shape)
			}
		case "pace":
			fmt.Fprintf(b, "Travel rate varies with a %s pace layer; its instantaneous phase is unknown.\n", layer.Shape)
		}
	}
	if !movingCenter {
		fmt.Fprintf(b, "The stroke window remains anchored at %d%% within its active band.\n", f.AnchorPercent)
	}
}
