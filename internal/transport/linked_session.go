package transport

// LinkedSessionStatus distinguishes saved playback choices from participants
// retired during this run. It contains no device identifiers or raw errors.
type LinkedSessionStatus struct {
	Status          string   `json:"status"`
	ContinueHealthy bool     `json:"continue_healthy"`
	RetiredDevices  []string `json:"retired_devices"`
	// IdleDevices are selected participants that this scalar-only run does not
	// drive. The UI must not describe them as following the active device.
	IdleDevices []string `json:"idle_devices"`
}

// LinkedSessionStatusProvider exposes state only; it never polls or commands.
type LinkedSessionStatusProvider interface {
	LinkedSessionStatus() LinkedSessionStatus
}

// LinkedSessionStatus reports this run's latched failure/retirement state.
func (f *FanOut) LinkedSessionStatus() LinkedSessionStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	status := LinkedSessionStatus{Status: "ready", ContinueHealthy: f.continueHealthy, RetiredDevices: []string{}, IdleDevices: []string{}}
	for index, retired := range f.retired {
		if retired {
			status.RetiredDevices = append(status.RetiredDevices, f.participants[index].ID)
			status.Status = "degraded"
		} else if f.idleInRunLocked(index) {
			status.IdleDevices = append(status.IdleDevices, f.participants[index].ID)
		}
	}
	if f.fault != nil {
		status.Status = "failed"
	}
	return status
}
