package transport

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestTransportOwnersPreserveNeutralFrameContract runs one semantic fixture
// through every dispatch owner. Handy owners retain neutral point timing and
// let the device apply its stroke window; Intiface projects that same window
// host-side and converts consecutive points directly to LinearCmd segments.
//
//nolint:funlen // Keeping all three owner subtests together makes contract drift visible in one fixture.
func TestTransportOwnersPreserveNeutralFrameContract(t *testing.T) {
	fixture := AppendPointsCommand{
		StreamID: "contract",
		Points: []TimedPoint{
			{PositionPercent: 10.25, TimeMillis: 0},
			{PositionPercent: 50.5, TimeMillis: 40},
			{PositionPercent: 89.75, TimeMillis: 100},
		},
	}
	window := StrokeWindowCommand{MinPercent: 20, MaxPercent: 80, ReverseDirection: true}

	t.Run("cloud_rest", func(t *testing.T) {
		var mu sync.Mutex
		var bodies = map[string][]byte{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			bodies[r.URL.Path] = body
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"hsp_available":true}`))
		}))
		defer server.Close()
		owner := newTestCloudTransport(t, server.URL)
		if _, err := owner.SetStrokeWindow(context.Background(), window); err != nil {
			t.Fatal(err)
		}
		if _, err := owner.AppendPoints(context.Background(), fixture); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		windowBody := append([]byte(nil), bodies["/slider/stroke"]...)
		pointsBody := append([]byte(nil), bodies["/hsp/add"]...)
		mu.Unlock()
		assertHandyContractPayload(t, windowBody, pointsBody)
		assertSemanticFixtureUnchanged(t, fixture)
	})

	t.Run("browser_bluetooth", func(t *testing.T) {
		bridge := NewBrowserBluetoothBridge()
		connected := true
		bridge.ConnectClient(BrowserBluetoothClientStatus{ClientID: "contract-client", Connected: &connected})
		owner := newTestBrowserBluetoothTransport(t, bridge, BrowserBluetoothOptions{})

		windowDone := make(chan resultAndError, 1)
		go func() {
			result, err := owner.SetStrokeWindow(context.Background(), window)
			windowDone <- resultAndError{result: result, err: err}
		}()
		windowCommands, err := bridge.NextCommands(context.Background(), "contract-client", time.Second)
		if err != nil || len(windowCommands) != 1 {
			t.Fatalf("stroke-window command = %+v, %v", windowCommands, err)
		}
		bridge.Acknowledge("contract-client", BrowserBluetoothBridgeAck{ID: windowCommands[0].ID, OK: true})
		if outcome := readBluetoothOutcome(t, windowDone); outcome.err != nil {
			t.Fatal(outcome.err)
		}

		pointsDone := make(chan resultAndError, 1)
		go func() {
			result, err := owner.AppendPoints(context.Background(), fixture)
			pointsDone <- resultAndError{result: result, err: err}
		}()
		pointCommands, err := bridge.NextCommands(context.Background(), "contract-client", time.Second)
		if err != nil || len(pointCommands) != 1 {
			t.Fatalf("points command = %+v, %v", pointCommands, err)
		}
		bridge.Acknowledge("contract-client", BrowserBluetoothBridgeAck{ID: pointCommands[0].ID, OK: true})
		if outcome := readBluetoothOutcome(t, pointsDone); outcome.err != nil {
			t.Fatal(outcome.err)
		}

		if windowCommands[0].Body["min"] != 20 || windowCommands[0].Body["max"] != 80 {
			t.Fatalf("stroke window = %+v, want 20..80", windowCommands[0].Body)
		}
		points := pointCommands[0].Body["points"].([]map[string]any)
		assertHandyPoints(t, points)
		assertSemanticFixtureUnchanged(t, fixture)
	})

	t.Run("intiface", func(t *testing.T) {
		server := newFakeButtplugServer(t, 1000)
		defer server.Close()
		owner := connectTestIntiface(t, server, 32)
		defer closeTestIntiface(t, owner)
		if err := owner.SelectDevice(7, 0); err != nil {
			t.Fatal(err)
		}
		if _, err := owner.SetStrokeWindow(context.Background(), window); err != nil {
			t.Fatal(err)
		}
		if _, err := owner.AppendPoints(context.Background(), fixture); err != nil {
			t.Fatal(err)
		}
		if _, err := owner.Play(context.Background(), PlayCommand{StreamID: fixture.StreamID}); err != nil {
			t.Fatal(err)
		}
		first := server.waitForKind(t, "LinearCmd", time.Second)
		second := server.waitForKind(t, "LinearCmd", time.Second)
		assertLinearVector(t, first, 0, 40, 0.497)
		assertLinearVector(t, second, 0, 60, 0.2615)
		assertSemanticFixtureUnchanged(t, fixture)
		_, _ = owner.Stop(context.Background(), StopCommand{Reason: "contract_complete"})
	})
}

func assertHandyContractPayload(t *testing.T, windowBody, pointsBody []byte) {
	t.Helper()
	var stroke struct {
		Min float64 `json:"min"`
		Max float64 `json:"max"`
	}
	if err := json.Unmarshal(windowBody, &stroke); err != nil || stroke.Min != 0.2 || stroke.Max != 0.8 {
		t.Fatalf("stroke window = %s (%+v, %v), want 0.2..0.8", windowBody, stroke, err)
	}
	var appendBody struct {
		Points []struct {
			X int   `json:"x"`
			T int64 `json:"t"`
		} `json:"points"`
	}
	if err := json.Unmarshal(pointsBody, &appendBody); err != nil {
		t.Fatalf("decode Handy points: %v", err)
	}
	points := make([]map[string]any, len(appendBody.Points))
	for index, point := range appendBody.Points {
		points[index] = map[string]any{"x": point.X, "t": point.T}
	}
	assertHandyPoints(t, points)
}

func assertHandyPoints(t *testing.T, points []map[string]any) {
	t.Helper()
	if len(points) != 3 {
		t.Fatalf("point count = %d, want 3 (no resampling)", len(points))
	}
	wantX := []int{90, 50, 10}
	wantT := []int64{0, 40, 100}
	for index, point := range points {
		gotX := int64(numberValue(point["x"]))
		gotT := int64(numberValue(point["t"]))
		if gotX != int64(wantX[index]) || gotT != wantT[index] {
			t.Fatalf("point %d = %+v, want x=%d t=%d", index, point, wantX[index], wantT[index])
		}
	}
}

func numberValue(value any) float64 {
	switch number := value.(type) {
	case int:
		return float64(number)
	case int64:
		return float64(number)
	case float64:
		return number
	default:
		return math.NaN()
	}
}

func assertSemanticFixtureUnchanged(t *testing.T, fixture AppendPointsCommand) {
	t.Helper()
	wantPositions := []float64{10.25, 50.5, 89.75}
	wantTimes := []int64{0, 40, 100}
	for index, point := range fixture.Points {
		if point.PositionPercent != wantPositions[index] || point.TimeMillis != wantTimes[index] {
			t.Fatalf("semantic fixture mutated at %d: %+v", index, point)
		}
	}
}
