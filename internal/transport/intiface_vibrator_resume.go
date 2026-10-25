package transport

import "math"

// VibratorResumePermit binds a manual request to the same stopped device and
// last human-selected intensity. Its fields cannot be supplied by a model.
type VibratorResumePermit struct {
	control, epoch, revision uint64
	intensity                float64
}

func (v *IntifaceVibrator) resumableIntensityLocked() float64 {
	if !v.selection.selected || v.intensity != 0 || v.stream.playing || !v.testUntil.IsZero() || v.playbackStateLocked() != "idle" {
		return 0
	}
	level := math.Min(v.resumeIntensity, v.config.Ceiling)
	if v.effectiveIntensityLocked(level) <= 0 {
		return 0
	}
	return level
}

// ManualResumePermit offers the previous human intensity without arming output.
// Only the application may attach it to a fresh admitted manual chat request.
func (v *IntifaceVibrator) ManualResumePermit() (VibratorResumePermit, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	level := v.resumableIntensityLocked()
	return VibratorResumePermit{v.control, v.epoch, v.levelRevision, level}, level > 0
}

func (v *IntifaceVibrator) resumePermitCurrentLocked(permit VibratorResumePermit) bool {
	return permit.intensity > 0 && permit.control == v.control && permit.epoch == v.epoch &&
		permit.revision == v.levelRevision && permit.intensity == v.resumableIntensityLocked()
}

// ResumePermitCurrent rejects Stop, reconnection and human control changes.
func (v *IntifaceVibrator) ResumePermitCurrent(permit VibratorResumePermit) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.resumePermitCurrentLocked(permit)
}

// ResumeIntensityForManualRequest restores only the prior human level. It does
// not play anything; the existing engine start path must accept a new target.
// The returned rollback silences a failed start without undoing newer controls.
func (v *IntifaceVibrator) ResumeIntensityForManualRequest(permit VibratorResumePermit) (func(), error) {
	v.session.writeMu.Lock()
	v.mu.Lock()
	if !v.resumePermitCurrentLocked(permit) {
		v.mu.Unlock()
		v.session.writeMu.Unlock()
		return nil, errVibratorControlStale
	}
	v.intensity, v.resumeIntensity = permit.intensity, 0
	v.levelRevision++
	revision := v.levelRevision
	v.invalidateOutputLocked()
	v.addEventLocked("manual_resume", 0, nil)
	v.mu.Unlock()
	v.session.writeMu.Unlock()
	return func() {
		v.session.writeMu.Lock()
		defer v.session.writeMu.Unlock()
		v.mu.Lock()
		defer v.mu.Unlock()
		if v.epoch != permit.epoch || v.levelRevision != revision || v.intensity != permit.intensity {
			return
		}
		v.intensity, v.resumeIntensity = 0, permit.intensity
		v.levelRevision++
		v.invalidateOutputLocked()
		v.addEventLocked("manual_resume_failed", 0, nil)
		v.signalLocked()
	}, nil
}
