//go:build liveeval

package chat

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Roleplay flow: persona presets in scenes where the person reacts, invites and
// gives cues but never names a speed, depth or pattern. The question is whether
// the reply and the motion tell the same story turn by turn, and whether the
// motion moves with the scene. Every turn is retained with its compiled feel
// before and after, for TestRoleplayFlowJudge and motion-atlas review.
//
// MAGICHANDY_ROLEPLAY_SET selects matrix, scenes or all (the default).
// MAGICHANDY_ROLEPLAY_FILTER keeps scenes whose key contains any of its
// comma-separated parts. MAGICHANDY_ROLEPLAY_MODE=creative_v2 uses Creative v2
// instead of Layered, and MAGICHANDY_ROLEPLAY_AUTOPILOT=1 has Autopilot
// composing between chat turns (the planner itself is not simulated).
// MAGICHANDY_ROLEPLAY_SCENE_OFF=1 plays the scenes with scene motion switched
// off for quicker replies (ADR 0068).
func TestHostedRoleplayFlowLive(t *testing.T) {
	provider, model := hostedLayeredLiveProvider(t)
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	scenes := selectedRoleplayScenes()
	if len(scenes) == 0 {
		t.Fatal("no scenes selected; check MAGICHANDY_ROLEPLAY_SET and MAGICHANDY_ROLEPLAY_FILTER")
	}
	for _, scene := range scenes {
		t.Run(strings.ReplaceAll(scene.key, "/", "_"), func(t *testing.T) {
			rows = append(rows, runRoleplayScene(t, provider, model, scene)...)
			saveHostedLayeredRows(t, rows)
		})
	}
}

type roleplayPersona struct {
	key, name, desc string
	voice           VoiceLevel
	style           ReactionStyle
}

type roleplayScene struct {
	key     string
	persona roleplayPersona
	running bool
	speed   int
	turns   []string
}

// roleplayArc starts stopped. It greets, invites, praises, asks for more, says
// the person is close, urges, and winds down, without naming any control.
var roleplayArc = []string{
	"Hey. I'm glad you're here; I've been looking forward to this all day.",
	"I'm lying back now. I'm all yours, whenever you're ready.",
	"Mmm. That feels really good.",
	"Please, I want more.",
	"Oh god. I'm getting close.",
	"Don't you dare stop now.",
	"That was incredible. Stay with me for a minute.",
}

func roleplayMatrix() []roleplayScene {
	scenes := []roleplayScene{}
	for _, voice := range []VoiceLevel{VoiceUtility, VoiceWarm, VoiceIntimate, VoiceExplicit} {
		for _, style := range []ReactionStyle{StyleNeutral, StylePlayful, StyleTender, StyleDominant, StyleSubmissive, StyleTeasing} {
			persona := roleplayPersona{key: string(voice) + "_" + string(style), name: "Alex", voice: voice, style: style,
				desc: "Alex is in their thirties, easygoing and attentive."}
			scenes = append(scenes, roleplayScene{key: "arc/" + persona.key, persona: persona, speed: 30, turns: roleplayArc})
		}
	}
	return scenes
}

