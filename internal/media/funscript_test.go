package media

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFunscriptNormalizesActionsAndBuildsClockLockedSlice(t *testing.T) {
	catalog := openTestCatalog(t)
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "Session.mp4"), "video")
	writeTestFile(t, filepath.Join(root, "Session.funscript"), `{
		"version":"1.0",
		"actions":[
			{"at":1500,"pos":10},
			{"at":500,"pos":20},
			{"at":1000,"pos":80},
			{"at":1000,"pos":70}
		]
	}`)
	runTestScan(t, catalog, root)
	video := listTestVideos(t, catalog)[0]

	script, err := catalog.LoadFunscript(t.Context(), video.ID)
	if err != nil {
		t.Fatalf("LoadFunscript: %v", err)
	}
	if script.VideoID != video.ID || script.ActionCount != 4 || script.DurationMillis != 1500 {
		t.Fatalf("script metadata = %+v", script)
	}
	if script.Actions[0] != (FunscriptAction{AtMillis: 0, Position: 20}) ||
		script.Actions[2] != (FunscriptAction{AtMillis: 1000, Position: 70}) {
		t.Fatalf("normalized actions = %+v", script.Actions)
	}

	timeline, err := script.TimelineFrom(750, 2)
	if err != nil {
		t.Fatalf("TimelineFrom: %v", err)
	}
	if timeline.DurationMillis != 375 || len(timeline.Points) != 3 {
		t.Fatalf("timeline = %+v", timeline)
	}
	if got := timeline.Points[0].PositionPercent; math.Abs(got-45) > 0.001 {
		t.Fatalf("interpolated start = %.3f, want 45", got)
	}
	if timeline.Points[1].TimeMillis != 125 || timeline.Points[2].TimeMillis != 375 {
		t.Fatalf("rate-scaled points = %+v", timeline.Points)
	}
}

func TestLoadFunscriptRejectsInvalidAndOversizedDocuments(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		content string
		want    error
	}{
		{name: "malformed", content: `{`, want: ErrFunscriptInvalid},
		{name: "missing fields", content: `{"actions":[{"at":0},{"at":10,"pos":20}]}`, want: ErrFunscriptInvalid},
		{name: "position", content: `{"actions":[{"at":0,"pos":-1},{"at":10,"pos":20}]}`, want: ErrFunscriptInvalid},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			catalog := openTestCatalog(t)
			root := t.TempDir()
			writeTestFile(t, filepath.Join(root, "sample.mp4"), "video")
			writeTestFile(t, filepath.Join(root, "sample.funscript"), testCase.content)
			runTestScan(t, catalog, root)
			_, err := catalog.LoadFunscript(t.Context(), listTestVideos(t, catalog)[0].ID)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("LoadFunscript error = %v, want %v", err, testCase.want)
			}
		})
	}

	catalog := openTestCatalog(t)
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "large.mp4"), "video")
	path := filepath.Join(root, "large.funscript")
	file, err := os.Create(path) // #nosec G304 -- path is inside t.TempDir.
	if err != nil {
		t.Fatalf("create oversized script: %v", err)
	}
	if err := file.Truncate(MaxMediaFunscriptBytes + 1); err != nil {
		_ = file.Close()
		t.Fatalf("truncate oversized script: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close oversized script: %v", err)
	}
	runTestScan(t, catalog, root)
	_, err = catalog.LoadFunscript(t.Context(), listTestVideos(t, catalog)[0].ID)
	if !errors.Is(err, ErrFunscriptTooLarge) {
		t.Fatalf("oversized error = %v, want %v", err, ErrFunscriptTooLarge)
	}
}

func TestTimelineFromRejectsCompletedOrInvalidRate(t *testing.T) {
	script := Funscript{
		VideoID:        "video",
		Name:           "Video",
		DurationMillis: 1000,
		Actions:        []FunscriptAction{{AtMillis: 0, Position: 0}, {AtMillis: 1000, Position: 100}},
	}
	if _, err := script.TimelineFrom(1000, 1); !errors.Is(err, ErrFunscriptComplete) {
		t.Fatalf("completed error = %v", err)
	}
	if _, err := script.TimelineFrom(0, 5); err == nil {
		t.Fatal("out-of-range playback rate was accepted")
	}
}

