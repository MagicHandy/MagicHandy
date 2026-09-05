package patterns

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestLibrarySeedsBuiltinsAndPersistsEnablement(t *testing.T) {
	dir := t.TempDir()
	library, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	patterns, err := library.ListPatterns()
	if err != nil {
		t.Fatal(err)
	}
	if len(patterns) != len(motion.BuiltinPatternDefinitions()) {
		t.Fatalf("seeded patterns = %d, want %d", len(patterns), len(motion.BuiltinPatternDefinitions()))
	}
	if patterns[0].Origin != OriginBuiltin || !patterns[0].Enabled {
		t.Fatalf("first built-in = %+v", patterns[0])
	}
	disabled := false
	updated, err := library.UpdatePattern(patterns[0].ID, PatternPatch{Enabled: &disabled})
	if err != nil || updated.Enabled {
		t.Fatalf("disable = %+v err=%v", updated, err)
	}
	if err := library.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	persisted, err := reopened.Pattern(updated.ID)
	if err != nil || persisted.Enabled {
		t.Fatalf("persisted built-in enablement = %+v err=%v", persisted, err)
	}
	choices, err := reopened.EnabledChoices()
	if err != nil {
		t.Fatal(err)
	}
	for _, choice := range choices {
		if choice.ID == updated.ID {
			t.Fatalf("disabled pattern %q leaked into curation choices", updated.ID)
		}
	}
}

