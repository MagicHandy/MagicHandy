package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	diagnosticspkg "github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

const (
	cloudTransportName = "handy_cloud_rest"
	cloudTraceSource   = "manual_cloud_transport"
)

type cloudRuntime struct {
	baseURL     string
	client      *http.Client
	mu          sync.Mutex
	diagnostics transport.TransportDiagnostics
}

type cloudCommandResponse struct {
	Error       string                         `json:"error,omitempty"`
	Result      transport.CommandResult        `json:"result"`
	Diagnostics transport.TransportDiagnostics `json:"diagnostics"`
}

type cloudStateResponse struct {
	Error       string                         `json:"error,omitempty"`
	State       transport.HSPStateSnapshot     `json:"state"`
	Result      transport.CommandResult        `json:"result"`
	Diagnostics transport.TransportDiagnostics `json:"diagnostics"`
}

type cloudErrorResponse struct {
	Error       string                         `json:"error"`
	Diagnostics transport.TransportDiagnostics `json:"diagnostics"`
}

func newCloudRuntime(runtime Runtime) cloudRuntime {
	return cloudRuntime{
		baseURL:     strings.TrimSpace(runtime.CloudBaseURL),
		client:      runtime.CloudHTTPClient,
		diagnostics: defaultCloudDiagnostics(),
	}
}

func (s *Server) handleCloudDiagnostics(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.cloudDiagnostics())
}

func (s *Server) handleCloudConnectionCheck(w http.ResponseWriter, r *http.Request) {
	cloud, err := s.newCloudTransport()
	if err != nil {
		s.writeCloudSetupError(w, err)
		return
	}

	check, err := cloud.CheckConnection(r.Context())
	diagnostics := s.saveCloudDiagnostics(cloud.Diagnostics())
	check.Diagnostics = diagnostics
	if err != nil && !isHSPUnavailable(err) {
		writeJSON(w, http.StatusBadGateway, cloudErrorResponse{
			Error:       safeCloudErrorMessage(err),
			Diagnostics: diagnostics,
		})
		return
	}
	writeJSON(w, http.StatusOK, check)
}

