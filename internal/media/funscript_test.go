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