// roleplayTypes pair a kind of scene with a persona that suits it. Only the
// last one uses roleplay words for intensity; the rest give no motion cue.
func roleplayTypes() []roleplayScene {
	return []roleplayScene{
		{key: "type/tease_and_deny", running: true, speed: 30, persona: roleplayPersona{key: "nina", name: "Nina", voice: VoiceExplicit, style: StyleTeasing,
			desc: "Nina is thirty-one, quick-witted and patient in a dangerous way. She likes making people wait for her."},
			turns: []string{"I've been good all week. Are you going to make me wait again?", "Please. I need it.", "You're so mean to me.", "I'm right there, please let me...", "Okay. I'll be patient for you."}},
		{key: "type/in_charge", speed: 30, persona: roleplayPersona{key: "marcus", name: "Marcus", voice: VoiceIntimate, style: StyleDominant,
			desc: "Marcus is forty, calm and certain. He decides how the evening goes and expects to be listened to."},
			turns: []string{"Tell me what you want from me tonight.", "Yes. Whatever you say.", "Am I doing okay?", "I'm getting close. Can I?", "Thank you."}},
		{key: "type/follow_my_lead", running: true, speed: 25, persona: roleplayPersona{key: "lena", name: "Lena", voice: VoiceWarm, style: StyleSubmissive,
			desc: "Lena is twenty-eight, sweet and eager to please, happiest when she's doing something you like."},
			turns: []string{"I want you to take care of me tonight.", "Just like that.", "A little more for me?", "I'm close...", "Perfect. Stay right there with me."}},
		{key: "type/slow_romance", running: true, speed: 20, persona: roleplayPersona{key: "jamie", name: "Jamie", voice: VoiceIntimate, style: StyleTender,
			desc: "Jamie is thirty-five, gentle and unhurried, and pays close attention to how you're feeling."},
			turns: []string{"Come here. I missed you.", "Let's take our time tonight.", "That's nice.", "I love being this close to you.", "Don't let go."}},
		{key: "type/playful", running: true, speed: 35, persona: roleplayPersona{key: "theo", name: "Theo", voice: VoiceWarm, style: StylePlayful,
			desc: "Theo is twenty-nine, a shameless flirt who turns everything into a game."},
			turns: []string{"Bet you can't make me blush tonight.", "Okay, that was cute.", "Is that all you've got?", "Ha, okay, okay, you win.", "Now what?"}},
		{key: "type/rough", running: true, speed: 40, persona: roleplayPersona{key: "kat", name: "Kat", voice: VoiceExplicit, style: StyleDominant,
			desc: "Kat is thirty-three, blunt and hungry, and never asks twice."},
			turns: []string{"I'm yours tonight. Use me however you want.", "Fuck, yes.", "Harder...", "Don't stop, I'm so close.", "Holy shit. Okay. I need a second."}},
	}
}

func selectedRoleplayScenes() []roleplayScene {
	scenes := []roleplayScene{}
	switch os.Getenv("MAGICHANDY_ROLEPLAY_SET") {
	case "matrix":
		scenes = roleplayMatrix()
	case "scenes":
		scenes = roleplayTypes()
	case "", "all":
		scenes = append(roleplayMatrix(), roleplayTypes()...)
	default:
		return nil
	}
	filter := os.Getenv("MAGICHANDY_ROLEPLAY_FILTER")
	if filter == "" {
		return scenes
	}
	kept := []roleplayScene{}
	for _, scene := range scenes {
		for _, part := range strings.Split(filter, ",") {
			if part != "" && strings.Contains(scene.key, part) {
				kept = append(kept, scene)
				break
			}
		}
	}
	return kept
}