func TestTimelineFromAnchorsArbitraryVideoTimestamps(t *testing.T) {
	script := Funscript{
		VideoID: "seek-test",
		Name:    "Seek test",
		Actions: []FunscriptAction{
			{AtMillis: 0, Position: 0},
			{AtMillis: 1000, Position: 100},
			{AtMillis: 1500, Position: 25},
			{AtMillis: 5000, Position: 75},
		},
		DurationMillis: 5000,
	}
	for _, test := range []struct {
		name          string
		at            int64
		rate          float64
		wantPosition  float64
		wantNextTime  int64
		wantNextValue float64
	}{
		{name: "exact action", at: 1000, rate: 1, wantPosition: 100, wantNextTime: 500, wantNextValue: 25},
		{name: "between actions", at: 1250, rate: 1, wantPosition: 62.5, wantNextTime: 250, wantNextValue: 25},
		{name: "playback rate", at: 1250, rate: 2, wantPosition: 62.5, wantNextTime: 125, wantNextValue: 25},
		{name: "near end", at: 4999, rate: 1, wantPosition: 74.985714, wantNextTime: 1, wantNextValue: 75},
	} {
		t.Run(test.name, func(t *testing.T) {
			timeline, err := script.TimelineFrom(test.at, test.rate)
			if err != nil {
				t.Fatalf("TimelineFrom: %v", err)
			}
			if len(timeline.Points) < 2 {
				t.Fatalf("timeline points = %+v, want exact anchor and a future action", timeline.Points)
			}
			if got := timeline.Points[0]; got.TimeMillis != 0 || math.Abs(got.PositionPercent-test.wantPosition) > 0.0001 {
				t.Fatalf("anchor = %+v, want t=0 position %.6f", got, test.wantPosition)
			}
			if got := timeline.Points[1]; got.TimeMillis != test.wantNextTime || got.PositionPercent != test.wantNextValue {
				t.Fatalf("next point = %+v, want t=%d position %.2f", got, test.wantNextTime, test.wantNextValue)
			}
		})
	}
}

func TestTimelineFromPreservesReportedReversalsNearOneSeventeen(t *testing.T) {
	script := Funscript{
		VideoID: "reported-script", Name: "Reported script", DurationMillis: 83_703,
		Actions: []FunscriptAction{
			{AtMillis: 60_000, Position: 60},
			{AtMillis: 70_017, Position: 52},
			{AtMillis: 70_905, Position: 84},
			{AtMillis: 72_218, Position: 46},
			{AtMillis: 73_152, Position: 84},
			{AtMillis: 74_286, Position: 43},
			{AtMillis: 75_011, Position: 73},
			{AtMillis: 76_157, Position: 46},
			{AtMillis: 77_134, Position: 81},
			{AtMillis: 78_088, Position: 41},
			{AtMillis: 79_012, Position: 66},
			{AtMillis: 79_988, Position: 37},
			{AtMillis: 80_980, Position: 67},
			{AtMillis: 82_095, Position: 35},
			{AtMillis: 82_786, Position: 70},
			{AtMillis: 83_703, Position: 41},
		},
	}
	timeline, err := script.TimelineFrom(61_863, 1)
	if err != nil {
		t.Fatalf("TimelineFrom: %v", err)
	}

	want := []FunscriptAction{
		{AtMillis: 14_294, Position: 46},
		{AtMillis: 15_271, Position: 81},
		{AtMillis: 16_225, Position: 41},
	}
	for _, action := range want {
		found := false
		for _, point := range timeline.Points {
			if point.TimeMillis == action.AtMillis && point.PositionPercent == float64(action.Position) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("timeline points = %+v, missing reported reversal %+v", timeline.Points, action)
		}
	}
}
