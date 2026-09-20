package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	diagnosticspkg "github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

const bluetoothTraceSource = "manual_browser_bluetooth"

type bluetoothRuntime struct {
	bridge      *transport.BrowserBluetoothBridge
	gateway     bluetoothGatewayRuntime
	mu          sync.Mutex
	diagnostics transport.TransportDiagnostics
}

type bluetoothStateResponse struct {
	Error       string                                   `json:"error,omitempty"`
	State       transport.HSPStateSnapshot               `json:"state"`
	Result      transport.CommandResult                  `json:"result"`
	Diagnostics transport.TransportDiagnostics           `json:"diagnostics"`
	Bridge      transport.BrowserBluetoothBridgeSnapshot `json:"bridge"`
}

type bluetoothErrorResponse struct {
	Error       string                                   `json:"error"`
	Diagnostics transport.TransportDiagnostics           `json:"diagnostics"`
	Bridge      transport.BrowserBluetoothBridgeSnapshot `json:"bridge"`
}

type bluetoothStatusResponse struct {
	Gateway       bluetoothGatewaySnapshot                 `json:"gateway"`
	Status        string                                   `json:"status"`
	DispatchOwner string                                   `json:"dispatch_owner"`
	Bluetooth     transport.BrowserBluetoothBridgeSnapshot `json:"bluetooth"`
	Diagnostics   transport.TransportDiagnostics           `json:"diagnostics"`
	Warning       string                                   `json:"warning,omitempty"`
}

type bluetoothCommandsResponse struct {
	Status    string                                    `json:"status"`
	Commands  []transport.BrowserBluetoothBridgeCommand `json:"commands"`
	Bluetooth transport.BrowserBluetoothBridgeSnapshot  `json:"bluetooth"`
}

type bluetoothAckRequest struct {
	ClientID      string         `json:"client_id"`
	ID            string         `json:"id"`
	OK            bool           `json:"ok"`
	Status        string         `json:"status,omitempty"`
	ElapsedMillis float64        `json:"elapsed_ms,omitempty"`
	Error         string         `json:"error,omitempty"`
	Response      map[string]any `json:"response,omitempty"`
}

type bluetoothDisconnectRequest struct {
	ClientID string `json:"client_id"`
	Message  string `json:"message,omitempty"`
}

func newBluetoothRuntime(runtime Runtime) bluetoothRuntime {
	bridge := runtime.BrowserBluetoothBridge
	if bridge == nil {
		bridge = transport.NewBrowserBluetoothBridge()
	}
	return bluetoothRuntime{
		bridge:      bridge,
		diagnostics: defaultBluetoothDiagnostics(),
	}
}

func (s *Server) handleBluetoothDiagnostics(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"diagnostics": s.bluetoothDiagnostics(),
		"bridge":      s.bluetoothSnapshot(),
	})
}

func (s *Server) handleBluetoothStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var status transport.BrowserBluetoothClientStatus
		if err := decodeJSON(r, &status); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if s.auth.authenticationRequired() {
			lease, unlock, ok := s.lockBluetoothGateway(w, r, gatewayClientID(status.ClientID))
			if !ok {
				return
			}
			status.ClientID = lease.transportID
			s.bluetooth.bridge.UpdateClient(status)
			if status.Connected != nil && !*status.Connected {
				lease.cancel()
			}
			unlock()
			s.checkBluetoothGatewayLifetime("")
		} else {
			s.bluetooth.bridge.UpdateClient(status)
		}
	}
	writeJSON(w, http.StatusOK, s.bluetoothStatus(r))
}

func (s *Server) handleBluetoothConnect(w http.ResponseWriter, r *http.Request) {
	var status transport.BrowserBluetoothClientStatus
	if err := decodeJSON(r, &status); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(status.ClientID) == "" {
		writeError(w, http.StatusBadRequest, errors.New("missing browser Bluetooth client id"))
		return
	}
	if !s.requireController(w, r) {
		return
	}
	if !s.registerBluetoothGateway(w, r, status) {
		return
	}
	writeJSON(w, http.StatusOK, s.bluetoothStatus(r))
}

