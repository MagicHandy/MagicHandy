package llm

import (
	"bytes"
	"context"
	_ "embed" // chat template fixes ship inside the binary
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Chat template fixes MagicHandy can apply when it launches a managed model.
const (
	// TemplateFixGemma4CloseThinking swaps in Google's Gemma 4 chat template
	// with an empty thought channel closed whenever thinking is off. Several
	// fine-tuned Gemma 4 builds ship an older template without that close and
	// start hidden reasoning on every turn: replies take seconds and often run
	// out of tokens before the JSON the app needs.
	TemplateFixGemma4CloseThinking = "gemma4-close-thinking"
)

// Where a model's template fix comes from.
const (
	// TemplateFixSourceKnown marks a fix applied automatically because the
	// model embeds a chat template MagicHandy has measured.
	TemplateFixSourceKnown = "known"
	// TemplateFixSourceUser marks a fix the user turned on for a template
	// with the same defect that MagicHandy has not measured.
	TemplateFixSourceUser = "user"
)

// ErrNoTemplateFix reports a model with no chat template fix to change.
var ErrNoTemplateFix = errors.New("this model has no chat template fix to change")

const templateFixOverridesKey = "llm.template_fix_overrides"

// gemma4CloseThinkingTemplate is Google's canonical Gemma 4 chat template
// (google/gemma-4-E4B-it, published 2026-07-09, Apache-2.0) plus one marked
// addition: when thinking is off, the generation prompt opens and closes an
// empty thought channel, as Google's later Gemma 4 templates do.
//
//go:embed templates/gemma4-close-thinking.jinja
var gemma4CloseThinkingTemplate []byte

var templateFixTemplates = map[string][]byte{
	TemplateFixGemma4CloseThinking: gemma4CloseThinkingTemplate,
}

// knownTemplateFixes maps the SHA-256 of an embedded chat template to the fix
// measured for it (docs/model-suitability.md). Keying on the template rather
// than the file covers every quantization of a build that shares it.
var knownTemplateFixes = map[string]string{
	// gemma-4-E4B-it heretic builds by HTNZ555 (QAT UD-Q4_K_XL), mradermacher
	// (Heretic-Ultra) and NullpoLab (ARA Refusals5).
	"33204f1acb5bd0002713e16a593847f24ceeafe711ed88bda2a352dc996a3373": TemplateFixGemma4CloseThinking,
	// gemma-4-E4B-it ultra-uncensored heretic by llmfan46.
	"781d10940fbc44be40064b5d43a056fc486c84ceaa55538226368b57314132bf": TemplateFixGemma4CloseThinking,
	// gemma-4-E4B-it heretic by Abiray and by igorls.
	"55572b8d3c8342044e25874c73fe5234b661fa0a57a57f6ef75b58e03d7d959a": TemplateFixGemma4CloseThinking,
}

// templateFixFor decides a ready model's fix: a known template is fixed
// automatically; an unmeasured template with the same defect is offered and
// applied only after the user turns it on.
func templateFixFor(record ModelRecord, overrides map[string]string) (fix, source, offer string) {
	if known, ok := knownTemplateFixes[record.inspection.ChatTemplateSHA256]; ok && record.inspection.ChatTemplateSHA256 != "" {
		return known, TemplateFixSourceKnown, ""
	}
	candidate := offeredTemplateFix(record.inspection)
	if candidate == "" {
		return "", "", ""
	}
	if overrides[record.ID] == candidate {
		return candidate, TemplateFixSourceUser, ""
	}
	return "", "", candidate
}

// offeredTemplateFix recognizes the defect in a template MagicHandy has not
// measured: a Gemma 4 model with no template at all, or one whose template
// never closes the thought channel.
func offeredTemplateFix(inspection ggufInspection) string {
	if inspection.Architecture != "gemma4" {
		return ""
	}
	if inspection.ChatTemplateSHA256 == "" ||
		(inspection.ChatTemplateGemma4 && !inspection.ChatTemplateClosesThinking) {
		return TemplateFixGemma4CloseThinking
	}
	return ""
}

// applyTemplateFixes annotates ready records with their chat template fix.
func (m *ModelManager) applyTemplateFixes(ctx context.Context, records []ModelRecord) {
	overrides := m.templateFixOverrides(ctx)
	for i := range records {
		if records[i].State != modelStateReady {
			continue
		}
		records[i].TemplateFix, records[i].TemplateFixSource, records[i].TemplateFixOffer = templateFixFor(records[i], overrides)
	}
}

// templateFixOverrides reads the user's per-model choices. A missing or
// damaged document means no choices; it must never hide the model list.
func (m *ModelManager) templateFixOverrides(ctx context.Context) map[string]string {
	overrides := map[string]string{}
	var document string
	err := m.db.SQL().QueryRowContext(ctx, `SELECT value FROM app_kv WHERE key = ?`, templateFixOverridesKey).Scan(&document)
	if err != nil {
		return overrides
	}
	if json.Unmarshal([]byte(document), &overrides) != nil {
		return map[string]string{}
	}
	return overrides
}

// SetTemplateFix turns the offered template fix on or off for one model. A
// fix built in for a known template cannot be turned off.
func (m *ModelManager) SetTemplateFix(ctx context.Context, id string, enabled bool) (ModelRecord, error) {
	m.templateFixMu.Lock()
	defer m.templateFixMu.Unlock()
	record, err := m.Model(ctx, id)
	if err != nil {
		return ModelRecord{}, err
	}
	fix := record.TemplateFixOffer
	if record.TemplateFixSource == TemplateFixSourceUser {
		fix = record.TemplateFix
	}
	if fix == "" {
		return record, ErrNoTemplateFix
	}
	overrides := m.templateFixOverrides(ctx)
	if enabled {
		overrides[record.ID] = fix
	} else {
		delete(overrides, record.ID)
	}
	payload, err := json.Marshal(overrides)
	if err != nil {
		return ModelRecord{}, fmt.Errorf("encode template fix choices: %w", err)
	}
	if _, err := m.db.SQL().ExecContext(ctx, `
		INSERT INTO app_kv(key, value, updated_at)
		VALUES(?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at
	`, templateFixOverridesKey, string(payload), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return ModelRecord{}, modelInventoryError("save template fix choice", err)
	}
	return m.Model(ctx, record.ID)
}

// TemplateFixFile writes the fix's template beneath the model store and
// returns the path the runner loads with --chat-template-file.
func (m *ModelManager) TemplateFixFile(fix string) (string, error) {
	payload, ok := templateFixTemplates[fix]
	if !ok {
		return "", fmt.Errorf("unknown chat template fix %q", fix)
	}
	m.templateFileMu.Lock()
	defer m.templateFileMu.Unlock()
	directory := filepath.Join(filepath.Dir(m.modelsDir), "templates")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", fmt.Errorf("create chat template directory: %w", err)
	}
	path := filepath.Join(directory, fix+".jinja")
	// #nosec G304 -- path is a fixed name beneath the app-owned model store.
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, payload) {
		return path, nil
	}
	temporary, err := os.CreateTemp(directory, fix+"-*.tmp")
	if err != nil {
		return "", fmt.Errorf("stage chat template fix: %w", err)
	}
	staged := temporary.Name()
	_, writeErr := temporary.Write(payload)
	closeErr := temporary.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(staged)
		return "", fmt.Errorf("write chat template fix: %w", errors.Join(writeErr, closeErr))
	}
	if err := os.Rename(staged, path); err != nil {
		_ = os.Remove(staged)
		return "", fmt.Errorf("install chat template fix: %w", err)
	}
	return path, nil
}
