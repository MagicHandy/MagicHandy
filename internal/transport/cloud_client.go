package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultCloudBaseURL      = "https://www.handyfeeling.com/api/handy-rest/v3/"
	serverTimeSyncTTLSeconds = 300
	cloudMinimumBufferedLead = 1500 * time.Millisecond
)

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
	Message       string               `json:"message,omitempty"`
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

	mu         sync.Mutex
	hspMu      sync.Mutex
	motionGate motionCommandGate
	nextID     int
	diagnosis  TransportDiagnostics

	activeStreamID         string
	nextHSPStreamID        uint32
	hspPointCount          int
	playbackStartedAt      time.Time
	serverTimeOffsetMillis int64
	serverTimeSyncedAt     time.Time
}

// MotionSamplingCapabilities reports API v3's whole-percent HSP endpoint
// resolution. The shared engine uses this only to reduce redundant wire knots;
// CloudRESTTransport remains a mapping/dispatch owner.
func (*CloudRESTTransport) MotionSamplingCapabilities() MotionSamplingCapabilities {
	return MotionSamplingCapabilities{
		PositionResolutionPercent: 1,
		MaximumPointsPerAppend:    maximumCloudHSPAddPoints,
	}
}

// MotionTimingCapabilities reserves enough accepted HSP coverage for Cloud
// round trips and the engine's dispatch tick. A live trace observed a 958 ms
// add response; 1.5 seconds keeps a useful margin without making retargets
// needlessly sluggish.
func (*CloudRESTTransport) MotionTimingCapabilities() MotionTimingCapabilities {
	return MotionTimingCapabilities{MinimumBufferedLead: cloudMinimumBufferedLead}
}

