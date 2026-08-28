// Package httpapi exposes the local browser UI and core HTTP API routes.
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/media"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/patterns"
	"github.com/mapledaemon/MagicHandy/internal/persona"
	"github.com/mapledaemon/MagicHandy/internal/transport"
	"github.com/mapledaemon/MagicHandy/internal/updatecheck"
	"github.com/mapledaemon/MagicHandy/internal/voice"
)

const serviceName = "magichandy"
const stopSequenceHeader = "X-MagicHandy-Stop-Sequence"
const maxJSONBodyBytes = 64 << 10

// VersionInfo identifies the build served by the HTTP API.
type VersionInfo struct {
	Version string
	Commit  string
}

// Runtime contains app runtime collaborators exposed through HTTP diagnostics.
type Runtime struct {
	Traces                 *diagnostics.TraceRing
	Transport              transport.DiagnosticsProvider
	MotionTransport        transport.Transport
	LLMProvider            llm.Provider
	LLMHTTPClient          *http.Client
	CloudBaseURL           string
	CloudHTTPClient        *http.Client
	IntifaceHTTPClient     *http.Client
	BrowserBluetoothBridge *transport.BrowserBluetoothBridge
	// ExecutablePath makes first-party worker discovery deterministic and
	// injectable in tests. Empty falls back to os.Executable.
	ExecutablePath      string
	UpdateHTTPClient    *http.Client
	UpdateReleaseAPIURL string
	// Accounts is the process account domain. Nil creates one over Store's
	// shared datastore. AuthenticationRequired protects every route except the
	// deliberately public health, login/bootstrap, and Emergency Stop edges.
	Accounts               *accounts.Store
	AuthenticationRequired bool
	SecureCookies          bool
	AllowedBrowserHosts    []string
}

// Server owns the local HTTP routes and embedded static asset serving.
type Server struct {
	static              fs.FS
	logger              *slog.Logger
	store               *config.Store
	accounts            *accounts.Store
	auth                authenticationRuntime
	traces              *diagnostics.TraceRing
	transport           transport.DiagnosticsProvider
	cloud               cloudRuntime
	bluetooth           bluetoothRuntime
	intiface            intifaceRuntime
	motion              motionRuntime
	llm                 llmRuntime
	llmRequests         llmRequestCoordinator
	llmAutoloadMu       sync.Mutex
	llmAutoloadCancel   context.CancelFunc
	llmAutoloadWG       sync.WaitGroup
	llmAutoloadID       uint64
	models              *llm.ModelManager
	managedLLM          *llm.ManagedLlamaRuntimeManager
	setup               *setupManager
	updates             *updatecheck.Checker
	controller          controllerRuntime
	personalization     personalizationRuntime
	personas            *persona.Store
	modes               *modes.Manager
	voice               *voice.Manager
	voiceExecutable     string
	voiceDataDir        string
	voiceAutoloadMu     sync.Mutex
	voiceAutoloadCancel context.CancelFunc
	voiceAutoloadWG     sync.WaitGroup
	voiceAutoloadID     uint64
	stopSequence        atomic.Uint64
	quiescing           atomic.Bool
	lifecycleCtx        context.Context
	lifecycleCancel     context.CancelFunc
	quiesceOnce         sync.Once
	closeOnce           sync.Once
	settingsLifecycleMu sync.Mutex
	chatLifecycleMu     sync.Mutex
	personaMutationMu   sync.Mutex
	chatCancelMu        sync.Mutex
	chatCancels         map[uint64]context.CancelFunc
	nextChatID          uint64
	chatSpeechMu        sync.Mutex
	chatSpeechRequests  map[int64]string
	hostPathPicker      hostPathPicker
	chatLog             *chat.MessageLog
	patterns            *patterns.Library
	media               *media.Catalog
	mediaSync           *mediaSyncRuntime
	started             time.Time
	version             VersionInfo
	handler             http.Handler
}

