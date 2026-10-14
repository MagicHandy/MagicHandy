package transport

// LinkedSessionStatus distinguishes saved playback choices from participants
// retired during this run. It contains no device identifiers or raw errors.
type LinkedSessionStatus struct {
	Status          string   `json:"status"`
	ContinueHealthy bool     `json:"continue_healthy"`
	RetiredDevices  []string `json:"retired_devices"`
}

// LinkedSessionStatusProvider exposes state only; it never polls or commands.
type LinkedSessionStatusProvider interface {
	LinkedSessionStatus() LinkedSessionStatus
}

// LinkedSessionStatus reports this run's latched failure/retirement state.
func (f *FanOut) LinkedSessionStatus() LinkedSessionStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	status := LinkedSessionStatus{Status: "ready", ContinueHealthy: f.continueHealthy, RetiredDevices: []string{}}
	for index, retired := range f.retired {
		if retired {
			name := "handy"
			if index == 1 {
				name = "coyote"
			}
			status.RetiredDevices = append(status.RetiredDevices, name)
			status.Status = "degraded"
		}
	}
	if f.fault != nil {
		status.Status = "failed"
	}
	return status
}
