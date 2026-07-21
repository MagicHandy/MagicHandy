package patterns

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestFunscriptImportKeepsProgramsDistinctAndStripsPatternGaps(t *testing.T) {
	library, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = library.Close() })
	data := []byte(`{
		"version":"1.0","inverted":false,"actions":[
			{"at":0,"pos":20},{"at":1000,"pos":80},{"at":2000,"pos":20},
			{"at":8000,"pos":20},{"at":9000,"pos":80},{"at":10000,"pos":20}
		]}`)

	programImport, err := library.Import("example.funscript", data, importAsProgram)
	if err != nil || programImport.Program == nil || programImport.Pattern != nil {
		t.Fatalf("program import = %+v err=%v", programImport, err)
	}
	if programImport.Program.DurationMillis != 10000 || programImport.GapsStripped != 0 {
		t.Fatalf("program timing changed: %+v", programImport.Program)
	}

	patternImport, err := library.Import("example.funscript", data, importAsPattern)
	if err != nil || patternImport.Pattern == nil || patternImport.Program != nil {
		t.Fatalf("pattern import = %+v err=%v", patternImport, err)
	}
	if patternImport.GapsStripped != 1 || patternImport.Pattern.CycleMillis < motion.RoutineCycleFloorMillis {
		t.Fatalf("pattern hygiene = %+v", patternImport)
	}
	minimum, maximum := 100.0, 0.0
	for _, point := range patternImport.Pattern.Points {
		minimum = min(minimum, point.PositionPercent)
		maximum = max(maximum, point.PositionPercent)
	}
	if minimum != 0 || maximum != 100 {
		t.Fatalf("relative span = %.1f..%.1f, want 0..100", minimum, maximum)
	}
}

func TestFunscriptImportBoundsAndInversion(t *testing.T) {
	library, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = library.Close() })
	data := []byte(`{"inverted":true,"actions":[{"at":0,"pos":0},{"at":1000,"pos":100}]}`)
	result, err := library.Import("invert.funscript", data, importAsProgram)
	if err != nil {
		t.Fatal(err)
	}
	if result.Program.Points[0].PositionPercent != 100 || result.Program.Points[len(result.Program.Points)-1].PositionPercent != 0 {
		t.Fatalf("inverted points = %+v", result.Program.Points)
	}
	oversized := make([]byte, MaxImportBytes+1)
	if _, err := library.Import("large.funscript", oversized, importAsProgram); err == nil || !contains(err.Error(), "exceeds") {
		t.Fatalf("oversized import error = %v", err)
	}
	invalid := []byte(fmt.Sprintf(`{"actions":[{"at":0,"pos":0},{"at":%d,"pos":100}]}`, maxContentDuration+1))
	if _, err := library.Import("long.funscript", invalid, importAsProgram); err == nil {
		t.Fatal("import accepted overlong funscript")
	}
	if _, err := library.Import("bad-target.funscript", data, "sequence"); err == nil {
		t.Fatal("import accepted an unknown funscript target")
	}
}

func TestFunscriptImportRejectsMalformedContracts(t *testing.T) {
	library, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = library.Close() })

	for name, data := range map[string]string{
		"unknown schema":    `{"schema":"other.motion.v1","actions":[{"at":0,"pos":0},{"at":1000,"pos":100}]}`,
		"empty schema":      `{"schema":"","actions":[{"at":0,"pos":0},{"at":1000,"pos":100}]}`,
		"null schema":       `{"schema":null,"actions":[{"at":0,"pos":0},{"at":1000,"pos":100}]}`,
		"missing position":  `{"actions":[{"at":0},{"at":1000,"pos":100}]}`,
		"null position":     `{"actions":[{"at":0,"pos":null},{"at":1000,"pos":100}]}`,
		"invalid version":   `{"version":1,"actions":[{"at":0,"pos":0},{"at":1000,"pos":100}]}`,
		"invalid inversion": `{"inverted":"true","actions":[{"at":0,"pos":0},{"at":1000,"pos":100}]}`,
		"null inversion":    `{"inverted":null,"actions":[{"at":0,"pos":0},{"at":1000,"pos":100}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := library.Import("invalid.funscript", []byte(data), importAsProgram); err == nil {
				t.Fatalf("import accepted %s", name)
			}
		})
	}

	actions := make([]map[string]any, maximumRawPoints+1)
	for index := range actions {
		actions[index] = map[string]any{"at": index, "pos": index % 101}
	}
	data, err := json.Marshal(map[string]any{"actions": actions})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := library.Import("too-many.funscript", data, importAsProgram); err == nil || !contains(err.Error(), "4096") {
		t.Fatalf("oversized action count error = %v", err)
	}
}

func TestFunscriptProgramImportPreservesSourceKnots(t *testing.T) {
	library, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = library.Close() })
	data := []byte(`{"actions":[{"at":0,"pos":0},{"at":500,"pos":50},{"at":1000,"pos":100}]}`)

	result, err := library.Import("linear.funscript", data, importAsProgram)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Program.Points) != 3 || result.Program.Points[1].TimeMillis != 500 || result.Program.Points[1].PositionPercent != 50 {
		t.Fatalf("program points = %+v, want all source knots", result.Program.Points)
	}
}

func TestFunscriptPatternImportPreservesLongValidCycle(t *testing.T) {
	library, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = library.Close() })
	data := []byte(`{"actions":[{"at":0,"pos":20},{"at":3000,"pos":80},{"at":6000,"pos":20},{"at":9000,"pos":80},{"at":12000,"pos":20}]}`)

	result, err := library.Import("long-loop.funscript", data, importAsPattern)
	if err != nil {
		t.Fatal(err)
	}
	if result.Pattern == nil {
		t.Fatal("long loop imported without a pattern result")
	}
	if result.Pattern.CycleMillis != 12000 {
		t.Fatalf("cycle = %d, want selected 12000ms", result.Pattern.CycleMillis)
	}
	if len(result.Pattern.Points) != 5 || result.Pattern.Points[3].TimeMillis != 9000 {
		t.Fatalf("stored long-loop knots = %+v", result.Pattern.Points)
	}
}

func TestFunscriptPatternImportRejectsNoiseScaleAndStabilizesChatter(t *testing.T) {
	library, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = library.Close() })

	noise := []byte(`{"actions":[{"at":0,"pos":50},{"at":1000,"pos":52},{"at":2000,"pos":49},{"at":3000,"pos":51}]}`)
	if _, err := library.Import("noise.funscript", noise, importAsPattern); err == nil || !contains(err.Error(), "no usable motion span") {
		t.Fatalf("noise-scale pattern error = %v", err)
	}

	chatter := []byte(`{"actions":[{"at":0,"pos":20},{"at":1000,"pos":80},{"at":1100,"pos":79},{"at":2000,"pos":81},{"at":4000,"pos":20}]}`)
	result, err := library.Import("chatter.funscript", chatter, importAsPattern)
	if err != nil {
		t.Fatal(err)
	}
	anchors := reversalAnchors(result.Pattern.Points)
	if len(anchors) != 3 {
		t.Fatalf("imported chatter points = %+v anchors = %v, want one meaningful peak", result.Pattern.Points, anchors)
	}
}

func contains(value, part string) bool {
	for index := 0; index+len(part) <= len(value); index++ {
		if value[index:index+len(part)] == part {
			return true
		}
	}
	return false
}
