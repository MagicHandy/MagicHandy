package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/openaiauth"
)

type cloudReadiness struct {
	Connection    *config.ModelConnection `json:"connection,omitempty"`
	Provider      string                  `json:"provider"`
	Model         string                  `json:"model"`
	Ready         bool                    `json:"ready"`
	State         string                  `json:"state"`
	Message       string                  `json:"message,omitempty"`
	CheckedAt     time.Time               `json:"checked_at,omitempty"`
	ElapsedMillis int64                   `json:"elapsed_ms,omitempty"`
}

type cloudPlanningRuntime struct {
	providers       map[string]llm.Provider
	connectionTests map[string]cloudReadiness
	revision        uint64
	auth            *openaiauth.Manager
	openErr         error
	baseURL         string
	client          *http.Client
	mu              sync.Mutex
	models          []llm.CloudModel
	readiness       cloudReadiness
}

func (s *Server) initCloudPlanning(runtime Runtime) {
	s.cloudPlanning.baseURL = runtime.OpenAIBaseURL
	s.cloudPlanning.client = runtime.OpenAIHTTPClient
	s.cloudPlanning.auth, s.cloudPlanning.openErr = openaiauth.Open(openaiauth.Options{
		DataDir: s.store.DataDir(), LazyStorage: true, Client: runtime.OpenAIHTTPClient, Issuer: runtime.OpenAIIssuer,
		OnChange: s.invalidateCloudPlanning,
	})
}

func (s *Server) invalidateCloudPlanning() {
	s.cloudRequests.invalidate()
	s.hostedRequests.invalidate()
	s.cloudPlanning.mu.Lock()
	s.cloudPlanning.readiness = cloudReadiness{State: "untested"}
	s.cloudPlanning.models = nil
	s.cloudPlanning.providers = nil
	s.cloudPlanning.revision++
	s.cloudPlanning.mu.Unlock()
	if s.modes != nil {
		s.modes.NotifyAutopilotSettingsChanged()
	}
}

func (s *Server) cloudPlanningRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/llm/cloud", s.handleCloudPlanningStatus)
	mux.HandleFunc("GET /api/llm/cloud/models", s.handleCloudModels)
	mux.HandleFunc("POST /api/llm/cloud/sign-in", s.handleCloudSignIn)
	mux.HandleFunc("DELETE /api/llm/cloud/sign-in", s.handleCancelCloudSignIn)
	mux.HandleFunc("POST /api/llm/cloud/select", s.handleCloudSelect)
	mux.HandleFunc("POST /api/llm/cloud/disconnect", s.handleCloudAccountDisconnect)
	mux.HandleFunc("PUT /api/llm/cloud/decisions-key", s.handleDecisionsKey)
	mux.HandleFunc("POST /api/llm/cloud/test", s.handleCloudTest)
	mux.HandleFunc("POST /api/llm/cloud/welcome", s.handleCloudWelcome)
}

func (s *Server) handleCloudWelcome(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.cloudManager(w, r, true)
	if !ok {
		return
	}
	if err := manager.AcknowledgeWelcome(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, s.cloudStatus(r.Context()))
}

func (s *Server) cloudManager(w http.ResponseWriter, r *http.Request, mutating bool) (*openaiauth.Manager, bool) {
	if !s.capabilities(r).ConfigureHost {
		writeError(w, http.StatusForbidden, errors.New("administrator access is required"))
		return nil, false
	}
	if mutating && !s.requireController(w, r) {
		return nil, false
	}
	if s.cloudPlanning.auth == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("protected OpenAI account storage is unavailable on this host"))
		return nil, false
	}
	return s.cloudPlanning.auth, true
}

