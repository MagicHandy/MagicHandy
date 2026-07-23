package httpapi

import (
	"net/http"
	"path/filepath"
	"strconv"

	diagnosticspkg "github.com/mapledaemon/MagicHandy/internal/diagnostics"
)

func (s *Server) handyMotionLogEnabled() bool {
	settings, _ := s.store.Snapshot()
	return settings.Diagnostics.ShouldLogHandyMotion()
}

func (s *Server) handyMotionLogVerbose() bool {
	settings, _ := s.store.Snapshot()
	return settings.Diagnostics.VerboseHandyMotion()
}

func (s *Server) recordHandyMotionLog(event, source string, entry diagnosticspkg.HandyLogEntry) {
	if !s.handyMotionLogEnabled() {
		return
	}
	entry.Event = event
	if entry.Source == "" {
		entry.Source = source
	}
	if s.handyLog != nil {
		s.handyLog.Add(entry)
	}
	if s.handyMotionLogVerbose() {
		data := map[string]any{
			"event":  event,
			"source": source,
		}
		if entry.PlaybackState != "" {
			data["playback_state"] = entry.PlaybackState
		}
		if entry.BufferAheadMS != nil {
			data["buffer_ahead_ms"] = *entry.BufferAheadMS
		}
		if entry.StreamElapsed != nil {
			data["stream_elapsed_ms"] = *entry.StreamElapsed
		}
		if entry.PositionPct != nil {
			data["position_pct"] = *entry.PositionPct
		}
		if entry.DurationMS != nil {
			data["duration_ms"] = *entry.DurationMS
		}
		if entry.Error != "" {
			data["error"] = entry.Error
		}
		if len(entry.Details) > 0 {
			data["details"] = entry.Details
		}
		agentDebugLog("DEV", "handy_motion_log.go", event, data)
	}
}

func (s *Server) handyLogPath() string {
	return filepath.Join(s.store.DataDir(), "handy-motion-debug.log")
}

func (s *Server) handleHandyLog(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	entries := []diagnosticspkg.HandyLogEntry{}
	if s.handyLog != nil {
		entries = s.handyLog.Tail(limit)
	}
	payload := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		row := map[string]any{
			"ts":    entry.Timestamp,
			"event": entry.Event,
		}
		if entry.Source != "" {
			row["source"] = entry.Source
		}
		if entry.PositionPct != nil {
			row["position_pct"] = *entry.PositionPct
		}
		if entry.DurationMS != nil {
			row["duration_ms"] = *entry.DurationMS
		}
		if entry.Error != "" {
			row["error"] = entry.Error
		}
		if entry.PlaybackState != "" {
			row["playback_state"] = entry.PlaybackState
		}
		if entry.BufferAheadMS != nil {
			row["buffer_ahead_ms"] = *entry.BufferAheadMS
		}
		if entry.StreamElapsed != nil {
			row["stream_elapsed_ms"] = *entry.StreamElapsed
		}
		if len(entry.Details) > 0 {
			row["details"] = entry.Details
		}
		payload = append(payload, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path":    s.handyLogPath(),
		"count":   len(payload),
		"entries": payload,
	})
}

func diagnosticsHandyLogRecent(s *Server) []map[string]any {
	if s.handyLog == nil {
		return nil
	}
	entries := s.handyLog.Tail(80)
	out := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		row := map[string]any{
			"event": entry.Event,
		}
		if entry.Source != "" {
			row["source"] = entry.Source
		}
		if entry.PositionPct != nil {
			row["position_pct"] = *entry.PositionPct
		}
		if entry.DurationMS != nil {
			row["duration_ms"] = *entry.DurationMS
		}
		if entry.Error != "" {
			row["error"] = entry.Error
		}
		if entry.PlaybackState != "" {
			row["playback_state"] = entry.PlaybackState
		}
		if entry.BufferAheadMS != nil {
			row["buffer_ahead_ms"] = *entry.BufferAheadMS
		}
		if len(entry.Details) > 0 {
			for key, value := range entry.Details {
				row[key] = value
			}
		}
		out = append(out, row)
	}
	return out
}

func logChatAutoPoseDecision(
	s *Server,
	source string,
	llmPose string,
	afterStamina string,
	afterRamp string,
	staminaBefore, staminaAfter float64,
) {
	if !s.handyMotionLogEnabled() {
		return
	}
	s.recordHandyMotionLog("chat_auto_pose", source, diagnosticspkg.HandyLogEntry{
		Details: map[string]any{
			"posicao_llm":       llmPose,
			"posicao_stamina":   afterStamina,
			"posicao_ramp":      afterRamp,
			"stamina_before":    staminaBefore,
			"stamina_after":     staminaAfter,
			"pose_rotated":      llmPose != afterStamina,
			"pose_ramp_changed": afterStamina != afterRamp,
		},
	})
}
