package transport

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"
)

// BrowserBluetoothOptions controls browser-owned BLE dispatch mapping.
type BrowserBluetoothOptions struct {
	ReverseDirection bool
}

// BrowserBluetoothTransport dispatches HSP commands through a browser BLE bridge.
type BrowserBluetoothTransport struct {
	bridge  *BrowserBluetoothBridge
	options BrowserBluetoothOptions

	mu sync.Mutex

	hspMu                   sync.Mutex
	motionGate              motionCommandGate
	activeStreamID          string
	activeBluetoothStreamID int
	nextBluetoothStreamID   uint32

	diagnosis TransportDiagnostics
}

// NewBrowserBluetoothTransport returns a transport backed by a browser bridge.
func NewBrowserBluetoothTransport(bridge *BrowserBluetoothBridge, options BrowserBluetoothOptions) (*BrowserBluetoothTransport, error) {
	if bridge == nil {
		return nil, errors.New("browser Bluetooth bridge is required")
	}
	return &BrowserBluetoothTransport{
		bridge:  bridge,
		options: options,
		diagnosis: TransportDiagnostics{
			Name:          BrowserBluetoothName,
			PlaybackState: "unknown",
		},
	}, nil
}

// Stop sends an HSP stop command through the browser bridge.
func (t *BrowserBluetoothTransport) Stop(ctx context.Context, command StopCommand) (CommandResult, error) {
	recorded := Command{
		Kind: CommandKindStop,
		Stop: &StopCommand{Reason: command.Reason},
	}
	t.motionGate.beginStop()
	defer t.motionGate.endStop()
	t.hspMu.Lock()
	defer t.hspMu.Unlock()

	result, err := t.dispatch(ctx, recorded, "hsp/stop", nil)
	if result.OK {
		t.activeStreamID = ""
		t.activeBluetoothStreamID = 0
	}
	return result, err
}

// SetStrokeWindow sends a stroke envelope command through the browser bridge.
func (t *BrowserBluetoothTransport) SetStrokeWindow(ctx context.Context, command StrokeWindowCommand) (CommandResult, error) {
	admission, err := t.motionGate.admit()
	if err != nil {
		return t.recordBuildError(CommandKindStrokeWindow, err), err
	}
	t.hspMu.Lock()
	defer t.hspMu.Unlock()
	if err := t.motionGate.validate(admission); err != nil {
		return t.recordBuildError(CommandKindStrokeWindow, err), err
	}

	if err := validateStrokeWindow(command); err != nil {
		return t.recordBuildError(CommandKindStrokeWindow, err), err
	}
	recorded := Command{
		Kind:         CommandKindStrokeWindow,
		StrokeWindow: &command,
	}
	body := map[string]any{
		"min": command.MinPercent,
		"max": command.MaxPercent,
	}
	result, err := t.dispatch(ctx, recorded, "slider/stroke", body)
	if err == nil {
		t.options.ReverseDirection = command.ReverseDirection
	}
	return result, err
}

// AppendPoints sends timed points through browser-owned Handy HSP.
func (t *BrowserBluetoothTransport) AppendPoints(ctx context.Context, command AppendPointsCommand) (CommandResult, error) {
	admission, err := t.motionGate.admit()
	if err != nil {
		return t.recordBuildError(CommandKindPointsAdd, err), err
	}
	t.hspMu.Lock()
	defer t.hspMu.Unlock()
	if err := t.motionGate.validate(admission); err != nil {
		return t.recordBuildError(CommandKindPointsAdd, err), err
	}

	semanticStreamID, bluetoothStreamID, err := t.bluetoothStreamIDLocked(command.StreamID)
	if err != nil {
		return t.recordBuildError(CommandKindPointsAdd, err), err
	}
	if len(command.Points) == 0 {
		err := errors.New("HSP add requires at least one point")
		return t.recordBuildError(CommandKindPointsAdd, err), err
	}
	points := make([]map[string]any, len(command.Points))
	for index, point := range command.Points {
		x, ok := mapHandyPosition(point.PositionPercent, t.options.ReverseDirection)
		if !ok {
			err := fmt.Errorf("HSP point %d x must be between 0 and 100", index)
			return t.recordBuildError(CommandKindPointsAdd, err), err
		}
		if point.TimeMillis < 0 {
			err := fmt.Errorf("HSP point %d t must be non-negative", index)
			return t.recordBuildError(CommandKindPointsAdd, err), err
		}
		points[index] = map[string]any{
			"x": x,
			"t": point.TimeMillis,
		}
	}

	recorded := Command{
		Kind:      CommandKindPointsAdd,
		PointsAdd: cloneAppendPoints(command),
	}
	body := map[string]any{
		"stream_id": bluetoothStreamID,
		"points":    points,
	}
	result, err := t.dispatch(ctx, recorded, "hsp/add", body)
	if result.OK {
		t.activeStreamID = semanticStreamID
		t.activeBluetoothStreamID = bluetoothStreamID
	}
	return result, err
}

