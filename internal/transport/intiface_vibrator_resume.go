package transport

// VibratorResumePermit binds a manual request to the same stopped device.
// Its fields cannot be supplied by a model.
type VibratorResumePermit struct {
	control, epoch uint64
}

func (v *IntifaceVibrator) resumableLocked() bool {
	return v.selection.selected && v.suspended && !v.stream.playing && v.testUntil.IsZero() && v.playbackStateLocked() == "idle"
}

// ManualResumePermit offers to lift Stop's suspension without playing anything.
// Only the application may attach it to a fresh admitted manual chat request.
func (v *IntifaceVibrator) ManualResumePermit() (VibratorResumePermit, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return VibratorResumePermit{v.control, v.epoch}, v.resumableLocked()
}

func (v *IntifaceVibrator) resumePermitCurrentLocked(permit VibratorResumePermit) bool {
	return permit.control == v.control && permit.epoch == v.epoch && v.resumableLocked()
}

// ResumePermitCurrent rejects another Stop, reconnection and reselection.
func (v *IntifaceVibrator) ResumePermitCurrent(permit VibratorResumePermit) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.resumePermitCurrentLocked(permit)
}

// ResumeForManualRequest lifts the suspension for a fresh manual request. It
// does not play anything; the existing engine start path must accept a new
// target. The returned rollback suspends again after a failed start, unless a
// newer human action has resumed or rebound the vibrator meanwhile.
func (v *IntifaceVibrator) ResumeForManualRequest(permit VibratorResumePermit) (func(), error) {
	v.mu.Lock()
	if !v.resumePermitCurrentLocked(permit) {
		v.mu.Unlock()
		return nil, errVibratorControlStale
	}
	v.suspended = false
	v.levelRevision++
	v.resumeRevision++
	revision := v.resumeRevision
	v.addEventLocked("manual_resume", 0, nil)
	v.mu.Unlock()
	return func() {
		v.mu.Lock()
		defer v.mu.Unlock()
		if v.epoch != permit.epoch || v.resumeRevision != revision || v.suspended {
			return
		}
		v.suspended = true
		v.levelRevision++
		v.invalidateOutputLocked()
		v.addEventLocked("manual_resume_failed", 0, nil)
		v.signalLocked()
	}, nil
}
