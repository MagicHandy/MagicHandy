// Package estim models DG-LAB Coyote 3.0 electrical stimulation output: the
// documented Bluetooth packet formats, the user-owned strength handshake, and
// the conversion profile that turns the shared motion frame into two-channel
// pulse frames. It performs no I/O; links and dispatch owners live elsewhere.
//
// Protocol reference:
// https://github.com/dungeonlab-open/dglab-bluetooth-protocol (coyote/v3).
package estim

import (
	"errors"
	"fmt"
)

// Bluetooth identity of a Coyote 3.0 pulse host. UUIDs use the Bluetooth base
// UUID with DG-LAB's 16-bit service and characteristic values.
const (
	// DeviceName is the advertised name of a Coyote 3.0 in normal mode. The
	// recovery-mode name differs ("47L121000_O3") and exposes no pulse service.
	DeviceName                = "47L121000"
	ServiceUUID               = "0000180c-0000-1000-8000-00805f9b34fb"
	WriteCharacteristicUUID   = "0000150a-0000-1000-8000-00805f9b34fb"
	NotifyCharacteristicUUID  = "0000150b-0000-1000-8000-00805f9b34fb"
	BatteryServiceUUID        = "0000180a-0000-1000-8000-00805f9b34fb"
	BatteryCharacteristicUUID = "00001500-0000-1000-8000-00805f9b34fb"
)

// Protocol ranges and timing.
const (
	// MaxStrength is the device's absolute channel strength range, 0..200.
	MaxStrength = 200
	// MaxIntensity is the relative pulse-width range of one waveform frame.
	MaxIntensity = 100
	// MinPeriodMillis and MaxPeriodMillis bound one output unit (pulse plus
	// gap). DG-LAB calls this the waveform frequency: 10 ms is 100 Hz.
	MinPeriodMillis = 10
	MaxPeriodMillis = 1000
	// FrameMillis is the output window of one waveform frame.
	FrameMillis = 25
	// FramesPerPacket frames make up the 100 ms each B0 packet carries.
	FramesPerPacket = 4
	// PacketMillis is the B0 write cadence.
	PacketMillis = FrameMillis * FramesPerPacket
	// PacketBytes is the fixed B0 length; LimitsBytes the fixed BF length.
	PacketBytes = 20
	LimitsBytes = 7
	// MaxSequence is the largest 4-bit strength-change sequence number.
	MaxSequence = 15

	headerPacket         = 0xB0
	headerLimits         = 0xBF
	headerStrengthReport = 0xB1
)

// StrengthMode says how the device interprets a channel's strength value.
type StrengthMode uint8

// Strength interpretation modes, two bits per channel.
const (
	StrengthKeep     StrengthMode = 0b00
	StrengthIncrease StrengthMode = 0b01
	StrengthDecrease StrengthMode = 0b10
	StrengthSet      StrengthMode = 0b11
)

// StrengthChange is one channel's strength instruction inside a B0 packet.
type StrengthChange struct {
	Mode  StrengthMode
	Value int
}

// ChannelFrame is 25 ms of one channel: the output-unit period and the
// relative pulse width. Intensity 0 produces no pulses.
type ChannelFrame struct {
	PeriodMillis int
	Intensity    int
}

// Frame is one 25 ms output window for both channels.
type Frame struct {
	A ChannelFrame
	B ChannelFrame
}

// SilentFrame is a valid frame with no pulse width on either channel.
var SilentFrame = Frame{
	A: ChannelFrame{PeriodMillis: MinPeriodMillis},
	B: ChannelFrame{PeriodMillis: MinPeriodMillis},
}

// Packet is one B0 write: an optional strength change and 100 ms of waveform.
// SilenceA and SilenceB encode DG-LAB's documented single-channel form: an
// out-of-range value that makes the device discard that channel's waveform
// for the packet while its strength field still applies.
type Packet struct {
	Sequence int
	A        StrengthChange
	B        StrengthChange
	Frames   [FramesPerPacket]Frame
	SilenceA bool
	SilenceB bool
}

// Limits is the BF configuration: per-channel strength ceilings the device
// enforces against every source, including its own switches, plus the two
// balance parameters. The device keeps these values through power-off.
type Limits struct {
	StrengthA         int
	StrengthB         int
	FrequencyBalanceA int
	FrequencyBalanceB int
	IntensityBalanceA int
	IntensityBalanceB int
}