// Play starts or resumes timed-point playback through browser-owned Handy HSP.
func (t *BrowserBluetoothTransport) Play(ctx context.Context, command PlayCommand) (CommandResult, error) {
	admission, err := t.motionGate.admit()
	if err != nil {
		return t.recordBuildError(CommandKindPointsPlay, err), err
	}
	t.hspMu.Lock()
	defer t.hspMu.Unlock()
	if err := t.motionGate.validate(admission); err != nil {
		return t.recordBuildError(CommandKindPointsPlay, err), err
	}

	semanticStreamID, bluetoothStreamID, err := t.bluetoothStreamIDLocked(command.StreamID)
	if err != nil {
		return t.recordBuildError(CommandKindPointsPlay, err), err
	}
	if command.StartTimeMillis < 0 {
		err := errors.New("HSP play start time must be non-negative")
		return t.recordBuildError(CommandKindPointsPlay, err), err
	}
	recorded := Command{
		Kind:       CommandKindPointsPlay,
		PointsPlay: &command,
	}
	body := map[string]any{
		"stream_id":  bluetoothStreamID,
		"start_time": command.StartTimeMillis,
	}
	result, err := t.dispatch(ctx, recorded, "hsp/play", body)
	if result.OK {
		t.activeStreamID = semanticStreamID
		t.activeBluetoothStreamID = bluetoothStreamID
	}
	return result, err
}

// CheckConnection reports browser Bluetooth bridge readiness without sending a
// device command. Some Handy BLE commands are write-only in practice, and a
// state probe can destabilize an otherwise-ready browser-owned GATT link.
func (t *BrowserBluetoothTransport) CheckConnection(context.Context) (ConnectionCheckResult, error) {
	snapshot := t.bridge.Snapshot()
	diagnostics := t.Diagnostics()
	status := snapshot.Status
	if status == "" {
		status = "disconnected"
	}
	check := ConnectionCheckResult{
		OK:            snapshot.Ready,
		Status:        status,
		HSPAvailable:  snapshot.Ready,
		PlaybackState: diagnostics.PlaybackState,
		Diagnostics:   diagnostics,
	}
	if check.PlaybackState == "" {
		check.PlaybackState = "unknown"
	}
	if !snapshot.Ready {
		message := snapshot.Message
		if message == "" {
			message = "Browser Bluetooth is not ready."
		}
		return check, BrowserBluetoothError{Status: status, Message: message}
	}
	return check, nil
}

// ReadState reads browser Bluetooth HSP state.
func (t *BrowserBluetoothTransport) ReadState(ctx context.Context) (HSPStateSnapshot, CommandResult, error) {
	recorded := Command{Kind: CommandKindHSPState}
	result, ack, err := t.dispatchWithAck(ctx, recorded, "hsp/state", nil)
	return stateSnapshotFromBridgeAck(ack), result, err
}