// New wires the HTTP API to the embedded static assets and structured logger.
func New(static fs.FS, logger *slog.Logger, store *config.Store, runtime Runtime, version VersionInfo) (*Server, error) {
	if static == nil {
		return nil, errors.New("static filesystem is required")
	}
	if store == nil {
		return nil, errors.New("settings store is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	runtime = normalizeRuntime(runtime)
	accountStore, authRuntime, err := newAuthenticationComponents(store, runtime)
	if err != nil {
		return nil, err
	}

	// Settings owns the one process database handle. Every sibling persistence
	// domain borrows it so pooling, writer serialization, and shutdown have one
	// lifecycle boundary.
	personalization, err := newPersonalizationRuntime(store.Datastore())
	if err != nil {
		return nil, err
	}
	if personalization.memory.Recovered() {
		logger.Warn("memory store recovered with defaults", "data_dir", store.DataDir())
	}
	if personalization.prompts.Recovered() {
		logger.Warn("prompt set store recovered with defaults", "data_dir", store.DataDir())
	}
	modelManager, err := llm.OpenModelManagerWithDatabase(store.Datastore())
	if err != nil {
		personalization.Close()
		return nil, err
	}
	managedLLM, err := llm.OpenManagedLlamaRuntimeManager(store.DataDir())
	if err != nil {
		_ = modelManager.Close()
		personalization.Close()
		return nil, err
	}

	lifecycleCtx, lifecycleCancel := context.WithCancel(context.Background())
	server := &Server{
		static:          static,
		logger:          logger,
		store:           store,
		accounts:        accountStore,
		auth:            authRuntime,
		traces:          runtime.Traces,
		transport:       runtime.Transport,
		cloud:           newCloudRuntime(runtime),
		bluetooth:       newBluetoothRuntime(runtime),
		intiface:        newIntifaceRuntime(runtime),
		motion:          newMotionRuntime(runtime),
		llm:             newLLMRuntime(runtime),
		models:          modelManager,
		managedLLM:      managedLLM,
		updates:         newUpdateChecker(runtime, version),
		controller:      newControllerRuntime(),
		hostPathPicker:  systemHostPathPicker,
		personalization: personalization,
		lifecycleCtx:    lifecycleCtx,
		lifecycleCancel: lifecycleCancel,
		started:         time.Now().UTC(),
		version:         version,
	}
	server.setup = newSetupManager(
		lifecycleCtx,
		store.DataDir(),
		runtime.ExecutablePath,
		logger,
		server.applyInstalledVoiceModule,
		server.applyInstalledParakeet,
	)

	manager, err := server.newModeManager()
	if err != nil {
		lifecycleCancel()
		managedLLM.Close()
		_ = modelManager.Close()
		personalization.Close()
		return nil, err
	}
	server.modes = manager

	settings, _ := store.Snapshot()
	server.configureVoice(settings.Voice, runtime.ExecutablePath, store.DataDir())

	if err := server.openPersistentDomains(settings.Media.LibraryPaths, settings.Chat); err != nil {
		lifecycleCancel()
		server.modes.Shutdown()
		if server.voice != nil {
			server.voice.Shutdown()
		}
		managedLLM.Close()
		_ = modelManager.Close()
		personalization.Close()
		return nil, err
	}

	server.activate(runtime, settings)
	return server, nil
}

func (s *Server) activate(runtime Runtime, settings config.Settings) {
	mux := http.NewServeMux()
	s.routes(mux)
	s.handler = logRequests(s.logger, securityHeaders(
		runtime.SecureCookies,
		protectBrowserRequests(runtime.AllowedBrowserHosts, s.authenticateRequests(mux)),
	))
	s.startLLMAutoload(settings.LLM)
	s.startVoiceAutoload(settings.Voice)
	s.startMediaAutoScan(settings.Media)
}

func normalizeRuntime(runtime Runtime) Runtime {
	if runtime.Traces == nil {
		runtime.Traces = diagnostics.NewTraceRing(1)
	}
	if runtime.Transport == nil {
		runtime.Transport = transport.NewFake()
	}
	return runtime
}

func (s *Server) openPersistentDomains(mediaLocations []string, chatSettings config.ChatSettings) error {
	// Personas open first: a new session inherits the last-used persona, so the
	// table has to be readable before the chat log reconciles startup.
	personaStore, err := persona.OpenWithDatabase(s.store.Datastore())
	if err != nil {
		return err
	}
	s.personas = personaStore

	chatLog, err := chat.OpenMessageLogWithDatabase(s.store.Datastore())
	if err != nil {
		_ = personaStore.Close()
		return err
	}
	if _, err := chatLog.ReconcileStartup(chatSettings.StartupBehavior, chatSettings.KeepUnsavedOnExit); err != nil {
		_ = chatLog.Close()
		_ = personaStore.Close()
		return err
	}
	s.chatLog = chatLog

	patternLibrary, err := patterns.OpenWithDatabase(s.store.Datastore())
	if err != nil {
		_ = chatLog.Close()
		_ = personaStore.Close()
		return err
	}
	s.patterns = patternLibrary

	mediaCatalog, err := media.OpenWithDatabase(s.store.Datastore(), s.logger)
	if err != nil {
		_ = patternLibrary.Close()
		_ = chatLog.Close()
		_ = personaStore.Close()
		return err
	}
	removed, err := mediaCatalog.RetainLocations(context.Background(), mediaLocations)
	if err != nil {
		_ = mediaCatalog.Close()
		_ = patternLibrary.Close()
		_ = chatLog.Close()
		_ = personaStore.Close()
		return fmt.Errorf("reconcile media catalog locations: %w", err)
	}
	if removed > 0 {
		s.logger.Info("removed media outside configured locations", "video_count", removed)
	}
	s.media = mediaCatalog
	s.mediaSync = newMediaSyncRuntime(s)
	return nil
}

func (s *Server) configureVoice(settings config.VoiceSettings, executablePath, dataDir string) {
	s.voiceExecutable = executablePath
	s.voiceDataDir = dataDir
	s.voice = newVoiceManager(settings, executablePath, dataDir)
}

func (s *Server) requestLifecycleContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(s.lifecycleCtx)
	stopParentWatch := context.AfterFunc(parent, cancel)
	return ctx, func() {
		stopParentWatch()
		cancel()
	}
}

// Handler returns the HTTP handler for use by net/http servers and tests.
func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", s.handleHealth)
	s.authenticationRoutes(mux)
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("GET /api/controller", s.handleControllerState)
	mux.HandleFunc("POST /api/controller/takeover", s.handleControllerTakeover)
	s.settingsAndUpdateRoutes(mux)
	mux.HandleFunc("POST /api/host/path-picker", s.handleHostPathPicker)
	s.personalizationRoutes(mux)
	s.personaRoutes(mux)
	mux.HandleFunc("GET /api/diagnostics/prompt-composition", s.handlePromptComposition)
	s.llmRoutes(mux)
	s.chatRoutes(mux)
	mux.HandleFunc("GET /api/transport/diagnostics", s.handleTransportDiagnostics)
	mux.HandleFunc("GET /api/transport/cloud/diagnostics", s.handleCloudDiagnostics)
	mux.HandleFunc("POST /api/transport/cloud/check", s.handleCloudConnectionCheck)
	mux.HandleFunc("POST /api/transport/cloud/connect", s.handleCloudConnect)
	mux.HandleFunc("POST /api/transport/cloud/disconnect", s.handleCloudDisconnect)
	mux.HandleFunc("GET /api/transport/cloud/state", s.handleCloudState)
	mux.HandleFunc("GET /api/transport/cloud/events", s.handleCloudEvents)
	mux.HandleFunc("POST /api/transport/cloud/stop", s.handleCloudStop)
	mux.HandleFunc("GET /api/transport/bluetooth/diagnostics", s.handleBluetoothDiagnostics)
	mux.HandleFunc("GET /api/transport/bluetooth/status", s.handleBluetoothStatus)
	mux.HandleFunc("POST /api/transport/bluetooth/status", s.handleBluetoothStatus)
	mux.HandleFunc("POST /api/transport/bluetooth/connect", s.handleBluetoothConnect)
	mux.HandleFunc("POST /api/transport/bluetooth/disconnect", s.handleBluetoothDisconnect)
	mux.HandleFunc("GET /api/transport/bluetooth/commands", s.handleBluetoothCommands)
	mux.HandleFunc("POST /api/transport/bluetooth/ack", s.handleBluetoothAck)
	mux.HandleFunc("POST /api/transport/bluetooth/check", s.handleBluetoothConnectionCheck)
	mux.HandleFunc("GET /api/transport/bluetooth/state", s.handleBluetoothState)
	mux.HandleFunc("GET /api/transport/bluetooth/events", s.handleBluetoothEvents)
	mux.HandleFunc("POST /api/transport/bluetooth/stop", s.handleBluetoothStop)
	mux.HandleFunc("GET /api/transport/intiface/status", s.handleIntifaceStatus)
	mux.HandleFunc("POST /api/transport/intiface/connect", s.handleIntifaceConnect)
	mux.HandleFunc("POST /api/transport/intiface/disconnect", s.handleIntifaceDisconnect)
	mux.HandleFunc("POST /api/transport/intiface/scan", s.handleIntifaceStartScan)
	mux.HandleFunc("DELETE /api/transport/intiface/scan", s.handleIntifaceStopScan)
	mux.HandleFunc("POST /api/transport/intiface/select", s.handleIntifaceSelect)
	mux.HandleFunc("GET /api/transport/intiface/diagnostics", s.handleIntifaceDiagnostics)
	mux.HandleFunc("GET /api/motion/state", s.handleMotionState)
	mux.HandleFunc("GET /api/motion/events", s.handleMotionEvents)
	mux.HandleFunc("POST /api/motion/start", s.handleMotionStart)
	mux.HandleFunc("POST /api/motion/target", s.handleMotionTarget)
	mux.HandleFunc("POST /api/motion/quick", s.handleMotionQuick)
	mux.HandleFunc("POST /api/motion/pause", s.handleMotionPause)
	mux.HandleFunc("POST /api/motion/resume", s.handleMotionResume)
	mux.HandleFunc("POST /api/motion/stop", s.handleMotionStop)
	mux.HandleFunc("GET /api/modes", s.handleModesGet)
	mux.HandleFunc("POST /api/modes/start", s.handleModeStart)
	mux.HandleFunc("POST /api/modes/stop", s.handleModeStop)
	mux.HandleFunc("PUT /api/modes/autopilot/preferences", s.handleAutopilotPreferences)
	mux.HandleFunc("PUT /api/modes/autopilot/arc", s.handleAutopilotArc)
	s.libraryRoutes(mux)
	s.mediaRoutes(mux)
	s.voiceRoutes(mux)
	s.setupRoutes(mux)
	mux.HandleFunc("GET /api/traces", s.handleTraceExport)
	mux.HandleFunc("GET /api/traces/last-motion", s.handleLastMotionTrace)
	mux.HandleFunc("GET /", s.handleStatic)
}