// StrengthReport is a decoded B1 notification: the device's actual channel
// strengths after a change, tagged with the sequence that caused it (0 for a
// change made on the device itself).
type StrengthReport struct {
	Sequence int
	A        int
	B        int
}

var errUnknownNotification = errors.New("not a strength report")

// Encode returns the 20-byte B0 payload. Values are validated rather than
// clamped: a caller that produced an out-of-range value has a bug, and the
// device would otherwise silently drop or reinterpret it.
func (p Packet) Encode() ([PacketBytes]byte, error) {
	var out [PacketBytes]byte
	if p.Sequence < 0 || p.Sequence > MaxSequence {
		return out, fmt.Errorf("sequence %d outside 0..%d", p.Sequence, MaxSequence)
	}
	modeA, valueA, err := encodeStrength(p.A)
	if err != nil {
		return out, fmt.Errorf("channel A: %w", err)
	}
	modeB, valueB, err := encodeStrength(p.B)
	if err != nil {
		return out, fmt.Errorf("channel B: %w", err)
	}
	out[0] = headerPacket
	out[1] = byte(p.Sequence<<4) | modeA<<2 | modeB
	out[2] = valueA
	out[3] = valueB
	if err := encodeChannel(out[4:12], p.Frames, func(frame Frame) ChannelFrame { return frame.A }, p.SilenceA); err != nil {
		return out, fmt.Errorf("channel A: %w", err)
	}
	if err := encodeChannel(out[12:20], p.Frames, func(frame Frame) ChannelFrame { return frame.B }, p.SilenceB); err != nil {
		return out, fmt.Errorf("channel B: %w", err)
	}
	return out, nil
}

func encodeStrength(change StrengthChange) (byte, byte, error) {
	if change.Mode > StrengthSet {
		return 0, 0, fmt.Errorf("unknown strength mode %d", change.Mode)
	}
	if change.Mode == StrengthKeep {
		return byte(StrengthKeep), 0, nil
	}
	if change.Value < 0 || change.Value > MaxStrength {
		return 0, 0, fmt.Errorf("strength value %d outside 0..%d", change.Value, MaxStrength)
	}
	return byte(change.Mode), byte(change.Value), nil
}

func encodeChannel(out []byte, frames [FramesPerPacket]Frame, channel func(Frame) ChannelFrame, silenced bool) error {
	if silenced {
		// DG-LAB's documented form: valid-looking periods are irrelevant once
		// one intensity exceeds 100, so the whole channel is discarded.
		copy(out, []byte{0, 0, 0, 0, 0, 0, 0, MaxIntensity + 1})
		return nil
	}
	for index, frame := range frames {
		value := channel(frame)
		period, err := PeriodByte(value.PeriodMillis)
		if err != nil {
			return err
		}
		if value.Intensity < 0 || value.Intensity > MaxIntensity {
			return fmt.Errorf("intensity %d outside 0..%d", value.Intensity, MaxIntensity)
		}
		out[index] = period
		out[FramesPerPacket+index] = byte(value.Intensity)
	}
	return nil
}

// PeriodByte converts an output-unit period of 10..1000 ms to the compressed
// 10..240 wire value with DG-LAB's published integer formula.
func PeriodByte(periodMillis int) (byte, error) {
	switch {
	case periodMillis < MinPeriodMillis || periodMillis > MaxPeriodMillis:
		return 0, fmt.Errorf("period %d ms outside %d..%d", periodMillis, MinPeriodMillis, MaxPeriodMillis)
	case periodMillis <= 100:
		return byte(periodMillis), nil
	case periodMillis <= 600:
		return byte((periodMillis-100)/5 + 100), nil
	default:
		return byte((periodMillis-600)/10 + 200), nil
	}
}

