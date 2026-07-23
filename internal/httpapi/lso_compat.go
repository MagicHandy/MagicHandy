package httpapi

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/manualqueue"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

type chatMessageRecord struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp,omitempty"`
}

type lsoCompatRuntime struct {
	mu                 sync.RWMutex
	messages           []chatMessageRecord
	lastPersonaMessage string
}

func (s *Server) lsoPlaybackActive() bool {
	if s.chatAutoActive() || s.freestyleChaosActive() || s.chatChaosActive() {
		return true
	}
	if engine := s.currentMotionEngine(); engine != nil {
		return engine.Snapshot().Running
	}
	return false
}

func (s *Server) lsoManualQueueStatus() (mqCount int, mqPlaying, mqPaused, mqAutoloop bool, playerSnap manualqueue.Snapshot) {
	s.manualQueue.mu.Lock()
	mqCount = len(s.manualQueue.items)
	mqPlaying = s.manualQueue.playing
	mqPaused = s.manualQueue.paused
	mqAutoloop = s.manualQueue.autoloop
	s.manualQueue.mu.Unlock()

	playerSnap, _ = s.manualQueuePlayerSnapshot()
	return mqCount, mqPlaying, mqPaused, mqAutoloop, playerSnap
}

func (s *Server) lsoLLMStatus(settings config.Settings) (connected bool, errVal any) {
	provider, err := s.newLLMProvider(settings.LLM)
	if err != nil {
		return false, err.Error()
	}
	llmStatus := provider.Status(context.Background())
	if llmStatus.Managed && !llmStatus.Loaded {
		if llmStatus.Message != "" && llmStatus.Message != "llama.cpp runner is not loaded" {
			return false, llmStatus.Message
		}
		return true, nil
	}
	if !llmStatus.Available && llmStatus.Message != "" {
		return llmStatus.Available, llmStatus.Message
	}
	return llmStatus.Available, nil
}

func (s *Server) lsoLLMPayload(settings config.Settings) map[string]any {
	provider, err := s.newLLMProvider(settings.LLM)
	connected, errVal := s.lsoLLMStatus(settings)
	payload := map[string]any{
		"provider":       settings.LLM.Provider,
		"connected":      connected,
		"error":          errVal,
		"model":          settings.LLM.Model,
		"base_url":       selectedLLMBaseURL(settings.LLM),
		"llama_cpp_mode": settings.LLM.LlamaCPPMode,
		"managed":        settings.LLM.LlamaCPPMode == config.LlamaCPPModeManaged,
		"loaded":         false,
	}
	if err == nil {
		status := provider.Status(context.Background())
		payload["loaded"] = status.Loaded
		payload["available"] = status.Available
	}
	return payload
}

func (s *Server) lsoPersonaStatus() (personaID string, personaName string, personaAvatar any) {
	personaRow, personaID, _ := s.activePersonaSnapshot()
	personaName = "MagicHandy"
	if personaRow.ID != "" {
		personaName = personaRow.Name
		personaAvatar = s.personaAvatarURLFor(personaID)
	}
	return personaID, personaName, personaAvatar
}

func (s *Server) lsoDirectActive() bool {
	s.direct.mu.Lock()
	active := s.direct.active
	s.direct.mu.Unlock()
	return active
}

func (s *Server) lsoVisualSyncPrefs() (syncOffset int64, measuredRTT any) {
	s.visual.mu.Lock()
	syncOffset = int64(s.visual.syncPrefs.OffsetMS)
	if s.visual.syncPrefs.MeasuredRTTMS != nil {
		measuredRTT = *s.visual.syncPrefs.MeasuredRTTMS
	}
	s.visual.mu.Unlock()
	return syncOffset, measuredRTT
}

func (s *Server) lsoMotionPosition(playerSnap manualqueue.Snapshot) float64 {
	motionPos := 50.0
	if playerSnap.Running {
		motionPos = playerSnap.PositionPct
	} else if engine := s.currentMotionEngine(); engine != nil {
		if snap := engine.Snapshot(); snap.LastSample != nil {
			motionPos = float64(snap.LastSample.PositionPercent)
		}
	}
	s.direct.mu.Lock()
	if s.direct.lastPositionPct > 0 {
		motionPos = s.direct.lastPositionPct
	}
	s.direct.mu.Unlock()
	return motionPos
}

