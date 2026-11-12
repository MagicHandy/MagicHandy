package chat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func tempoTurnContext(running, airAllowed bool) *MotionContext {
	return &MotionContext{Running: running, Outputs: &OutputContext{Strokers: 1, PaceOnly: 1,
		Tempo: &TempoContext{Leads: true, Ready: true, Running: running, AirAllowed: airAllowed}}}
}

func TestTempoResponseParsesEachTierShape(t *testing.T) {
	state := tempoTurnContext(true, true)
	for _, test := range []struct {
		raw  string
		tier tempoTier
		want motion.TempoSpec
	}{
		{`{"reply":"Slow and steady.","tempo":{"mode":"steady","level_percent":25,"min_percent":null,"max_percent":null,"period_seconds":null,"routine":null},"air_steps":null,"stop":false}`,
			tempoSimple, motion.TempoSpec{Mode: motion.TempoModeSteady, LevelPercent: 25}},
		{`{"reply":"Bursts.","tempo":{"mode":"routine","level_percent":null,"min_percent":null,"max_percent":null,"period_seconds":null,"routine":"bursts"},"air_steps":null,"stop":false}`,
			tempoSimple, motion.TempoSpec{Mode: motion.TempoModeRoutine, Routine: "bursts"}},
		// A strict provider fills unused fields; they never change the meaning.
		{`{"reply":"Waves.","tempo":{"mode":"wave","level_percent":40,"min_percent":30,"max_percent":70,"period_seconds":8,"routine":"swells"},"air_steps":null,"stop":false}`,
			tempoIntermediate, motion.TempoSpec{Mode: motion.TempoModeWave, MinPercent: 30, MaxPercent: 70, PeriodSeconds: 8}},
		{`{"reply":"Building.","tempo":{"mode":"sequence","level_percent":null,"min_percent":null,"max_percent":null,"period_seconds":null,"routine":null,"loop":false,"sections":[{"mode":"ramp","level_percent":70,"min_percent":null,"max_percent":null,"period_seconds":20,"seconds":30},{"mode":"pause","level_percent":0,"min_percent":0,"max_percent":0,"period_seconds":0,"seconds":5},{"mode":"steady","level_percent":30,"min_percent":null,"max_percent":null,"period_seconds":null,"seconds":null}]},"air_steps":null,"stop":false}`,
			tempoAdvanced, motion.TempoSpec{Mode: motion.TempoModeSequence, Sections: []motion.TempoSection{
				{Mode: motion.TempoModeRamp, LevelPercent: 70, PeriodSeconds: 20, Seconds: 30},
				{Mode: motion.TempoModePause, Seconds: 5},
				{Mode: motion.TempoModeSteady, LevelPercent: 30},
			}}},
	} {
		response, err := parseTempoResponse(test.raw, test.tier, state)
		if err != nil {
			t.Fatalf("%s: %v", test.raw, err)
		}
		if response.Motion == nil || response.Motion.Tempo == nil || !response.Motion.Tempo.Equal(test.want) {
			t.Fatalf("%s: tempo = %+v", test.raw, response.Motion)
		}
	}
}