func runRoleplayScene(t *testing.T, provider *hostedLayeredCapture, model string, scene roleplayScene) []map[string]any {
	settings, mode := followthroughSettings(), roleplayMode(t)
	score := DefaultLayeredScore(scene.speed)
	if mode == MotionModeCreativeV2 {
		score = FreshCreativeV2Score(scene.speed)
	}
	score.Seed = 20261109
	running := scene.running
	history, requests, replies := []llm.Message{}, []string{}, []string{}
	rows := []map[string]any{}
	for i, message := range scene.turns {
		before, wasRunning := *motion.CloneFlowSpec(&score), running
		state := MotionContext{Running: running, MotionMode: mode, Layered: &before, SpeedPercent: before.SpeedPercent,
			SpeedMinPercent: settings.SpeedMinPercent, SpeedMaxPercent: settings.SpeedMaxPercent, Autopilot: os.Getenv("MAGICHANDY_ROLEPLAY_AUTOPILOT") == "1"}
		state.UserRequests = SelectRecentUserRequests(requests)
		for n := range state.UserRequests {
			state.UserRequestSecondsAgo = append(state.UserRequestSecondsAgo, (len(state.UserRequests)-n)*25)
		}
		state = withFollowthroughFacts(t, state)
		caps := Capabilities{Motion: true, MotionMode: mode, AdvancedMotion: true, PreserveConversationText: true,
			Voice: scene.persona.voice, Style: scene.persona.style, ReplyLength: ReplyLengthBalanced, HistoryMessages: 256, HistoryBytes: 96000,
			SceneMotionOff: os.Getenv("MAGICHANDY_ROLEPLAY_SCENE_OFF") == "1"}
		conversation := &ConversationContext{PersonaName: scene.persona.name, PersonaDescription: scene.persona.desc,
			RecentAssistantReplies: replies[max(0, len(replies)-3):]}
		svc := Service{Provider: provider, Model: model, MaxTokens: 1024, ReasoningMode: roleplayReasoning(), Capabilities: &caps,
			MotionContext: &state, ConversationContext: conversation, HostedResponseRepair: true,
			PromptBudget: PromptBudgetSettings{ContextSize: 131072, MaxOutputTokens: 1024}}
		started, calls := time.Now(), provider.calls
		result, err := svc.Complete(t.Context(), Request{Message: message, History: history}, nil)
		command, action := result.Response.Motion, "none"
		if err == nil && command != nil && command.Action != "" {
			action = command.Action
			switch command.Action {
			case "stop":
				running = false
			case "start", "update":
				if command.Layered != nil {
					score = *motion.CloneFlowSpec(command.Layered)
				}
				running = running || command.Action == "start"
			}
		}
		reply := result.Response.Reply
		row := hostedLayeredLiveRow(fmt.Sprintf("%s/%02d", scene.key, i+1), model, message, result.Raw, before, command, settings, started, err, err == nil && reply != "")
		delete(row, "intent_pass") // Acceptance alone does not establish scene intent.
		delete(row, "expected_recipe")
		row["response_accepted"] = err == nil && strings.TrimSpace(reply) != ""
		row["method"], row["mode"], row["autopilot"] = "roleplay", string(mode), state.Autopilot
		row["scene"], row["turn"], row["persona"] = scene.key, i+1, scene.persona.key
		row["voice"], row["style"], row["reply"], row["action"] = string(scene.persona.voice), string(scene.persona.style), reply, action
		row["running_before"], row["running_after"] = wasRunning, running
		row["motion_command"] = command
		if !running || err != nil {
			delete(row, "after") // A stopped or rejected turn has no moving output.
			if err == nil {
				row["selected"] = "stop"
			}
		}
		row["feel_before"], row["feel_after"] = roleplayFeel(t, before, wasRunning), roleplayFeel(t, score, running)
		row["repaired"], row["fallback"], row["provider_calls"] = result.Repaired, result.SemanticFallback, provider.calls-calls
		rows = append(rows, row)
		t.Logf("turn %d action=%s running=%t->%t speed=%d->%d err=%v", i+1, action, wasRunning, running, before.SpeedPercent, score.SpeedPercent, err)
		history = append(history, llm.Message{Role: "user", Content: message})
		requests = append(requests, message)
		if err != nil || strings.TrimSpace(reply) == "" {
			t.Errorf("turn %d failed; raw response retained in capture", i+1)
			continue // Unaccepted replies must not become conversation history.
		}
		history = append(history, llm.Message{Role: "assistant", Content: reply})
		replies = append(replies, reply)
	}
	return rows
}

func roleplayMode(t *testing.T) MotionMode {
	t.Helper()
	switch os.Getenv("MAGICHANDY_ROLEPLAY_MODE") {
	case "creative_v2":
		return MotionModeCreativeV2
	case "", "layered":
		return MotionModeLayered
	default:
		t.Fatal("MAGICHANDY_ROLEPLAY_MODE must be layered or creative_v2")
		return ""
	}
}

func roleplayReasoning() string {
	if effort := os.Getenv("MAGICHANDY_LIVE_REASONING"); effort != "" {
		return effort
	}
	return "off"
}

// roleplayFeel is the compiled feel the person gets: pace as mean commanded
// travel, the depth range reached and the mean stroke length. Stopped motion
// has no feel.
func roleplayFeel(t *testing.T, spec motion.FlowSpec, running bool) map[string]float64 {
	if !running {
		return nil
	}
	settings := followthroughSettings()
	target, err := motion.FlowTarget(spec, settings)
	if err != nil {
		t.Errorf("compile for feel failed: %v", err)
		return nil
	}
	summary := motion.NewMotionPlan("roleplay", target, settings, 0, 0, time.Unix(0, 0)).Perceptual
	return map[string]float64{"travel": summary.CommandedMeanTravelPerSecond, "min": summary.PositionMinPercent,
		"max": summary.PositionMaxPercent, "stroke": summary.MeanStrokePercent, "speed": float64(spec.SpeedPercent)}
}
