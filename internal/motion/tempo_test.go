package motion

import (
	"context"
	"errors"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestTempoSpecValidation(t *testing.T) {
	valid := []TempoSpec{
		{Mode: TempoModeFollow},
		{Mode: TempoModeSteady, LevelPercent: 40},
		{Mode: TempoModeRamp, LevelPercent: 0, PeriodSeconds: 20},
		{Mode: TempoModeWave, MinPercent: 20, MaxPercent: 60, PeriodSeconds: 8},
		{Mode: TempoModePulse, MinPercent: 0, MaxPercent: 90, PeriodSeconds: 2},
		{Mode: TempoModePause},
		{Mode: TempoModeRoutine, Routine: "swells", Seed: 9},
		{Mode: TempoModeSequence, Sections: []TempoSection{
			{Mode: TempoModeRamp, LevelPercent: 60, PeriodSeconds: 20, Seconds: 30},
			{Mode: TempoModePause, Seconds: 5},
			{Mode: TempoModeSteady, LevelPercent: 30},
		}},
		{Mode: TempoModeSequence, Loop: true, Sections: []TempoSection{
			{Mode: TempoModeSteady, LevelPercent: 30, Seconds: 3},
			{Mode: TempoModeSteady, LevelPercent: 70, Seconds: 3},
		}},
	}
	for _, spec := range valid {
		if err := spec.Validate(); err != nil {
			t.Errorf("%+v: %v", spec, err)
		}
	}
	invalid := []TempoSpec{
		{Mode: "fast"},
		{Mode: TempoModeFollow, LevelPercent: 40},
		{Mode: TempoModeSteady, LevelPercent: 0},
		{Mode: TempoModeSteady, LevelPercent: 101},
		{Mode: TempoModeSteady, LevelPercent: 40, PeriodSeconds: 3},
		{Mode: TempoModeRamp, LevelPercent: 50},
		{Mode: TempoModeRamp, LevelPercent: 50, PeriodSeconds: 601},
		{Mode: TempoModeWave, MinPercent: 60, MaxPercent: 60, PeriodSeconds: 8},
		{Mode: TempoModeWave, MinPercent: 20, MaxPercent: 60, PeriodSeconds: 1},
		{Mode: TempoModePulse, MinPercent: 20, MaxPercent: 60, PeriodSeconds: math.NaN()},
		{Mode: TempoModePause, LevelPercent: 10},
		{Mode: TempoModeRoutine, Routine: "unknown"},
		{Mode: TempoModeRoutine, Routine: "swells", LevelPercent: 30},
		{Mode: TempoModeSteady, LevelPercent: 40, Loop: true},
		{Mode: TempoModeSequence},
		{Mode: TempoModeSequence, Sections: []TempoSection{{Mode: TempoModeSteady, LevelPercent: 40}, {Mode: TempoModeSteady, LevelPercent: 50}}},
		{Mode: TempoModeSequence, Loop: true, Sections: []TempoSection{{Mode: TempoModeSteady, LevelPercent: 40, Seconds: 1}}},
		{Mode: TempoModeSequence, Sections: []TempoSection{{Mode: TempoModeFollow, Seconds: 5}}},
		{Mode: TempoModeSequence, Sections: make([]TempoSection, maxTempoSections+1)},
	}
	for _, spec := range invalid {
		if err := spec.Validate(); err == nil {
			t.Errorf("%+v was accepted", spec)
		}
	}
}

func TestTempoEnvelopesStayBoundedAndContinuous(t *testing.T) {
	specs := []TempoSpec{
		{Mode: TempoModeSteady, LevelPercent: 70},
		{Mode: TempoModeRamp, LevelPercent: 90, PeriodSeconds: 5},
		{Mode: TempoModeWave, MinPercent: 10, MaxPercent: 85, PeriodSeconds: 2},
		{Mode: TempoModePulse, MinPercent: 5, MaxPercent: 95, PeriodSeconds: 2},
		{Mode: TempoModeSequence, Loop: true, Sections: []TempoSection{
			{Mode: TempoModeSteady, LevelPercent: 20, Seconds: 4},
			{Mode: TempoModeWave, MinPercent: 60, MaxPercent: 90, PeriodSeconds: 3, Seconds: 7},
			{Mode: TempoModeRamp, LevelPercent: 10, PeriodSeconds: 2, Seconds: 3},
		}},
	}
	for _, routine := range TempoRoutines() {
		specs = append(specs, TempoSpec{Mode: TempoModeRoutine, Routine: routine.ID, Seed: 42})
	}
	for _, spec := range specs {
		plan := newTempoPlan(spec, 1000, 35, 1)
		previous := plan.LevelAt(1000)
		if math.Abs(previous-35) > 1e-9 {
			t.Fatalf("%s did not start from the level it replaced: %g", spec.Mode+spec.Routine, previous)
		}
		for at := int64(1001); at < 400_000; at += 5 {
			level := plan.LevelAt(at)
			// A 2 s pulse across 90 points moves ~2.1 per 5 ms at its edge; a
			// seam between sections or envelopes would jump far more.
			// A pause stops at once by design.
			if math.IsNaN(level) || level < 0 || level > 100 || (level != 0 && math.Abs(level-previous) > 2.5) {
				t.Fatalf("%s%s at %d: %g after %g", spec.Mode, spec.Routine, at, level, previous)
			}
			previous = level
		}
	}
}

func TestTempoSectionsPlayInOrderAndHold(t *testing.T) {
	spec := TempoSpec{Mode: TempoModeSequence, Sections: []TempoSection{
		{Mode: TempoModeSteady, LevelPercent: 30, Seconds: 10},
		{Mode: TempoModePause, Seconds: 5},
		{Mode: TempoModeSteady, LevelPercent: 60},
	}}
	plan := newTempoPlan(spec, 0, 30, 1)
	for _, check := range []struct {
		at   int64
		want float64
	}{{5000, 30}, {12000, 0}, {14900, 0}, {17000, 60}, {600_000, 60}} {
		if got := plan.LevelAt(check.at); math.Abs(got-check.want) > 1e-9 {
			t.Fatalf("level at %d = %g, want %g", check.at, got, check.want)
		}
	}
	// A ramp starts where the previous section left off.
	ramped := newTempoPlan(TempoSpec{Mode: TempoModeRamp, LevelPercent: 80, PeriodSeconds: 10}, 0, 20, 1)
	if ramped.LevelAt(0) != 20 || math.Abs(ramped.LevelAt(5000)-50) > 1e-9 || ramped.LevelAt(30_000) != 80 {
		t.Fatalf("ramp = %g, %g, %g", ramped.LevelAt(0), ramped.LevelAt(5000), ramped.LevelAt(30_000))
	}
}

func TestTempoLoopCarriesAnUnfinishedRampWithoutJumping(t *testing.T) {
	spec := TempoSpec{Mode: TempoModeSequence, Loop: true, Sections: []TempoSection{
		{Mode: TempoModeRamp, LevelPercent: 80, PeriodSeconds: 20, Seconds: 4},
	}}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	plan := newTempoPlan(spec, 0, 20, 1)
	for pass := int64(1); pass < 20; pass++ {
		seam := pass * 4000
		if delta := math.Abs(plan.LevelAt(seam) - plan.LevelAt(seam-1)); delta > 0.05 {
			t.Fatalf("unfinished ramp jumps by %g at pass %d", delta, pass)
		}
	}
}

func TestTempoPauseResumeContinuesTheEnvelope(t *testing.T) {
	var clock atomic.Int64
	clock.Store(1_000_000)
	engine, err := NewEngine(EngineOptions{Transport: transport.NewFake(transport.WithTempo()),
		Now: func() time.Time { return time.UnixMilli(clock.Load()) }, DispatchInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
	spec := TempoSpec{Mode: TempoModeRamp, LevelPercent: 80, PeriodSeconds: 20}
	if _, err := engine.Start(t.Context(), MotionTarget{TempoOnly: true, Tempo: &spec}, config.DefaultSettings().Motion); err != nil {
		t.Fatal(err)
	}
	clock.Add(6500)
	engine.mu.Lock()
	played := engine.estimatedPlaybackMillisLocked(engine.now())
	want := engine.tempo.LevelAt(played + 1500)
	engine.mu.Unlock()
	if _, err := engine.Pause(t.Context(), "pause"); err != nil {
		t.Fatal(err)
	}
	clock.Add(20_000)
	if _, err := engine.Resume(t.Context(), "resume"); err != nil {
		t.Fatal(err)
	}
	engine.mu.Lock()
	got := engine.tempo.LevelAt(1500)
	engine.mu.Unlock()
	if math.Abs(got-want) > 0.001 {
		t.Fatalf("resume restarted the tempo: got %g, want continuing level %g", got, want)
	}
}

func TestTempoRoutinesValidateForEverySeed(t *testing.T) {
	if len(TempoRoutines()) < 6 {
		t.Fatalf("routines = %d", len(TempoRoutines()))
	}
	for _, routine := range TempoRoutines() {
		if routine.Name == "" || routine.Description == "" {
			t.Fatalf("routine %q needs a name and description", routine.ID)
		}
		distinct := map[float64]bool{}
		for seed := uint64(0); seed < 200; seed++ {
			sections := routine.sections(seed)
			spec := TempoSpec{Mode: TempoModeSequence, Sections: sections, Loop: routine.Loop}
			if err := spec.Validate(); err != nil {
				t.Fatalf("%s seed %d: %v", routine.ID, seed, err)
			}
			distinct[newTempoPlan(spec, 0, 0, 1).LevelAt(20_000)] = true
		}
		if len(distinct) < 3 {
			t.Fatalf("%s barely varies with its seed: %v", routine.ID, distinct)
		}
	}
}

func tempoFrames(commands []transport.Command) []transport.TempoFrame {
	var frames []transport.TempoFrame
	for _, command := range commands {
		if command.Kind == transport.CommandKindPointsAdd && command.PointsAdd != nil && command.PointsAdd.Tempo != nil {
			frames = append(frames, *command.PointsAdd.Tempo)
		}
	}
	return frames
}

func levelAt(frames []transport.TempoFrame, at int64) (float64, bool) {
	for index := len(frames) - 1; index >= 0; index-- {
		points := frames[index].Points
		if len(points) > 0 && points[0].TimeMillis <= at && at <= points[len(points)-1].TimeMillis {
			for _, point := range points {
				if point.TimeMillis >= at {
					return point.LevelPercent, true
				}
			}
		}
	}
	return 0, false
}

func TestTempoOnlyStartDrivesOnlyTheTempoOwner(t *testing.T) {
	linear, tempo := transport.NewFake(), transport.NewFake(transport.WithTempo())
	owner := transport.NewLinkedSession([]transport.LinkedParticipant{{ID: "handy", Label: "Handy", Transport: linear}, {ID: "vacuglide", Label: "VacuGlide", Transport: tempo}}, false)
	engine := newTestEngine(t, owner, diagnostics.NewTraceRing(64), time.Hour)
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
	target := testTarget()
	target.TempoOnly, target.Tempo = true, &TempoSpec{Mode: TempoModeSteady, LevelPercent: 40}
	state, err := engine.Start(t.Context(), target, config.DefaultSettings().Motion)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Running || len(linear.Commands()) != 0 || countCommands(tempo.Commands(), transport.CommandKindPointsPlay) != 1 {
		t.Fatalf("tempo-only start reached the idle Handy: %d commands", len(linear.Commands()))
	}
	frames := tempoFrames(tempo.Commands())
	if len(frames) == 0 {
		t.Fatal("the tempo owner received no tempo")
	}
	if level, ok := levelAt(frames, 1500); !ok || level != 40 {
		t.Fatalf("tempo at 1.5 s = %g (%v), want 40 after the blend", level, ok)
	}
	if state.Tempo == nil || state.Tempo.Mode != TempoModeSteady || len(state.Tempo.Ahead) != tempoReadoutAheadSeconds {
		t.Fatalf("readout = %+v", state.Tempo)
	}
	if owner.Diagnostics().PlaybackState == "stale" {
		t.Fatal("the idle Handy was treated as a failed participant")
	}
	linked := owner.(transport.LinkedSessionStatusProvider).LinkedSessionStatus()
	if len(linked.IdleDevices) != 1 || linked.IdleDevices[0] != "handy" || len(linked.RetiredDevices) != 0 {
		t.Fatalf("idle participant was not reported accurately: %+v", linked)
	}
	if _, err := engine.Stop(t.Context(), "global_stop"); err != nil {
		t.Fatal(err)
	}
	if countCommands(linear.Commands(), transport.CommandKindStop) != 1 || countCommands(tempo.Commands(), transport.CommandKindStop) != 1 {
		t.Fatal("global Stop missed an output")
	}
}

func TestTempoOnlyPlanHasNoPhantomPositionPattern(t *testing.T) {
	spec := TempoSpec{Mode: TempoModeSteady, LevelPercent: 80}
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard} {
		settings := config.DefaultSettings().Motion
		settings.HandyModel = model
		plan := NewMotionPlan("tempo", MotionTarget{TempoOnly: true, Tempo: &spec}, settings, 0, 0, time.Unix(0, 0))
		if err := plan.compilationError(); err != nil {
			t.Fatal(err)
		}
		if plan.Target.PatternID != "" || plan.Target.SpeedPercent != 0 || !plan.Loop {
			t.Fatalf("tempo-only plan inherited Handy content: %+v", plan.Target)
		}
		for at := int64(0); at <= 12_000; at += 25 {
			if got := plan.SampleAt(at).PositionPercent; got != 50 {
				t.Fatalf("tempo-only plan produced phantom position %g at %d", got, at)
			}
		}
	}
}

func TestTempoOnlyNeedsATempoOwnerAndATempo(t *testing.T) {
	engine := newTestEngine(t, transport.NewFake(), diagnostics.NewTraceRing(64), time.Hour)
	target := testTarget()
	target.TempoOnly, target.Tempo = true, &TempoSpec{Mode: TempoModeSteady, LevelPercent: 40}
	if _, err := engine.Start(t.Context(), target, config.DefaultSettings().Motion); !errors.Is(err, transport.ErrTempoUnavailable) {
		t.Fatalf("tempo-only start without a tempo owner = %v", err)
	}
	engine = newTestEngine(t, transport.NewFake(transport.WithTempo()), diagnostics.NewTraceRing(64), time.Hour)
	target.Tempo = &TempoSpec{Mode: TempoModeFollow}
	if _, err := engine.Start(t.Context(), target, config.DefaultSettings().Motion); err == nil {
		t.Fatal("a tempo-only run accepted follow")
	}
}

func TestTempoFollowsPositionsOnTheHandyModelScale(t *testing.T) {
	levels := map[string]float64{}
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard} {
		tempo := transport.NewFake(transport.WithTempo())
		engine := newTestEngine(t, tempo, diagnostics.NewTraceRing(64), time.Hour)
		settings := config.DefaultSettings().Motion
		settings.HandyModel = model
		target := MotionTarget{PatternID: PatternFullSweeps, SpeedPercent: 50, Source: "test"}
		state, err := engine.Start(t.Context(), target, settings)
		if err != nil {
			t.Fatal(err)
		}
		if state.Target.Tempo == nil || state.Target.Tempo.Mode != TempoModeFollow {
			t.Fatalf("positions did not lead: %+v", state.Target.Tempo)
		}
		engine.mu.Lock()
		follow := engine.followTempoLevelsLocked(2000, 10_000, tempoFollowSampleMillis)
		frame := engine.tempoFrameLocked(0, nil)
		engine.mu.Unlock()
		low, high, sum := 100.0, 0.0, 0.0
		for _, point := range follow {
			low, high, sum = math.Min(low, point.LevelPercent), math.Max(high, point.LevelPercent), sum+point.LevelPercent
		}
		levels[model] = sum / float64(len(follow))
		if high-low > 4 {
			t.Fatalf("%s: steady sweeps hunt between %g and %g", model, low, high)
		}
		if frame == nil || len(frame.Points) == 0 {
			t.Fatal("follow produced no frame")
		}
		_, _ = engine.Stop(context.Background(), "cleanup")
	}
	original, standard := levels[config.HandyModelOriginal], levels[config.HandyModel2Standard]
	// Full sweeps at speed 50 run a little over half of the model's top pace.
	if original < 40 || original > 65 {
		t.Fatalf("follow level at speed 50 = %g", original)
	}
	if math.Abs(original-standard) > 4 {
		t.Fatalf("the Handy model setting moved the tempo: %g vs %g", original, standard)
	}
}

func TestApplyTempoReplacesTheEnvelopeAtOnce(t *testing.T) {
	tempo := transport.NewFake(transport.WithTempo())
	engine := newTestEngine(t, tempo, diagnostics.NewTraceRing(64), time.Hour)
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
	target := testTarget()
	target.TempoOnly, target.Tempo = true, &TempoSpec{Mode: TempoModeSteady, LevelPercent: 30}
	if _, err := engine.Start(t.Context(), target, config.DefaultSettings().Motion); err != nil {
		t.Fatal(err)
	}
	before := len(tempo.Commands())
	spec := TempoSpec{Mode: TempoModeWave, MinPercent: 50, MaxPercent: 90, PeriodSeconds: 6}
	state, err := engine.ApplyTempo(t.Context(), spec, engine.AdmissionGeneration())
	if err != nil {
		t.Fatal(err)
	}
	after := tempo.Commands()[before:]
	if len(after) != 1 || after[0].PointsAdd == nil || len(after[0].PointsAdd.Points) != 0 || after[0].PointsAdd.Tempo == nil {
		t.Fatalf("ApplyTempo sent %d commands, want one tempo frame without positions", len(after))
	}
	frame := after[0].PointsAdd.Tempo
	if frame.Points[0].TimeMillis > 0 && frame.Points[0].TimeMillis > state.RunningMillis {
		t.Fatalf("the new envelope starts after playback: %d", frame.Points[0].TimeMillis)
	}
	if state.Target.Tempo == nil || !state.Target.Tempo.Equal(spec) || !state.Target.TempoOnly {
		t.Fatalf("target = %+v", state.Target)
	}
	if _, err := engine.ApplyTempo(t.Context(), TempoSpec{Mode: TempoModeFollow}, engine.AdmissionGeneration()); err == nil {
		t.Fatal("a tempo-only run accepted follow")
	}
	if _, err := engine.ApplyTempo(t.Context(), spec, engine.AdmissionGeneration()+1); !errors.Is(err, errRunInvalidated) {
		t.Fatalf("a stale admission = %v", err)
	}
}

func TestApplyTempoNeedsATempoOwner(t *testing.T) {
	engine := newTestEngine(t, transport.NewFake(), diagnostics.NewTraceRing(64), time.Hour)
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
	if _, err := engine.Start(t.Context(), testTarget(), config.DefaultSettings().Motion); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.ApplyTempo(t.Context(), TempoSpec{Mode: TempoModeSteady, LevelPercent: 50}, engine.AdmissionGeneration()); !errors.Is(err, transport.ErrTempoUnavailable) {
		t.Fatalf("tempo without a tempo owner = %v", err)
	}
	if state := engine.Snapshot(); state.Tempo != nil || state.Target.Tempo != nil {
		t.Fatalf("a Handy-only run carries tempo: %+v", state.Tempo)
	}
}

func TestPositionTargetPromotesATempoOnlyRun(t *testing.T) {
	linear, tempo := transport.NewFake(), transport.NewFake(transport.WithTempo())
	owner := transport.NewLinkedSession([]transport.LinkedParticipant{{ID: "handy", Label: "Handy", Transport: linear}, {ID: "vacuglide", Label: "VacuGlide", Transport: tempo}}, false)
	engine := newTestEngine(t, owner, diagnostics.NewTraceRing(64), time.Hour)
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
	target := testTarget()
	target.TempoOnly, target.Tempo = true, &TempoSpec{Mode: TempoModeRoutine, Routine: "bursts", Seed: 3}
	if _, err := engine.Start(t.Context(), target, config.DefaultSettings().Motion); err != nil {
		t.Fatal(err)
	}
	state, err := engine.ApplyTarget(t.Context(), testTarget(), "library")
	if err != nil {
		t.Fatal(err)
	}
	if state.Target.TempoOnly || state.Target.Tempo == nil || state.Target.Tempo.Mode != TempoModeFollow {
		t.Fatalf("promoted target = %+v", state.Target)
	}
	if countCommands(linear.Commands(), transport.CommandKindPointsPlay) != 1 {
		t.Fatal("the promoted run did not start the Handy through its ordinary Play")
	}
}

func TestTempoOnlyPauseResumeKeepsTheTempo(t *testing.T) {
	tempo := transport.NewFake(transport.WithTempo())
	engine := newTestEngine(t, tempo, diagnostics.NewTraceRing(64), time.Hour)
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
	target := testTarget()
	spec := TempoSpec{Mode: TempoModePulse, MinPercent: 20, MaxPercent: 70, PeriodSeconds: 4}
	target.TempoOnly, target.Tempo = true, &spec
	if _, err := engine.Start(t.Context(), target, config.DefaultSettings().Motion); err != nil {
		t.Fatal(err)
	}
	paused, err := engine.Pause(t.Context(), "pause")
	if err != nil {
		t.Fatal(err)
	}
	if paused.Tempo == nil || paused.Tempo.LevelPercent != 0 {
		t.Fatalf("paused readout = %+v", paused.Tempo)
	}
	if _, err := engine.ApplyTempo(t.Context(), spec, engine.AdmissionGeneration()); err == nil {
		t.Fatal("tempo changed paused output")
	}
	resumed, err := engine.Resume(t.Context(), "resume")
	if err != nil || !resumed.Target.TempoOnly || resumed.Target.Tempo == nil || !resumed.Target.Tempo.Equal(spec) {
		t.Fatalf("resume lost the tempo: %+v %v", resumed.Target, err)
	}
	if _, err := engine.Stop(t.Context(), "stop"); err != nil {
		t.Fatal(err)
	}
	started, err := engine.Start(t.Context(), testTarget(), config.DefaultSettings().Motion)
	if err != nil || started.Target.TempoOnly || started.Target.Tempo == nil || started.Target.Tempo.Mode != TempoModeFollow {
		t.Fatalf("a new run inherited the tempo override: %+v %v", started.Target, err)
	}
}
