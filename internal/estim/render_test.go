package estim

import (
	"math"
	"testing"
)

func defaultProfile() Profile {
	return Profile{Follow: FollowStroke, MinHz: 40, MaxHz: 100}
}

func strokeTimeline(t *testing.T, durationMillis int64, periodMillis int64) *Timeline {
	t.Helper()
	timeline := &Timeline{}
	var points []Point
	for at := int64(0); at <= durationMillis; at += periodMillis / 2 {
		position := 0.0
		if (at/(periodMillis/2))%2 == 1 {
			position = 100
		}
		points = append(points, Point{TimeMillis: at, Position: position})
	}
	if err := timeline.Append(points); err != nil {
		t.Fatal(err)
	}
	return timeline
}

func render(renderer *Renderer, timeline *Timeline, fromMillis, toMillis float64) [][FramesPerPacket]Frame {
	var packets [][FramesPerPacket]Frame
	for start := fromMillis; start < toMillis; start += PacketMillis {
		packet, _ := renderer.Packet(timeline, start)
		packets = append(packets, packet)
	}
	return packets
}

func assertValidPacket(t *testing.T, packet [FramesPerPacket]Frame) {
	t.Helper()
	encoded, err := (Packet{Frames: packet}).Encode()
	if err != nil {
		t.Fatalf("rendered packet is not encodable: %v (%+v)", err, packet)
	}
	_ = encoded
}

func TestRendererOutputIsAlwaysEncodable(t *testing.T) {
	for _, follow := range []string{FollowStroke, FollowSpeed, FollowPosition} {
		for _, balance := range []int{-100, -40, 0, 60, 100} {
			renderer := NewRenderer(Profile{Follow: follow, MinHz: 1, MaxHz: 100, Balance: balance, SwapChannels: balance > 0})
			renderer.Restart(0)
			for _, packet := range render(renderer, strokeTimeline(t, 6000, 400), 0, 7000) {
				assertValidPacket(t, packet)
			}
		}
	}
}

func TestRendererFadesInAfterRestart(t *testing.T) {
	renderer := NewRenderer(defaultProfile())
	renderer.Restart(0)
	packets := render(renderer, strokeTimeline(t, 6000, 500), 0, 4000)
	first := packets[0][0]
	if first.A.Intensity > 3 || first.B.Intensity > 3 {
		t.Fatalf("the first frame after Play must start near zero, got %+v", first)
	}
	peak := 0
	for _, packet := range packets[25:] {
		for _, frame := range packet {
			peak = max(peak, frame.A.Intensity, frame.B.Intensity)
		}
	}
	if peak < 50 {
		t.Fatalf("output never reached a useful width after the fade (peak %d)", peak)
	}
}

func TestRendererIsSilentWhereTheTimelineEnds(t *testing.T) {
	renderer := NewRenderer(defaultProfile())
	renderer.Restart(-10_000)
	timeline := strokeTimeline(t, 1000, 400)
	packet, covered := renderer.Packet(timeline, 1100)
	if covered != 0 {
		t.Fatalf("covered = %d past the end of the timeline", covered)
	}
	for _, frame := range packet {
		if frame.A.Intensity != 0 || frame.B.Intensity != 0 {
			t.Fatalf("starved output must be silent, never the last value: %+v", packet)
		}
	}
}

func TestRendererStrokeBalanceFollowsPosition(t *testing.T) {
	renderer := NewRenderer(Profile{Follow: FollowPosition, MinHz: 40, MaxHz: 100})
	renderer.Restart(-10_000)
	base := &Timeline{}
	_ = base.Append([]Point{{TimeMillis: 0, Position: 0}, {TimeMillis: 1000, Position: 0}})
	packet, _ := renderer.Packet(base, 400)
	if packet[0].A.Intensity <= packet[0].B.Intensity {
		t.Fatalf("the base end must favor channel A: %+v", packet[0])
	}
	tip := &Timeline{}
	_ = tip.Append([]Point{{TimeMillis: 0, Position: 100}, {TimeMillis: 1000, Position: 100}})
	packet, _ = renderer.Packet(tip, 400)
	if packet[0].B.Intensity <= packet[0].A.Intensity {
		t.Fatalf("the tip end must favor channel B: %+v", packet[0])
	}
	renderer.SetProfile(Profile{Follow: FollowPosition, MinHz: 40, MaxHz: 100, SwapChannels: true})
	packet, _ = renderer.Packet(tip, 500)
	if packet[0].A.Intensity <= packet[0].B.Intensity {
		t.Fatalf("swapping channels must move the tip to A: %+v", packet[0])
	}
}

