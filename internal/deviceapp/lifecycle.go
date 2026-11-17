// Package deviceapp coordinates the single engine and its device owner across
// requests, planners, settings changes and shutdown.
package deviceapp

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

// Lifecycle serializes owner replacement and Stop independently of settings
// persistence. Stop can cancel a Resume already waiting on a transport.
type Lifecycle struct {
	admission sync.Mutex
	settings  sync.Mutex
	mu        sync.RWMutex
	engine    *motion.Engine
	owner     string
}

// Hold pins the selected engine across a compound admission or safety operation.
func (l *Lifecycle) Hold() func() { l.admission.Lock(); return l.admission.Unlock }

// HoldSettings spans a saved mutation and its runtime transitions.
func (l *Lifecycle) HoldSettings() func() { l.settings.Lock(); return l.settings.Unlock }

// Current returns a consistent engine/owner pair without waiting for wire I/O.
func (l *Lifecycle) Current() (*motion.Engine, string) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.engine, l.owner
}

// Install publishes a binding under admission.
func (l *Lifecycle) Install(engine *motion.Engine, owner string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.engine, l.owner = engine, owner
}

// Retire removes the binding before draining it. The caller holds admission.
func (l *Lifecycle) Retire(ctx context.Context, reason string) error {
	engine, _ := l.Current()
	l.Install(nil, "")
	if engine == nil {
		return nil
	}
	_, err := engine.Stop(ctx, reason)
	return err
}

// Admit selects or builds one engine and captures its Stop generation before releasing admission.
func (l *Lifecycle) Admit(selectOwner func() (string, bool, error), build func() (*motion.Engine, error)) (*motion.Engine, uint64, error) {
	release := l.Hold()
	defer release()
	owner, refreshIdle, err := selectOwner()
	if err != nil {
		return nil, 0, err
	}
	engine, oldOwner := l.Current()
	if engine != nil {
		state := engine.Observation()
		retired := state.LinkedSession != nil && (state.LinkedSession.Status == "degraded" || state.LinkedSession.Status == "failed")
		refresh := !state.Running && !state.Starting && !state.Paused && (state.LastError != "" || retired || refreshIdle)
		if oldOwner == owner && !refresh {
			return engine, engine.AdmissionGeneration(), nil
		}
		reason := "dispatch_owner_changed"
		if oldOwner == owner {
			reason = "inactive_run_replaced"
		}
		if err := l.Retire(context.Background(), reason); err != nil {
			return nil, 0, fmt.Errorf("previous motion owner could not be stopped: %w", err)
		}
	}
	engine, err = build()
	if err != nil {
		return nil, 0, err
	}
	l.Install(engine, owner)
	return engine, engine.AdmissionGeneration(), nil
}

// ApplySettings updates the retained engine, including a paused run.
func (l *Lifecycle) ApplySettings(ctx context.Context, settings config.MotionSettings) error {
	engine, _ := l.Current()
	if engine == nil {
		return nil
	}
	_, err := engine.RefreshSettings(ctx, settings, "settings_saved")
	return err
}

// Resume re-reads persisted settings under the same serialization used by edits.
func (l *Lifecycle) Resume(ctx context.Context, engine *motion.Engine, latest func() config.MotionSettings) (motion.ActiveMotionState, error) {
	admission := engine.AdmissionGeneration()
	release := l.HoldSettings()
	defer release()
	return engine.ResumeAtGeneration(ctx, "ui_resume", latest(), admission)
}

// StopResult carries the outcome from the engine or the fallback device path.
type StopResult struct {
	State  motion.ActiveMotionState
	Engine bool
	Result transport.CommandResult
}

// Stop dispatches exactly one of engine Stop or the fallback. The caller holds
// admission until it publishes completion to any overlapping Stop requests.
func (l *Lifecycle) Stop(ctx context.Context, reason string, outside func(string) error, fallback func() (transport.CommandResult, error)) (StopResult, error) {
	engine, owner := l.Current()
	if engine == nil {
		result, err := fallback()
		return StopResult{Result: result}, err
	}
	others := make(chan error, 1)
	go func() { others <- outside(owner) }()
	state, err := engine.Stop(ctx, reason)
	return StopResult{State: state, Engine: true}, errors.Join(err, <-others)
}
