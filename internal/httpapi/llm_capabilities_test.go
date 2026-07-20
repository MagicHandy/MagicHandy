package httpapi

import (
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

func TestIdleTargetCommandStartsMotion(t *testing.T) {
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
	if err != nil {
		t.Fatalf("idle target must start motion (small-model leniency): %v", err)
	}
	if !dispatch.Applied || !dispatch.Engine.Running {
		t.Fatalf("dispatch = %+v, want running engine", dispatch)
	}
	if dispatch.Engine.Target.AreaFocus == nil || dispatch.Engine.Target.AreaFocus.MinPercent != 66 {
		t.Fatalf("area focus not applied: %+v", dispatch.Engine.Target.AreaFocus)
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
