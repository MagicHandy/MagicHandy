package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func (s *Server) emitChatCompletionError(ctx context.Context, stop uint64, emit sseEmitter, result chat.Result, failure error, diagnostics chat.MessageDiagnostics) bool {
	if s.chatCanceled(ctx, stop) {
		s.emitChatCancellationAt(ctx, stop, emit, false)
		return true
	}
	if failure == nil {
		return false
	}
	s.logger.Warn("chat stream failed", "provider", diagnostics.Provider, "error", failure)
	payload := map[string]any{"message": failure.Error()}
	if message := s.chatInterruptionMessage(ctx, stop, failure); message != "" {
		payload["message"] = message
	}
	if repair := s.offerChatRepair(ctx, result, diagnostics); repair != nil {
		payload["repair"] = repair
	}
	_ = emit("error", payload)
	_ = emit("done", map[string]any{"ok": false})
	return true
}

func (s *Server) retainChatFailure(ctx context.Context, sequence uint64, sessionID string, result chat.Result, failure error, diagnostics chat.MessageDiagnostics) (chat.Result, error) {
	if failure == nil && !result.Malformed && !result.RepairAttempted {
		return result, failure
	}
	if !result.RepairAttempted && s.chatCanceled(ctx, sequence) && failure == nil {
		return result, failure
	}
	// Some providers return an SDK/transport error when their context ends.
	// Retain the lifecycle cause before detaching persistence, so diagnostics
	// and the public explanation classify the same interrupted request.
	if cause := context.Cause(ctx); failure != nil && cause != nil && !errors.Is(failure, cause) {
		failure = errors.Join(failure, cause)
	}
	outcome := ""
	if result.RepairAttempted {
		outcome = s.chatRepairCancellationOutcome(ctx, sequence, failure)
		// Diagnostics cannot publish a reply or touch motion. Keep the rejected
		// attempt after cancellation, under recordChatFailure's short deadline.
		ctx = context.WithoutCancel(ctx)
	}
	// Keep the technical cancellation evidence after its request has ended.
	// Persistence remains bounded and cannot resurrect a deleted session.
	if failure != nil && s.chatInterruptionMessage(ctx, sequence, failure) != "" {
		ctx = context.WithoutCancel(ctx)
	}
	id, message := s.recordChatFailure(ctx, sessionID, result, failure, diagnostics, outcome)
	result.FailureID = id
	if candidate, ok := ctx.Value(chatRepairContextKey{}).(*chatRepairCandidate); ok && result.RepairAttempted {
		candidate.evidenceID.Store(id)
	}
	result.MalformedError = message
	if failure != nil {
		failure = retainedChatFailure{message: message, cause: failure}
	}
	return result, failure
}

func (s *Server) chatFailureEvidence(result chat.Result, failure error, diagnostics chat.MessageDiagnostics, settings config.LLMSettings) chat.RequestFailure {
	message := result.MalformedError
	if failure != nil {
		message = failure.Error()
	}
	item := chat.RequestFailure{UserSeq: result.UserSeq, CreatedAt: time.Now().UTC().Format(time.RFC3339), Error: message, Diagnostics: diagnostics, MotionMode: settings.MotionGenerationMode,
		ReasoningMode: settings.ReasoningMode, Version: s.version.Version, Commit: s.version.Commit,
		Details: &chat.FailureContent{Raw: result.Raw, RepairRaw: result.RepairRaw}}
	if errors.Is(failure, context.Canceled) || errors.Is(failure, context.DeadlineExceeded) {
		item.Kind = "interrupted"
	}
	if result.RepairAttempted {
		item.Kind = "response_repair"
		item.RepairOutcome = "failed"
		item.Diagnostics.Repaired = result.Repaired
		item.Diagnostics.InitialMalformed = true
		item.Details.InitialError = result.InitialError
		if result.Repaired {
			item.RepairOutcome = "repaired"
			item.Error = result.InitialError
		}
	}
	if errors.Is(failure, llm.ErrOutputTruncated) {
		item.Kind = "output_limit"
	}
	if result.Input != nil {
		item.MaxTokens, item.ReasoningMode = result.Input.MaxTokens, result.Input.ReasoningMode
		item.Details.Schema = string(result.Input.JSONSchema)
		for _, message := range result.Input.Messages {
			if message.Role == "system" || message.Role == "developer" {
				if item.Details.Prompt != "" {
					item.Details.Prompt += "\n\n"
				}
				item.Details.Prompt += message.Content
			}
		}
	}
	return item
}