func (s *Server) lsoStatusPayload() map[string]any {
	settings, _ := s.store.Snapshot()
	intiface := s.intifaceDiagnostics()
	cloud := s.cloudDiagnostics()
	deviceState := resolveLsoDeviceStatus(settings, intiface, cloud, s.cloud.baseURL)

	playbackActive := s.lsoPlaybackActive()
	mqCount, mqPlaying, mqPaused, mqAutoloop, playerSnap := s.lsoManualQueueStatus()
	if proceduralSnap, ok := s.activeProceduralPlayerSnapshot(); ok {
		playerSnap = proceduralSnap
		mqPlaying = true
	}
	llmConnected, llmError := s.lsoLLMStatus(settings)
	llm := s.lsoLLMPayload(settings)
	appState, _ := s.store.DB().LoadAppState()
	personaID, personaName, personaAvatar := s.lsoPersonaStatus()
	directActive := s.lsoDirectActive()
	syncOffset, measuredRTT := s.lsoVisualSyncPrefs()
	motionPos := s.lsoMotionPosition(playerSnap)

	phase := "idle"
	activePose := ""
	if s.chatAutoActive() {
		autoState := s.chatAutoStateSnapshot()
		phase = "auto"
		activePose = string(autoState.Posicao)
	} else if mqPlaying {
		phase = "motion"
	}

	payload := map[string]any{
		"service":                            serviceName,
		"version":                            s.version.Version,
		"device_transport":                   mapOwnerToDeviceTransport(settings.Device.HSPDispatchOwner),
		"intiface_connected":                 intiface.Connected,
		"intiface_url":                       settings.Device.IntifaceURL,
		"intiface_error":                     intiface.Error,
		"device_label":                       deviceState.Label,
		"device_connected":                   deviceState.Connected,
		"handy_connected":                    deviceState.HandyConnected,
		"handy_error":                        deviceState.HandyError,
		"handy_base_url":                     deviceState.HandyBaseURL,
		"handy_key_configured":               settings.Device.HandyConnectionKey != "",
		"use_mock":                           false,
		"persona_id":                         personaID,
		"persona_name":                       personaName,
		"persona_avatar_url":                 personaAvatar,
		"ollama_connected":                   llmConnected,
		"ollama_error":                       llmError,
		"ollama_model":                       settings.LLM.Model,
		"ollama_url":                         selectedLLMBaseURL(settings.LLM),
		"llm":                                llm,
		"llm_provider":                       settings.LLM.Provider,
		"llm_connected":                      llmConnected,
		"llm_error":                          llmError,
		"llm_model":                          settings.LLM.Model,
		"llm_base_url":                       selectedLLMBaseURL(settings.LLM),
		"llm_cpp_mode":                       settings.LLM.LlamaCPPMode,
		"operation_mode":                     appState.OperationMode,
		"intensity":                          50,
		"min_position":                       settings.Motion.StrokeMinPercent,
		"max_position":                       settings.Motion.StrokeMaxPercent,
		"buffer_sec":                         0,
		"queue_preview":                      []any{},
		"phase":                              phase,
		"active_pose":                        activePose,
		"playback_active":                    playbackActive || mqPlaying,
		"direct_control_active":              directActive,
		"sync_offset_ms":                     syncOffset,
		"measured_rtt_ms":                    measuredRTT,
		"motion_position_pct":                motionPos,
		"emergency_stop":                     s.emergencyStopActive(),
		"chat_pending":                       false,
		"planner_busy":                       false,
		"auto_running":                       s.chatAutoActive(),
		"user_session_engaged":               len(s.lsoCompat.messages) > 0,
		"chat_auto":                          s.chatAutoPublicState(),
		"manual_queue_count":                 mqCount,
		"manual_queue_playing":               mqPlaying,
		"manual_queue_paused":                mqPaused,
		"manual_queue_autoloop":              mqAutoloop,
		"manual_queue_playhead_ms":           playerSnap.PlayheadMS,
		"manual_queue_duration_sec":          float64(playerSnap.DurationMS) / 1000.0,
		"manual_queue_progress_pct":          manualQueueProgressPct(playerSnap),
		"manual_queue_current_segment_index": playerSnap.CurrentSegment,
		"manual_queue_playback_mode":         manualQueuePlaybackMode(playerSnap),
		"footer_status":                      "MagicHandy unified",
		"ui":                                 "embedded",
		"features": map[string]string{
			"motion":   "manual",
			"chat":     "llama.cpp",
			"library":  "sqlite",
			"intiface": deviceState.IntifaceFeature,
		},
	}
	return payload
}

type lsoDeviceState struct {
	Connected       bool
	Label           string
	HandyConnected  bool
	HandyError      any
	HandyBaseURL    string
	IntifaceFeature string
}