// Diagnostics returns a safe browser Bluetooth diagnostics snapshot.
func (t *BrowserBluetoothTransport) Diagnostics() TransportDiagnostics {
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

func (t *BrowserBluetoothTransport) dispatch(ctx context.Context, command Command, path string, body map[string]any) (CommandResult, error) {
	result, _, err := t.dispatchWithAck(ctx, command, path, body)
	return result, err
}

func (t *BrowserBluetoothTransport) dispatchWithAck(ctx context.Context, command Command, path string, body map[string]any) (CommandResult, BrowserBluetoothBridgeAck, error) {
	start := time.Now()
	ack := t.bridge.SendCommand(ctx, command.Kind, path, body)
	result := CommandResult{
		CommandID:     ack.ID,
		Kind:          command.Kind,
		Transport:     BrowserBluetoothName,
		OK:            ack.OK,
		Status:        ack.Status,
		LatencyMillis: int64(ack.ElapsedMillis),
		CompletedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	if result.LatencyMillis == 0 {
		result.LatencyMillis = time.Since(start).Milliseconds()
	}
	if result.Status == "" {
		if ack.OK {
			result.Status = "browser_ack"
		} else {
			result.Status = "device_error"
		}
	}
	if !ack.OK {
		result.Error = ack.Error
	}
	command.ID = ack.ID
	command.IssuedAt = start.UTC().Format(time.RFC3339Nano)
	t.recordResult(command, result)
	if ack.OK {
		return result, ack, nil
	}
	return result, ack, BrowserBluetoothError{Status: result.Status, Message: ack.Error}
}

func (t *BrowserBluetoothTransport) recordBuildError(kind CommandKind, err error) CommandResult {
	t.mu.Lock()
	defer t.mu.Unlock()

	result := CommandResult{
		Kind:        kind,
		Transport:   BrowserBluetoothName,
		OK:          false,
		Status:      "failed",
		Error:       err.Error(),
		CompletedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	t.diagnosis.LastResult = &result
	t.diagnosis.LastError = err.Error()
	return result
}

func (t *BrowserBluetoothTransport) recordResult(command Command, result CommandResult) {
	t.mu.Lock()
	defer t.mu.Unlock()

	snapshot := t.bridge.Snapshot()
	playbackState := t.diagnosis.PlaybackState
	if playbackState == "" || playbackState == "unknown" {
		playbackState = "unknown"
	}
	if result.OK {
		switch result.Kind {
		case CommandKindStop:
			playbackState = "idle"
		case CommandKindPointsAdd:
			playbackState = "buffered"
		case CommandKindPointsPlay:
			playbackState = "playing"
		case CommandKindHSPState, CommandKindConnectionCheck:
			if state := playbackStateFromAck(snapshot.LastAck); state != "" {
				playbackState = state
			}
		}
		t.diagnosis.LastError = ""
	} else {
		t.diagnosis.LastError = result.Error
	}

	t.diagnosis.Name = BrowserBluetoothName
	t.diagnosis.Connected = snapshot.Ready
	t.diagnosis.PlaybackState = playbackState
	t.diagnosis.CommandCount++
	t.diagnosis.LastLatencyMillis = result.LatencyMillis
	t.diagnosis.LastCommand = &command
	t.diagnosis.LastResult = &result
}

func (t *BrowserBluetoothTransport) bluetoothStreamIDLocked(streamID string) (string, int, error) {
	cleaned, err := cleanStreamID(streamID)
	if err != nil {
		return "", 0, err
	}
	if id, ok := parseBluetoothStreamID(cleaned); ok {
		return cleaned, id, nil
	}
	if cleaned == t.activeStreamID && t.activeBluetoothStreamID >= 0 {
		return cleaned, t.activeBluetoothStreamID, nil
	}
	return cleaned, int(t.nextBluetoothStreamIDLocked()), nil
}

func parseBluetoothStreamID(streamID string) (int, bool) {
	id, err := strconv.Atoi(streamID)
	if err != nil || id < 0 {
		return 0, false
	}
	return id, true
}

func (t *BrowserBluetoothTransport) nextBluetoothStreamIDLocked() uint32 {
	if t.nextBluetoothStreamID == ^uint32(0) {
		t.nextBluetoothStreamID = 1
		return t.nextBluetoothStreamID
	}
	t.nextBluetoothStreamID++
	if t.nextBluetoothStreamID == 0 {
		t.nextBluetoothStreamID = 1
	}
	return t.nextBluetoothStreamID
}

func stateSnapshotFromBridgeAck(ack BrowserBluetoothBridgeAck) HSPStateSnapshot {
	snapshot := HSPStateSnapshot{
		Available: ack.OK,
	}
	if ack.Response == nil {
		return snapshot
	}
	if state, ok := ack.Response["hsp_state"].(map[string]any); ok {
		if playState, ok := state["play_state"].(string); ok {
			snapshot.PlaybackState = playState
		}
	}
	return snapshot
}

func playbackStateFromAck(ack *BrowserBluetoothBridgeAck) string {
	if ack == nil || ack.Response == nil {
		return ""
	}
	state, ok := ack.Response["hsp_state"].(map[string]any)
	if !ok {
		return ""
	}
	playState, _ := state["play_state"].(string)
	return playState
}