func (s *Server) handleBluetoothDisconnect(w http.ResponseWriter, r *http.Request) {
	var request bluetoothDisconnectRequest
	if err := decodeOptionalJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	finishGateway, ok := s.beginBluetoothGatewayDisconnect(w, r, request)
	if !ok {
		return
	}
	defer finishGateway()
	finishStop, _ := s.beginGlobalStop("bluetooth_disconnected", r.Context())
	defer finishStop()
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 15*time.Second)
	stopErr := s.stopAndClearMotionEngine(stopCtx, "bluetooth_disconnected")
	cancel()
	finishGateway()
	response := s.bluetoothStatus(r)
	if stopErr != nil {
		response.Warning = "Bluetooth disconnected, but the active motion Stop could not be confirmed: " +
			s.safeMotionErrorMessage(stopErr)
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleBluetoothCommands(w http.ResponseWriter, r *http.Request) {
	clientID := gatewayClientID(r.URL.Query().Get("client_id"))
	if clientID == "" {
		writeError(w, http.StatusBadRequest, errors.New("missing browser Bluetooth client id"))
		return
	}
	wait := 4 * time.Second
	if rawWait := strings.TrimSpace(r.URL.Query().Get("wait")); rawWait != "" {
		seconds, err := strconv.ParseFloat(rawWait, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid Bluetooth command wait"))
			return
		}
		wait = time.Duration(seconds * float64(time.Second))
	}
	ctx, cancel := s.requestLifecycleContext(r.Context())
	defer cancel()
	var lease *bluetoothGatewayLease
	if s.auth.authenticationRequired() {
		var unlock func()
		var ok bool
		lease, unlock, ok = s.lockBluetoothGateway(w, r, clientID)
		if !ok {
			return
		}
		clientID = lease.transportID
		unlock()
		endGateway := context.AfterFunc(lease.ctx, cancel)
		defer endGateway()
		if lease.ctx.Err() != nil {
			cancel()
		}
	}
	commands, err := s.bluetooth.bridge.NextCommands(ctx, clientID, wait)
	if err != nil && !errors.Is(err, context.Canceled) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if lease != nil {
		s.bluetooth.gateway.mu.Lock()
		current := s.bluetooth.gateway.lease == lease && lease.ctx.Err() == nil && ctx.Err() == nil
		s.bluetooth.gateway.mu.Unlock()
		if !current {
			writeError(w, http.StatusConflict, errors.New("bluetooth gateway access ended"))
			return
		}
	}
	writeJSON(w, http.StatusOK, bluetoothCommandsResponse{
		Status:    "success",
		Commands:  commands,
		Bluetooth: s.bluetoothSnapshot(),
	})
}

func (s *Server) handleBluetoothAck(w http.ResponseWriter, r *http.Request) {
	var request bluetoothAckRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(request.ClientID) == "" {
		writeError(w, http.StatusBadRequest, errors.New("missing browser Bluetooth client id"))
		return
	}
	if strings.TrimSpace(request.ID) == "" {
		writeError(w, http.StatusBadRequest, errors.New("missing Bluetooth command id"))
		return
	}
	clientID := gatewayClientID(request.ClientID)
	unlock := func() {}
	if s.auth.authenticationRequired() {
		lease, release, ok := s.lockBluetoothGateway(w, r, clientID)
		if !ok {
			return
		}
		clientID, unlock = lease.transportID, release
	}
	s.bluetooth.bridge.Acknowledge(clientID, transport.BrowserBluetoothBridgeAck{
		ID:            request.ID,
		OK:            request.OK,
		Status:        request.Status,
		ElapsedMillis: request.ElapsedMillis,
		Error:         request.Error,
		Response:      request.Response,
	})
	unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "success",
		"bluetooth": s.bluetoothSnapshot(),
	})
}

func (s *Server) handleBluetoothConnectionCheck(w http.ResponseWriter, r *http.Request) {
	bluetooth, err := s.newBluetoothTransport()
	if err != nil {
		s.writeBluetoothSetupError(w, err)
		return
	}

	check, err := bluetooth.CheckConnection(r.Context())
	diagnostics := s.saveBluetoothDiagnostics(bluetooth.Diagnostics())
	check.Diagnostics = diagnostics
	if err != nil {
		writeJSON(w, bluetoothCommandStatus(err, check.Diagnostics.LastResult), bluetoothErrorResponse{
			Error:       safeBluetoothErrorMessage(err),
			Diagnostics: diagnostics,
			Bridge:      s.bluetoothSnapshot(),
		})
		return
	}
	writeJSON(w, http.StatusOK, check)
}

func (s *Server) handleBluetoothState(w http.ResponseWriter, r *http.Request) {
	bluetooth, err := s.newBluetoothTransport()
	if err != nil {
		s.writeBluetoothSetupError(w, err)
		return
	}

	state, result, err := bluetooth.ReadState(r.Context())
	diagnostics := s.saveBluetoothDiagnostics(bluetooth.Diagnostics())
	payload := bluetoothStateResponse{
		State:       state,
		Result:      transport.SafeCommandResult(result),
		Diagnostics: diagnostics,
		Bridge:      s.bluetoothSnapshot(),
	}
	if err != nil {
		payload.Error = safeBluetoothErrorMessage(err)
		writeJSON(w, bluetoothCommandStatus(err, &result), payload)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleBluetoothEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming responses are unavailable"))
		return
	}
	setEventStreamHeaders(w)
	w.WriteHeader(http.StatusOK)
	_ = writeBluetoothSnapshotEvent(w, s.bluetoothSnapshot())
	flusher.Flush()

	ctx, cancel := s.requestLifecycleContext(r.Context())
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := writeBluetoothSnapshotEvent(w, s.bluetoothSnapshot()); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) handleBluetoothStop(w http.ResponseWriter, r *http.Request) {
	command := transport.StopCommand{Reason: "manual_bluetooth_stop"}
	if err := decodeOptionalJSON(r, &command); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	reason := strings.TrimSpace(command.Reason)
	if reason == "" {
		reason = "manual_bluetooth_stop"
	}
	s.handleEmergencyStop(w, r, reason)
}

