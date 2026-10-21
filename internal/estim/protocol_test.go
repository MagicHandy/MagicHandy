package estim

import (
	"encoding/hex"
	"strings"
	"testing"
)

func frames(periods [4]int, intensities [4]int, otherPeriods [4]int, otherIntensities [4]int) [FramesPerPacket]Frame {
	var out [FramesPerPacket]Frame
	for index := range out {
		out[index] = Frame{
			A: ChannelFrame{PeriodMillis: periods[index], Intensity: intensities[index]},
			B: ChannelFrame{PeriodMillis: otherPeriods[index], Intensity: otherIntensities[index]},
		}
	}
	return out
}

// The expected values are DG-LAB's own examples from coyote/v3/README.md.
func TestPacketEncodeMatchesManufacturerExamples(t *testing.T) {
	silentB := [4]int{10, 10, 10, 10}
	tests := []struct {
		name   string
		packet Packet
		want   string
	}{
		{
			name: "A only, no strength change",
			packet: Packet{
				Frames:   frames([4]int{10, 10, 10, 10}, [4]int{0, 10, 20, 30}, silentB, [4]int{}),
				SilenceB: true,
			},
			want: "B00000000A0A0A0A000A141E0000000000000065",
		},
		{
			name: "A only, rising period",
			packet: Packet{
				Frames:   frames([4]int{40, 60, 80, 100}, [4]int{100, 90, 90, 90}, silentB, [4]int{}),
				SilenceB: true,
			},
			want: "B0000000283C5064645A5A5A0000000000000065",
		},
		{
			name: "relative increase without a report",
			packet: Packet{
				A:        StrengthChange{Mode: StrengthIncrease, Value: 5},
				Frames:   frames([4]int{10, 10, 10, 10}, [4]int{0, 10, 20, 30}, silentB, [4]int{}),
				SilenceB: true,
			},
			want: "B00405000A0A0A0A000A141E0000000000000065",
		},
		{
			name: "relative increase with sequence 1",
			packet: Packet{
				Sequence: 1,
				A:        StrengthChange{Mode: StrengthIncrease, Value: 10},
				Frames:   frames([4]int{40, 60, 80, 100}, [4]int{100, 90, 90, 90}, silentB, [4]int{}),
				SilenceB: true,
			},
			want: "B0140A00283C5064645A5A5A0000000000000065",
		},
		{
			name: "both channels",
			packet: Packet{
				Frames: frames([4]int{15, 15, 15, 15}, [4]int{40, 50, 60, 70}, [4]int{10, 10, 10, 10}, [4]int{10, 10, 10, 10}),
			},
			want: "B00000000F0F0F0F28323C460A0A0A0A0A0A0A0A",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := test.packet.Encode()
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.ToUpper(hex.EncodeToString(encoded[:])); got != test.want {
				t.Fatalf("Encode() = %s, want %s", got, test.want)
			}
		})
	}
}

func TestPacketEncodeStrengthModes(t *testing.T) {
	packet := Packet{
		Sequence: 15,
		A:        StrengthChange{Mode: StrengthSet, Value: 5},
		B:        StrengthChange{Mode: StrengthIncrease, Value: 8},
		Frames:   [FramesPerPacket]Frame{SilentFrame, SilentFrame, SilentFrame, SilentFrame},
	}
	encoded, err := packet.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if encoded[1] != 0xFD || encoded[2] != 5 || encoded[3] != 8 {
		t.Fatalf("strength bytes = % X, want FD 05 08", encoded[1:4])
	}
	keep := Packet{A: StrengthChange{Mode: StrengthKeep, Value: 99}, Frames: packet.Frames}
	encoded, err = keep.Encode()
	if err != nil || encoded[2] != 0 {
		t.Fatalf("a kept channel must send value 0, got % X, %v", encoded[1:4], err)
	}
}

