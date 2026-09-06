package modes

import "sync"

// NotifyUserStop records an explicit user stop: the active mode ends and no
// keepalive may restart motion afterwards.
func (m *Manager) NotifyUserStop() {
	finish := m.BeginUserStop()
	finish()
}

// BeginUserPause blocks autonomous recovery before Engine.Pause performs its
// transport Stop. During that round-trip the engine is intentionally neither
// running nor fully marked paused; relying on Snapshot alone lets a mode start
// a replacement stream in that gap. The completion callback keeps the latch
// when the engine reached a paused state and rolls it back after an ordinary
// pause failure. The admission result is false when a newer Pause, Resume, or
// Stop superseded this request while it waited for another control operation.
func (m *Manager) BeginUserPause() (func(keepPaused bool), bool) {
	m.userIntentMu.Lock()
	m.mu.Lock()
	m.user.intentID++
	pauseID := m.user.intentID
	latched := m.loop.mode != ""
	if latched {
		if m.user.pending == nil {
			m.user.pending = make(map[uint64]struct{})
		}
		m.user.pending[pauseID] = struct{}{}
		m.user.paused = true
		m.loop.generation++
		m.cancelOperationLocked()
		for index := range m.motion.swayPoints {
			m.motion.swayPoints[index].generation = m.loop.generation
		}
	}
	m.mu.Unlock()
	m.userIntentMu.Unlock()

	// Pause and Resume transport operations execute one at a time. Intent is
	// recorded before waiting so a newer control can invalidate a queued older
	// one; Emergency Stop intentionally bypasses this gate.
	m.userControlMu.Lock()
	m.mu.Lock()
	admitted := m.user.intentID == pauseID
	m.mu.Unlock()

	var once sync.Once
	return func(keepPaused bool) {
		once.Do(func() {
			defer m.userControlMu.Unlock()
			if !latched {
				return
			}
			m.mu.Lock()
			defer m.mu.Unlock()
			if _, pending := m.user.pending[pauseID]; !pending {
				return
			}
			delete(m.user.pending, pauseID)
			if admitted && keepPaused && pauseID > m.user.resumeConfirmed && pauseID > m.user.pauseConfirmed {
				m.user.pauseConfirmed = pauseID
			}
			m.refreshUserPauseLocked()
		})
	}, admitted
}

// BeginUserResume returns a completion callback that releases the mode-level
// latch only when the engine successfully resumes and no newer Pause intent is
// pending. The next scheduler tick then continues the preserved phrase and
// clocks instead of creating a replacement start. A false admission result
// means the engine call must be skipped because a newer control intent won.
func (m *Manager) BeginUserResume() (func(resumed bool), bool) {
	m.userIntentMu.Lock()
	m.mu.Lock()
	m.user.intentID++
	resumeID := m.user.intentID
	m.mu.Unlock()
	m.userIntentMu.Unlock()

	m.userControlMu.Lock()
	m.mu.Lock()
	admitted := m.user.intentID == resumeID
	m.mu.Unlock()

	var once sync.Once
	return func(resumed bool) {
		once.Do(func() {
			defer m.userControlMu.Unlock()
			if !admitted || !resumed {
				return
			}
			m.mu.Lock()
			defer m.mu.Unlock()
			if resumeID > m.user.resumeConfirmed {
				m.user.resumeConfirmed = resumeID
			}
			m.refreshUserPauseLocked()
		})
	}, admitted
}

func (m *Manager) refreshUserPauseLocked() {
	paused := m.user.pauseConfirmed > m.user.resumeConfirmed
	if !paused {
		for pauseID := range m.user.pending {
			if pauseID > m.user.resumeConfirmed {
				paused = true
				break
			}
		}
	}
	m.user.paused = m.loop.mode != "" && paused
}

func (m *Manager) resetUserPauseLocked() {
	m.user.intentID++
	m.user.pauseConfirmed = 0
	m.user.resumeConfirmed = m.user.intentID
	m.user.pending = nil
	m.user.paused = false
}

// BeginUserStop marks autonomous work unable to restart and cancels its loop
// without waiting. The caller can stop the motion engine first, then invoke the
// returned function to drain and trace the mode goroutine.
func (m *Manager) BeginUserStop() func() {
	m.lifecycleMu.Lock()
	m.mu.Lock()
	m.resetUserPauseLocked()
	m.user.stopped = true
	m.chat.target = nil
	m.chat.keepalive = false
	m.chat.pending = false
	m.chat.activity = false
	m.chat.version++
	m.loop.generation++
	m.cancelOperationLocked()
	if m.loop.mode == "" {
		m.mu.Unlock()
		m.lifecycleMu.Unlock()
		return func() {}
	}
	mode := m.loop.mode
	cancel := m.loop.cancel
	done := m.loop.done
	m.loop.mode = ""
	m.loop.cancel = nil
	m.loop.done = nil
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			defer m.lifecycleMu.Unlock()
			if done != nil {
				<-done
			}
			m.trace(mode, "mode_stopped", nil, "user_stop")
		})
	}
}