func TestTempoResponseRejectsWhatTheTierOrStateDoesNotAllow(t *testing.T) {
	wave := `{"reply":"x","tempo":{"mode":"wave","level_percent":null,"min_percent":30,"max_percent":70,"period_seconds":8,"routine":null},"air_steps":null,"stop":false}`
	if _, err := parseTempoResponse(wave, tempoSimple, tempoTurnContext(true, false)); err == nil {
		t.Fatal("Simple accepted a wave")
	}
	air := `{"reply":"Tighter.","tempo":null,"air_steps":1,"stop":false}`
	if _, err := parseTempoResponse(air, tempoAdvanced, tempoTurnContext(true, false)); err == nil {
		t.Fatal("air changed without the person's permission")
	}
	if response, err := parseTempoResponse(air, tempoAdvanced, tempoTurnContext(true, true)); err != nil || response.Motion == nil || *response.Motion.AirSteps != 1 {
		t.Fatalf("permitted air = %+v, %v", response.Motion, err)
	}
	paused := tempoTurnContext(false, false)
	paused.Paused = true
	steady := `{"reply":"x","tempo":{"mode":"steady","level_percent":40,"min_percent":null,"max_percent":null,"period_seconds":null,"routine":null},"air_steps":null,"stop":false}`
	if _, err := parseTempoResponse(steady, tempoSimple, paused); err == nil {
		t.Fatal("a model resumed paused playback")
	}
	stop := `{"reply":"Stopping.","tempo":null,"air_steps":null,"stop":true}`
	if response, err := parseTempoResponse(stop, tempoSimple, tempoTurnContext(true, false)); err != nil || response.Motion == nil || response.Motion.Action != MotionActionStop {
		t.Fatalf("stop = %+v, %v", response.Motion, err)
	}
	if response, err := parseTempoResponse(`{"reply":"Just talking.","tempo":null,"air_steps":null,"stop":false}`, tempoSimple, tempoTurnContext(true, false)); err != nil || response.Motion != nil {
		t.Fatalf("a text-only reply carried motion: %+v, %v", response.Motion, err)
	}
	if _, err := parseTempoResponse(`{"reply":"x","reply":"y","tempo":null,"air_steps":null,"stop":false}`, tempoSimple, nil); err == nil {
		t.Fatal("duplicate keys were accepted")
	}
}

func TestTempoSchemaFollowsTierAndPermission(t *testing.T) {
	decode := func(raw json.RawMessage) string { return string(raw) }
	simple := decode(tempoResponseSchema(tempoSimple, tempoTurnContext(true, true)))
	if strings.Contains(simple, `"wave"`) || strings.Contains(simple, `"sections"`) || !strings.Contains(simple, `"air_steps":{"type":"null"}`) {
		t.Fatalf("Simple schema = %s", simple)
	}
	advanced := decode(tempoResponseSchema(tempoAdvanced, tempoTurnContext(true, true)))
	if !strings.Contains(advanced, `"sequence"`) || !strings.Contains(advanced, `"sections"`) || strings.Contains(advanced, `"air_steps":{"type":"null"}`) {
		t.Fatalf("Advanced schema = %s", advanced)
	}
	noAir := decode(tempoResponseSchema(tempoAdvanced, tempoTurnContext(true, false)))
	if !strings.Contains(noAir, `"air_steps":{"type":"null"}`) {
		t.Fatal("the schema offered air without permission")
	}
}