func TestPacketEncodeRejectsOutOfRangeValues(t *testing.T) {
	valid := [FramesPerPacket]Frame{SilentFrame, SilentFrame, SilentFrame, SilentFrame}
	invalid := []Packet{
		{Sequence: 16, Frames: valid},
		{Sequence: -1, Frames: valid},
		{A: StrengthChange{Mode: StrengthSet, Value: 201}, Frames: valid},
		{B: StrengthChange{Mode: StrengthIncrease, Value: -1}, Frames: valid},
		{A: StrengthChange{Mode: 4}, Frames: valid},
		{Frames: frames([4]int{9, 10, 10, 10}, [4]int{}, [4]int{10, 10, 10, 10}, [4]int{})},
		{Frames: frames([4]int{10, 10, 10, 1001}, [4]int{}, [4]int{10, 10, 10, 10}, [4]int{})},
		{Frames: frames([4]int{10, 10, 10, 10}, [4]int{0, 0, 0, 101}, [4]int{10, 10, 10, 10}, [4]int{})},
		{Frames: frames([4]int{10, 10, 10, 10}, [4]int{}, [4]int{10, 10, 10, 10}, [4]int{-1, 0, 0, 0})},
	}
	for index, packet := range invalid {
		if _, err := packet.Encode(); err == nil {
			t.Errorf("packet %d encoded without error: %+v", index, packet)
		}
	}
}

// DG-LAB publishes the conversion and these worked values; its table's 650 ms
// entry (204) disagrees with its own formula and is omitted.
func TestPeriodByteFollowsManufacturerFormula(t *testing.T) {
	cases := map[int]byte{
		10: 10, 20: 20, 50: 50, 100: 100, 110: 102, 111: 102, 125: 105, 142: 108,
		150: 110, 166: 113, 200: 120, 250: 130, 333: 146, 500: 180, 600: 200,
		680: 208, 700: 210, 750: 215, 800: 220, 900: 230, 1000: 240,
	}
	for period, want := range cases {
		got, err := PeriodByte(period)
		if err != nil || got != want {
			t.Errorf("PeriodByte(%d) = %d, %v; want %d", period, got, err, want)
		}
	}
	for _, period := range []int{0, 9, 1001} {
		if _, err := PeriodByte(period); err == nil {
			t.Errorf("PeriodByte(%d) accepted an out-of-range period", period)
		}
	}
}

func TestLimitsEncode(t *testing.T) {
	encoded, err := Limits{StrengthA: 30, StrengthB: 25, FrequencyBalanceA: 160, FrequencyBalanceB: 160}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.ToUpper(hex.EncodeToString(encoded[:])); got != "BF1E19A0A00000" {
		t.Fatalf("Encode() = %s", got)
	}
	for _, limits := range []Limits{{StrengthA: 201}, {StrengthB: -1}, {FrequencyBalanceA: 256}, {IntensityBalanceB: -1}} {
		if _, err := limits.Encode(); err == nil {
			t.Errorf("limits %+v encoded without error", limits)
		}
	}
}

func TestDecodeNotification(t *testing.T) {
	report, ok, err := DecodeNotification([]byte{0xB1, 1, 25, 0})
	if err != nil || !ok || report != (StrengthReport{Sequence: 1, A: 25}) {
		t.Fatalf("DecodeNotification(B1) = %+v, %v, %v", report, ok, err)
	}
	if _, ok, err := DecodeNotification([]byte{0xBE, 0}); ok || err != nil {
		t.Fatalf("a deprecated BE message must be ignored, got %v, %v", ok, err)
	}
	if _, ok, err := DecodeNotification(nil); ok || err != nil {
		t.Fatalf("an empty notification must be ignored, got %v, %v", ok, err)
	}
	for _, payload := range [][]byte{{0xB1, 1}, {0xB1, 0, 201, 0}} {
		if _, ok, err := DecodeNotification(payload); ok || err == nil {
			t.Errorf("DecodeNotification(% X) = %v, %v; want an error", payload, ok, err)
		}
	}
}