func resolveLsoDeviceStatus(
	settings config.Settings,
	intiface intifaceDiag,
	cloud transport.TransportDiagnostics,
	cloudBaseURL string,
) lsoDeviceState {
	switch settings.Device.HSPDispatchOwner {
	case config.DispatchOwnerCloudREST:
		state := lsoDeviceState{
			Connected:       cloud.Connected,
			HandyConnected:  cloud.Connected,
			HandyBaseURL:    cloudBaseURL,
			IntifaceFeature: "optional",
			Label:           "Handy Cloud API",
		}
		if cloud.LastError != "" {
			state.HandyError = cloud.LastError
		} else if settings.Device.HandyConnectionKey == "" {
			state.HandyError = "connection key not configured"
		}
		return state
	case config.DispatchOwnerIntiface:
		return lsoDeviceState{
			Connected:       intiface.DeviceReady,
			Label:           intiface.DeviceLabel,
			IntifaceFeature: "required",
		}
	default:
		return lsoDeviceState{
			Connected:       intiface.DeviceReady,
			Label:           intiface.DeviceLabel,
			IntifaceFeature: "optional",
		}
	}
}

type intifaceDiag struct {
	Connected   bool
	DeviceReady bool
	Error       any
	DeviceLabel string
}

func (s *Server) intifaceDiagnostics() intifaceDiag {
	client := s.intifaceClient()
	if client == nil {
		return intifaceDiag{}
	}
	errVal := any(nil)
	if last := client.LastError(); last != "" {
		errVal = last
	}
	ready := client.Connected() && client.SelectedDeviceID() != ""
	return intifaceDiag{
		Connected:   client.Connected(),
		DeviceReady: ready,
		Error:       errVal,
		DeviceLabel: client.SelectedDeviceName(),
	}
}

func (s *Server) handleChatMessages(w http.ResponseWriter, _ *http.Request) {
	s.lsoCompat.mu.RLock()
	defer s.lsoCompat.mu.RUnlock()
	msgs := make([]chatMessageRecord, len(s.lsoCompat.messages))
	copy(msgs, s.lsoCompat.messages)
	writeJSON(w, http.StatusOK, map[string]any{
		"messages":             msgs,
		"last_persona_message": s.lsoCompat.lastPersonaMessage,
	})
}

type chatSendRequest struct {
	Text string `json:"text"`
}