func (s *Server) recordChatFailure(ctx context.Context, sessionID string, result chat.Result, failure error, diagnostics chat.MessageDiagnostics, outcome string) (int64, string) {
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	settings, _ := s.store.Snapshot()
	modelSettings := settings.LLM.ConversationSettings()
	if candidate, ok := ctx.Value(chatRepairContextKey{}).(*chatRepairCandidate); ok {
		modelSettings = candidate.settings
	}
	item := s.chatFailureEvidence(result, failure, diagnostics, modelSettings)
	if outcome != "" {
		item.RepairOutcome = outcome
	}
	if err := s.prepareChatFailure(ctx, &item); err != nil {
		s.logger.Warn("chat failure diagnostics unavailable")
		return 0, "Model request failed; diagnostics are unavailable."
	}
	id, err := s.chatLog.SaveFailure(ctx, sessionID, item)
	if err != nil {
		s.logger.Warn("chat failure diagnostics could not be saved")
	}
	return id, item.Error
}

// Redact complete values before truncation so a credential crossing the size
// boundary can never leave a retained prefix behind.
func (s *Server) prepareChatFailure(ctx context.Context, item *chat.RequestFailure) error {
	if err := s.redactChatFailure(ctx, item); err != nil {
		return err
	}
	item.Error = boundedFailureText(item.Error, 2048, nil)
	if item.Details != nil {
		item.Details.Raw = boundedFailureText(item.Details.Raw, 16384, &item.Details.Truncated)
		item.Details.RepairRaw = boundedFailureText(item.Details.RepairRaw, 16384, &item.Details.Truncated)
		item.Details.InitialError = boundedFailureText(item.Details.InitialError, 2048, &item.Details.Truncated)
		item.Details.Prompt = boundedFailureText(item.Details.Prompt, 32768, &item.Details.Truncated)
		item.Details.Schema = boundedFailureText(item.Details.Schema, 65536, &item.Details.Truncated)
	}
	return nil
}

func boundedFailureText(value string, limit int, truncated *bool) string {
	if len(value) <= limit {
		return value
	}
	if truncated != nil {
		*truncated = true
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func (s *Server) redactChatFailure(ctx context.Context, item *chat.RequestFailure) error {
	d := &item.Diagnostics
	fields := []*string{&item.Error, &item.Version, &item.Commit, &item.MotionMode, &item.ReasoningMode,
		&d.Provider, &d.Model, &d.PromptSet, &d.PersonaID, &d.PersonaName, &d.RefusalFallbackFrom, &d.RefusalFallbackModel}
	if item.Details != nil {
		fields = append(fields, &item.Details.Raw, &item.Details.RepairRaw, &item.Details.InitialError, &item.Details.Prompt, &item.Details.Schema)
	}
	values := make([]string, len(fields))
	for index, field := range fields {
		values[index] = *field
	}
	if s.cloudPlanning.auth != nil {
		var err error
		values, err = s.cloudPlanning.auth.RedactDiagnostics(ctx, values)
		if err != nil {
			return err
		}
	}
	redactor := s.setup.setupReportRedactor()
	for index, value := range values {
		*fields[index] = redactor(value)
	}
	return nil
}

func (s *Server) handleChatFailures(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, errors.New("a chat session is required"))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.PathValue("id") == "" {
		items, err := s.chatLog.Failures(ctx, sessionID)
		if err != nil {
			s.writeChatStorageError(w, err)
			return
		}
		for index := range items {
			if err := s.redactChatFailure(ctx, &items[index]); err != nil {
				s.writeChatStorageError(w, err)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"failures": items})
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("invalid failed request"))
		return
	}
	item, err := s.chatLog.Failure(ctx, sessionID, id, r.URL.Query().Get("include_content") == "true")
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, errors.New("this failed request is no longer retained"))
		return
	}
	if err != nil {
		s.writeChatStorageError(w, err)
		return
	}
	if err := s.redactChatFailure(ctx, &item); err != nil {
		s.writeChatStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}