func (s *Server) newBluetoothTransport() (*transport.BrowserBluetoothTransport, error) {
	settings, _ := s.store.Snapshot()
	if settings.Device.HSPDispatchOwner != config.DispatchOwnerBrowserBluetooth {
		return nil, errors.New("browser Bluetooth dispatch owner is not selected")
	}
	return transport.NewBrowserBluetoothTransport(
		s.bluetooth.bridge,
		transport.BrowserBluetoothOptions{ReverseDirection: settings.Motion.ReverseDirection},
	)
}

func (s *Server) writeBluetoothSetupError(w http.ResponseWriter, err error) {
	diagnostics := s.saveBluetoothDiagnostics(bluetoothSetupDiagnostics(err))
	writeJSON(w, http.StatusBadRequest, bluetoothErrorResponse{
		Error:       safeBluetoothErrorMessage(err),
		Diagnostics: diagnostics,
		Bridge:      s.bluetoothSnapshot(),
	})
}

func (s *Server) saveBluetoothDiagnostics(diagnostics transport.TransportDiagnostics) transport.TransportDiagnostics {
	diagnostics = safeBluetoothDiagnostics(diagnostics)
	s.bluetooth.mu.Lock()
	defer s.bluetooth.mu.Unlock()
	s.bluetooth.diagnostics = diagnostics
	return diagnostics
}

func (s *Server) bluetoothDiagnostics() transport.TransportDiagnostics {
	s.bluetooth.mu.Lock()
	defer s.bluetooth.mu.Unlock()
	return safeBluetoothDiagnostics(s.bluetooth.diagnostics)
}

func (s *Server) bluetoothStatus(r *http.Request) bluetoothStatusResponse {
	settings, _ := s.store.Snapshot()
	return bluetoothStatusResponse{
		Gateway:       s.bluetoothGatewaySnapshot(r),
		Status:        "success",
		DispatchOwner: settings.Device.HSPDispatchOwner,
		Bluetooth:     s.clientBluetoothSnapshot(r),
		Diagnostics:   s.clientTransportDiagnostics(r, s.bluetoothDiagnostics()),
	}
}

func (s *Server) traceBluetoothResult(
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
		Source:           bluetoothTraceSource,
		Reason:           reason,
		TransportCommand: command,
		TransportResult:  &safeResult,
	})
}

func bluetoothCommandStatus(err error, result *transport.CommandResult) int {
	if err == nil {
		return http.StatusOK
	}
	var bluetoothErr transport.BrowserBluetoothError
	if errors.As(err, &bluetoothErr) {
		switch bluetoothErr.Status {
		case "browser_unsupported", "bridge_unavailable", "bridge_stale", "bridge_canceled", "bridge_timeout":
			return http.StatusBadRequest
		default:
			return http.StatusBadGateway
		}
	}
	if result == nil || result.CommandID == "" || result.Status == "failed" {
		return http.StatusBadRequest
	}
	return http.StatusBadGateway
}

func safeBluetoothErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func safeBluetoothDiagnostics(diagnostics transport.TransportDiagnostics) transport.TransportDiagnostics {
	if diagnostics.Name == "" {
		diagnostics.Name = transport.BrowserBluetoothName
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

func defaultBluetoothDiagnostics() transport.TransportDiagnostics {
	return transport.TransportDiagnostics{
		Name:          transport.BrowserBluetoothName,
		PlaybackState: "unknown",
	}
}

func bluetoothSetupDiagnostics(err error) transport.TransportDiagnostics {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result := transport.CommandResult{
		Kind:        transport.CommandKindConnectionCheck,
		Transport:   transport.BrowserBluetoothName,
		OK:          false,
		Status:      "failed",
		Error:       safeBluetoothErrorMessage(err),
		CompletedAt: now,
	}
	return transport.TransportDiagnostics{
		Name:          transport.BrowserBluetoothName,
		PlaybackState: "unknown",
		LastResult:    &result,
		LastError:     safeBluetoothErrorMessage(err),
	}
}

func writeBluetoothSnapshotEvent(w http.ResponseWriter, snapshot transport.BrowserBluetoothBridgeSnapshot) error {
	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode Bluetooth event: %w", err)
	}
	if _, err := w.Write([]byte("event: bluetooth\n")); err != nil {
		return err
	}
	if _, err := w.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	_, err = w.Write([]byte("\n\n"))
	return err
}