func TestLibraryRefreshesBuiltinTimingWithoutResettingUserPreferences(t *testing.T) {
	dir := t.TempDir()
	library, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	legacyPoints, err := json.Marshal([]motion.CurvePoint{
		{TimeMillis: 0, PositionPercent: 0},
		{TimeMillis: 3300, PositionPercent: 100},
		{TimeMillis: 6600, PositionPercent: 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := library.db.SQL().Exec(`
		UPDATE patterns
		SET name = 'My regular rhythm', enabled = 0, weight = 1.7,
			cycle_ms = 6600, points_json = ?
		WHERE id = ?
	`, string(legacyPoints), motion.PatternHardAndRegular); err != nil {
		t.Fatal(err)
	}
	if err := library.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	got, err := reopened.Pattern(string(motion.PatternHardAndRegular))
	if err != nil {
		t.Fatal(err)
	}
	want, found := motion.BuiltinPatternDefinition(motion.PatternHardAndRegular)
	if !found {
		t.Fatal("Hard and Regular built-in is missing")
	}
	pointsMatch := slices.EqualFunc(got.Points, want.Points, func(left, right motion.CurvePoint) bool {
		return left.TimeMillis == right.TimeMillis && left.PositionPercent == right.PositionPercent
	})
	if got.CycleMillis != want.CycleMillis || !pointsMatch {
		t.Fatalf("refreshed timing = cycle %d points %+v, want cycle %d points %+v", got.CycleMillis, got.Points, want.CycleMillis, want.Points)
	}
	if got.Name != "My regular rhythm" || got.Enabled || got.Weight != 1.7 {
		t.Fatalf("user-owned preferences changed during timing refresh: %+v", got)
	}
}

func TestLibraryMigratesOnlyUntouchedLegacyBuiltinNames(t *testing.T) {
	dir := t.TempDir()
	library, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	customName := "My descending pattern"
	if _, err := library.db.SQL().Exec(`UPDATE patterns SET name = ? WHERE id = ?`, "Building Up", motion.PatternBuildingUp); err != nil {
		t.Fatal(err)
	}
	if _, err := library.db.SQL().Exec(`UPDATE patterns SET name = ? WHERE id = ?`, customName, motion.PatternEasingDown); err != nil {
		t.Fatal(err)
	}
	if _, err := library.db.SQL().Exec(`UPDATE patterns SET name = ? WHERE id = ?`, "Fast Drive 1", "curated-fast-drive-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := library.db.SQL().Exec(`UPDATE patterns SET name = ? WHERE id = ?`, "Swell", motion.PatternSwell); err != nil {
		t.Fatal(err)
	}
	if _, err := library.db.SQL().Exec(`UPDATE patterns SET name = ? WHERE id = ?`, "Surge and Settle", motion.PatternSurgeAndSettle); err != nil {
		t.Fatal(err)
	}
	if err := library.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	for id, want := range map[string]string{
		string(motion.PatternBuildingUp):     "Ascending Window",
		string(motion.PatternEasingDown):     customName,
		string(motion.PatternSwell):          "Rising Window Arc",
		string(motion.PatternSurgeAndSettle): "Full Sweep and Mid Blocks",
		"curated-fast-drive-1":               "Return Depth Shuffle",
	} {
		pattern, patternErr := reopened.Pattern(id)
		if patternErr != nil || pattern.Name != want {
			t.Errorf("pattern %q name = %q, err=%v; want %q", id, pattern.Name, patternErr, want)
		}
	}
}

func TestLibraryReconcilesGeneratedBuiltinsAndPurgesImportedClipDuplicates(t *testing.T) {
	dir := t.TempDir()
	library, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	pointsJSON := `[{"time_ms":0,"position_percent":0},{"time_ms":6600,"position_percent":100}]`
	tagsJSON := `["imported","curated"]`
	// Resolved rather than named: the curated set is re-curated periodically by
	// scripts/curated-pattern-labeller.js, which renumbers ids.
	victim := anyExperimentalCuratedID(t)
	if _, err := library.db.SQL().Exec(`
		UPDATE patterns SET description = '', cycle_ms = 6600, points_json = ?, tags_json = ?
		WHERE id = ?
	`, pointsJSON, tagsJSON, victim); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := library.db.SQL().Exec(`
		INSERT INTO patterns(id, name, description, origin, kind, enabled, weight,
			cycle_ms, points_json, tags_json, created_at, updated_at)
		VALUES(?, ?, '10.0s clip from legacy-source', 'user', 'routine', 1, 1.0, 6600, ?, ?, ?, ?)
	`, "pattern-imported-clip", "Imported Clip", pointsJSON, tagsJSON, now, now); err != nil {
		t.Fatal(err)
	}
	if err := library.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	restored, err := reopened.Pattern(victim)
	if err != nil {
		t.Fatalf("restored generated built-in: %v", err)
	}
	if restored.Origin != OriginBuiltin || !slices.Contains(restored.Tags, motion.TagExperimental) || len(restored.Points) <= 2 {
		t.Fatalf("restored generated built-in = %+v", restored)
	}
	if _, err := reopened.Pattern("pattern-imported-clip"); !errors.Is(err, ErrPatternNotFound) {
		t.Fatalf("imported clip duplicate error = %v, want ErrPatternNotFound", err)
	}
	patterns, err := reopened.ListPatterns()
	if err != nil {
		t.Fatal(err)
	}
	if len(patterns) != len(motion.BuiltinPatternDefinitions()) {
		t.Fatalf("patterns after purge = %d, want %d", len(patterns), len(motion.BuiltinPatternDefinitions()))
	}
}

func TestBuiltinsDoNotConsumeUserPatternCapacity(t *testing.T) {
	library, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = library.Close() })
	now := time.Now().UTC().Format(time.RFC3339Nano)
	pointsJSON := `[{"time_ms":0,"position_percent":0},{"time_ms":6600,"position_percent":0}]`
	// Enough extra rows that flipping every row to user origin is over the limit
	// whatever the built-in catalog currently holds. A fixed count silently
	// stopped testing anything when curation shrank the catalog.
	var existing int
	if err := library.db.SQL().QueryRow("SELECT COUNT(*) FROM patterns").Scan(&existing); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < maxPatterns-existing+1; index++ {
		if _, err := library.db.SQL().Exec(`
			INSERT INTO patterns(id, name, description, origin, kind, enabled, weight,
				cycle_ms, points_json, tags_json, created_at, updated_at)
			VALUES(?, ?, '', 'builtin', 'routine', 1, 1.0, 6600, ?, '[]', ?, ?)
		`, fmt.Sprintf("capacity-builtin-%d", index), fmt.Sprintf("Capacity builtin %d", index), pointsJSON, now, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := library.ensurePatternCapacity(); err != nil {
		t.Fatalf("built-ins consumed user capacity: %v", err)
	}
	if _, err := library.db.SQL().Exec("UPDATE patterns SET origin = 'user'"); err != nil {
		t.Fatal(err)
	}
	if err := library.ensurePatternCapacity(); !errors.Is(err, ErrLibraryLimit) {
		t.Fatalf("user capacity error = %v, want ErrLibraryLimit", err)
	}
}

// anyExperimentalCuratedID returns a curated built-in carrying the experimental
// tag. Tests resolve one instead of naming it: curation renumbers ids, and a
// hard-coded name turns a routine re-curation into an unrelated test failure.
func anyExperimentalCuratedID(t *testing.T) string {
	t.Helper()
	for _, definition := range motion.BuiltinPatternDefinitions() {
		id := string(definition.ID)
		if strings.HasPrefix(id, "curated-") && slices.Contains(definition.Tags, motion.TagExperimental) {
			return id
		}
	}
	t.Fatal("no experimental curated built-in in the catalog")
	return ""
}

func TestLibraryReconcilesRetiredAndPromotedBuiltins(t *testing.T) {
	dir := t.TempDir()
	library, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	promoted := motion.PromotedBuiltinPatternDefinitions()
	legacyUserIDs := make(map[motion.PatternID]string, len(promoted))
	for index, definition := range promoted {
		if _, err := library.db.SQL().Exec(`DELETE FROM patterns WHERE id = ?`, definition.ID); err != nil {
			t.Fatal(err)
		}
		legacyUserID := fmt.Sprintf("pattern-promoted-legacy-%d", index)
		legacyUserIDs[definition.ID] = legacyUserID
		if err := library.insertPattern(Pattern{
			ID: legacyUserID, Name: definition.Name, Description: "Imported funscript example.",
			Origin: OriginUser, Kind: definition.Kind, Enabled: index == 1, Weight: 1.7 + float64(index)/10,
			CycleMillis: definition.CycleMillis, Points: definition.Points, Tags: []string{"imported"},
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	for index, id := range motion.RetiredBuiltinPatternIDs() {
		if err := library.insertPattern(Pattern{
			ID: string(id), Name: fmt.Sprintf("Retired built-in %d", index), Origin: OriginBuiltin,
			Kind: motion.PatternKindRoutine, Enabled: false, Weight: 1,
			CycleMillis: motion.RoutineCycleFloorMillis,
			Points: []motion.CurvePoint{
				{TimeMillis: 0, PositionPercent: 0},
				{TimeMillis: motion.RoutineCycleFloorMillis / 2, PositionPercent: 80},
				{TimeMillis: motion.RoutineCycleFloorMillis, PositionPercent: 0},
			},
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := library.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	for _, id := range motion.RetiredBuiltinPatternIDs() {
		if _, err := reopened.Pattern(string(id)); !errors.Is(err, ErrPatternNotFound) {
			t.Fatalf("retired built-in %q error = %v, want ErrPatternNotFound", id, err)
		}
	}
	for index, definition := range promoted {
		if _, err := reopened.Pattern(legacyUserIDs[definition.ID]); !errors.Is(err, ErrPatternNotFound) {
			t.Fatalf("promoted duplicate %q error = %v, want ErrPatternNotFound", definition.ID, err)
		}
		canonical, err := reopened.Pattern(string(definition.ID))
		if err != nil {
			t.Fatal(err)
		}
		if canonical.Origin != OriginBuiltin || canonical.Enabled || canonical.Weight != 1.7+float64(index)/10 {
			t.Fatalf("promoted canonical %q = %+v", definition.ID, canonical)
		}
	}
}

func TestLibraryPreservesUserPatternWithRetiredBuiltinID(t *testing.T) {
	dir := t.TempDir()
	library, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := library.insertPattern(Pattern{
		ID: string(motion.PatternBroadAndTight), Name: "Personal contrast replacement", Origin: OriginUser,
		Kind: motion.PatternKindRoutine, Enabled: true, Weight: 1,
		CycleMillis: motion.RoutineCycleFloorMillis,
		Points: []motion.CurvePoint{
			{TimeMillis: 0, PositionPercent: 20},
			{TimeMillis: motion.RoutineCycleFloorMillis / 2, PositionPercent: 80},
			{TimeMillis: motion.RoutineCycleFloorMillis, PositionPercent: 20},
		},
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := library.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	pattern, err := reopened.Pattern(string(motion.PatternBroadAndTight))
	if err != nil {
		t.Fatal(err)
	}
	if pattern.Origin != OriginUser || pattern.Name != "Personal contrast replacement" {
		t.Fatalf("user pattern with retired ID = %+v", pattern)
	}
}

func TestAuthoredPatternIsSparseEditableAndShareable(t *testing.T) {
	library, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = library.Close() })
	input := authoredFixture()
	created, err := library.CreatePattern(input)
	if err != nil {
		t.Fatalf("CreatePattern: %v", err)
	}
	if created.Origin != OriginUser || len(created.Points) >= len(input.Points) || created.CycleMillis < motion.RoutineCycleFloorMillis {
		t.Fatalf("created pattern = %+v", created)
	}
	if len(created.PreviewSamples) < 3 {
		t.Fatalf("backend preview samples = %d, want a sampled curve", len(created.PreviewSamples))
	}

	data, filename, err := library.ExportPattern(created.ID)
	if err != nil {
		t.Fatalf("ExportPattern: %v", err)
	}
	if !strings.HasSuffix(filename, ".mhpattern.json") || !strings.Contains(string(data), PatternFileSchema) {
		t.Fatalf("export filename=%q data=%s", filename, data)
	}
	imported, err := library.Import(filename, data, importAsPattern)
	if err != nil || imported.Pattern == nil {
		t.Fatalf("reimport = %+v err=%v", imported, err)
	}
	if imported.Pattern.ID == created.ID || imported.Pattern.Name != created.Name {
		t.Fatalf("reimported pattern = %+v", imported.Pattern)
	}
}

func TestPatternNamesAreEditableButBuiltinCurvesStayCanonical(t *testing.T) {
	dir := t.TempDir()
	library, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	created, err := library.CreatePattern(authoredFixture())
	if err != nil {
		t.Fatal(err)
	}
	name := "Renamed pattern"
	renamed, err := library.UpdatePattern(created.ID, PatternPatch{Name: &name})
	if err != nil || renamed.Name != name {
		t.Fatalf("rename = %+v err=%v", renamed, err)
	}
	builtinName := "Renamed built-in"
	builtin, err := library.UpdatePattern(string(motion.PatternStroke), PatternPatch{Name: &builtinName})
	if err != nil || builtin.Name != builtinName {
		t.Fatalf("built-in rename = %+v err=%v", builtin, err)
	}
	points := []motion.CurvePoint{{TimeMillis: 0}, {TimeMillis: motion.RoutineCycleFloorMillis}}
	if _, err := library.UpdatePattern(string(motion.PatternStroke), PatternPatch{Points: &points}); !errors.Is(err, ErrBuiltinPattern) {
		t.Fatalf("built-in curve edit error = %v, want ErrBuiltinPattern", err)
	}
	if err := library.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	for id, want := range map[string]string{created.ID: name, string(motion.PatternStroke): builtinName} {
		pattern, err := reopened.Pattern(id)
		if err != nil || pattern.Name != want {
			t.Fatalf("persisted name for %q = %q err=%v, want %q", id, pattern.Name, err, want)
		}
	}
}

func TestFeedbackUndoDoesNotOverwriteDirectWeightEdit(t *testing.T) {
	library, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = library.Close() })
	patterns, err := library.ListPatterns()
	if err != nil {
		t.Fatal(err)
	}
	pattern := patterns[0]
	feedback, _, err := library.ApplyFeedback(pattern.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	weight := 2.25
	if _, err := library.UpdatePattern(pattern.ID, PatternPatch{Weight: &weight}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := library.UndoFeedback(feedback.ID); !errors.Is(err, ErrFeedbackOrder) {
		t.Fatalf("UndoFeedback after direct edit error = %v, want ErrFeedbackOrder", err)
	}
}

func TestFeedbackIsVisibleReversibleAndAutoDisableIsOptIn(t *testing.T) {
	library, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = library.Close() })
	patterns, err := library.ListPatterns()
	if err != nil {
		t.Fatal(err)
	}
	pattern := patterns[0]
	weight := 0.3
	pattern, err = library.UpdatePattern(pattern.ID, PatternPatch{Weight: &weight})
	if err != nil {
		t.Fatal(err)
	}

	feedback, after, err := library.ApplyFeedback(pattern.ID, -1)
	if err != nil {
		t.Fatal(err)
	}
	if !after.Enabled || after.Weight >= pattern.Weight || feedback.WeightBefore != pattern.Weight {
		t.Fatalf("default feedback = %+v pattern=%+v", feedback, after)
	}
	if _, _, err := library.UndoFeedback(feedback.ID); err != nil {
		t.Fatalf("undo default feedback: %v", err)
	}
	if err := library.SetAutoDisable(true); err != nil {
		t.Fatal(err)
	}
	feedback, after, err = library.ApplyFeedback(pattern.ID, -1)
	if err != nil {
		t.Fatal(err)
	}
	autoDisable, err := library.AutoDisable()
	if err != nil {
		t.Fatal(err)
	}
	if after.Enabled || !autoDisable {
		t.Fatalf("opt-in auto-disable did not become visible: %+v", after)
	}
	_, restored, err := library.UndoFeedback(feedback.ID)
	if err != nil || !restored.Enabled || math.Abs(restored.Weight-pattern.Weight) > 0.0001 {
		t.Fatalf("undo restored=%+v err=%v", restored, err)
	}
}

func TestPreviewMatchesMotionPlanShapeAfterRetiming(t *testing.T) {
	input := authoredFixture()
	preview, err := PreviewPattern(input)
	if err != nil {
		t.Fatal(err)
	}
	definition := motion.PatternDefinition{
		ID: "preview-match", Name: "Preview", Kind: motion.PatternKindRoutine,
		CycleMillis: preview.CycleMillis, Points: preview.Points,
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 100
	plan := motion.NewMotionPlan("preview", motion.MotionTarget{
		PatternID: definition.ID, Pattern: &definition, SpeedPercent: 100,
	}, settings, 0, 0, time.Unix(0, 0))
	for _, sample := range preview.Samples {
		playedAt := int64(math.Round(
			float64(sample.TimeMillis) * float64(plan.PeriodMillis) / float64(preview.CycleMillis),
		))
		got := plan.SampleAt(playedAt).PositionPercent
		if math.Abs(got-sample.PositionPercent) > 0.51 {
			t.Fatalf("sample at %d = %.3f, preview %.3f", sample.TimeMillis, got, sample.PositionPercent)
		}
	}
}

func TestSimplificationPreservesDirectionReversals(t *testing.T) {
	points := []motion.CurvePoint{
		{TimeMillis: 0, PositionPercent: 0},
		{TimeMillis: 100, PositionPercent: 10},
		{TimeMillis: 200, PositionPercent: 20},
		{TimeMillis: 300, PositionPercent: 15},
		{TimeMillis: 400, PositionPercent: 10},
		{TimeMillis: 500, PositionPercent: 30},
		{TimeMillis: 600, PositionPercent: 20},
	}
	simplified := simplifyPreservingReversals(points, 100)
	for _, expected := range []float64{20, 10, 30} {
		found := false
		for _, point := range simplified {
			found = found || point.PositionPercent == expected
		}
		if !found {
			t.Fatalf("reversal %.0f missing from %+v", expected, simplified)
		}
	}
}

func TestSimplificationRemovesInsignificantReversalChatter(t *testing.T) {
	points := []motion.CurvePoint{
		{TimeMillis: 0, PositionPercent: 0},
		{TimeMillis: 100, PositionPercent: 20},
		{TimeMillis: 110, PositionPercent: 19},
		{TimeMillis: 200, PositionPercent: 30},
		{TimeMillis: 400, PositionPercent: 0},
	}
	stabilized := stabilizeReversalChatter(points, motion.MinimumPatternReversalProminence)
	anchors := reversalAnchors(stabilized)
	if len(anchors) != 3 || stabilized[anchors[1]].PositionPercent != 30 {
		t.Fatalf("stabilized points = %+v anchors = %v, want only the meaningful 30%% reversal", stabilized, anchors)
	}
}

func authoredFixture() PatternInput {
	points := make([]motion.CurvePoint, 0, 101)
	for index := range 101 {
		phase := float64(index) / 100
		position := 50 - 45*math.Cos(phase*2*math.Pi)
		position += math.Sin(float64(index)*1.7) * 0.4
		points = append(points, motion.CurvePoint{TimeMillis: int64(index) * 66, PositionPercent: position})
	}
	return PatternInput{
		Name: "Drawn wave", Description: "Authored test curve.",
		Kind: motion.PatternKindRoutine, CycleMillis: 6600,
		Points: points, Tags: []string{"drawn", "smooth"}, SimplifyError: 1,
	}
}

func TestPatternFileRejectsUnknownFields(t *testing.T) {
	library, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = library.Close() })
	file := map[string]any{
		"schema": PatternFileSchema, "name": "Bad", "kind": "routine",
		"cycle_ms": 6600, "points": []map[string]any{{"time_ms": 0, "position_percent": 0}, {"time_ms": 6600, "position_percent": 0}},
		"transport_command": "unsafe",
	}
	data, _ := json.Marshal(file)
	if _, err := library.Import("bad.json", data, importAsPattern); err == nil {
		t.Fatal("pattern import accepted an unknown transport field")
	}
}

func TestUserPatternIDTruncatesUnicodeByRune(t *testing.T) {
	library, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = library.Close() })
	input := authoredFixture()
	input.Name = strings.Repeat("界", 40)
	pattern, err := library.CreatePattern(input)
	if err != nil {
		t.Fatalf("CreatePattern: %v", err)
	}
	if !utf8.ValidString(pattern.ID) {
		t.Fatalf("generated ID is not valid UTF-8: %q", pattern.ID)
	}
	if _, err := library.Pattern(pattern.ID); err != nil {
		t.Fatalf("read Unicode pattern ID: %v", err)
	}
}

func TestAuthoringRejectsUnboundedTimes(t *testing.T) {
	input := authoredFixture()
	input.Points[0].TimeMillis = -1
	if _, err := PreviewPattern(input); err == nil {
		t.Fatal("preview accepted a negative point time")
	}
	input = authoredFixture()
	input.CycleMillis = maxContentDuration + 1
	if _, err := PreviewPattern(input); err == nil {
		t.Fatal("preview accepted content longer than 24 hours")
	}
}

func TestPatternRejectsCorruptStoredTags(t *testing.T) {
	library, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = library.Close() })
	created, err := library.CreatePattern(PatternInput{
		Name: "Corrupt tags fixture", Kind: motion.PatternKindBurst, CycleMillis: 1000,
		Points: []motion.CurvePoint{
			{TimeMillis: 0},
			{TimeMillis: 500, PositionPercent: 100},
			{TimeMillis: 1000},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := library.db.SQL().Exec(`UPDATE patterns SET tags_json = '{' WHERE id = ?`, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := library.Pattern(created.ID); err == nil {
		t.Fatal("Pattern accepted corrupt stored tags")
	}
	if _, err := library.ListPatterns(); err == nil {
		t.Fatal("ListPatterns hid corrupt stored tags as an empty library")
	}
}

func TestPatternUpdateRejectsNonFiniteWeight(t *testing.T) {
	library, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = library.Close() })
	weight := math.NaN()
	if _, err := library.UpdatePattern(string(motion.PatternStroke), PatternPatch{Weight: &weight}); err == nil {
		t.Fatal("UpdatePattern accepted a non-finite weight")
	}
}

func TestLibraryReadFailuresAreNotReportedAsEmptyState(t *testing.T) {
	library, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := library.Close(); err != nil {
		t.Fatal(err)
	}

	checks := []struct {
		name string
		run  func() error
	}{
		{name: "snapshot", run: func() error { _, err := library.Snapshot(); return err }},
		{name: "summary", run: func() error { _, err := library.Summary(); return err }},
		{name: "patterns", run: func() error { _, err := library.ListPatterns(); return err }},
		{name: "programs", run: func() error { _, err := library.ListPrograms(); return err }},
		{name: "choices", run: func() error { _, err := library.EnabledChoices(); return err }},
		{name: "feedback", run: func() error { _, err := library.FeedbackHistory(30); return err }},
		{name: "auto-disable", run: func() error { _, err := library.AutoDisable(); return err }},
		{name: "resolve", run: func() error {
			_, _, err := library.ResolveEnabled(string(motion.PatternStroke))
			return err
		}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.run(); err == nil {
				t.Fatalf("%s hid a closed database as empty or missing state", check.name)
			}
		})
	}
}

func TestFeedbackRollsBackWhenAutoDisablePreferenceCannotBeRead(t *testing.T) {
	library, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = library.Close() })
	before, err := library.Pattern(string(motion.PatternStroke))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := library.db.SQL().Exec("DROP TABLE app_kv"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := library.ApplyFeedback(before.ID, -1); err == nil {
		t.Fatal("ApplyFeedback ignored a failed auto-disable preference read")
	}
	after, err := library.Pattern(before.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Weight != before.Weight || after.Enabled != before.Enabled {
		t.Fatalf("failed feedback was not rolled back: before=%+v after=%+v", before, after)
	}
	var feedbackCount int
	if err := library.db.SQL().QueryRow("SELECT COUNT(*) FROM pattern_feedback").Scan(&feedbackCount); err != nil {
		t.Fatal(err)
	}
	if feedbackCount != 0 {
		t.Fatalf("failed feedback persisted %d history rows", feedbackCount)
	}
}
