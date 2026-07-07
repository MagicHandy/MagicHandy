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
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/library"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/transport"
	"github.com/mapledaemon/MagicHandy/internal/transport/intiface"
)

const serviceName = "magichandy"

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
	BrowserBluetoothBridge *transport.BrowserBluetoothBridge
	IntifaceClient         *intiface.Client
}

// Server owns the local HTTP routes and embedded static asset serving.
type Server struct {
	static          fs.FS
	logger          *slog.Logger
	store           *config.Store
	traces          *diagnostics.TraceRing
	transport       transport.DiagnosticsProvider
	cloud           cloudRuntime
	bluetooth       bluetoothRuntime
	intiface        intifaceRuntime
	motion          motionRuntime
	llm             llmRuntime
	controller      controllerRuntime
	personalization personalizationRuntime
	statusCompat    statusCompatRuntime
	modes           *modes.Manager
	library         *library.Service
	direct          directRuntime
	manualQueue     manualQueueRuntime
	lsoCompat       lsoCompatRuntime
	started         time.Time
	version         VersionInfo
	handler         http.Handler
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
	if runtime.Traces == nil {
		runtime.Traces = diagnostics.NewTraceRing(1)
	}
	if runtime.Transport == nil {
		runtime.Transport = transport.NewFake()
	}

	personalization, err := newPersonalizationRuntime(store.DataDir())
	if err != nil {
		return nil, err
	}
	if personalization.memory.Recovered() {
		logger.Warn("memory store recovered with defaults", "data_dir", store.DataDir())
	}
	if personalization.prompts.Recovered() {
		logger.Warn("prompt set store recovered with defaults", "data_dir", store.DataDir())
	}

	server := &Server{
		static:          static,
		logger:          logger,
		store:           store,
		traces:          runtime.Traces,
		transport:       runtime.Transport,
		cloud:           newCloudRuntime(runtime),
		bluetooth:       newBluetoothRuntime(runtime),
		intiface:        newIntifaceRuntime(runtime.IntifaceClient),
		motion:          newMotionRuntime(runtime),
		llm:             newLLMRuntime(runtime),
		controller:      newControllerRuntime(),
		personalization: personalization,
		started:         time.Now().UTC(),
		version:         version,
	}

	manager, err := server.newModeManager()
	if err != nil {
		return nil, err
	}
	server.modes = manager

	libraryStore, err := library.Open(filepath.Join(store.DataDir(), "magichandy.db"))
	if err != nil {
		return nil, fmt.Errorf("open pattern library: %w", err)
	}
	server.library = library.NewService(libraryStore)

	mux := http.NewServeMux()
	server.routes(mux)
	server.handler = logRequests(logger, mux)

	if runtime.IntifaceClient != nil {
		server.startIntifaceReconnectLoop(context.Background())
		server.startIntifaceBootstrapLoop(context.Background())
	}

	server.tryAutoImportLSO()

	return server, nil
}

