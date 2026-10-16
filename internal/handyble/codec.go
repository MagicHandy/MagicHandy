// Package handyble implements firmware-4 Handy RPC over a byte-oriented BLE link.
// It knows the device protocol, not the motion engine or the native radio stack.
package handyble

import (
	"errors"
	"fmt"
	"math"
)

// Method is a firmware RPC field number from handy-public-rpc commit 2398008c.
type Method uint32

// Supported RPC methods use the same wire contract as Browser Bluetooth.
const (
	StopCurrentMode Method = 715 // firmware 4.0.13+: Stop regardless of the active mode
	ClockSet        Method = 709
	ClockGet        Method = 712
	Stroke          Method = 841
	Setup           Method = 860
	Add             Method = 861
	Play            Method = 863
	Stop            Method = 864
	State           Method = 867
)

// Point carries an integer percentage and an absolute stream time in milliseconds.
type Point struct {
	Time     uint32
	Position uint32
}

// Request is a device command, after shared transport validation and quantization.
// Looping is deliberately not exposed: loss of the controller must exhaust a
// finite buffer. Play always pauses on starvation.
type Request struct {
	Method      Method
	ID          uint32
	StreamID    uint32
	Min, Max    float32 // normalized 0..1 stroke window, not point positions
	Points      []Point
	Tail        uint32
	Flush       bool
	Start       int32
	ServerTime  *uint64
	ClockOffset int64
	RoundTrip   int32
}

// Encode returns a complete protobuf RPC envelope. GATT frames must never be
// formed by splitting this byte slice; split Add point lists before encoding.
func Encode(r Request) ([]byte, error) {
	var body []byte
	switch r.Method {
	case Stroke:
		if math.IsNaN(float64(r.Min)) || math.IsNaN(float64(r.Max)) || r.Min < 0 || r.Max > 1 || r.Min >= r.Max {
			return nil, errors.New("invalid Handy stroke window")
		}
		body = appendFloat(body, 1, r.Min)
		body = appendFloat(body, 2, r.Max)
	case Setup:
		body = appendUint(body, 1, uint64(r.StreamID))
	case Add:
		if len(r.Points) == 0 || uint64(len(r.Points)) > uint64(r.Tail)+1 {
			return nil, errors.New("invalid Handy point tail")
		}
		for i, p := range r.Points {
			if p.Position > 100 || (i > 0 && p.Time <= r.Points[i-1].Time) {
				return nil, errors.New("invalid Handy point")
			}
			point := appendUint(nil, 1, uint64(p.Time))
			point = appendUint(point, 2, uint64(p.Position))
			body = appendBytes(body, 1, point)
		}
		body = appendUint(body, 2, boolUint(r.Flush))
		body = appendUint(body, 3, uint64(r.Tail))
	case Play:
		body = appendUint(body, 1, uint64(int64(r.Start))) // #nosec G115 -- protobuf int32 requires two's-complement sign extension.
		if r.ServerTime != nil {
			body = appendUint(body, 2, *r.ServerTime)
		}
		body = appendFloat(body, 3, 1)
		body = appendUint(body, 4, 0)
		body = appendUint(body, 5, 1)
	case ClockSet:
		body = appendUint(body, 1, uint64(r.ClockOffset<<1)^uint64(r.ClockOffset>>63)) // #nosec G115 -- protobuf sint64 zigzag deliberately reinterprets signed bits.
		body = appendUint(body, 2, uint64(int64(r.RoundTrip)))                         // #nosec G115 -- protobuf int32 requires two's-complement sign extension.
	case Stop, StopCurrentMode, State, ClockGet:
	default:
		return nil, fmt.Errorf("unsupported Handy RPC method %d", r.Method)
	}
	request := appendBytes(nil, uint32(r.Method), body)
	request = appendUint(request, 2, uint64(r.ID))
	return appendBytes(appendUint(nil, 1, 1), 2, request), nil
}

// HSPState contains observed firmware state, not a commanded-position estimate.
type HSPState struct {
	PlayState   string
	StreamID    uint32
	Points      uint32
	CurrentTime int32
	Tail        int32
}

// Message is a decoded response or notification with optional firmware feedback.
type Message struct {
	Notification bool
	ID           uint32
	Method       Method
	State        *HSPState
	DeviceTime   *uint32
	ErrorCode    *uint64
}

// Decode is bounded and accepts protobuf's omitted zero fields and unknown
// fields, while rejecting truncated/overflowed frames. Error presence matters
// even when firmware omits its message text or returns code zero.
func Decode(data []byte) (Message, error) {
	var message Message
	fields, err := parseFields(data)
	if err != nil {
		return message, err
	}
	typ := fields.uint(1)
	var nested []byte
	switch typ {
	case 3:
		nested = fields.bytes(4)
	case 4:
		nested = fields.bytes(5)
		message.Notification = true
	default:
		return message, errors.New("unknown Handy RPC message type")
	}
	if nested == nil {
		return message, errors.New("missing Handy RPC payload")
	}
	fields, err = parseFields(nested)
	if err != nil {
		return message, err
	}
	idField := uint32(1)
	if message.Notification {
		idField = 2
	}
	id := fields.uint(idField)
	if id > math.MaxUint32 {
		return message, errors.New("invalid Handy response ID")
	}
	message.ID = uint32(id)
	if !message.Notification {
		if e := fields.bytes(2); e != nil {
			errorFields, parseErr := parseFields(e)
			if parseErr != nil {
				return message, parseErr
			}
			code := errorFields.uint(1)
			message.ErrorCode = &code
		}
	}
	for _, f := range fields {
		if f.wire != 2 || f.number <= 10 {
			continue
		}
		if err := decodeResult(&message, f); err != nil {
			return message, err
		}
	}
	return message, nil
}

func decodeResult(message *Message, f wireField) error {
	message.Method = Method(f.number)
	if f.number == 712 && !message.Notification {
		clock, err := parseFields(f.data)
		if err != nil {
			return err
		}
		v := clock.uint(1)
		if v > math.MaxUint32 {
			return errors.New("invalid Handy clock")
		}
		t := uint32(v)
		message.DeviceTime = &t
	} else if f.number >= 860 && f.number <= 874 {
		body, err := parseFields(f.data)
		if err != nil {
			return err
		}
		if state := body.bytes(1); state != nil {
			message.State, err = decodeState(state)
			return err
		}
	}
	return nil
}

func decodeState(data []byte) (*HSPState, error) {
	f, err := parseFields(data)
	if err != nil {
		return nil, err
	}
	states := []string{"not_initialized", "playing", "stopped", "paused", "starving"}
	state := "unknown"
	if n := f.uint(1); n < uint64(len(states)) {
		state = states[n]
	}
	stream, points := f.uint(10), f.uint(2)
	if stream > math.MaxUint32 || points > math.MaxUint32 {
		return nil, errors.New("invalid Handy HSP state counter")
	}
	currentTime, err := signed32(f.uint(5))
	if err != nil {
		return nil, err
	}
	tail, err := signed32(f.uint(11))
	if err != nil {
		return nil, err
	}
	return &HSPState{PlayState: state, StreamID: uint32(stream), Points: uint32(points), CurrentTime: currentTime, Tail: tail}, nil
}

func signed32(value uint64) (int32, error) {
	if value <= math.MaxInt32 {
		return int32(value), nil
	}
	if value <= math.MaxUint32 {
		return -1 - int32(math.MaxUint32-value), nil
	}
	difference := uint64(math.MaxUint64) - value
	if difference <= math.MaxInt32 {
		return -1 - int32(difference), nil
	}
	return 0, errors.New("invalid protobuf int32")
}