// PlaybackStartTime reports the estimated stream origin at the successful
// Play request midpoint. This excludes Cloud setup and prebuffer latency from
// the engine's playback clock.
func (t *CloudRESTTransport) PlaybackStartTime() time.Time {
	t.hspMu.Lock()
	defer t.hspMu.Unlock()
	return t.playbackStartedAt
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
		return nil, fmt.Errorf("parse cloud REST base URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("cloud REST base URL must include scheme and host")
	}
	if parsed.Path == "" {
		parsed.Path = "/"
	} else if !strings.HasSuffix(parsed.Path, "/") {
		parsed.Path += "/"
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
	t.motionGate.beginStop()
	defer t.motionGate.endStop()
	t.hspMu.Lock()
	defer t.hspMu.Unlock()

	result, err := t.dispatch(ctx, t.builder.BuildStop())
	t.playbackStartedAt = time.Time{}
	if err == nil {
		t.activeStreamID = ""
		t.hspPointCount = 0
	}
	return result, err
}

// SetStrokeWindow sends a Cloud REST stroke-window command.
func (t *CloudRESTTransport) SetStrokeWindow(ctx context.Context, command StrokeWindowCommand) (CommandResult, error) {
	admission, err := t.motionGate.admit()
	if err != nil {
		return t.recordBuildError(CommandKindStrokeWindow, err), err
	}
	t.hspMu.Lock()
	defer t.hspMu.Unlock()
	if err := t.motionGate.validate(admission); err != nil {
		return t.recordBuildError(CommandKindStrokeWindow, err), err
	}

	request, err := t.builder.BuildStrokeWindow(command)
	if err != nil {
		return t.recordBuildError(CommandKindStrokeWindow, err), err
	}
	result, err := t.dispatch(ctx, request)
	if err == nil {
		t.builder.options.ReverseDirection = command.ReverseDirection
	}
	return result, err
}

// AppendPoints sends timed points through Cloud REST HSP.
func (t *CloudRESTTransport) AppendPoints(ctx context.Context, command AppendPointsCommand) (CommandResult, error) {
	admission, err := t.motionGate.admit()
	if err != nil {
		return t.recordBuildError(CommandKindPointsAdd, err), err
	}
	t.hspMu.Lock()
	defer t.hspMu.Unlock()
	if err := t.motionGate.validate(admission); err != nil {
		return t.recordBuildError(CommandKindPointsAdd, err), err
	}

	streamID, err := cleanStreamID(command.StreamID)
	if err != nil {
		return t.recordBuildError(CommandKindPointsAdd, err), err
	}
	if streamID != t.activeStreamID {
		setup := HSPSetupCommand{StreamID: t.nextSetupStreamIDLocked()}
		request, err := t.builder.BuildHSPSetup(setup)
		if err != nil {
			return t.recordBuildError(CommandKindHSPSetup, err), err
		}
		if result, err := t.dispatch(ctx, request); err != nil {
			return result, err
		}
		t.activeStreamID = streamID
		t.hspPointCount = 0
		t.playbackStartedAt = time.Time{}
	}

	tailPointStreamIndex := t.hspPointCount + len(command.Points)
	request, err := t.builder.buildHSPAdd(command, t.hspPointCount == 0, tailPointStreamIndex)
	if err != nil {
		return t.recordBuildError(CommandKindPointsAdd, err), err
	}
	result, err := t.dispatch(ctx, request)
	if err == nil {
		t.hspPointCount = tailPointStreamIndex
	}
	return result, err
}

// Play starts or resumes timed-point playback through Cloud REST HSP.
func (t *CloudRESTTransport) Play(ctx context.Context, command PlayCommand) (CommandResult, error) {
	admission, err := t.motionGate.admit()
	if err != nil {
		return t.recordBuildError(CommandKindPointsPlay, err), err
	}
	t.hspMu.Lock()
	defer t.hspMu.Unlock()
	if err := t.motionGate.validate(admission); err != nil {
		return t.recordBuildError(CommandKindPointsPlay, err), err
	}

	command.ServerTimeMillis = t.estimatedServerTimeMillisLocked(ctx)
	request, err := t.builder.BuildHSPPlay(command)
	if err != nil {
		return t.recordBuildError(CommandKindPointsPlay, err), err
	}
	dispatchedAt := time.Now()
	result, err := t.dispatch(ctx, request)
	if err != nil {
		t.playbackStartedAt = time.Time{}
		return result, err
	}
	acknowledgedAt := time.Now()
	t.playbackStartedAt = dispatchedAt.Add(acknowledgedAt.Sub(dispatchedAt) / 2).
		Add(-time.Duration(command.StartTimeMillis) * time.Millisecond)
	return result, nil
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
		check.Message = err.Message
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
	httpRequest, err := t.newStateEventsRequest(ctx, request)
	if err != nil {
		safeErr := safeCloudEventStreamError(err)
		t.recordError(CommandKindHSPEvents, safeErr)
		return safeErr
	}
	httpRequest.Header.Set("Accept", "text/event-stream")

	start := time.Now()
	response, err := t.client.Do(httpRequest)
	if err != nil {
		safeErr := safeCloudEventStreamError(err)
		t.recordError(CommandKindHSPEvents, safeErr)
		return safeErr
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		err := fmt.Errorf("cloud REST HSP events returned HTTP %d", response.StatusCode)
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
		safeErr := safeCloudEventStreamError(err)
		t.recordError(CommandKindHSPEvents, safeErr)
		return safeErr
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
		err := fmt.Errorf("cloud REST %s returned HTTP %d", request.Operation, response.StatusCode)
		result := t.recordHTTPResult(request, response.StatusCode, time.Since(start), err)
		t.rememberPlaybackState(body)
		return result, body, err
	}
	if err := cloudAPIResponseError(request.Operation, body); err != nil {
		result := t.recordHTTPResult(request, response.StatusCode, time.Since(start), err)
		return result, body, err
	}

	result := t.recordHTTPResult(request, response.StatusCode, time.Since(start), nil)
	t.rememberPlaybackState(body)
	return result, body, nil
}

func cloudAPIResponseError(operation string, body []byte) error {
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || len(envelope.Error) == 0 {
		return nil
	}
	raw := strings.TrimSpace(string(envelope.Error))
	if raw == "" || raw == "null" || raw == "false" {
		return nil
	}
	if code := cloudAPIErrorCode(envelope.Error); code != "" {
		return fmt.Errorf("cloud REST %s rejected request (%s)", operation, code)
	}
	return fmt.Errorf("cloud REST %s rejected request", operation)
}

func cloudAPIErrorCode(raw json.RawMessage) string {
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return ""
	}
	for _, key := range []string{"code", "name", "type"} {
		value, ok := object[key].(string)
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 64 {
			continue
		}
		valid := true
		for _, character := range value {
			if (character >= 'a' && character <= 'z') ||
				(character >= 'A' && character <= 'Z') ||
				(character >= '0' && character <= '9') ||
				character == '_' || character == '-' || character == '.' {
				continue
			}
			valid = false
			break
		}
		if valid {
			return value
		}
	}
	return ""
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
	httpRequest.Header.Set("X-Api-Key", request.Auth.ApplicationID)
	httpRequest.Header.Set("X-Connection-Key", request.Auth.ConnectionKey)
	if request.Body != nil {
		httpRequest.Header.Set("Content-Type", "application/json")
	}
	return httpRequest, nil
}