// Handler returns the HTTP handler for use by net/http servers and tests.
func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) routes(mux *http.ServeMux) { //nolint:funlen // route table is intentionally explicit
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /health", s.handleHealthCompat)
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("GET /api/controller", s.handleControllerState)
	mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	mux.HandleFunc("PUT /api/settings", s.handlePutSettings)
	mux.HandleFunc("POST /api/settings/reset", s.handleSettingsReset)
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
	mux.HandleFunc("GET /api/llm/status", s.handleLLMStatus)
	mux.HandleFunc("POST /api/llm/load", s.handleLLMLoad)
	mux.HandleFunc("POST /api/llm/unload", s.handleLLMUnload)
	mux.HandleFunc("GET /api/diagnostics", s.handleDiagnosticsCompat)
	mux.HandleFunc("POST /api/diagnostics/ping-ollama", s.handlePingOllamaCompat)
	mux.HandleFunc("POST /api/chat/stream", s.handleChatStream)
	mux.HandleFunc("GET /api/chat/messages", s.handleChatMessages)
	mux.HandleFunc("POST /api/chat/send", s.handleChatSend)
	mux.HandleFunc("GET /api/transport/diagnostics", s.handleTransportDiagnostics)
	mux.HandleFunc("GET /api/transport/cloud/diagnostics", s.handleCloudDiagnostics)
	mux.HandleFunc("POST /api/transport/cloud/check", s.handleCloudConnectionCheck)
	mux.HandleFunc("GET /api/transport/cloud/state", s.handleCloudState)
	mux.HandleFunc("GET /api/transport/cloud/events", s.handleCloudEvents)
	mux.HandleFunc("POST /api/transport/cloud/stroke-window", s.handleCloudStrokeWindow)
	mux.HandleFunc("POST /api/transport/cloud/hsp-add", s.handleCloudHSPAdd)
	mux.HandleFunc("POST /api/transport/cloud/hsp-play", s.handleCloudHSPPlay)
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
	mux.HandleFunc("POST /api/transport/bluetooth/stroke-window", s.handleBluetoothStrokeWindow)
	mux.HandleFunc("POST /api/transport/bluetooth/hsp-add", s.handleBluetoothHSPAdd)
	mux.HandleFunc("POST /api/transport/bluetooth/hsp-play", s.handleBluetoothHSPPlay)
	mux.HandleFunc("POST /api/transport/bluetooth/stop", s.handleBluetoothStop)
	mux.HandleFunc("GET /api/device/transport", s.handleDeviceTransportGet)
	mux.HandleFunc("POST /api/device/transport", s.handleDeviceTransportPost)
	mux.HandleFunc("POST /api/device/connect", s.handleDeviceConnect)
	mux.HandleFunc("POST /api/device/bootstrap", s.handleDeviceBootstrap)
	mux.HandleFunc("POST /api/device/scan", s.handleDeviceScan)
	mux.HandleFunc("POST /api/device/select", s.handleDeviceSelect)
	mux.HandleFunc("POST /api/emergency-stop", s.handleEmergencyStop)
	mux.HandleFunc("POST /api/emergency-stop/clear", s.handleEmergencyStopClear)
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
	mux.HandleFunc("GET /api/traces", s.handleTraceExport)
	s.registerLibraryRoutes(mux)
	s.registerPersonaRoutes(mux)
	s.registerDirectRoutes(mux)
	s.registerManualQueueRoutes(mux)
	s.registerUIPreferenceRoutes(mux)
	mux.HandleFunc("GET /{path...}", s.handleSPACatchAll)
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

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if clientID := clientIDFromRequest(r); clientID != "" {
		_ = s.controller.Touch(clientID)
	}
	writeJSON(w, http.StatusOK, s.lsoStatusPayload())
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	settings, status := s.store.PublicSnapshot()
	transportDiagnostics := s.transport.Diagnostics()
	writeJSON(w, http.StatusOK, map[string]any{
		"service":        serviceName,
		"version":        s.version.Version,
		"commit":         s.version.Commit,
		"uptime_seconds": int64(time.Since(s.started).Seconds()),
		"data_dir":       status.DataDir,
		"settings_path":  status.SettingsPath,
		"settings":       settings,
		"settings_status": map[string]any{
			"source":         status.Source,
			"using_defaults": status.UsingDefaults,
			"recovered":      status.Recovered,
			"migrated":       status.Migrated,
			"message":        status.Message,
			"loaded_at":      status.LoadedAt,
		},
		"features": map[string]string{
			"chat":      "local_llm_streaming",
			"motion":    "manual",
			"transport": "cloud_rest_browser_bluetooth_manual",
			"voice":     "not_implemented",
		},
		"llm":                 s.llmState(r.Context()),
		"controller":          s.controllerState(r),
		"memory":              s.memoryState(),
		"modes":               s.modes.Status(),
		"motion":              s.motionState(),
		"transport":           transportDiagnostics,
		"cloud_transport":     s.cloudDiagnostics(),
		"bluetooth_transport": s.bluetoothDiagnostics(),
		"bluetooth_bridge":    s.bluetooth.bridge.Snapshot(),
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

	current, _ := s.store.Snapshot()
	next, err := current.ApplyUpdate(update)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	saved, err := s.store.Save(next)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("settings could not be saved"))
		return
	}

	s.applySettingsRuntimeTransition(r.Context(), current, next)

	_, status := s.store.Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"settings": saved.Public(),
		"status":   status,
	})
}

func (s *Server) handleTransportDiagnostics(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.transport.Diagnostics())
}

func (s *Server) handleTraceExport(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.traces.Export())
}

func (s *Server) handleSPACatchAll(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	s.handleStatic(w, r)
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
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		panic(fmt.Errorf("encode JSON response: %w", err))
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{
		"error": err.Error(),
	})
}

func decodeJSON(r *http.Request, target any) error {
	defer func() {
		_ = r.Body.Close()
	}()

	decoder := json.NewDecoder(io.LimitReader(r.Body, 64*1024))
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
