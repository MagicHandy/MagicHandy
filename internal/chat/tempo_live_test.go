//go:build liveeval

package chat

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// This opt-in evaluation uses production prompts, providers and parsers, with
// no transport. Every response, including failed selections, is retained.
func TestTempoLiveResponses(t *testing.T) {
	if os.Getenv("MAGICHANDY_TEMPO_EVAL") != "1" {
		t.Skip("opt-in native tempo evaluation")
	}
	provider, model, hosted := naturalLiveProvider(t)
	level := os.Getenv("MAGICHANDY_NATURAL_LEVEL")
	advanced := hosted || level == config.ModelCapabilityAdvanced
	intermediate := !advanced && level == config.ModelCapabilityIntermediate
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: advanced,
		IntermediateMotion: intermediate, SimpleMotion: !advanced && !intermediate, PreserveConversationText: hosted}
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	cases := []struct {
		name, message, want                string
		value                              int
		stopped, air, disconnected, paused bool
	}{
		{name: "start_exact", message: "Start a steady tempo at 25 percent now. Leave the air alone.", want: "steady", value: 25, stopped: true},
		{name: "fast_exact", message: "Set the tempo to 88 percent and hold it steady. Leave the air alone.", want: "steady", value: 88},
		{name: "slow_exact", message: "Set the tempo to 12 percent and hold it steady.", want: "steady", value: 12},
		{name: "maximum", message: "Use the fastest steady tempo I have allowed.", want: "steady", value: 100},
		{name: "small_faster", message: "A little faster now, keep it steady and leave the air alone.", want: "faster"},
		{name: "slower", message: "Slow the steady rhythm down, without changing the air.", want: "slower"},
		{name: "unchanged", message: "This tempo is right. Keep it exactly as it is.", want: "none"},
		{name: "explain_idle", message: "What does the Tempo range setting mean? Just explain; leave the motor stopped.", want: "none", stopped: true},
		{name: "explain_buildup", message: "What does Session buildup do? Just explain; leave playback stopped.", want: "none", stopped: true},
		{name: "explain_running", message: "What is the difference between a wave and a pulse? Explain only, without changing anything.", want: "none"},
		{name: "motor_pause", message: "Pause just the motor now, keeping the session active.", want: "pause"},
		{name: "stop", message: "Stop the session now.", want: "stop"},
		{name: "bursts", message: "Play the Bursts routine now.", want: "bursts"},
		{name: "swells", message: "Play the Swells routine now.", want: "swells"},
		{name: "air_forbidden", message: "Make the sleeve tighter by one step, and leave the tempo unchanged.", want: "none"},
		{name: "unavailable", message: "Start a steady tempo at 35 percent now.", want: "none", stopped: true, disconnected: true},
		{name: "paused", message: "Change the tempo to 60 percent.", want: "none", stopped: true, paused: true},
		{name: "depth_unavailable", message: "Stroke only the upper tip without changing the tempo.", want: "none"},
	}
	if intermediate || advanced {
		cases = append(cases, []struct {
			name, message, want                string
			value                              int
			stopped, air, disconnected, paused bool
		}{
			{name: "ramp", message: "Ease the tempo smoothly from where it is now to 70 percent over 12 seconds, then hold it there.", want: "ramp", value: 70},
			{name: "ramp_to_pause", message: "Slow the motor smoothly down to zero over 10 seconds, leaving the session active and the air unchanged.", want: "ramp_to_pause"},
			{name: "wave", message: "Make the tempo rise and fall smoothly between 25 and 75 percent, one cycle every 8 seconds.", want: "wave"},
			{name: "pulse", message: "Use rounded pulses from 20 to 80 percent, one every 6 seconds.", want: "pulse"},
			{name: "air_tighter", message: "Tighten the sleeve by one air step. Keep the tempo unchanged.", want: "air", value: 1, air: true},
			{name: "air_looser", message: "Loosen the sleeve by one air step. Keep the tempo unchanged.", want: "air", value: -1, air: true},
		}...)
	}
	if advanced {
		cases = append(cases, struct {
			name, message, want                string
			value                              int
			stopped, air, disconnected, paused bool
		}{name: "compound", message: "Plan this sequence: hold 25 percent for 5 seconds, ramp to 70 over 8 seconds, pause the motor for 3 seconds, then hold 35 percent until I change it. Do not loop it or change the air.", want: "sequence"})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := tempoTurnContext(!tc.stopped, tc.air)
			state.Paused = tc.paused
			state.Outputs.Tempo.Ready = !tc.disconnected
			if !tc.stopped {
				state.Outputs.Tempo.Current = &motion.TempoSpec{Mode: motion.TempoModeSteady, LevelPercent: 40}
				state.Outputs.Tempo.Level = 40
			}
			service := Service{Provider: provider, Model: model, Capabilities: &caps, MotionContext: state,
				ReasoningMode: "off", MaxTokens: naturalMaxTokens(), PromptBudget: naturalPromptBudget()}
			before := provider.calls
			started := time.Now()
			result, err := service.Complete(t.Context(), Request{Message: tc.message}, nil)
			pass := err == nil && result.Response.Reply != "" && !result.Repaired && !result.SemanticFallback && tempoLiveIntent(result.Response.Motion, tc.want, tc.value)
			row := map[string]any{"name": tc.name, "model": model, "level": level, "kind": "chat", "message": tc.message,
				"context": state.Outputs.Tempo, "raw": result.Raw, "raw_attempts": append([]string(nil), provider.raws[before:]...),
				"reply": result.Response.Reply, "expected": tc.want, "intent_pass": pass, "elapsed_ms": time.Since(started).Milliseconds(),
				"calls": provider.calls - before, "repaired": result.Repaired, "command": result.Response.Motion, "start_level": state.Outputs.Tempo.Level}
			if err != nil {
				row["error"] = err.Error()
			}
			rows = append(rows, row)
			saveHostedLayeredRows(t, rows)
			t.Logf("intent=%v calls=%d elapsed_ms=%v error=%v", pass, provider.calls-before, row["elapsed_ms"], err)
			if !pass {
				t.Error("native tempo intent mismatch; retained in capture")
			}
		})
	}
	for _, running := range []bool{false, true} {
		for turn := 0; turn < 3; turn++ {
			t.Run(fmt.Sprintf("autopilot_%t_%d", running, turn), func(t *testing.T) {
				state := tempoTurnContext(running, false)
				if running {
					state.Outputs.Tempo.Current = &motion.TempoSpec{Mode: motion.TempoModeRoutine, Routine: "swells", Seed: 17}
					state.Outputs.Tempo.Level = 40
				}
				autoCaps := caps
				autoCaps.AutoTiming = advanced
				service := AutopilotService{Provider: provider, Model: model, Capabilities: autoCaps, MotionContext: state,
					ReasoningMode: "off", MaxTokens: naturalMaxTokens(), PromptBudget: naturalPromptBudget()}
				message := "Autopilot is active. Plan the next stretch of native tempo within the current permissions. Let a satisfying phrase unfold, using the conversation and current tempo to decide whether to change it or hold."
				if !running {
					message += " There is no active tempo yet; choose a starting plan."
				}
				before := provider.calls
				started := time.Now()
				result, err := service.Complete(t.Context(), AutopilotKindMotion, Request{Message: message})
				pass := err == nil && provider.calls-before == 1 && (running || (result.Motion != nil && result.Motion.Tempo != nil))
				row := map[string]any{"name": t.Name(), "model": model, "level": level, "kind": "autopilot", "message": message,
					"context": state.Outputs.Tempo, "raw_attempts": append([]string(nil), provider.raws[before:]...), "intent": result.Intent,
					"intent_pass": pass, "elapsed_ms": time.Since(started).Milliseconds(), "calls": provider.calls - before,
					"command": result.Motion, "next_check_seconds": result.NextCheckSeconds, "start_level": state.Outputs.Tempo.Level}
				if err != nil {
					row["error"] = err.Error()
				}
				rows = append(rows, row)
				saveHostedLayeredRows(t, rows)
				t.Logf("intent=%v calls=%d elapsed_ms=%v error=%v", pass, provider.calls-before, row["elapsed_ms"], err)
				if !pass {
					t.Error("native Autopilot intent mismatch; retained in capture")
				}
			})
		}
	}
}

