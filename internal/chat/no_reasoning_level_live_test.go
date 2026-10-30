//go:build liveeval

package chat

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// withLiveLevel lets every hosted suite measure Advanced (no reasoning), ADR
// 0065, without editing each fixture's Advanced capabilities:
// MAGICHANDY_LIVE_LEVEL=advanced_no_reasoning applies the production basis
// schema, guide and parser at the
// provider boundary. MAGICHANDY_LIVE_PLAN_LOG appends each private basis for
// review; it is never part of the scored reply.
func withLiveLevel(provider llm.Provider) llm.Provider {
	if os.Getenv("MAGICHANDY_LIVE_LEVEL") != "advanced_no_reasoning" {
		return provider
	}
	return noReasoningLevelProvider{provider}
}

type noReasoningLevelProvider struct{ llm.Provider }

// Only interactive chat schemas carry both a reply and edits; Autopilot and
// reviewer requests keep their own contracts, as in production.
func (p noReasoningLevelProvider) StreamChat(ctx context.Context, request llm.ChatRequest, emit func(string) error) (string, error) {
	if !planNoteChatSchema(request.JSONSchema) {
		return p.Provider.StreamChat(ctx, request, emit)
	}
	mode := MotionModeCreativeV2
	if len(request.Messages) > 0 && strings.Contains(request.Messages[0].Content, "Patch the Layered score") {
		mode = MotionModeLayered
	}
	request.JSONSchema = withChatPlanSchema(request.JSONSchema)
	request.Messages = withChatPlanMessages(request.Messages, mode)
	raw, err := p.Provider.StreamChat(ctx, request, nil)
	if err != nil {
		return raw, err
	}
	if stripped, note, stripErr := takeChatPlanNote(raw); stripErr == nil {
		raw = stripped
		planNoteLog(note)
	}
	if emit != nil {
		if err := emit(raw); err != nil {
			return "", err
		}
	}
	return raw, nil
}

func planNoteChatSchema(raw json.RawMessage) bool {
	var schema map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &schema) != nil {
		return false
	}
	objects := []any{schema}
	for _, keyword := range []string{"oneOf", "anyOf"} {
		branches, _ := schema[keyword].([]any)
		objects = append(objects, branches...)
	}
	for _, value := range objects {
		object, _ := value.(map[string]any)
		properties, _ := object["properties"].(map[string]any)
		if properties["reply"] != nil && properties["edits"] != nil {
			return true
		}
	}
	return false
}

var planNoteLogMu sync.Mutex

func planNoteLog(note string) {
	path := os.Getenv("MAGICHANDY_LIVE_PLAN_LOG")
	if path == "" {
		return
	}
	encoded, _ := json.Marshal(map[string]string{"test": os.Getenv("MAGICHANDY_EXPERIMENT_CAPTURE"), "basis": note})
	planNoteLogMu.Lock()
	defer planNoteLogMu.Unlock()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	_, _ = file.Write(append(encoded, '\n'))
}
