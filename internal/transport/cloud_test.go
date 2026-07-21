package transport

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"strings"
	"testing"
)

const cloudSecretFixture = "user-secret-key"

func TestCloudRequestGoldenShape(t *testing.T) {
	builder := newCloudBuilder(t, CloudBuildOptions{})
	requests := buildGoldenCloudRequests(t, builder)

	got, err := json.MarshalIndent(requests, "", "  ")
	if err != nil {
		t.Fatalf("marshal cloud requests: %v", err)
	}
	got = append(got, '\n')

	want, err := os.ReadFile("testdata/cloud_requests.golden.json")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	want = []byte(strings.ReplaceAll(string(want), "\r\n", "\n"))
	if string(got) != string(want) {
		t.Fatalf("cloud request shape mismatch\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestHSPPointValuesStayZeroToOneHundred(t *testing.T) {
	builder := newCloudBuilder(t, CloudBuildOptions{})
	request, err := builder.BuildHSPAdd(AppendPointsCommand{
		StreamID: "bounds",
		Points: []TimedPoint{
			{PositionPercent: 0, TimeMillis: 0},
			{PositionPercent: 100, TimeMillis: 250},
		},
	})
	if err != nil {
		t.Fatalf("BuildHSPAdd: %v", err)
	}

	points := request.Body.(cloudHSPAddBody).Points
	if points[0].X != 0 || points[1].X != 100 {
		t.Fatalf("points = %+v, want 0 and 100", points)
	}

	_, err = builder.BuildHSPAdd(AppendPointsCommand{
		StreamID: "bad",
		Points:   []TimedPoint{{PositionPercent: 101, TimeMillis: 0}},
	})
	if err == nil {
		t.Fatal("invalid HSP x value was accepted")
	}
}

func TestCloudHandyEncodingQuantizesFractionalPointsAndRejectsNonFinite(t *testing.T) {
	builder := newCloudBuilder(t, CloudBuildOptions{ReverseDirection: true})
	request, err := builder.BuildHSPAdd(AppendPointsCommand{
		StreamID: "fractional",
		Points:   []TimedPoint{{PositionPercent: 25.25, TimeMillis: 0}, {PositionPercent: 75.75, TimeMillis: 250}},
	})
	if err != nil {
		t.Fatalf("BuildHSPAdd: %v", err)
	}
	points := request.Body.(cloudHSPAddBody).Points
	if points[0].X != 75 || points[1].X != 24 {
		t.Fatalf("encoded points = %+v, want rounded reverse positions 75 and 24", points)
	}

	for _, position := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		_, err := builder.BuildHSPAdd(AppendPointsCommand{
			StreamID: "non-finite",
			Points:   []TimedPoint{{PositionPercent: position}},
		})
		if err == nil {
			t.Fatalf("non-finite position %v was accepted", position)
		}
	}
}

func TestCloudReverseQuantizationMirrorsNativeSteps(t *testing.T) {
	for _, position := range []float64{0, 0.5, 25.5, 50.5, 99.5, 100} {
		forward, ok := quantizeHandyPosition(position, false)
		if !ok {
			t.Fatalf("forward position %.1f was rejected", position)
		}
		reverse, ok := quantizeHandyPosition(position, true)
		if !ok || reverse != 100-forward {
			t.Fatalf("position %.1f encoded as forward=%d reverse=%d, want mirrored native steps", position, forward, reverse)
		}
	}
}

func TestCloudHSPAddRejectsOversizedPointBatch(t *testing.T) {
	points := make([]TimedPoint, maximumCloudHSPAddPoints+1)
	for index := range points {
		points[index] = TimedPoint{PositionPercent: 50, TimeMillis: int64(index)}
	}
	_, err := newCloudBuilder(t, CloudBuildOptions{}).BuildHSPAdd(AppendPointsCommand{
		StreamID: "oversized", Points: points,
	})
	if err == nil || !strings.Contains(err.Error(), "at most 100") {
		t.Fatalf("oversized HSP add error = %v, want point-limit rejection", err)
	}
}

func TestStrokeWindowDoesNotRewriteHSPPoints(t *testing.T) {
	builder := newCloudBuilder(t, CloudBuildOptions{})
	window, err := builder.BuildStrokeWindow(StrokeWindowCommand{MinPercent: 20, MaxPercent: 80})
	if err != nil {
		t.Fatalf("BuildStrokeWindow: %v", err)
	}
	if window.Body.(cloudStrokeWindowBody).Min != 0.2 || window.Body.(cloudStrokeWindowBody).Max != 0.8 {
		t.Fatalf("stroke window = %+v, want 0.2..0.8", window.Body)
	}

	add, err := builder.BuildHSPAdd(AppendPointsCommand{
		StreamID: "range",
		Points: []TimedPoint{
			{PositionPercent: 0, TimeMillis: 0},
			{PositionPercent: 50, TimeMillis: 125},
			{PositionPercent: 100, TimeMillis: 250},
		},
	})
	if err != nil {
		t.Fatalf("BuildHSPAdd: %v", err)
	}

	points := add.Body.(cloudHSPAddBody).Points
	if points[0].X != 0 || points[1].X != 50 || points[2].X != 100 {
		t.Fatalf("stroke window was baked into HSP points: %+v", points)
	}
}

func TestReverseDirectionMapsAtTransportBoundary(t *testing.T) {
	semantic := AppendPointsCommand{
		StreamID: "reverse",
		Points: []TimedPoint{
			{PositionPercent: 15, TimeMillis: 0},
			{PositionPercent: 85, TimeMillis: 250},
		},
	}
	builder := newCloudBuilder(t, CloudBuildOptions{ReverseDirection: true})

	request, err := builder.BuildHSPAdd(semantic)
	if err != nil {
		t.Fatalf("BuildHSPAdd: %v", err)
	}

	points := request.Body.(cloudHSPAddBody).Points
	if points[0].X != 85 || points[1].X != 15 {
		t.Fatalf("reverse points = %+v, want 85 and 15", points)
	}
	if semantic.Points[0].PositionPercent != 15 || semantic.Points[1].PositionPercent != 85 {
		t.Fatalf("semantic command was mutated: %+v", semantic.Points)
	}
}

func TestHSPSpeedIntentDoesNotBecomePhysicalVelocity(t *testing.T) {
	builder := newCloudBuilder(t, CloudBuildOptions{})
	request, err := builder.BuildHSPAdd(AppendPointsCommand{
		StreamID: "timing",
		Points: []TimedPoint{
			{PositionPercent: 10, TimeMillis: 100},
			{PositionPercent: 90, TimeMillis: 375},
		},
	})
	if err != nil {
		t.Fatalf("BuildHSPAdd: %v", err)
	}

	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	if strings.Contains(string(data), "velocity") || strings.Contains(string(data), "speed_percent") {
		t.Fatalf("HSP request contains physical speed feedback fields: %s", data)
	}
	points := request.Body.(cloudHSPAddBody).Points
	if points[0].T != 100 || points[1].T != 375 {
		t.Fatalf("HSP timestamp spacing changed: %+v", points)
	}
}

func TestHSPUnavailableErrorsAreSpecificAndNoFallback(t *testing.T) {
	tests := []struct {
		name          string
		prerequisite  CloudPrerequisites
		wantCode      string
		wantField     string
		wantSubstring string
	}{
		{
			name:          "missing application id",
			prerequisite:  validCloudPrerequisites(func(p *CloudPrerequisites) { p.ApplicationID = "" }),
			wantCode:      "missing_application_id",
			wantField:     "application_id",
			wantSubstring: "Application ID",
		},
		{
			name:          "malformed connection key",
			prerequisite:  validCloudPrerequisites(func(p *CloudPrerequisites) { p.ConnectionKey = "bad" }),
			wantCode:      "malformed_connection_key",
			wantField:     "connection_key",
			wantSubstring: "connection key",
		},
		{
			name:          "firmware v4 required",
			prerequisite:  validCloudPrerequisites(func(p *CloudPrerequisites) { p.FirmwareMajor = 3 }),
			wantCode:      "firmware_v4_required",
			wantField:     "firmware_major",
			wantSubstring: "firmware v4",
		},
		{
			name:          "api v3 required",
			prerequisite:  validCloudPrerequisites(func(p *CloudPrerequisites) { p.APIMajor = 2 }),
			wantCode:      "api_v3_required",
			wantField:     "api_major",
			wantSubstring: "API v3",
		},
		{
			name:          "hsp unavailable",
			prerequisite:  validCloudPrerequisites(func(p *CloudPrerequisites) { p.HSPAvailable = false }),
			wantCode:      "hsp_unavailable",
			wantField:     "hsp",
			wantSubstring: "HSP is unavailable",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewCloudRESTBuilder(test.prerequisite, CloudBuildOptions{})
			var unavailable HSPUnavailableError
			if !errors.As(err, &unavailable) {
				t.Fatalf("error = %T %[1]v, want HSPUnavailableError", err)
			}
			if unavailable.Code != test.wantCode || unavailable.Field != test.wantField {
				t.Fatalf("error = %+v, want code %q field %q", unavailable, test.wantCode, test.wantField)
			}
			if !unavailable.NoFallback {
				t.Fatalf("error = %+v, want no fallback", unavailable)
			}
			if !strings.Contains(unavailable.Message, test.wantSubstring) {
				t.Fatalf("message = %q, want substring %q", unavailable.Message, test.wantSubstring)
			}
		})
	}
}