func (s *Server) handleCloudState(w http.ResponseWriter, r *http.Request) {
	cloud, err := s.newCloudTransport()
	if err != nil {
		s.writeCloudSetupError(w, err)
		return
	}

	state, result, err := cloud.ReadState(r.Context())
	diagnostics := s.saveCloudDiagnostics(cloud.Diagnostics())
	payload := cloudStateResponse{
		State:       state,
		Result:      transport.SafeCommandResult(result),
		Diagnostics: diagnostics,
	}
	if err != nil {
		payload.Error = safeCloudErrorMessage(err)
		writeJSON(w, cloudCommandStatus(err, result), payload)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleCloudEvents(w http.ResponseWriter, r *http.Request) {
	cloud, err := s.newCloudTransport()
	if err != nil {
		s.writeCloudSetupError(w, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming responses are unavailable"))
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	var writeErr error
	setEventStreamHeaders(w)
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	err = cloud.ListenStateEvents(ctx, func(event transport.HSPStateEvent) {
		if writeErr != nil {
			return
		}
		writeErr = writeStateEvent(w, event)
		if writeErr != nil {
			cancel()
			return
		}
		flusher.Flush()
	})
	s.saveCloudDiagnostics(cloud.Diagnostics())
	if err != nil && writeErr == nil && !errors.Is(err, context.Canceled) {
		s.logger.Warn("cloud HSP event stream failed", "error", safeCloudErrorMessage(err))
	}
}

func (s *Server) handleCloudStrokeWindow(w http.ResponseWriter, r *http.Request) {
	var command transport.StrokeWindowCommand
	if err := decodeJSON(r, &command); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cloud, err := s.newCloudTransport()
	if err != nil {
		s.writeCloudSetupError(w, err)
		return
	}

	result, err := cloud.SetStrokeWindow(r.Context(), command)
	s.writeCloudCommandResponse(w, string(transport.CommandKindStrokeWindow), cloud, result, err)
}

func (s *Server) handleCloudHSPAdd(w http.ResponseWriter, r *http.Request) {
	var command transport.HSPAddCommand
	if err := decodeJSON(r, &command); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cloud, err := s.newCloudTransport()
	if err != nil {
		s.writeCloudSetupError(w, err)
		return
	}

	result, err := cloud.AddHSP(r.Context(), command)
	s.writeCloudCommandResponse(w, string(transport.CommandKindHSPAdd), cloud, result, err)
}

func (s *Server) handleCloudHSPPlay(w http.ResponseWriter, r *http.Request) {
	var command transport.HSPPlayCommand
	if err := decodeJSON(r, &command); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cloud, err := s.newCloudTransport()
	if err != nil {
		s.writeCloudSetupError(w, err)
		return
	}

	result, err := cloud.PlayHSP(r.Context(), command)
	s.writeCloudCommandResponse(w, string(transport.CommandKindHSPPlay), cloud, result, err)
}

func (s *Server) handleCloudStop(w http.ResponseWriter, r *http.Request) {
	command := transport.StopCommand{Reason: "manual_cloud_stop"}
	if err := decodeOptionalJSON(r, &command); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cloud, err := s.newCloudTransport()
	if err != nil {
		s.writeCloudSetupError(w, err)
		return
	}

	result, err := cloud.Stop(r.Context(), command)
	s.writeCloudCommandResponse(w, string(transport.CommandKindStop), cloud, result, err)
}

func (s *Server) newCloudTransport() (*transport.CloudRESTTransport, error) {
	settings, _ := s.store.Snapshot()
	if settings.Device.HSPDispatchOwner != config.DispatchOwnerCloudREST {
		return nil, errors.New("Cloud REST dispatch owner is not selected")
	}

	return transport.NewCloudRESTTransport(
		transport.CloudPrerequisites{
			ApplicationID: resolveCloudApplicationID(settings),
			ConnectionKey: settings.Device.HandyConnectionKey,
			FirmwareMajor: 4,
			APIMajor:      3,
			HSPAvailable:  true,
		},
		transport.CloudBuildOptions{ReverseDirection: settings.Motion.ReverseDirection},
		transport.CloudEndpointConfig{BaseURL: s.cloud.baseURL},
		s.cloud.client,
	)
}

func (s *Server) writeCloudCommandResponse(
	w http.ResponseWriter,
	reason string,
	cloud *transport.CloudRESTTransport,
	result transport.CommandResult,
	err error,
) {
	diagnostics := s.saveCloudDiagnostics(cloud.Diagnostics())
	s.traceCloudResult(reason, diagnostics, result)
	payload := cloudCommandResponse{
		Result:      transport.SafeCommandResult(result),
		Diagnostics: diagnostics,
	}
	status := http.StatusOK
	if err != nil {
		payload.Error = safeCloudErrorMessage(err)
		status = cloudCommandStatus(err, result)
	}
	writeJSON(w, status, payload)
}

func (s *Server) writeCloudSetupError(w http.ResponseWriter, err error) {
	diagnostics := s.saveCloudDiagnostics(cloudSetupDiagnostics(err))
	writeJSON(w, http.StatusBadRequest, cloudErrorResponse{
		Error:       safeCloudErrorMessage(err),
		Diagnostics: diagnostics,
	})
}

func (s *Server) saveCloudDiagnostics(diagnostics transport.TransportDiagnostics) transport.TransportDiagnostics {
	diagnostics = safeCloudDiagnostics(diagnostics)
	s.cloud.mu.Lock()
	defer s.cloud.mu.Unlock()
	s.cloud.diagnostics = diagnostics
	return diagnostics
}

func (s *Server) cloudDiagnostics() transport.TransportDiagnostics {
	s.cloud.mu.Lock()
	defer s.cloud.mu.Unlock()
	return safeCloudDiagnostics(s.cloud.diagnostics)
}

func (s *Server) traceCloudResult(
	reason string,
	diagnostics transport.TransportDiagnostics,
	result transport.CommandResult,
) {
	var command *transport.Command
	if diagnostics.LastCommand != nil {
		safeCommand := transport.SafeCommand(*diagnostics.LastCommand)
		command = &safeCommand
	}
	safeResult := transport.SafeCommandResult(result)
	s.traces.Add(diagnosticspkg.MotionTraceRow{
		Source:           cloudTraceSource,
		Reason:           reason,
		TransportCommand: command,
		TransportResult:  &safeResult,
	})
}

func resolveCloudApplicationID(settings config.Settings) string {
	if settings.Device.APIApplicationIDSource == config.ApplicationIDSourceDeveloperOverride {
		return strings.TrimSpace(settings.Device.APIApplicationIDOverride)
	}
	return config.BundledAPIApplicationID
}

func cloudCommandStatus(err error, result transport.CommandResult) int {
	if err == nil {
		return http.StatusOK
	}
	if isHSPUnavailable(err) || result.CommandID == "" || result.Status == "failed" {
		return http.StatusBadRequest
	}
	return http.StatusBadGateway
}

func safeCloudErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	var unavailable transport.HSPUnavailableError
	if errors.As(err, &unavailable) {
		return unavailable.Message
	}
	if strings.Contains(err.Error(), " returned HTTP ") {
		return "Cloud REST request failed; see diagnostics"
	}
	return err.Error()
}

func isHSPUnavailable(err error) bool {
	var unavailable transport.HSPUnavailableError
	return errors.As(err, &unavailable)
}

func safeCloudDiagnostics(diagnostics transport.TransportDiagnostics) transport.TransportDiagnostics {
	if diagnostics.Name == "" {
		diagnostics.Name = cloudTransportName
	}
	if diagnostics.PlaybackState == "" {
		diagnostics.PlaybackState = "unknown"
	}
	if diagnostics.LastCommand != nil {
		command := transport.SafeCommand(*diagnostics.LastCommand)
		diagnostics.LastCommand = &command
	}
	if diagnostics.LastResult != nil {
		result := transport.SafeCommandResult(*diagnostics.LastResult)
		diagnostics.LastResult = &result
	}
	if diagnostics.LastError != "" {
		diagnostics.LastError = "redacted"
	}
	return diagnostics
}

func defaultCloudDiagnostics() transport.TransportDiagnostics {
	return transport.TransportDiagnostics{
		Name:          cloudTransportName,
		PlaybackState: "unknown",
	}
}

func cloudSetupDiagnostics(err error) transport.TransportDiagnostics {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result := transport.CommandResult{
		Kind:        transport.CommandKindConnectionCheck,
		Transport:   cloudTransportName,
		OK:          false,
		Status:      "failed",
		Error:       safeCloudErrorMessage(err),
		CompletedAt: now,
	}
	return transport.TransportDiagnostics{
		Name:          cloudTransportName,
		PlaybackState: "unknown",
		LastResult:    &result,
		LastError:     safeCloudErrorMessage(err),
	}
}

func decodeOptionalJSON(r *http.Request, target any) error {
	defer func() {
		_ = r.Body.Close()
	}()

	data, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		return fmt.Errorf("decode JSON request: %w", err)
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil
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

func setEventStreamHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func writeStateEvent(w io.Writer, event transport.HSPStateEvent) error {
	if event.Event != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", cleanEventName(event.Event)); err != nil {
			return err
		}
	}
	for _, line := range strings.Split(string(event.Data), "\n") {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "\n")
	return err
}

func cleanEventName(eventName string) string {
	eventName = strings.ReplaceAll(eventName, "\r", " ")
	return strings.ReplaceAll(eventName, "\n", " ")
}