func (t *CloudRESTTransport) newStateEventsRequest(ctx context.Context, request CloudRequest) (*http.Request, error) {
	endpoint := t.baseURL.ResolveReference(&url.URL{Path: request.Path})
	query := endpoint.Query()
	query.Set("ck", request.Auth.ConnectionKey)
	query.Set("apikey", request.Auth.ApplicationID)
	query.Set("events", strings.Join(cloudHSPStateEvents, ","))
	endpoint.RawQuery = query.Encode()

	httpRequest, err := http.NewRequestWithContext(ctx, request.Method, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create Cloud REST request: %w", err)
	}
	httpRequest.Header.Set("Accept", "application/json")
	return httpRequest, nil
}

func (t *CloudRESTTransport) nextSetupStreamIDLocked() uint32 {
	if t.nextHSPStreamID == ^uint32(0) {
		t.nextHSPStreamID = 1
		return t.nextHSPStreamID
	}
	t.nextHSPStreamID++
	if t.nextHSPStreamID == 0 {
		t.nextHSPStreamID = 1
	}
	return t.nextHSPStreamID
}

func (t *CloudRESTTransport) estimatedServerTimeMillisLocked(ctx context.Context) int64 {
	now := time.Now().UTC()
	if t.serverTimeSyncedAt.IsZero() || now.Sub(t.serverTimeSyncedAt) > serverTimeSyncTTLSeconds*time.Second {
		_ = t.refreshServerTimeOffsetLocked(ctx, now)
	}
	if t.serverTimeSyncedAt.IsZero() {
		return unixMillis(now)
	}
	return unixMillis(now) + t.serverTimeOffsetMillis
}

func (t *CloudRESTTransport) refreshServerTimeOffsetLocked(ctx context.Context, started time.Time) error {
	endpoint := t.baseURL.ResolveReference(&url.URL{Path: "servertime"})
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("create Cloud REST server-time request: %w", err)
	}

	response, err := t.client.Do(request)
	if err != nil {
		return err
	}
	defer func() {
		_ = response.Body.Close()
	}()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("cloud REST server time returned HTTP %d", response.StatusCode)
	}
	serverTimeMillis, ok := parseServerTimeMillis(body)
	if !ok {
		return errors.New("cloud REST server time response did not include a recognized timestamp")
	}
	ended := time.Now().UTC()
	localMidpoint := (unixMillis(started) + unixMillis(ended)) / 2
	t.serverTimeOffsetMillis = serverTimeMillis - localMidpoint
	t.serverTimeSyncedAt = ended
	return nil
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

func (t *CloudRESTTransport) rememberPlaybackState(body []byte) {
	snapshot := stateSnapshotFromBody(body)
	if snapshot.PlaybackState == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.diagnosis.PlaybackState = snapshot.PlaybackState
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
	if request.command != nil {
		return cloneCommand(*request.command)
	}
	command := Command{
		Kind: CommandKind(request.Operation),
	}
	switch body := request.Body.(type) {
	case cloudStrokeWindowBody:
		command.StrokeWindow = &StrokeWindowCommand{
			MinPercent: fractionPercent(body.Min),
			MaxPercent: fractionPercent(body.Max),
		}
	case cloudHSPSetupBody:
		command.HSPSetup = &HSPSetupCommand{
			StreamID: body.StreamID,
		}
	case cloudHSPAddBody:
		points := make([]TimedPoint, len(body.Points))
		for index, point := range body.Points {
			points[index] = TimedPoint{
				PositionPercent: float64(point.X),
				TimeMillis:      point.T,
			}
		}
		command.PointsAdd = &AppendPointsCommand{
			StreamID: body.StreamID,
			Points:   points,
		}
	case cloudHSPPlayBody:
		command.PointsPlay = &PlayCommand{
			StreamID:         body.StreamID,
			StartTimeMillis:  body.StartTimeMillis,
			ServerTimeMillis: body.ServerTimeMillis,
		}
	case cloudStopBody:
		command.Stop = &StopCommand{Reason: "stop"}
	}
	return command
}

func stateSnapshotFromBody(body []byte) HSPStateSnapshot {
	snapshot := HSPStateSnapshot{
		Raw: cloneRaw(body),
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return snapshot
	}
	availableSignal := false
	unavailableSignal := false
	for _, candidate := range statePayloadCandidates(payload) {
		available, unavailable := stateAvailabilitySignals(candidate)
		availableSignal = availableSignal || available
		unavailableSignal = unavailableSignal || unavailable
		if snapshot.PlaybackState == "" {
			snapshot.PlaybackState = statePlaybackValue(candidate)
			availableSignal = availableSignal || snapshot.PlaybackState != ""
		}
	}
	snapshot.Available = availableSignal && !unavailableSignal
	return snapshot
}