// Encode returns the 7-byte BF payload.
func (l Limits) Encode() ([LimitsBytes]byte, error) {
	var out [LimitsBytes]byte
	out[0] = headerLimits
	fields := []struct {
		value int
		high  int
	}{
		{l.StrengthA, MaxStrength}, {l.StrengthB, MaxStrength},
		{l.FrequencyBalanceA, 255}, {l.FrequencyBalanceB, 255},
		{l.IntensityBalanceA, 255}, {l.IntensityBalanceB, 255},
	}
	for index, field := range fields {
		value, err := boundedByte(field.value, field.high)
		if err != nil {
			return out, err
		}
		out[index+1] = value
	}
	return out, nil
}

// boundedByte converts value after checking it against 0..high (high <= 255).
func boundedByte(value, high int) (byte, error) {
	if value < 0 || value > high || value > 255 {
		return 0, fmt.Errorf("value %d outside 0..%d", value, high)
	}
	return byte(value), nil
}

// DecodeNotification parses a 0x150B notification. It reports ok=false for
// message types other than B1 (the deprecated BE included).
func DecodeNotification(payload []byte) (report StrengthReport, ok bool, err error) {
	if len(payload) == 0 || payload[0] != headerStrengthReport {
		return StrengthReport{}, false, nil
	}
	if len(payload) < 4 {
		return StrengthReport{}, false, fmt.Errorf("%w: B1 has %d bytes", errUnknownNotification, len(payload))
	}
	report = StrengthReport{Sequence: int(payload[1]), A: int(payload[2]), B: int(payload[3])}
	if report.A > MaxStrength || report.B > MaxStrength {
		return StrengthReport{}, false, fmt.Errorf("%w: strength %d/%d outside 0..%d", errUnknownNotification, report.A, report.B, MaxStrength)
	}
	return report, true, nil
}

// PeriodFromByte inverts PeriodByte for a valid 10..240 wire value, returning
// the shortest period that encodes to it.
func PeriodFromByte(value byte) (int, error) {
	switch {
	case value < MinPeriodMillis || value > 240:
		return 0, fmt.Errorf("period byte %d outside 10..240", value)
	case value <= 100:
		return int(value), nil
	case value <= 200:
		return (int(value)-100)*5 + 100, nil
	default:
		return (int(value)-200)*10 + 600, nil
	}
}

// DecodePacket parses a B0 payload the way the device reads it. A channel
// with any out-of-range waveform value is reported as silenced, matching the
// device discarding that channel's four frames.
func DecodePacket(payload []byte) (Packet, error) {
	if len(payload) != PacketBytes || payload[0] != headerPacket {
		return Packet{}, fmt.Errorf("not a %d-byte B0 packet", PacketBytes)
	}
	packet := Packet{
		Sequence: int(payload[1] >> 4),
		A:        StrengthChange{Mode: StrengthMode(payload[1] >> 2 & 0b11), Value: int(payload[2])},
		B:        StrengthChange{Mode: StrengthMode(payload[1] & 0b11), Value: int(payload[3])},
	}
	packet.SilenceA = decodeChannel(payload[4:12], &packet.Frames, func(frame *Frame) *ChannelFrame { return &frame.A })
	packet.SilenceB = decodeChannel(payload[12:20], &packet.Frames, func(frame *Frame) *ChannelFrame { return &frame.B })
	return packet, nil
}

func decodeChannel(data []byte, frames *[FramesPerPacket]Frame, channel func(*Frame) *ChannelFrame) bool {
	for index := range frames {
		period, err := PeriodFromByte(data[index])
		if err != nil || data[FramesPerPacket+index] > MaxIntensity {
			for other := range frames {
				*channel(&frames[other]) = ChannelFrame{}
			}
			return true
		}
		*channel(&frames[index]) = ChannelFrame{PeriodMillis: period, Intensity: int(data[FramesPerPacket+index])}
	}
	return false
}

// DecodeLimits parses a BF payload.
func DecodeLimits(payload []byte) (Limits, error) {
	if len(payload) != LimitsBytes || payload[0] != headerLimits {
		return Limits{}, fmt.Errorf("not a %d-byte BF packet", LimitsBytes)
	}
	return Limits{
		StrengthA:         int(payload[1]),
		StrengthB:         int(payload[2]),
		FrequencyBalanceA: int(payload[3]),
		FrequencyBalanceB: int(payload[4]),
		IntensityBalanceA: int(payload[5]),
		IntensityBalanceB: int(payload[6]),
	}, nil
}
