package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const defaultCloudBaseURL = "https://www.handyfeeling.com"

// CloudEndpointConfig controls the Cloud REST endpoint paths used by the client.
type CloudEndpointConfig struct {
	BaseURL string
}

// ConnectionCheckResult is the safe result of a Cloud REST connection check.
type ConnectionCheckResult struct {
	OK            bool                 `json:"ok"`
	Status        string               `json:"status"`
	StatusCode    int                  `json:"status_code,omitempty"`
	HSPAvailable  bool                 `json:"hsp_available"`
	PlaybackState string               `json:"playback_state,omitempty"`
	LatencyMillis int64                `json:"latency_ms"`
	Diagnostics   TransportDiagnostics `json:"diagnostics"`
}

// HSPStateSnapshot is a safe snapshot of Cloud REST HSP state.
type HSPStateSnapshot struct {
	Available     bool            `json:"available"`
	PlaybackState string          `json:"playback_state,omitempty"`
	Raw           json.RawMessage `json:"raw,omitempty"`
}

// HSPStateEvent is one event from the HSP state event stream.
type HSPStateEvent struct {
	Event string          `json:"event,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// CloudRESTTransport dispatches HSP commands over Handy Cloud REST when called.
type CloudRESTTransport struct {
	builder *CloudRESTBuilder
	baseURL *url.URL
	client  *http.Client

	mu        sync.Mutex
	nextID    int
	diagnosis TransportDiagnostics
}

// NewCloudRESTTransport validates prerequisites and creates a live Cloud REST transport.
func NewCloudRESTTransport(prerequisites CloudPrerequisites, options CloudBuildOptions, endpoint CloudEndpointConfig, client *http.Client) (*CloudRESTTransport, error) {
	builder, err := NewCloudRESTBuilder(prerequisites, options)
	if err != nil {
		return nil, err
	}

	baseURL := strings.TrimSpace(endpoint.BaseURL)
	if baseURL == "" {
		baseURL = defaultCloudBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse Cloud REST base URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("Cloud REST base URL must include scheme and host")
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	return &CloudRESTTransport{
		builder: builder,
		baseURL: parsed,
		client:  client,
		nextID:  1,
		diagnosis: TransportDiagnostics{
			Name:          cloudRESTName,
			Connected:     false,
			PlaybackState: "unknown",
		},
	}, nil
}

// Stop sends a Cloud REST stop command.
func (t *CloudRESTTransport) Stop(ctx context.Context, _ StopCommand) (CommandResult, error) {
	return t.dispatch(ctx, t.builder.BuildStop())
}

// SetStrokeWindow sends a Cloud REST stroke-window command.
func (t *CloudRESTTransport) SetStrokeWindow(ctx context.Context, command StrokeWindowCommand) (CommandResult, error) {
	request, err := t.builder.BuildStrokeWindow(command)
	if err != nil {
		return t.recordBuildError(CommandKindStrokeWindow, err), err
	}
	return t.dispatch(ctx, request)
}

// AddHSP sends a Cloud REST HSP add command.
func (t *CloudRESTTransport) AddHSP(ctx context.Context, command HSPAddCommand) (CommandResult, error) {
	request, err := t.builder.BuildHSPAdd(command)
	if err != nil {
		return t.recordBuildError(CommandKindHSPAdd, err), err
	}
	return t.dispatch(ctx, request)
}

// PlayHSP sends a Cloud REST HSP play command.
func (t *CloudRESTTransport) PlayHSP(ctx context.Context, command HSPPlayCommand) (CommandResult, error) {
	request, err := t.builder.BuildHSPPlay(command)
	if err != nil {
		return t.recordBuildError(CommandKindHSPPlay, err), err
	}
	return t.dispatch(ctx, request)
}

// CheckConnection probes Cloud REST HSP state without sending motion.
func (t *CloudRESTTransport) CheckConnection(ctx context.Context) (ConnectionCheckResult, error) {
	request := t.builder.BuildConnectionCheck()
	result, body, err := t.dispatchWithBody(ctx, request)
	snapshot := stateSnapshotFromBody(body)
	check := ConnectionCheckResult{
		OK:            result.OK && snapshot.Available,
		Status:        result.Status,
		StatusCode:    statusCodeFromStatus(result.Status),
		HSPAvailable:  result.OK && snapshot.Available,
		PlaybackState: snapshot.PlaybackState,
		LatencyMillis: result.LatencyMillis,
		Diagnostics:   t.Diagnostics(),
	}
	if err != nil {
		return check, err
	}
	if !snapshot.Available {
		err := hspUnavailable("hsp_unavailable", "hsp", "HSP is unavailable for this device/API state")
		t.recordError(CommandKindConnectionCheck, err)
		check.Diagnostics = t.Diagnostics()
		return check, err
	}
	return check, nil
}

// ReadState reads Cloud REST HSP state without sending motion.
func (t *CloudRESTTransport) ReadState(ctx context.Context) (HSPStateSnapshot, CommandResult, error) {
	result, body, err := t.dispatchWithBody(ctx, t.builder.BuildHSPState())
	return stateSnapshotFromBody(body), result, err
}

// ListenStateEvents reads an HSP server-sent event stream until the context ends.
func (t *CloudRESTTransport) ListenStateEvents(ctx context.Context, onEvent func(HSPStateEvent)) error {
	request := t.builder.BuildHSPEvents()
	httpRequest, err := t.newHTTPRequest(ctx, request)
	if err != nil {
		t.recordError(CommandKindHSPEvents, err)
		return err
	}
	httpRequest.Header.Set("Accept", "text/event-stream")

	start := time.Now()
	response, err := t.client.Do(httpRequest)
	if err != nil {
		t.recordError(CommandKindHSPEvents, err)
		return err
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		err := fmt.Errorf("Cloud REST HSP events returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
		t.recordHTTPResult(request, response.StatusCode, time.Since(start), err)
		return err
	}
	t.recordHTTPResult(request, response.StatusCode, time.Since(start), nil)

	scanner := bufio.NewScanner(response.Body)
	var eventName string
	var dataLines []string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			emitStateEvent(eventName, dataLines, onEvent)
			eventName = ""
			dataLines = nil
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		t.recordError(CommandKindHSPEvents, err)
		return err
	}
	emitStateEvent(eventName, dataLines, onEvent)
	return nil
}

// Diagnostics returns a safe Cloud REST diagnostics snapshot.
func (t *CloudRESTTransport) Diagnostics() TransportDiagnostics {
	t.mu.Lock()
	defer t.mu.Unlock()

	diagnostics := t.diagnosis
	if diagnostics.LastCommand != nil {
		command := SafeCommand(*diagnostics.LastCommand)
		diagnostics.LastCommand = &command
	}
	if diagnostics.LastResult != nil {
		result := SafeCommandResult(*diagnostics.LastResult)
		diagnostics.LastResult = &result
	}
	if diagnostics.LastError != "" {
		diagnostics.LastError = "redacted"
	}
	return diagnostics
}

func (t *CloudRESTTransport) dispatch(ctx context.Context, request CloudRequest) (CommandResult, error) {
	result, _, err := t.dispatchWithBody(ctx, request)
	return result, err
}

func (t *CloudRESTTransport) dispatchWithBody(ctx context.Context, request CloudRequest) (CommandResult, []byte, error) {
	httpRequest, err := t.newHTTPRequest(ctx, request)
	if err != nil {
		return t.recordBuildError(CommandKind(request.Operation), err), nil, err
	}

	start := time.Now()
	response, err := t.client.Do(httpRequest)
	if err != nil {
		result := t.recordHTTPResult(request, 0, time.Since(start), err)
		return result, nil, err
	}
	defer func() {
		_ = response.Body.Close()
	}()

	body, readErr := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if readErr != nil {
		result := t.recordHTTPResult(request, response.StatusCode, time.Since(start), readErr)
		return result, body, readErr
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		err := fmt.Errorf("Cloud REST %s returned HTTP %d: %s", request.Operation, response.StatusCode, strings.TrimSpace(string(body)))
		result := t.recordHTTPResult(request, response.StatusCode, time.Since(start), err)
		return result, body, err
	}

	result := t.recordHTTPResult(request, response.StatusCode, time.Since(start), nil)
	return result, body, nil
}

func (t *CloudRESTTransport) newHTTPRequest(ctx context.Context, request CloudRequest) (*http.Request, error) {
	endpoint := t.baseURL.ResolveReference(&url.URL{Path: request.Path})

	var body io.Reader
	if request.Body != nil {
		data, err := json.Marshal(request.Body)
		if err != nil {
			return nil, fmt.Errorf("encode Cloud REST request body: %w", err)
		}
		body = bytes.NewReader(data)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, request.Method, endpoint.String(), body)
	if err != nil {
		return nil, fmt.Errorf("create Cloud REST request: %w", err)
	}
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("X-Application-ID", request.Auth.ApplicationID)
	httpRequest.Header.Set("X-Connection-Key", request.Auth.ConnectionKey)
	if request.Body != nil {
		httpRequest.Header.Set("Content-Type", "application/json")
	}
	return httpRequest, nil
}

func (t *CloudRESTTransport) recordBuildError(kind CommandKind, err error) CommandResult {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.diagnosis.LastError = err.Error()
	result := CommandResult{
		Kind:        kind,
		Transport:   cloudRESTName,
		OK:          false,
		Status:      "failed",
		Error:       err.Error(),
		CompletedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	t.diagnosis.LastResult = &result
	return result
}

func (t *CloudRESTTransport) recordError(kind CommandKind, err error) {
	_ = t.recordBuildError(kind, err)
}

func (t *CloudRESTTransport) recordHTTPResult(request CloudRequest, statusCode int, elapsed time.Duration, err error) CommandResult {
	t.mu.Lock()
	defer t.mu.Unlock()

	command := commandFromCloudRequest(request)
	command.ID = fmt.Sprintf("cloud-%06d", t.nextID)
	command.IssuedAt = time.Now().UTC().Format(time.RFC3339Nano)
	t.nextID++
	result := CommandResult{
		CommandID:     command.ID,
		Kind:          command.Kind,
		Transport:     cloudRESTName,
		OK:            err == nil,
		Status:        fmt.Sprintf("http_%d", statusCode),
		LatencyMillis: elapsed.Milliseconds(),
		CompletedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	if statusCode == 0 {
		result.Status = "network_error"
	}
	if err != nil {
		result.Error = err.Error()
		t.diagnosis.LastError = err.Error()
	} else {
		t.diagnosis.LastError = ""
	}

	t.diagnosis.Name = cloudRESTName
	t.diagnosis.Connected = err == nil
	t.diagnosis.CommandCount++
	t.diagnosis.LastLatencyMillis = result.LatencyMillis
	t.diagnosis.LastCommand = &command
	t.diagnosis.LastResult = &result
	return result
}

func commandFromCloudRequest(request CloudRequest) Command {
	command := Command{
		Kind: CommandKind(request.Operation),
	}
	switch body := request.Body.(type) {
	case cloudStrokeWindowBody:
		command.StrokeWindow = &StrokeWindowCommand{
			MinPercent: body.Min,
			MaxPercent: body.Max,
		}
	case cloudHSPAddBody:
		points := make([]TimedPoint, len(body.Points))
		for index, point := range body.Points {
			points[index] = TimedPoint{
				PositionPercent: point.X,
				TimeMillis:      point.T,
			}
		}
		command.HSPAdd = &HSPAddCommand{
			StreamID: body.StreamID,
			Points:   points,
		}
	case cloudHSPPlayBody:
		command.HSPPlay = &HSPPlayCommand{
			StreamID:        body.StreamID,
			StartTimeMillis: body.StartTimeMillis,
		}
	case cloudStopBody:
		command.Stop = &StopCommand{Reason: body.Reason}
	}
	return command
}

func stateSnapshotFromBody(body []byte) HSPStateSnapshot {
	snapshot := HSPStateSnapshot{
		Available: true,
		Raw:       cloneRaw(body),
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return snapshot
	}
	if available, ok := payload["hsp_available"].(bool); ok {
		snapshot.Available = available
	}
	if available, ok := payload["available"].(bool); ok {
		snapshot.Available = available
	}
	if state, ok := payload["playback_state"].(string); ok {
		snapshot.PlaybackState = state
	}
	if state, ok := payload["state"].(string); ok && snapshot.PlaybackState == "" {
		snapshot.PlaybackState = state
	}
	return snapshot
}

func statusCodeFromStatus(status string) int {
	var code int
	if _, err := fmt.Sscanf(status, "http_%d", &code); err == nil {
		return code
	}
	return 0
}

func emitStateEvent(eventName string, dataLines []string, onEvent func(HSPStateEvent)) {
	if onEvent == nil || len(dataLines) == 0 {
		return
	}
	data := strings.Join(dataLines, "\n")
	onEvent(HSPStateEvent{
		Event: eventName,
		Data:  json.RawMessage(data),
	})
}

func cloneRaw(data []byte) json.RawMessage {
	if len(data) == 0 {
		return nil
	}
	clone := make([]byte, len(data))
	copy(clone, data)
	return clone
}
