package httpapi

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Model check states.
const (
	modelCheckIdle     = "idle"
	modelCheckRunning  = "running"
	modelCheckComplete = "complete"
	modelCheckFailed   = "failed"
	modelCheckCanceled = "canceled"
)

// Model check items the host decides; the scripted turns decide the rest.
const (
	modelCheckItemLoad     = "load"
	modelCheckItemGPU      = "gpu"
	modelCheckItemTemplate = "template"
	modelCheckItemRuntime  = "runtime"
)

// modelCheckReport is the backend-authoritative model check result. It
// carries verdicts and numbers; the browser words them in the user's locale.
type modelCheckReport struct {
	State       string                 `json:"state"`
	Step        string                 `json:"step,omitempty"`
	Error       string                 `json:"error,omitempty"`
	StartedAt   string                 `json:"started_at,omitempty"`
	FinishedAt  string                 `json:"finished_at,omitempty"`
	Provider    string                 `json:"provider,omitempty"`
	Model       string                 `json:"model,omitempty"`
	Managed     bool                   `json:"managed"`
	ReplyLength string                 `json:"reply_length,omitempty"`
	LoadMillis  int64                  `json:"load_millis"`
	LoadReport  llm.ManagedLoadReport  `json:"load_report"`
	Runtime     *modelCheckRuntime     `json:"runtime,omitempty"`
	Template    *modelCheckTemplate    `json:"template,omitempty"`
	Items       []chat.ModelCheckItem  `json:"items"`
	Turns       []chat.ModelCheckTurn  `json:"turns"`
	Summary     chat.ModelCheckSummary `json:"summary"`
}

type modelCheckRuntime struct {
	Version  string `json:"version,omitempty"`
	Expected string `json:"expected"`
	Current  bool   `json:"current"`
}

// modelCheckTemplate reports the managed model's chat template fix state and,
// through ModelID, which model an offered fix would change.
type modelCheckTemplate struct {
	ModelID      string `json:"model_id"`
	Architecture string `json:"architecture,omitempty"`
	Fix          string `json:"fix,omitempty"`
	FixSource    string `json:"fix_source,omitempty"`
	FixOffer     string `json:"fix_offer,omitempty"`
}

// modelCheckRuntimeState owns the one model check a server runs at a time.
type modelCheckRuntimeState struct {
	mu     sync.Mutex
	report modelCheckReport
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func (m *modelCheckRuntimeState) snapshot() modelCheckReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	report := m.report
	if report.State == "" {
		report.State = modelCheckIdle
	}
	report.Items = append([]chat.ModelCheckItem{}, report.Items...)
	report.Turns = append([]chat.ModelCheckTurn{}, report.Turns...)
	return report
}

func (m *modelCheckRuntimeState) update(change func(*modelCheckReport)) {
	m.mu.Lock()
	change(&m.report)
	m.mu.Unlock()
}

func (s *Server) handleModelCheck(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.modelCheck.snapshot())
}

// handleStartModelCheck runs the model check in the background. It is only
// ever started by the user; nothing starts it automatically.
func (s *Server) handleStartModelCheck(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	settings, _ := s.store.Snapshot()
	s.modelCheck.mu.Lock()
	if s.modelCheck.report.State == modelCheckRunning {
		s.modelCheck.mu.Unlock()
		writeError(w, http.StatusConflict, errors.New("a model check is already running"))
		return
	}
	ctx, cancel := context.WithCancel(s.lifecycleCtx)
	s.modelCheck.cancel = cancel
	s.modelCheck.report = modelCheckReport{
		State:       modelCheckRunning,
		Step:        modelCheckItemLoad,
		StartedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Provider:    settings.LLM.Provider,
		Model:       settings.LLM.Model,
		Managed:     managedLLMSelected(settings.LLM),
		ReplyLength: settings.LLM.ReplyLength,
	}
	s.modelCheck.wg.Add(1)
	s.modelCheck.mu.Unlock()
	go func() {
		defer s.modelCheck.wg.Done()
		defer cancel()
		s.runModelCheck(ctx, settings)
	}()
	writeJSON(w, http.StatusAccepted, s.modelCheck.snapshot())
}

func (s *Server) handleCancelModelCheck(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	s.modelCheck.mu.Lock()
	if cancel := s.modelCheck.cancel; cancel != nil && s.modelCheck.report.State == modelCheckRunning {
		cancel()
	}
	s.modelCheck.mu.Unlock()
	writeJSON(w, http.StatusOK, s.modelCheck.snapshot())
}

// stopModelCheck cancels a running check and waits for it during shutdown.
func (s *Server) stopModelCheck() {
	s.modelCheck.mu.Lock()
	if cancel := s.modelCheck.cancel; cancel != nil {
		cancel()
	}
	s.modelCheck.mu.Unlock()
	s.modelCheck.wg.Wait()
}