func (s *Server) cloudStatus(ctx context.Context) any {
	connection := s.cloudPlanning.auth.Status(ctx)
	s.cloudPlanning.mu.Lock()
	models := append([]llm.CloudModel{}, s.cloudPlanning.models...)
	readiness := s.cloudPlanning.readiness
	s.cloudPlanning.mu.Unlock()
	settings, _ := s.store.Snapshot()
	keys := map[string]bool{}
	readinessByConnection := map[string]cloudReadiness{}
	for _, modelConnection := range settings.LLM.Connections {
		keys[modelConnection.ID] = s.cloudPlanning.auth.ConnectionKeySet(ctx, modelConnection)
		readinessByConnection[modelConnection.ID] = s.connectionReadiness(modelConnection, connection.Generation)
	}
	return map[string]any{"connection": connection, "models": models, "readiness": readiness, "motion_planner": settings.LLM.MotionPlanner, "connection_keys": keys, "connection_readiness": readinessByConnection}
}

func (s *Server) handleCloudPlanningStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.cloudManager(w, r, false); !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.cloudStatus(r.Context()))
}

func (s *Server) handleCloudSignIn(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.cloudManager(w, r, true)
	if !ok {
		return
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		writeError(w, http.StatusConflict, errors.New("open MagicHandy on its host computer to connect ChatGPT; sign-in returns to that computer"))
		return
	}
	var body struct {
		ProfileID string `json:"profile_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	authorizationURL, err := manager.Start(r.Context(), body.ProfileID, scheme+"://"+r.Host+"/#/setup/reconfigure")
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"authorization_url": authorizationURL})
}

func (s *Server) handleCancelCloudSignIn(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.cloudManager(w, r, true)
	if !ok {
		return
	}
	manager.Cancel()
	writeJSON(w, http.StatusOK, s.cloudStatus(r.Context()))
}

func (s *Server) handleCloudSelect(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.cloudManager(w, r, true)
	if !ok {
		return
	}
	var body struct {
		ProfileID string `json:"profile_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.invalidateCloudPlanning()
	if err := manager.Select(r.Context(), body.ProfileID); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, s.cloudStatus(r.Context()))
}

func (s *Server) handleCloudAccountDisconnect(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.cloudManager(w, r, true)
	if !ok {
		return
	}
	var body struct {
		ProfileID string `json:"profile_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.invalidateCloudPlanning()
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	confirmed, err := manager.Disconnect(ctx, body.ProfileID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revocation_confirmed": confirmed, "status": s.cloudStatus(r.Context())})
}

func (s *Server) handleDecisionsKey(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.cloudManager(w, r, true)
	if !ok {
		return
	}
	var body struct {
		APIKey string `json:"api_key"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.invalidateCloudPlanning()
	if err := manager.SetAPIKey(r.Context(), body.APIKey); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, s.cloudStatus(r.Context()))
}

func (s *Server) cloudOptions(ctx context.Context, provider, model string) (llm.OpenAIOptions, error) {
	if s.cloudPlanning.auth == nil {
		return llm.OpenAIOptions{}, &llm.CloudError{Kind: "unavailable"}
	}
	var token llm.TokenSource
	var err error
	if provider == config.MotionPlannerDecisions {
		token, err = s.cloudPlanning.auth.APIKeySource(ctx)
	} else {
		token, err = s.cloudPlanning.auth.ActiveTokenSource(ctx)
	}
	return llm.OpenAIOptions{Model: model, Token: token, PlanUsage: provider == config.MotionPlannerChatGPT, Client: s.cloudPlanning.client, BaseURL: s.cloudPlanning.baseURL, Timeout: 90 * time.Second}, err
}

func (s *Server) connectedChatGPT(ctx context.Context, model string) (*llm.OpenAIResponsesProvider, error) {
	options, err := s.cloudOptions(ctx, config.MotionPlannerChatGPT, model)
	if err != nil {
		return nil, err
	}
	return llm.NewOpenAIResponsesProvider(options)
}

