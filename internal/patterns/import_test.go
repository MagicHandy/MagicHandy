package patterns

import (
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

func contains(value, part string) bool {
	for index := 0; index+len(part) <= len(value); index++ {
		if value[index:index+len(part)] == part {
			return true
		}
	}
	return false
}
