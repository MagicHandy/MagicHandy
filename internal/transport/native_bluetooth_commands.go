package transport

import (
	"errors"
	"fmt"
	"github.com/mapledaemon/MagicHandy/internal/handyble"
	"math"
)

// The shared HSP transport constructs these bodies with validated, typed Go
// values. No browser or HTTP client may submit raw native commands.
func nativeHandyRequest(path string, body map[string]any) (handyble.Request, error) {
	var request handyble.Request
	switch path {
	case "hsp/stop":
		// A connected Handy may still be in a mode selected by its buttons or
		// another client. HSP Stop rejects that state with ERROR_WRONG_MODE.
		request.Method = handyble.StopCurrentMode
	case "hsp/state":
		request.Method = handyble.State
	case "slider/stroke":
		request.Method = handyble.Stroke
		minValue, minOK := body["min"].(int)
		maxValue, maxOK := body["max"].(int)
		if !minOK || !maxOK {
			return request, errors.New("invalid Handy stroke values")
		}
		request.Min, request.Max = float32(minValue)/100, float32(maxValue)/100
	case "hsp/add", "hsp/play":
		stream, ok := body["stream_id"].(int)
		if !ok || stream < 0 || int64(stream) > math.MaxUint32 {
			return request, errors.New("invalid Handy stream ID")
		}
		request.StreamID = uint32(stream)
		if path == "hsp/play" {
			request.Method = handyble.Play
			start, ok := body["start_time"].(int64)
			if !ok || start < math.MinInt32 || start > math.MaxInt32 {
				return request, errors.New("invalid Handy start time")
			}
			request.Start = int32(start)
		} else {
			request.Method = handyble.Add
			tail, ok := body["tail_point_stream_index"].(int)
			if !ok || tail < 0 || int64(tail) > math.MaxUint32 {
				return request, errors.New("invalid Handy point index")
			}
			request.Tail = uint32(tail)
			var err error
			request.Points, err = nativeHandyPoints(body["points"])
			if err != nil {
				return request, err
			}
		}
	default:
		return request, fmt.Errorf("native Handy command is not implemented: %s", path)
	}
	return request, nil
}

func nativeHandyPoints(value any) ([]handyble.Point, error) {
	points, ok := value.([]map[string]any)
	if !ok {
		return nil, errors.New("invalid Handy points")
	}
	result := make([]handyble.Point, 0, len(points))
	for _, p := range points {
		x, xOK := p["x"].(float64)
		t, tOK := p["t"].(int64)
		if !xOK || !tOK || t < 0 || t > math.MaxUint32 || math.IsNaN(x) || x < 0 || x > 100 || math.Trunc(x) != x {
			return nil, errors.New("invalid Handy point values")
		}
		result = append(result, handyble.Point{Time: uint32(t), Position: uint32(x)})
	}
	return result, nil
}
