package transport

// BrowserBluetoothPlaybackState contains only playback telemetry from Handy
// notifications. Sequence orders concurrent HTTP reports within one gateway
// connection; stream_id prevents a delayed notification reviving an old run.
type BrowserBluetoothPlaybackState struct {
	Sequence             uint64 `json:"sequence"`
	CommandID            string `json:"command_id"`
	StreamID             int    `json:"stream_id"`
	PlayState            string `json:"play_state"`
	Points               int    `json:"points"`
	CurrentPoint         int    `json:"current_point"`
	CurrentTimeMillis    int64  `json:"current_time_ms"`
	TailPointStreamIndex int    `json:"tail_point_stream_index"`
}

func (s BrowserBluetoothPlaybackState) valid() bool {
	if s.Sequence == 0 || s.CommandID == "" || len(s.CommandID) > 64 || s.StreamID < 0 || uint64(s.StreamID) > uint64(^uint32(0)) ||
		s.Points < 0 || s.Points > 1000000 || s.CurrentPoint < -1 || s.CurrentPoint > 1000000 ||
		s.CurrentTimeMillis < -1 || s.CurrentTimeMillis > 1<<31-1 || s.TailPointStreamIndex < -1 {
		return false
	}
	switch s.PlayState {
	case "not_initialized", "playing", "stopped", "paused", "starving":
		return true
	default:
		return false
	}
}

func cloneBluetoothPlayback(state *BrowserBluetoothPlaybackState) *BrowserBluetoothPlaybackState {
	if state == nil {
		return nil
	}
	stateCopy := *state
	return &stateCopy
}

func (b *BrowserBluetoothBridge) acceptPlaybackLocked(state *BrowserBluetoothPlaybackState) {
	if state == nil || !state.valid() || !b.feedbackActive || state.CommandID != b.feedbackCommandID || state.StreamID != b.feedbackStreamID || state.Sequence <= b.feedbackSequence {
		return
	}
	b.feedbackSequence = state.Sequence
	b.playback = cloneBluetoothPlayback(state)
}

func (b *BrowserBluetoothBridge) preparePlaybackCommandLocked(path string, body map[string]any, commandID string) {
	if path == "hsp/stop" {
		b.feedbackActive = false
		b.playback = nil
		return
	}
	if path != "hsp/add" && path != "hsp/play" {
		return
	}
	streamID, ok := body["stream_id"].(int)
	if !ok {
		return
	}
	if !b.feedbackActive || b.feedbackStreamID != streamID || path == "hsp/play" {
		b.playback = nil
	}
	b.feedbackStreamID, b.feedbackActive = streamID, true
	b.feedbackCommandID = commandID
}