func managedLLMSelected(settings config.LLMSettings) bool {
	return settings.Provider == config.LLMProviderLlamaCPP && settings.LlamaCPPMode == config.LlamaCPPModeManaged
}

func (s *Server) runModelCheck(ctx context.Context, settings config.Settings) {
	err := s.executeModelCheck(ctx, settings)
	s.modelCheck.update(func(report *modelCheckReport) {
		report.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		report.Step = ""
		switch {
		case err == nil:
			report.State = modelCheckComplete
		case errors.Is(err, context.Canceled) || ctx.Err() != nil:
			report.State = modelCheckCanceled
		default:
			report.State = modelCheckFailed
			report.Error = err.Error()
			if !hasModelCheckItem(report.Items, modelCheckItemLoad) {
				report.Items = append([]chat.ModelCheckItem{{ID: modelCheckItemLoad, Status: chat.ModelCheckFail}}, report.Items...)
			}
		}
	})
}

func hasModelCheckItem(items []chat.ModelCheckItem, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

// executeModelCheck loads the selected model, then sends the scripted turns
// through the same chat service a real turn uses. It holds one interactive
// LLM slot for the whole run, so Stop cancels it like any chat turn, and it
// never hands a reply's motion to the engine.
func (s *Server) executeModelCheck(ctx context.Context, settings config.Settings) error {
	providerCtx, _, release, err := s.llmRequests.acquire(ctx, llmRequestInteractive)
	if err != nil {
		return err
	}
	defer release()
	provider, err := s.newLLMProvider(providerCtx, settings.LLM)
	if err != nil {
		return err
	}
	if err := s.loadModelForCheck(providerCtx, provider); err != nil {
		return err
	}
	s.recordModelCheckHost(providerCtx, provider, settings.LLM)
	service, err := s.modelCheckService(providerCtx, provider, settings)
	if err != nil {
		return err
	}
	s.modelCheck.update(func(report *modelCheckReport) { report.Step = "turns" })
	turns, err := chat.RunModelCheckTurns(providerCtx, service, func(turn chat.ModelCheckTurn) {
		s.modelCheck.update(func(report *modelCheckReport) { report.Turns = append(report.Turns, turn) })
	})
	if err != nil {
		return err
	}
	summary := chat.SummarizeModelCheckTurns(turns)
	capabilities := chatCapabilities(settings.LLM, nil)
	items := chat.JudgeModelCheckTurns(summary, chat.ModelCheckJudgement{
		MaxAverageWords: chat.ModelCheckMaxAverageWords(capabilities.ReplyLength),
		AdultVoice:      capabilities.Voice == chat.VoiceIntimate || capabilities.Voice == chat.VoiceExplicit,
	})
	s.modelCheck.update(func(report *modelCheckReport) {
		report.Summary = summary
		report.Items = append(report.Items, items...)
	})
	return nil
}

func (s *Server) loadModelForCheck(ctx context.Context, provider llm.Provider) error {
	started := time.Now()
	if loadable, ok := provider.(llm.LoadableProvider); ok {
		status := loadable.Load(ctx)
		if !status.Available {
			if err := ctx.Err(); err != nil {
				return err
			}
			return errors.New(status.Message)
		}
	} else if status := provider.Status(ctx); !status.Available {
		// A reachable server that cannot find the model still fails each turn
		// with the server's own error, which the turns then report.
		if err := ctx.Err(); err != nil {
			return err
		}
		return errors.New(firstNonBlank(status.Message, "the selected model is not available"))
	}
	s.modelCheck.update(func(report *modelCheckReport) {
		report.LoadMillis = time.Since(started).Milliseconds()
		report.Items = append(report.Items, chat.ModelCheckItem{ID: modelCheckItemLoad, Status: chat.ModelCheckPass})
	})
	return nil
}

// recordModelCheckHost adds what only the host knows about a managed model:
// GPU placement, the chat template fix, and whether the runtime is current.
func (s *Server) recordModelCheckHost(ctx context.Context, provider llm.Provider, settings config.LLMSettings) {
	managed, ok := provider.(*llm.ManagedLlamaCPPProvider)
	if !ok || !managedLLMSelected(settings) {
		return
	}
	loadReport := managed.LoadReport()
	runtimeStatus := s.managedLLM.Snapshot().Runtime
	record, recordErr := s.models.Model(ctx, settings.Model)
	s.modelCheck.update(func(report *modelCheckReport) {
		report.LoadReport = loadReport
		report.Items = append(report.Items, chat.ModelCheckItem{ID: modelCheckItemGPU, Status: gpuVerdict(loadReport)})
		if recordErr == nil {
			report.Template = &modelCheckTemplate{
				ModelID: record.ID, Architecture: record.Architecture,
				Fix: record.TemplateFix, FixSource: record.TemplateFixSource, FixOffer: record.TemplateFixOffer,
			}
			templateStatus := chat.ModelCheckPass
			if record.TemplateFixOffer != "" {
				templateStatus = chat.ModelCheckWarn
			}
			report.Items = append(report.Items, chat.ModelCheckItem{ID: modelCheckItemTemplate, Status: templateStatus})
		}
		report.Runtime = &modelCheckRuntime{Version: runtimeStatus.Version, Expected: runtimeStatus.ExpectedVersion, Current: runtimeStatus.Current}
		runtimeVerdict := chat.ModelCheckPass
		if !runtimeStatus.Current {
			runtimeVerdict = chat.ModelCheckWarn
		}
		report.Items = append(report.Items, chat.ModelCheckItem{ID: modelCheckItemRuntime, Status: runtimeVerdict})
	})
}

func gpuVerdict(report llm.ManagedLoadReport) string {
	switch {
	case report.TotalLayers == 0:
		return chat.ModelCheckSkip
	case report.FullyOffloaded():
		return chat.ModelCheckPass
	default:
		return chat.ModelCheckWarn
	}
}

// modelCheckService configures the chat service exactly as a chat turn does,
// minus the parts that belong to one conversation: no persona, no memories,
// no history beyond the check's own turns, and a stopped motion context so
// the result does not depend on what the device is doing.
func (s *Server) modelCheckService(ctx context.Context, provider llm.Provider, settings config.Settings) (chat.Service, error) {
	prompt, ok, err := s.personalization.prompts.ResolveContext(ctx, settings.LLM.PromptSet)
	if err != nil {
		return chat.Service{}, err
	}
	if !ok {
		prompt, _ = chat.BuiltinPromptSetByID(chat.DefaultPromptSetID)
	}
	capabilities := chatCapabilities(settings.LLM, nil)
	capabilities.MoodTracking = capabilities.Voice != chat.VoiceUtility
	patterns, err := s.chatPatternChoicesFor(capabilities)
	if err != nil {
		return chat.Service{}, err
	}
	stopped := chat.MotionContext{
		SpeedMinPercent: settings.Motion.SpeedMinPercent,
		SpeedMaxPercent: settings.Motion.SpeedMaxPercent,
		MotionMode:      chatMotionMode(settings.LLM.MotionGenerationMode),
		Envelope:        motion.CurrentPlanningEnvelope(settings.Motion),
	}
	return chat.Service{
		Provider:              provider,
		Prompt:                prompt,
		Model:                 settings.LLM.Model,
		MaxTokens:             settings.LLM.ChatMaxOutputTokens(settings.LLM.ReplyLength),
		ReasoningMode:         settings.LLM.ReasoningMode,
		ReasoningBudgetTokens: managedLlamaReasoningBudget(settings.LLM, s.managedLLM.Snapshot().Runtime.Current),
		PromptBudget:          chatPromptBudget(settings.LLM),
		Patterns:              patterns,
		MotionContext:         &stopped,
		ConversationContext: &chat.ConversationContext{
			PersonaDescription: settings.LLM.PersonaDescription,
			UserAnatomy:        settings.LLM.UserAnatomy,
			CustomAnatomy:      settings.LLM.CustomAnatomy,
		},
		Capabilities: &capabilities,
	}, nil
}

// handleLLMModelTemplateFix turns an offered chat template fix on or off for
// one managed model, then restarts the runner if that model is loaded so the
// next reply uses the change.
func (s *Server) handleLLMModelTemplateFix(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if !decodeModelManagerRequest(w, r, &body) {
		return
	}
	if body.Enabled == nil {
		writeError(w, http.StatusBadRequest, errors.New("enabled is required"))
		return
	}
	record, err := s.models.SetTemplateFix(r.Context(), r.PathValue("id"), *body.Enabled)
	switch {
	case errors.Is(err, llm.ErrModelNotFound):
		writeError(w, http.StatusNotFound, err)
		return
	case errors.Is(err, llm.ErrNoTemplateFix):
		writeError(w, http.StatusConflict, err)
		return
	case err != nil:
		s.writeModelManagerStorageError(w, err)
		return
	}
	settings, _ := s.store.Snapshot()
	if managedLLMSelected(settings.LLM) && settings.LLM.Model == record.ID {
		if err := s.restartSelectedLLM(settings.LLM); err != nil {
			s.logger.Warn("LLM runner did not restart after a template fix change", "error", err)
		}
	}
	writeJSON(w, http.StatusOK, record)
}

// restartSelectedLLM closes the cached provider the way a runtime settings
// change does, so the next request starts the runner with the new template.
func (s *Server) restartSelectedLLM(settings config.LLMSettings) error {
	finishChange := s.llmRequests.beginChange()
	defer finishChange()
	s.stopLLMAutoload()
	err := s.closeLLM()
	s.startLLMAutoload(settings)
	return err
}