func TestRendererSpeedSetsPulseWidthNotStrength(t *testing.T) {
	slow := NewRenderer(Profile{Follow: FollowSpeed, MinHz: 40, MaxHz: 100})
	fast := NewRenderer(Profile{Follow: FollowSpeed, MinHz: 40, MaxHz: 100})
	slow.Restart(-10_000)
	fast.Restart(-10_000)
	slowPackets := render(slow, strokeTimeline(t, 8000, 2000), 0, 6000)
	fastPackets := render(fast, strokeTimeline(t, 8000, 400), 0, 6000)
	average := func(packets [][FramesPerPacket]Frame) float64 {
		total := 0
		for _, packet := range packets[20:] {
			for _, frame := range packet {
				total += frame.A.Intensity
			}
		}
		return float64(total) / float64(len(packets[20:])*FramesPerPacket)
	}
	if average(fastPackets) <= average(slowPackets)+10 {
		t.Fatalf("faster strokes should widen pulses: slow %.1f fast %.1f", average(slowPackets), average(fastPackets))
	}
}

// The saved speed band gives pace the contrast the saturating curve lost, but
// never widens the pulses past that curve at any speed: at a given strength
// the change can only soften output (ADR 0063).
func TestRendererSpeedBandAddsContrastWithoutWidening(t *testing.T) {
	band := Profile{Follow: FollowSpeed, MinHz: 40, MaxHz: 100, SpeedLow: 0.291, SpeedHigh: 3.636}
	previous := 0.0
	for speed := 0.0; speed <= 6; speed += 0.05 {
		drive, curve := speedDrive(speed, band), 1-math.Exp(-speed/speedScale)
		if drive > curve+1e-12 || drive < previous-1e-12 {
			t.Fatalf("speed %.2f: drive %.4f (curve %.4f, previous %.4f) must not exceed the curve or fall", speed, drive, curve, previous)
		}
		previous = drive
	}
	ratio := speedDrive(2.4, band) / speedDrive(1, band)
	if curveRatio := (1 - math.Exp(-2.4/speedScale)) / (1 - math.Exp(-1/speedScale)); ratio < curveRatio+0.25 {
		t.Fatalf("pace contrast %.2f, the curve alone gives %.2f", ratio, curveRatio)
	}
	if got := speedDrive(1, Profile{Follow: FollowSpeed}); math.Abs(got-(1-math.Exp(-1/speedScale))) > 1e-12 {
		t.Fatalf("without a band the curve stays: %.4f", got)
	}
}

func TestRendererHoldsFrequencySteps(t *testing.T) {
	renderer := NewRenderer(Profile{Follow: FollowStroke, MinHz: 10, MaxHz: 100})
	renderer.Restart(-10_000)
	timeline := strokeTimeline(t, 10_000, 300)
	previous, changedAt := 0, -1.0
	for start := 0.0; start < 9000; start += PacketMillis {
		packet, _ := renderer.Packet(timeline, start)
		period := packet[0].A.PeriodMillis
		for _, frame := range packet {
			if frame.A.PeriodMillis != period || frame.B.PeriodMillis != period {
				t.Fatalf("frames in one packet must share a period: %+v", packet)
			}
		}
		if previous != 0 && period != previous {
			if changedAt >= 0 && start-changedAt < frequencyHoldMillis {
				t.Fatalf("period changed again after %.0f ms", start-changedAt)
			}
			changedAt = start
		}
		previous = period
	}
	if changedAt < 0 {
		t.Fatal("the pulse rate never followed the rising tempo")
	}
}

func TestReferencePacketPulsesOnlyTheAdjustedChannel(t *testing.T) {
	profile := Profile{Follow: FollowStroke, MinHz: 40, MaxHz: 80}
	first := ReferencePacket(profile, ChannelB, 0, ReferencePackets)
	last := ReferencePacket(profile, ChannelB, ReferencePackets-1, ReferencePackets)
	for _, frame := range append(first[:], last[:]...) {
		if frame.A.Intensity != 0 {
			t.Fatalf("the other channel must stay silent: %+v", frame)
		}
	}
	if first[0].B.Intensity >= MaxIntensity || last[3].B.Intensity != MaxIntensity {
		t.Fatalf("reference must ramp in and reach full width: first %+v last %+v", first[0], last[3])
	}
	if first[0].B.PeriodMillis != PeriodForHz(40) || last[0].B.PeriodMillis != PeriodForHz(80) {
		t.Fatalf("reference must cover both ends of the band: %+v %+v", first[0], last[0])
	}
	assertValidPacket(t, first)
}
