package httpapi

import (
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestZoneAreaFocusLocalizesNamedZones(t *testing.T) {
	cases := map[string]*motion.AreaFocus{
		chat.AreaZoneTip:   {MinPercent: 66, MaxPercent: 100},
		chat.AreaZoneShaft: {MinPercent: 33, MaxPercent: 67},
		chat.AreaZoneBase:  {MinPercent: 0, MaxPercent: 34},
		chat.AreaZoneFull:  nil,
	}
	for zone, want := range cases {
		focus, ok := zoneAreaFocus(zone)
		if !ok {
			t.Fatalf("zone %q not recognized", zone)
		}
		if want == nil {
			if focus != nil {
				t.Fatalf("zone %q => %+v, want cleared focus", zone, focus)
			}
			continue
		}
		if focus == nil || *focus != *want {
			t.Fatalf("zone %q => %+v, want %+v", zone, focus, want)
		}
	}
	if _, ok := zoneAreaFocus("everywhere"); ok {
		t.Fatal("unknown zone must not resolve")
	}
}

func TestResolveAreaFocusPersistsUntilChanged(t *testing.T) {
	running := motion.ActiveMotionState{
		Running: true,
		Target:  motion.MotionTarget{AreaFocus: &motion.AreaFocus{MinPercent: 66, MaxPercent: 100}},
	}

	// A plain adjustment keeps the active focus (focus persists until changed).
	carried := resolveAreaFocus("", running)
	if carried == nil || carried.MinPercent != 66 {
		t.Fatalf("unset zone must carry the running focus, got %+v", carried)
	}
	if carried == running.Target.AreaFocus {
		t.Fatal("carried focus must be a copy, not the shared pointer")
	}

	// "full" explicitly clears it.
	if cleared := resolveAreaFocus(chat.AreaZoneFull, running); cleared != nil {
		t.Fatalf("full must clear the focus, got %+v", cleared)
	}

	// A new zone replaces it.
	if replaced := resolveAreaFocus(chat.AreaZoneBase, running); replaced == nil || replaced.MaxPercent != 34 {
		t.Fatalf("zone change must replace the focus, got %+v", replaced)
	}

	// Idle motion with no zone has no focus to carry.
	if idle := resolveAreaFocus("", motion.ActiveMotionState{}); idle != nil {
		t.Fatalf("idle unset zone => %+v, want nil", idle)
	}
}

func TestChatPatternChoicesGateExperimentalPatterns(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	defaults := chatCapabilities(config.LLMSettings{})
	if defaults.ExperimentalPatterns {
		t.Fatal("experimental patterns must default off")
	}
	gated, err := server.chatPatternChoicesFor(defaults)
	if err != nil {
		t.Fatalf("gated choices: %v", err)
	}
	for _, choice := range gated {
		if choice.ID == string(motion.PatternWaves) || choice.ID == string(motion.PatternFlutter) {
			t.Fatalf("experimental pattern %q leaked into the default catalog", choice.ID)
		}
	}

	open, err := server.chatPatternChoicesFor(chat.FullCapabilities())
	if err != nil {
		t.Fatalf("open choices: %v", err)
	}
	found := map[string]bool{}
	for _, choice := range open {
		found[choice.ID] = true
	}
	for _, want := range []motion.PatternID{motion.PatternWaves, motion.PatternClimb, motion.PatternFlutter} {
		if !found[string(want)] {
			t.Fatalf("experimental pattern %q missing with the gate enabled", want)
		}
	}
	if !found[string(motion.PatternStroke)] {
		t.Fatal("builtin stroke missing from the catalog")
	}
}

func TestIdleTargetCommandCannotStartMotion(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{
		Traces:          diagnostics.NewTraceRing(256),
		Transport:       fake,
		MotionTransport: fake,
	})
	t.Cleanup(server.Close)

	speed := 30
	dispatch, err := server.dispatchChatMotion(t.Context(), &chat.MotionCommand{
		Action:       chat.MotionActionTarget,
		SpeedPercent: &speed,
		Area:         chat.AreaZoneTip,
	})
	if err == nil || !strings.Contains(err.Error(), "use start") {
		t.Fatalf("idle target error = %v, want explicit start requirement", err)
	}
	if dispatch.Applied {
		t.Fatalf("idle target was applied: %+v", dispatch)
	}
	if engine := server.currentMotionEngine(); engine != nil && engine.Snapshot().Running {
		t.Fatal("idle target started the motion engine")
	}
}

func TestChatPatternChoicesSkipStorageWhenControlDisabled(t *testing.T) {
	server := &Server{}
	for _, capabilities := range []chat.Capabilities{
		{},
		{Motion: true},
	} {
		choices, err := server.chatPatternChoicesFor(capabilities)
		if err != nil || len(choices) != 0 {
			t.Fatalf("disabled capabilities %+v read pattern storage: choices=%v err=%v", capabilities, choices, err)
		}
	}
}

func TestChatAreaZoneDescribesEngineFocus(t *testing.T) {
	cases := []struct {
		focus *motion.AreaFocus
		want  string
	}{
		{nil, chat.AreaZoneFull},
		{&motion.AreaFocus{MinPercent: 66, MaxPercent: 100}, chat.AreaZoneTip},
		{&motion.AreaFocus{MinPercent: 33, MaxPercent: 67}, chat.AreaZoneShaft},
		{&motion.AreaFocus{MinPercent: 0, MaxPercent: 34}, chat.AreaZoneBase},
		{&motion.AreaFocus{MinPercent: 20, MaxPercent: 60}, "custom"},
	}
	for _, test := range cases {
		if got := chatAreaZone(test.focus); got != test.want {
			t.Fatalf("chatAreaZone(%+v) = %q, want %q", test.focus, got, test.want)
		}
	}
}

func TestRecentChatPatternIDsCollapseTraceNoise(t *testing.T) {
	ring := diagnostics.NewTraceRing(16)
	for _, row := range []diagnostics.MotionTraceRow{
		{Source: "manual_ui", Target: &diagnostics.MotionTraceTarget{PatternIdentifier: "stroke"}},
		{Source: "chat", Target: &diagnostics.MotionTraceTarget{PatternIdentifier: "pulse"}},
		{Source: "chat", Target: &diagnostics.MotionTraceTarget{PatternIdentifier: "pulse"}},
		{Source: "chat", Retarget: &diagnostics.MotionTraceRetarget{NextPatternIdentifier: "tease"}},
		{Source: "chat", Target: &diagnostics.MotionTraceTarget{PatternIdentifier: "tease"}},
		{Source: "chat", Target: &diagnostics.MotionTraceTarget{PatternIdentifier: "waves"}},
	} {
		ring.Add(row)
	}
	server := &Server{traces: ring}
	if got := strings.Join(server.recentChatPatternIDs(2), ","); got != "tease,waves" {
		t.Fatalf("recent chat patterns = %q, want tease,waves", got)
	}
}

func TestLLMCapabilitiesSettingsRoundTrip(t *testing.T) {
	// Nil (older payloads) resolves to defaults: everything but experimental.
	resolved := config.LLMSettings{}.Capabilities()
	if !resolved.Motion || !resolved.Patterns || !resolved.AreaFocus || resolved.ExperimentalPatterns {
		t.Fatalf("default capabilities = %+v", resolved)
	}

	// An explicit all-false choice survives resolution.
	explicit := config.LLMSettings{MotionCapabilities: &config.LLMMotionCapabilities{}}
	if got := explicit.Capabilities(); got.Motion || got.Patterns {
		t.Fatalf("explicit chat-only capabilities = %+v", got)
	}
}