func tempoLiveIntent(command *MotionCommand, want string, value int) bool {
	if want == "none" {
		return command == nil
	}
	if command == nil {
		return false
	}
	if want == "stop" {
		return command.Action == MotionActionStop
	}
	if want == "air" {
		return command.AirSteps != nil && *command.AirSteps == value && command.Tempo == nil
	}
	spec := command.Tempo
	if spec == nil || command.AirSteps != nil {
		return false
	}
	switch want {
	case "steady":
		return spec.Mode == motion.TempoModeSteady && spec.LevelPercent == value
	case "faster":
		return spec.Mode == motion.TempoModeSteady && spec.LevelPercent > 40 && spec.LevelPercent < 75
	case "slower":
		return spec.Mode == motion.TempoModeSteady && spec.LevelPercent < 40
	case "pause":
		return spec.Mode == motion.TempoModePause && command.Action != MotionActionStop
	case "bursts", "swells":
		return spec.Mode == motion.TempoModeRoutine && spec.Routine == want
	case "ramp":
		return spec.Mode == motion.TempoModeRamp && spec.LevelPercent == value && spec.PeriodSeconds == 12
	case "ramp_to_pause":
		return spec.Mode == motion.TempoModeRamp && spec.LevelPercent == 0 && spec.PeriodSeconds == 10 && command.Action != MotionActionStop
	case "wave":
		return spec.Mode == motion.TempoModeWave && spec.MinPercent == 25 && spec.MaxPercent == 75 && spec.PeriodSeconds == 8
	case "pulse":
		return spec.Mode == motion.TempoModePulse && spec.MinPercent == 20 && spec.MaxPercent == 80 && spec.PeriodSeconds == 6
	case "sequence":
		return spec.Mode == motion.TempoModeSequence && !spec.Loop && len(spec.Sections) == 4 &&
			spec.Sections[0].LevelPercent == 25 && spec.Sections[0].Seconds == 5 &&
			spec.Sections[1].LevelPercent == 70 && spec.Sections[1].PeriodSeconds == 8 && spec.Sections[1].Seconds == 8 &&
			spec.Sections[2].Mode == motion.TempoModePause && spec.Sections[2].Seconds == 3 && spec.Sections[3].LevelPercent == 35
	}
	return false
}