func (s *Server) settingsAndUpdateRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	mux.HandleFunc("PUT /api/settings", s.handlePutSettings)
	mux.HandleFunc("PUT /api/settings/llm-motion-mode", s.handlePutLLMMotionMode)
	mux.HandleFunc("GET /api/update", s.handleUpdateStatus)
	mux.HandleFunc("PUT /api/settings/device/connection-key", s.handlePutConnectionKey)
	mux.HandleFunc("POST /api/settings/reset", s.handleSettingsReset)
}

func (s *Server) handlePutLLMMotionMode(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body struct {
		Mode string `json:"mode"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	body.Mode = strings.ToLower(strings.TrimSpace(body.Mode))
	if body.Mode != config.LLMMotionModeDynamic && body.Mode != config.LLMMotionModePattern && body.Mode != config.LLMMotionModeOff {
		writeError(w, http.StatusBadRequest, fmt.Errorf("unknown LLM motion generation mode %q", body.Mode))
		return
	}
	var updateErr error
	_, saved, saveErr, runtimeErr := s.updateSettingsAndRuntime(r.Context(), func(current config.Settings) (config.Settings, error) {
		current.LLM.MotionGenerationMode = body.Mode
		capabilities := current.LLM.Capabilities()
		capabilities.Motion = body.Mode != config.LLMMotionModeOff
		if body.Mode == config.LLMMotionModePattern {
			capabilities.Patterns = true
		}
		current.LLM.MotionCapabilities = &capabilities
		var next config.Settings
		next, updateErr = config.NormalizeSettings(current)
		return next, updateErr
	})
	if updateErr != nil {
		writeError(w, http.StatusBadRequest, updateErr)
		return
	}
	if saveErr != nil {
		writeError(w, http.StatusInternalServerError, errors.New("LLM motion mode could not be saved"))
		return
	}
	if s.modes != nil {
		s.modes.Stop("llm_motion_mode_changed")
	}
	payload := map[string]any{"settings": saved.Public(), "mode": saved.LLM.MotionGenerationMode}
	status := http.StatusOK
	if runtimeErr != nil {
		status = http.StatusBadGateway
		payload["error"] = "LLM motion mode was saved, but the active runtime could not apply it"
	}
	writeJSON(w, status, payload)
}

func newUpdateChecker(runtime Runtime, version VersionInfo) *updatecheck.Checker {
	return updatecheck.New(updatecheck.Options{
		CurrentVersion: version.Version,
		Endpoint:       runtime.UpdateReleaseAPIURL,
		HTTPClient:     runtime.UpdateHTTPClient,
	})
}

func (s *Server) llmRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/llm/status", s.handleLLMStatus)
	mux.HandleFunc("POST /api/llm/load", s.handleLLMLoad)
	mux.HandleFunc("POST /api/llm/unload", s.handleLLMUnload)
	mux.HandleFunc("GET /api/llm/duplicates", s.handleManagedLLMDuplicates)
	mux.HandleFunc("POST /api/llm/duplicates/terminate", s.handleTerminateManagedLLMDuplicates)
	mux.HandleFunc("GET /api/llm/runtime", s.handleManagedLLMRuntime)
	mux.HandleFunc("POST /api/llm/runtime/build", s.handleBuildManagedLLMRuntime)
	mux.HandleFunc("DELETE /api/llm/runtime/build", s.handleCancelManagedLLMRuntimeBuild)
	mux.HandleFunc("GET /api/llm/models", s.handleLLMModels)
	mux.HandleFunc("DELETE /api/llm/models/{id}", s.handleDeleteLLMModel)
	mux.HandleFunc("GET /api/llm/ollama/models", s.handleOllamaModels)
	mux.HandleFunc("POST /api/llm/ollama/scan", s.handleOllamaScan)
	mux.HandleFunc("POST /api/llm/imports/ollama", s.handleOllamaImport)
	mux.HandleFunc("POST /api/llm/imports/gguf", s.handleGGUFImport)
	mux.HandleFunc("GET /api/llm/imports/{id}", s.handleLLMImport)
	mux.HandleFunc("DELETE /api/llm/imports/{id}", s.handleCancelLLMImport)
}

func (s *Server) personalizationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/memory", s.handleMemoryGet)
	mux.HandleFunc("POST /api/memory", s.handleMemoryAdd)
	mux.HandleFunc("POST /api/memory/enabled", s.handleMemorySetEnabled)
	mux.HandleFunc("POST /api/memory/clear", s.handleMemoryClear)
	mux.HandleFunc("PATCH /api/memory/{id}", s.handleMemoryPatchItem)
	mux.HandleFunc("DELETE /api/memory/{id}", s.handleMemoryRemove)
	mux.HandleFunc("GET /api/prompt-sets", s.handlePromptSetsGet)
	mux.HandleFunc("POST /api/prompt-sets", s.handlePromptSetCreate)
	mux.HandleFunc("PUT /api/prompt-sets/{id}", s.handlePromptSetUpdate)
	mux.HandleFunc("DELETE /api/prompt-sets/{id}", s.handlePromptSetDelete)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"service":   serviceName,
		"version":   s.version.Version,
		"commit":    s.version.Commit,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	settings, status := s.store.PublicSnapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"service":        serviceName,
		"version":        s.version.Version,
		"commit":         s.version.Commit,
		"uptime_seconds": int64(time.Since(s.started).Seconds()),
		"stop_sequence":  s.stopSequence.Load(),
		"ui":             "embedded",
		"settings": map[string]any{
			"source":         status.Source,
			"using_defaults": status.UsingDefaults,
			"recovered":      status.Recovered,
			"migrated":       status.Migrated,
			"imported":       status.Imported,
			"version":        settings.Version,
		},
		"features": map[string]string{
			"accounts":  "backend_api_no_gui",
			"chat":      "local_llm_streaming",
			"library":   "patterns_programs_authoring_media",
			"motion":    "manual",
			"transport": "cloud_rest_browser_bluetooth_intiface_manual",
			"voice":     "optional_worker_protocol_v1",
		},
	})
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	settings, status := s.store.PublicSnapshot()
	transportDiagnostics := s.transport.Diagnostics()
	writeJSON(w, http.StatusOK, map[string]any{
		"service":        serviceName,
		"version":        s.version.Version,
		"commit":         s.version.Commit,
		"uptime_seconds": int64(time.Since(s.started).Seconds()),
		"stop_sequence":  s.stopSequence.Load(),
		"data_dir":       status.DataDir,
		"settings_path":  status.SettingsPath,
		"datastore_path": status.DatastorePath,
		"settings":       settings,
		"settings_status": map[string]any{
			"source":                   status.Source,
			"using_defaults":           status.UsingDefaults,
			"recovered":                status.Recovered,
			"migrated":                 status.Migrated,
			"imported":                 status.Imported,
			"message":                  status.Message,
			"loaded_at":                status.LoadedAt,
			"datastore_recovered_path": status.DatastoreRecoveredPath,
			"legacy_settings_path":     status.LegacySettingsPath,
			"legacy_archived_path":     status.LegacyArchivedPath,
		},
		"features": map[string]string{
			"accounts":  "backend_api_no_gui",
			"chat":      "local_llm_streaming",
			"library":   "patterns_programs_authoring_media",
			"motion":    "manual",
			"transport": "cloud_rest_browser_bluetooth_intiface_manual",
			"voice":     "optional_worker_protocol_v1",
		},
		"llm":                 s.llmState(r.Context()),
		"controller":          s.controllerState(r),
		"memory":              s.memoryState(),
		"modes":               s.modes.Status(),
		"voice":               s.voiceState(),
		"chat":                s.chatState(),
		"library":             s.libraryState(),
		"media":               s.mediaState(r.Context()),
		"motion":              s.motionState(),
		"transport":           transportDiagnostics,
		"cloud_transport":     s.cloudDiagnostics(),
		"bluetooth_transport": s.bluetoothDiagnostics(),
		"bluetooth_bridge":    s.bluetooth.bridge.Snapshot(),
		"intiface_transport":  s.intifaceSnapshot(),
		"trace":               s.traces.Summary(),
	})
}

func (s *Server) handleGetSettings(w http.ResponseWriter, _ *http.Request) {
	settings, status := s.store.PublicSnapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"settings": settings,
		"status":   status,
	})
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}

	var update config.SettingsUpdate
	if err := decodeJSON(r, &update); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var updateErr error
	_, saved, saveErr, runtimeErr := s.updateSettingsAndRuntime(r.Context(), func(current config.Settings) (config.Settings, error) {
		var next config.Settings
		next, updateErr = current.ApplyUpdate(update)
		return next, updateErr
	})
	if updateErr != nil {
		writeError(w, http.StatusBadRequest, updateErr)
		return
	}
	if saveErr != nil {
		writeError(w, http.StatusInternalServerError, errors.New("settings could not be saved"))
		return
	}

	_, status := s.store.Snapshot()
	payload := map[string]any{
		"settings": saved.Public(),
		"status":   status,
	}
	responseStatus := http.StatusOK
	if runtimeErr != nil {
		responseStatus = http.StatusBadGateway
		payload["error"] = "settings were saved, but the active runtime could not apply them"
	}
	writeJSON(w, responseStatus, payload)
}

func (s *Server) handlePutConnectionKey(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}

	var update struct {
		ConnectionKey string `json:"connection_key"`
	}
	if err := decodeJSON(r, &update); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	update.ConnectionKey = strings.TrimSpace(update.ConnectionKey)
	if update.ConnectionKey == "" {
		writeError(w, http.StatusBadRequest, errors.New("connection key is required"))
		return
	}

	_, saved, saveErr, runtimeErr := s.updateSettingsAndRuntime(r.Context(), func(current config.Settings) (config.Settings, error) {
		current.Device.HandyConnectionKey = update.ConnectionKey
		return current, nil
	})
	if saveErr != nil {
		writeError(w, http.StatusInternalServerError, errors.New("connection key could not be saved"))
		return
	}

	_, status := s.store.Snapshot()
	payload := map[string]any{
		"settings": saved.Public(),
		"status":   status,
	}
	responseStatus := http.StatusOK
	if runtimeErr != nil {
		responseStatus = http.StatusBadGateway
		payload["error"] = "connection key was saved, but the active device runtime could not be stopped"
	}
	writeJSON(w, responseStatus, payload)
}

func (s *Server) handleTransportDiagnostics(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.transport.Diagnostics())
}

func (s *Server) handleTraceExport(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.currentTraceExport())
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	name := cleanAssetName(r.URL.Path)
	data, err := fs.ReadFile(s.static, name)
	if err != nil {
		if strings.Contains(path.Base(name), ".") {
			http.NotFound(w, r)
			return
		}
		name = "index.html"
		data, err = fs.ReadFile(s.static, name)
		if err != nil {
			http.Error(w, "embedded UI is unavailable", http.StatusInternalServerError)
			return
		}
	}

	setStaticHeaders(w, name)
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}

func cleanAssetName(urlPath string) string {
	name := strings.TrimPrefix(path.Clean("/"+urlPath), "/")
	if name == "." || name == "" {
		return "index.html"
	}
	return name
}

func setStaticHeaders(w http.ResponseWriter, name string) {
	w.Header().Set("X-Content-Type-Options", "nosniff")

	switch path.Ext(name) {
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		status = http.StatusInternalServerError
		data = []byte(`{"error":"response could not be encoded"}`)
	}
	data = append(data, '\n')

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{
		"error": err.Error(),
	})
}

func decodeJSON(r *http.Request, target any) error {
	data, err := readJSONBody(r)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode JSON request: %w", err)
	}
	var extra struct{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("decode JSON request: multiple JSON values are not allowed")
	}
	return nil
}

func readJSONBody(r *http.Request) ([]byte, error) {
	defer func() {
		_ = r.Body.Close()
	}()

	data, err := io.ReadAll(io.LimitReader(r.Body, maxJSONBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("decode JSON request: %w", err)
	}
	if len(data) > maxJSONBodyBytes {
		return nil, fmt.Errorf("decode JSON request: body exceeds %d bytes", maxJSONBodyBytes)
	}
	return data, nil
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.status != 0 {
		return
	}
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(data)
	r.bytes += n
	return n, err
}

func (r *statusRecorder) Flush() {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func logRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}

		next.ServeHTTP(recorder, r)

		logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"bytes", recorder.bytes,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func protectBrowserRequests(allowedHosts []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isBrowserRequest(r) && (!isAllowedBrowserHost(r.Host, allowedHosts) || !isSameOriginBrowserRequest(r)) {
			writeError(w, http.StatusForbidden, errors.New("browser requests must use an allowed MagicHandy origin"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isAllowedBrowserHost(host string, allowedHosts []string) bool {
	if len(allowedHosts) == 0 {
		return isLoopbackHost(host)
	}
	host = hostWithoutPort(host)
	for _, allowed := range allowedHosts {
		if strings.EqualFold(host, hostWithoutPort(allowed)) {
			return true
		}
	}
	return false
}

func hostWithoutPort(hostPort string) string {
	host := strings.TrimSpace(hostPort)
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	return strings.Trim(strings.ToLower(host), "[]")
}

func isBrowserRequest(r *http.Request) bool {
	return r.Header.Get("Origin") != "" ||
		r.Header.Get("Sec-Fetch-Site") != "" ||
		r.Header.Get("Sec-Fetch-Mode") != "" ||
		r.Header.Get("Sec-Fetch-Dest") != ""
}
