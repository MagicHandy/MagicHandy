package handyble

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

// These vectors come from the existing browser encoder and are checked there
// too. They lock compatibility at the wire boundary instead of comparing two
// invocations of the new encoder.
func TestBrowserWireParity(t *testing.T) {
	data, err := os.ReadFile("testdata/browser-wire-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Path string
		ID   uint32
		Hex  string
		Body struct {
			StreamID    uint32 `json:"stream_id"`
			Min, Max    float32
			Points      []struct{ T, X uint32 }
			Tail        uint32  `json:"tail_point_stream_index"`
			Start       int32   `json:"start_time"`
			ServerTime  *uint64 `json:"server_time"`
			ClockOffset int64   `json:"clock_offset"`
			RTD         int32   `json:"rtd"`
		}
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	methods := map[string]Method{"hsp/stop": Stop, "hsp/setup": Setup, "slider/stroke": Stroke, "hsp/add": Add, "hsp/play": Play, "clock/offset/set": ClockSet, "clock/offset/get": ClockGet}
	for _, v := range vectors {
		t.Run(v.Path, func(t *testing.T) {
			r := Request{Method: methods[v.Path], ID: v.ID, StreamID: v.Body.StreamID, Min: v.Body.Min / 100, Max: v.Body.Max / 100, Tail: v.Body.Tail, Start: v.Body.Start, ServerTime: v.Body.ServerTime, ClockOffset: v.Body.ClockOffset, RoundTrip: v.Body.RTD}
			for _, p := range v.Body.Points {
				r.Points = append(r.Points, Point{Time: p.T, Position: p.X})
			}
			got, err := Encode(r)
			if err != nil || hex.EncodeToString(got) != v.Hex {
				t.Fatalf("wire = %x, %v; want %s", got, err, v.Hex)
			}
		})
	}
}

func TestDecodeOmittedZerosErrorsAndBounds(t *testing.T) {
	response := appendBytes(appendUint(nil, 1, 3), 4, appendBytes(appendUint(nil, 1, 2), 867, appendBytes(nil, 1, []byte{})))
	got, err := Decode(response)
	if err != nil || got.ID != 2 || got.State == nil || got.State.PlayState != "not_initialized" || got.State.Tail != 0 {
		t.Fatalf("omitted zeros: %+v %v", got, err)
	}
	errOnly := appendBytes(appendUint(nil, 1, 3), 4, appendBytes(appendUint(nil, 1, 2), 2, []byte{}))
	if got, err := Decode(errOnly); err != nil || got.ErrorCode == nil || *got.ErrorCode != 0 {
		t.Fatalf("code-only error: %+v %v", got, err)
	}
	for _, data := range [][]byte{{0}, {0x08, 0x80}, {0x22, 0x7f}, make([]byte, 513), {0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x02}} {
		if _, err := Decode(data); err == nil {
			t.Fatalf("accepted malformed frame %x", data)
		}
	}
	unknown := appendUint(response, 2000, 7)
	if _, err := Decode(unknown); err != nil {
		t.Fatal(err)
	}
}

func FuzzDecode(f *testing.F) {
	f.Add([]byte{8, 3, 34, 2, 8, 1})
	f.Fuzz(func(_ *testing.T, data []byte) { _, _ = Decode(data) })
}
