package httpapi

import (
	"context"

	"github.com/mapledaemon/MagicHandy/internal/persona"
)

func (s *Server) addDiagnosticPersonalization(ctx context.Context, archive *diagnosticArchive, options diagnosticReportOptions) error {
	settings, _ := s.store.PublicSnapshot()
	if options.Personas {
		items, err := s.personas.List(ctx)
		if err != nil {
			return err
		}
		lore := make(map[string][]persona.LoreEntry, len(items))
		for _, item := range items {
			entries, err := s.personas.ListLore(ctx, item.ID)
			if err != nil {
				return err
			}
			lore[item.ID] = entries
		}
		if err := archive.addJSON("personas.json", map[string]any{"personas": items, "lore": lore,
			"default_description": settings.LLM.PersonaDescription, "custom_anatomy": settings.LLM.CustomAnatomy}); err != nil {
			return err
		}
	}
	if options.Prompts {
		sets, err := s.promptSetsPayload()
		if err != nil {
			return err
		}
		if err := archive.addJSON("prompts.json", map[string]any{"prompt_sets": sets,
			"voice_reference_text": settings.Voice.TTSReferenceText, "voice_tone_prompt": settings.Voice.TTSTonePrompt}); err != nil {
			return err
		}
	}
	if options.Memories {
		memories, err := s.personalization.memory.Snapshot()
		if err != nil {
			return err
		}
		if err := archive.addJSON("memories.json", memories); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) addDiagnosticFailures(ctx context.Context, archive *diagnosticArchive, options diagnosticReportOptions) error {
	items, err := s.chatLog.FailuresForExport(ctx, options.ModelOutput || options.Prompts)
	if err != nil {
		return err
	}
	// Build the content projection explicitly so independent options cannot
	// accidentally re-export the entire stored prompt/output bundle.
	reports := make([]map[string]any, 0, len(items))
	for _, item := range items {
		content := item.Details
		item.Details = nil
		report := map[string]any{"session_id": item.SessionID, "failure": item.RequestFailure}
		if content != nil {
			selected := map[string]any{"truncated": content.Truncated}
			if options.ModelOutput {
				selected["raw_response"], selected["repair_response"] = content.Raw, content.RepairRaw
			}
			if options.Prompts {
				selected["system_prompt"], selected["response_schema"] = content.Prompt, content.Schema
			}
			report["content"] = selected
		}
		reports = append(reports, report)
	}
	return archive.addJSON("chat/failures.json", reports)
}