func (s *Server) handleChatSend(w http.ResponseWriter, r *http.Request) {
	var body chatSendRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	text := trimSpace(body.Text)
	// #region agent log
	agentDebugLog("D", "lso_compat.go:handleChatSend", "chat send received", map[string]any{
		"textLen": len(text), "clientID": clientIDFromRequest(r),
	})
	// #endregion
	if text == "" {
		writeError(w, http.StatusBadRequest, errEmptyChatMessage)
		return
	}
	if isChatStopMessage(text) {
		if !s.requireController(w, r) {
			return
		}
		if s.modes != nil {
			s.modes.NotifyChatStop()
			s.modes.NotifyUserStop()
		}
		s.stopChatAutoLoop(r.Context())
		s.cancelChatChaosMotion(r.Context())
		s.stopAndClearMotionEngine(r.Context(), "chat_stop")
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "stopped": true})
		return
	}

	s.appendChatMessage("user", text)

	if s.operationModeAuto() {
		if _, err := s.ensureLLMReady(r.Context()); err != nil {
			writeError(w, http.StatusServiceUnavailable, err)
			return
		}
		if s.chatAutoActive() {
			s.enqueueChatAutoUserText(text)
		} else {
			s.startChatAutoLoop(r.Context())
			s.enqueueChatAutoUserText(text)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"pending": true,
			"reply":   "",
			"stopped": false,
		})
		return
	}

	settings, _ := s.store.Snapshot()
	provider, err := s.ensureLLMReady(r.Context())
	// #region agent log
	agentDebugLog("A", "lso_compat.go:handleChatSend", "ensureLLMReady", map[string]any{
		"ok": err == nil, "err": errString(err),
		"provider": settings.LLM.Provider, "mode": settings.LLM.LlamaCPPMode,
		"runnerPath": settings.LLM.LlamaCPPRunnerPath, "modelPath": settings.LLM.LlamaCPPModelPath,
	})
	// #endregion
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	prompt := s.chatPromptForRequest(settings)
	service := chat.Service{
		Provider:             provider,
		Prompt:               prompt,
		Model:                settings.LLM.Model,
		Memories:             s.personalization.memory.PromptTexts(),
		UserProfile:          settings.UserProfile,
		MotionGenerationMode: settings.Motion.MotionGenerationMode,
	}

	history := s.chatHistoryForLLM()
	result, err := service.Complete(r.Context(), chat.Request{
		Message: text,
		History: history,
	}, nil)
	// #region agent log
	agentDebugLog("B", "lso_compat.go:handleChatSend", "chat complete", map[string]any{
		"ok": err == nil, "err": errString(err),
		"malformed": result.Malformed, "malformedErr": result.MalformedError,
		"repaired": result.Repaired, "rawLen": len(result.Raw), "repairRawLen": len(result.RepairRaw),
		"rawPreview": truncateForLog(result.Raw, 200),
	})
	// #endregion
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	if result.Malformed {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":        false,
			"error":     result.MalformedError,
			"malformed": true,
		})
		return
	}

	reply := result.Response.Reply
	s.appendChatMessage("assistant", reply)
	s.lsoCompat.mu.Lock()
	s.lsoCompat.lastPersonaMessage = reply
	s.lsoCompat.mu.Unlock()

	clientID := clientIDFromRequest(r)
	controllerActive := s.controller.Touch(clientID).Active
	var motionDispatch chatMotionDispatch
	var motionErr error
	if controllerActive {
		motionDispatch, motionErr = s.dispatchChatMotionForResult(r.Context(), result.Response.Motion, settings)
	} else if result.Response.Motion != nil && result.Response.Motion.Action != "" && result.Response.Motion.Action != chat.MotionActionNone {
		motionDispatch = chatMotionDispatch{
			Action: result.Response.Motion.Action,
			Error:  "this client is read-only; motion was not applied",
		}
	}
	// #region agent log
	agentDebugLog("C", "lso_compat.go:handleChatSend", "motion dispatch", map[string]any{
		"clientID": clientID, "controllerActive": controllerActive,
		"action": motionActionName(result.Response.Motion), "applied": motionDispatch.Applied,
		"motionErr": errString(motionErr), "procedural": config.UsesProceduralMotionGeneration(settings.Motion.MotionGenerationMode),
	})
	// #endregion
	response := map[string]any{
		"ok":      true,
		"reply":   reply,
		"pending": false,
		"stopped": false,
		"motion":  result.Response.Motion,
	}
	if motionDispatch.Applied || motionDispatch.Action != "" {
		response["motion_dispatch"] = motionDispatch
	}
	if motionErr != nil {
		response["motion_error"] = motionErr.Error()
		s.logger.Warn("chat motion dispatch failed", "error", motionErr, "action", result.Response.Motion)
	}
	if s.operationModeAuto() {
		s.startChatAutoLoop(r.Context())
	} else {
		s.stopChatAutoLoop(r.Context())
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) appendChatMessage(role, content string) {
	trimmed := strings.TrimSpace(content)
	if role == "assistant" && (strings.HasPrefix(trimmed, "{") || strings.Contains(trimmed, `"motion_`)) {
		// #region agent log
		agentDebugLog("CA3", "lso_compat.go:appendChatMessage", "suspicious_assistant_content", map[string]any{
			"preview": truncateForLog(trimmed, 250), "len": len(trimmed),
		})
		// #endregion
	}
	s.lsoCompat.mu.Lock()
	defer s.lsoCompat.mu.Unlock()
	s.lsoCompat.messages = append(s.lsoCompat.messages, chatMessageRecord{
		Role:      role,
		Content:   content,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if len(s.lsoCompat.messages) > 200 {
		s.lsoCompat.messages = s.lsoCompat.messages[len(s.lsoCompat.messages)-200:]
	}
}

func (s *Server) chatHistoryForLLM() []llm.Message {
	s.lsoCompat.mu.RLock()
	defer s.lsoCompat.mu.RUnlock()
	history := make([]llm.Message, 0, len(s.lsoCompat.messages))
	for _, msg := range s.lsoCompat.messages {
		history = append(history, llm.Message{Role: msg.Role, Content: msg.Content})
	}
	return history
}

var errEmptyChatMessage = errorString("chat message is required")

type errorString string

func (e errorString) Error() string { return string(e) }

func trimSpace(value string) string {
	for len(value) > 0 && (value[0] == ' ' || value[0] == '\t' || value[0] == '\n') {
		value = value[1:]
	}
	for len(value) > 0 {
		last := value[len(value)-1]
		if last != ' ' && last != '\t' && last != '\n' {
			break
		}
		value = value[:len(value)-1]
	}
	return value
}

func manualQueueProgressPct(snap manualqueue.Snapshot) float64 {
	if snap.DurationMS <= 0 {
		return 0
	}
	pct := float64(snap.PlayheadMS) * 100 / float64(snap.DurationMS)
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

func manualQueuePlaybackMode(snap manualqueue.Snapshot) any {
	if !snap.Running {
		return nil
	}
	return "script"
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func truncateForLog(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "..."
}

func motionActionName(command *chat.MotionCommand) string {
	if command == nil {
		return ""
	}
	return command.Action
}