func (s *Server) handleCloudModels(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.cloudManager(w, r, false); !ok {
		return
	}
	ctx, _, release, err := s.cloudRequests.acquire(r.Context(), llmRequestInteractive)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	defer release()
	provider, err := s.connectedChatGPT(ctx, "catalog")
	if err == nil {
		var models []llm.CloudModel
		models, err = provider.Models(ctx)
		if err == nil && ctx.Err() == nil {
			s.cloudPlanning.mu.Lock()
			s.cloudPlanning.models = models
			s.cloudPlanning.mu.Unlock()
		}
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, s.cloudStatus(ctx))
}

func (s *Server) handleCloudTest(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.cloudManager(w, r, true); !ok {
		return
	}
	var body struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if body.Provider != config.MotionPlannerChatGPT && body.Provider != config.MotionPlannerDecisions {
		writeError(w, http.StatusBadRequest, errors.New("select a cloud provider to test"))
		return
	}
	ctx, _, release, err := s.cloudRequests.acquire(r.Context(), llmRequestInteractive)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	defer release()
	readiness := cloudReadiness{Provider: body.Provider, Model: body.Model, State: "unavailable", CheckedAt: time.Now()}
	generation := s.cloudPlanning.auth.Generation()
	err = s.testCloudGeneration(ctx, body.Provider, body.Model)
	if ctx.Err() != nil {
		writeError(w, http.StatusConflict, errors.New("cloud test was canceled by an account or settings change"))
		return
	}
	if err == nil {
		readiness.Ready = true
		readiness.State = "ready"
	} else {
		readiness.Message = err.Error()
		var outcome *llm.CloudError
		if errors.As(err, &outcome) {
			readiness.State = outcome.Kind
		}
	}
	s.cloudPlanning.mu.Lock()
	s.cloudPlanning.readiness = readiness
	s.cloudPlanning.mu.Unlock()
	if body.Provider == config.MotionPlannerChatGPT {
		connection := config.ModelConnection{ID: "chatgpt", Name: "ChatGPT", Provider: config.LLMProviderChatGPT, Model: body.Model}.Normalize()
		s.recordConnectionReadiness(connection, generation, readiness.Ready, readiness.Message, 0)
	}
	writeJSON(w, http.StatusOK, s.cloudStatus(ctx))
}

func (s *Server) testCloudGeneration(ctx context.Context, providerID, model string) error {
	options, err := s.cloudOptions(ctx, providerID, model)
	if err != nil {
		return err
	}
	if providerID == config.MotionPlannerDecisions {
		choice, err := llm.ChooseOpenAICandidate(ctx, options, "Text-only readiness check. Select ready.", []llm.DecisionChoice{{Value: "ready", Description: "Connection readiness confirmed."}, {Value: "keep_current", Description: "Abstain."}})
		if err != nil {
			return err
		}
		if choice != "ready" {
			return &llm.CloudError{Kind: "incomplete"}
		}
		return nil
	}
	provider, err := llm.NewOpenAIResponsesProvider(options)
	if err != nil {
		return err
	}
	models, err := provider.Models(ctx)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(models, func(m llm.CloudModel) bool { return m.Slug == model }) {
		return &llm.CloudError{Kind: "model"}
	}
	raw, err := provider.StreamChat(ctx, llm.ChatRequest{Messages: []llm.Message{{Role: "developer", Content: "This is a text-only connection test. Return exactly {\"ready\":true}."}, {Role: "user", Content: "Confirm readiness."}}, JSONSchema: json.RawMessage(`{"type":"object","properties":{"ready":{"type":"boolean"}},"required":["ready"],"additionalProperties":false}`)}, nil)
	if err != nil {
		return err
	}
	var answer struct {
		Ready bool `json:"ready"`
	}
	if json.Unmarshal([]byte(raw), &answer) != nil || !answer.Ready {
		return &llm.CloudError{Kind: "incomplete"}
	}
	s.cloudPlanning.mu.Lock()
	s.cloudPlanning.models = models
	s.cloudPlanning.mu.Unlock()
	return nil
}