func TestTempoTurnReplacesThePositionVocabulary(t *testing.T) {
	provider := &scriptedProvider{responses: []string{`{"reply":"Easing into a slow rhythm.","tempo":{"mode":"steady","level_percent":20,"min_percent":null,"max_percent":null,"period_seconds":null,"routine":null},"air_steps":null,"stop":false}`}}
	caps := FullCapabilities()
	caps.Motion, caps.MotionMode, caps.AdvancedMotion = true, MotionModeLayered, true
	service := Service{Provider: provider, Capabilities: &caps, MotionContext: tempoTurnContext(false, false)}
	result, err := service.Complete(context.Background(), Request{Message: "start slow"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Response.Motion == nil || result.Response.Motion.Tempo == nil || result.Response.Motion.Tempo.LevelPercent != 20 {
		t.Fatalf("motion = %+v", result.Response.Motion)
	}
	system := provider.requests[0].Messages[0].Content
	if !strings.Contains(system, "THE DEVICE: The stroker is a VacuGlide") || !strings.Contains(system, "TEMPO CONTEXT") {
		t.Fatal("the tempo contract is missing")
	}
	for _, position := range []string{"DEPTH FRAME", "center_percent", "PACE-ONLY STROKER", `"action":"start"`} {
		if strings.Contains(system, position) {
			t.Fatalf("the tempo turn still teaches the position vocabulary: %q", position)
		}
	}
	if !strings.Contains(string(provider.requests[0].JSONSchema), `"tempo"`) {
		t.Fatal("the tempo schema was not sent")
	}
}

func TestTempoTurnRepairsOnceThenKeepsTheWords(t *testing.T) {
	provider := &scriptedProvider{responses: []string{
		`{"reply":"Faster now.","tempo":{"mode":"steady","level_percent":0,"min_percent":null,"max_percent":null,"period_seconds":null,"routine":null},"air_steps":null,"stop":false}`,
		`{"reply":"Faster now.","tempo":{"mode":"steady","level_percent":70,"min_percent":null,"max_percent":null,"period_seconds":null,"routine":null},"air_steps":null,"stop":false}`,
	}}
	caps := FullCapabilities()
	caps.Motion = true
	service := Service{Provider: provider, Capabilities: &caps, MotionContext: tempoTurnContext(true, false)}
	result, err := service.Complete(context.Background(), Request{Message: "faster"}, nil)
	if err != nil || !result.Repaired || result.Response.Motion == nil || result.Response.Motion.Tempo.LevelPercent != 70 {
		t.Fatalf("repaired = %+v, %v", result, err)
	}
}

func TestHandyTurnsKeepTheirPrompt(t *testing.T) {
	provider := &scriptedProvider{responses: []string{`{"reply":"ok"}`}}
	caps := FullCapabilities()
	caps.Motion = false
	state := &MotionContext{Outputs: &OutputContext{Strokers: 2, PaceOnly: 1, Tempo: &TempoContext{Leads: false}}}
	service := Service{Provider: provider, Capabilities: &caps, MotionContext: state}
	if _, err := service.Complete(context.Background(), Request{Message: "hello"}, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(provider.requests[0].Messages[0].Content, "THE DEVICE: The stroker is a VacuGlide") {
		t.Fatal("a session led by positions received the tempo contract")
	}
}

func TestTempoAutopilotParsesAHoldAndAChange(t *testing.T) {
	state := tempoTurnContext(true, false)
	hold, err := parseTempoAutopilot(`{"intent":"let it ride","tempo":null,"air_steps":null,"next":"later"}`, tempoIntermediate, state)
	if err != nil || hold.Motion != nil || hold.Next != AutopilotTimingLater || hold.Variability != "settled" {
		t.Fatalf("hold = %+v, %v", hold, err)
	}
	change, err := parseTempoAutopilot(`{"intent":"swell","tempo":{"mode":"wave","level_percent":null,"min_percent":35,"max_percent":75,"period_seconds":10,"routine":null},"air_steps":null,"next":"soon"}`, tempoIntermediate, state)
	if err != nil || change.Motion == nil || change.Motion.Tempo.Mode != motion.TempoModeWave || change.Intent != "swell" {
		t.Fatalf("change = %+v, %v", change, err)
	}
	if _, err := parseTempoAutopilot(`{"intent":"x","tempo":null,"air_steps":null,"next":"eventually"}`, tempoIntermediate, state); err == nil {
		t.Fatal("an unknown timing was accepted")
	}
	schema := string(tempoAutopilotSchema(tempoIntermediate, state, true, 10))
	if !strings.Contains(schema, nextCheckField) || strings.Contains(schema, `"reply"`) {
		t.Fatalf("Autopilot schema = %s", schema)
	}
}

func TestHostedTempoHonorsTheRepairSetting(t *testing.T) {
	bad := `{"reply":"Faster now.","tempo":{"mode":"steady","level_percent":0},"air_steps":null,"stop":false}`
	good := `{"reply":"Faster now.","tempo":{"mode":"steady","level_percent":70},"air_steps":null,"stop":false}`
	for _, enabled := range []bool{false, true} {
		provider := &scriptedProvider{responses: []string{bad, good}}
		caps := Capabilities{Motion: true, AdvancedMotion: true, PreserveConversationText: true}
		svc := Service{Provider: provider, Capabilities: &caps, MotionContext: tempoTurnContext(true, false), HostedResponseRepair: enabled}
		result, err := svc.Complete(t.Context(), Request{Message: "Faster now."}, nil)
		if enabled {
			if err != nil || !result.Repaired || len(provider.requests) != 2 || result.Response.Motion == nil {
				t.Fatalf("enabled repair = %+v, %v", result, err)
			}
		} else if err == nil || result.Response.Motion != nil || !result.CanRepair() || len(provider.requests) != 1 {
			t.Fatalf("disabled repair was ignored: calls=%d repairable=%v error=%v", len(provider.requests), result.CanRepair(), err)
		}
	}
}

func TestTempoRepairCancellationCannotBecomeASuccess(t *testing.T) {
	provider := &scriptedProvider{responses: []string{
		`{"reply":"Faster now.","tempo":{"mode":"steady","level_percent":0},"air_steps":null,"stop":false}`, ""},
		responseErrors: []error{nil, context.Canceled}}
	caps := Capabilities{Motion: true, IntermediateMotion: true}
	svc := Service{Provider: provider, Capabilities: &caps, MotionContext: tempoTurnContext(true, false)}
	result, err := svc.Complete(t.Context(), Request{Message: "Faster now."}, nil)
	if !errors.Is(err, context.Canceled) || result.Response.Reply != "" {
		t.Fatalf("canceled repair returned a successful claim: %+v, %v", result.Response, err)
	}
}

func TestTempoContinuationKeepsTheSavedPlanAndSeed(t *testing.T) {
	state := tempoTurnContext(false, false)
	state.Outputs.Tempo.Previous = &motion.TempoSpec{Mode: motion.TempoModeRoutine, Routine: "wandering", Seed: 9123}
	for _, tier := range []tempoTier{tempoSimple, tempoIntermediate, tempoAdvanced} {
		response, err := parseTempoResponse(`{"reply":"Continuing the same routine.","tempo":{"mode":"resume"},"air_steps":null,"stop":false}`, tier, state)
		if err != nil || response.Motion == nil || !response.Motion.Tempo.Equal(*state.Outputs.Tempo.Previous) {
			t.Fatalf("tier %d continuation = %+v, %v", tier, response, err)
		}
		if response.Motion.Tempo == state.Outputs.Tempo.Previous {
			t.Fatal("continuation shares mutable saved state")
		}
	}
	state.Outputs.Tempo.Running = true
	if _, err := parseTempoField(json.RawMessage(`{"mode":"resume"}`), tempoAdvanced, state); err == nil {
		t.Fatal("a stale resume restarted a running plan")
	}
}

func TestTempoSpeechUsesNativeVocabularyAndCanStayQuiet(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		provider := &scriptedProvider{responses: []string{`{"intent":"quiet","reply":"","tempo":null,"air_steps":null,"next":"later"}`}}
		svc := AutopilotService{Provider: provider, Capabilities: Capabilities{Motion: allowed, MotionMode: MotionModeLayered, IntermediateMotion: true}, MotionContext: tempoTurnContext(true, true)}
		result, err := svc.Complete(t.Context(), AutopilotKindSpeech, Request{Message: "A check-in is due."})
		if err != nil || result.Reply != "" || result.Motion != nil || result.Next != AutopilotTimingLater {
			t.Fatalf("quiet check-in = %+v, %v", result, err)
		}
		system := provider.requests[0].Messages[0].Content
		if !strings.Contains(system, "VacuGlide") || strings.Contains(system, "center_percent") || strings.Contains(system, "DEPTH FRAME") {
			t.Fatal("spoken check-in used the position contract")
		}
	}
	raw := `{"intent":"ease the tempo","reply":"Easing the tempo.","tempo":{"mode":"steady","level_percent":25},"air_steps":null,"next":"normal"}`
	if response, err := parseTempoSpeech(raw, tempoSimple, tempoTurnContext(true, false), true); err != nil || response.Motion == nil {
		t.Fatalf("native speech change = %+v, %v", response, err)
	}
	if _, err := parseTempoSpeech(raw, tempoSimple, tempoTurnContext(true, false), false); err == nil {
		t.Fatal("speech changed motion without authority")
	}
}