func TestCloudRequestsOmitSecrets(t *testing.T) {
	builder := newCloudBuilder(t, CloudBuildOptions{})
	requests := buildGoldenCloudRequests(t, builder)
	for _, request := range requests {
		data, err := json.Marshal(request)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		if strings.Contains(string(data), cloudSecretFixture) {
			t.Fatalf("cloud request leaked connection key: %s", data)
		}
	}
}

func newCloudBuilder(t *testing.T, options CloudBuildOptions) *CloudRESTBuilder {
	t.Helper()

	builder, err := NewCloudRESTBuilder(validCloudPrerequisites(), options)
	if err != nil {
		t.Fatalf("NewCloudRESTBuilder: %v", err)
	}
	return builder
}

func buildGoldenCloudRequests(t *testing.T, builder *CloudRESTBuilder) []CloudRequest {
	t.Helper()

	strokeWindow, err := builder.BuildStrokeWindow(StrokeWindowCommand{MinPercent: 10, MaxPercent: 90})
	if err != nil {
		t.Fatalf("BuildStrokeWindow: %v", err)
	}
	add, err := builder.BuildHSPAdd(AppendPointsCommand{
		StreamID: "stream-1",
		Points: []TimedPoint{
			{PositionPercent: 0, TimeMillis: 0},
			{PositionPercent: 50, TimeMillis: 125},
			{PositionPercent: 100, TimeMillis: 250},
		},
	})
	if err != nil {
		t.Fatalf("BuildHSPAdd: %v", err)
	}
	play, err := builder.BuildHSPPlay(PlayCommand{StreamID: "stream-1"})
	if err != nil {
		t.Fatalf("BuildHSPPlay: %v", err)
	}
	setup, err := builder.BuildHSPSetup(HSPSetupCommand{StreamID: 1})
	if err != nil {
		t.Fatalf("BuildHSPSetup: %v", err)
	}
	return []CloudRequest{
		strokeWindow,
		setup,
		add,
		play,
		builder.BuildStop(),
	}
}

func validCloudPrerequisites(mutators ...func(*CloudPrerequisites)) CloudPrerequisites {
	prerequisites := CloudPrerequisites{
		ApplicationID: "app-public-id",
		ConnectionKey: cloudSecretFixture,
		FirmwareMajor: 4,
		APIMajor:      3,
		HSPAvailable:  true,
	}
	for _, mutate := range mutators {
		mutate(&prerequisites)
	}
	return prerequisites
}
