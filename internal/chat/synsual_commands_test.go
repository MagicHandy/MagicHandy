package chat

import (
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestParseSynsualAssistantResponse(t *testing.T) {
	raw := "Of course, let's play! *teasing smirk*\n\ncowintense()"
	response, err := ParseSynsualAssistantResponse(raw)
	if err != nil {
		t.Fatalf("ParseSynsualAssistantResponse() error = %v", err)
	}
	if response.Reply == "" || response.Motion == nil {
		t.Fatalf("unexpected response: %+v", response)
	}
	if response.Motion.Action != MotionActionStart {
		t.Fatalf("action = %q, want start", response.Motion.Action)
	}
	if response.Motion.Regiao != "meio_cabeca" {
		t.Fatalf("regiao = %q", response.Motion.Regiao)
	}
	if response.Motion.PhysicalAction != "riding" {
		t.Fatalf("physical = %q", response.Motion.PhysicalAction)
	}
	if len(response.Motion.StrokeRange) != 2 || response.Motion.StrokeRange[0] <= 0 {
		t.Fatalf("stroke_range = %v", response.Motion.StrokeRange)
	}
}

func TestParseSynsualAssistantResponseGreetingWithoutCommand(t *testing.T) {
	raw := "Hi there, darling. Want to play a little game?"
	response, err := ParseSynsualAssistantResponse(raw)
	if err != nil {
		t.Fatalf("ParseSynsualAssistantResponse() error = %v", err)
	}
	if response.Motion == nil || response.Motion.Action != MotionActionNone {
		t.Fatalf("expected no motion, got %+v", response.Motion)
	}
}

func TestParseSynsualAssistantResponseUnknownCommand(t *testing.T) {
	_, err := ParseSynsualAssistantResponse("tease me\n\nnotARealCommand()")
	if err == nil {
		t.Fatal("expected unknown command error")
	}
}

func TestComposeSystemSynsualMode(t *testing.T) {
	set, ok := BuiltinPromptSetByID(PromptSetIDClarissaSynsualV1)
	if !ok {
		t.Fatal("missing clarissa prompt set")
	}
	system := ComposeSystemForMode(set, nil, config.MotionGenerationModeSynsual)
	if !strings.Contains(system, "Clarissa") || !strings.Contains(system, "bjtip()") {
		t.Fatalf("synsual system missing command block")
	}
	if strings.Contains(system, `"action"`) {
		t.Fatalf("synsual system should not include JSON director contract")
	}
}