func stateAvailabilitySignals(candidate map[string]any) (bool, bool) {
	availableSignal := false
	unavailableSignal := false
	for _, key := range []string{"ok", "hsp_available", "available"} {
		available, ok := candidate[key].(bool)
		if !ok {
			continue
		}
		availableSignal = availableSignal || available
		unavailableSignal = unavailableSignal || !available
	}
	return availableSignal, unavailableSignal
}

func statePlaybackValue(candidate map[string]any) string {
	for _, key := range []string{"playback_state", "state", "play_state", "playState"} {
		if state, ok := parseHSPPlaybackState(candidate[key]); ok {
			return state
		}
	}
	return ""
}

func parseHSPPlaybackState(value any) (string, bool) {
	if state, ok := value.(string); ok {
		state = strings.TrimSpace(state)
		return state, state != ""
	}

	var numeric float64
	switch value := value.(type) {
	case float64:
		numeric = value
	case float32:
		numeric = float64(value)
	case int:
		numeric = float64(value)
	case int32:
		numeric = float64(value)
	case int64:
		numeric = float64(value)
	case uint:
		numeric = float64(value)
	case uint32:
		numeric = float64(value)
	case uint64:
		if value > 1<<53 {
			return "", false
		}
		numeric = float64(value)
	default:
		return "", false
	}
	if math.IsNaN(numeric) || math.IsInf(numeric, 0) || numeric != math.Trunc(numeric) {
		return "", false
	}
	states := [...]string{"not_initialized", "playing", "stopped", "paused", "starving"}
	index := int(numeric)
	if index < 0 || index >= len(states) {
		return "", false
	}
	return states[index], true
}

func statePayloadCandidates(payload any) []map[string]any {
	var candidates []map[string]any
	var add func(any, int)
	add = func(value any, depth int) {
		object, ok := value.(map[string]any)
		if !ok {
			return
		}
		candidates = append(candidates, object)
		if depth >= 3 {
			return
		}
		for _, key := range []string{
			"result",
			"data",
			"state",
			"hsp_state",
			"hspState",
			"responseHspSetup",
			"responseHspAdd",
			"responseHspPlay",
			"responseHspStop",
			"responseHspPause",
			"responseHspResume",
			"responseHspStateGet",
		} {
			add(object[key], depth+1)
		}
	}
	add(payload, 0)
	return candidates
}

func parseServerTimeMillis(body []byte) (int64, bool) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		text := strings.TrimSpace(string(body))
		if text == "" {
			return 0, false
		}
		return parseServerTimeValue(text)
	}
	return findServerTimeMillis(payload)
}

func findServerTimeMillis(value any) (int64, bool) {
	if parsed, ok := parseServerTimeValue(value); ok {
		return parsed, true
	}
	object, ok := value.(map[string]any)
	if !ok {
		return 0, false
	}
	for _, key := range []string{"server_time", "serverTime", "server_time_ms", "serverTimeMs", "time", "now"} {
		if parsed, ok := parseServerTimeValue(object[key]); ok {
			return parsed, true
		}
	}
	for _, key := range []string{"result", "data", "state"} {
		if parsed, ok := findServerTimeMillis(object[key]); ok {
			return parsed, true
		}
	}
	return 0, false
}

func parseServerTimeValue(value any) (int64, bool) {
	var parsed float64
	switch typed := value.(type) {
	case json.Number:
		value, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		parsed = value
	case float64:
		parsed = typed
	case string:
		value, err := json.Number(strings.TrimSpace(typed)).Float64()
		if err != nil {
			return 0, false
		}
		parsed = value
	default:
		return 0, false
	}
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed <= 0 || parsed >= float64(math.MaxInt64) {
		return 0, false
	}
	return int64(math.Floor(parsed + 0.5)), true
}

func safeCloudEventStreamError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	default:
		return errors.New("cloud REST HSP event stream failed")
	}
}

func unixMillis(value time.Time) int64 {
	return value.UnixNano() / int64(time.Millisecond)
}

func fractionPercent(value float64) int {
	return int(value*100 + 0.5)
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

var cloudHSPStateEvents = []string{
	"device_status",
	"device_connected",
	"device_disconnected",
	"device_error",
	"mode_changed",
	"hamp_state_changed",
	"hdsp_state_changed",
	"hsp_state_changed",
	"hsp_threshold_reached",
	"hsp_starving",
	"hsp_looping",
	"hsp_paused_on_starving",
	"hsp_resumed_on_not_starving",
	"stroke_changed",
	"slider_blocked",
	"slider_unblocked",
	"temp_high",
	"temp_ok",
	"low_memory_error",
	"low_memory_warning",
}

func cloneRaw(data []byte) json.RawMessage {
	if len(data) == 0 {
		return nil
	}
	clone := make([]byte, len(data))
	copy(clone, data)
	return clone
}
