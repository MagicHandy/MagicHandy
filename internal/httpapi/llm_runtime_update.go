package httpapi

import (
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// runtimeUpdatePollInterval paces the wait for an automatic runtime update to
// finish before the model loads again.
const runtimeUpdatePollInterval = time.Second

// managedRuntimeUpdateDue reports whether the installed managed llama.cpp is
// older than the runtime this release pins, is the chat engine in use, and the
// user lets MagicHandy install it without asking. The pin only moves with a
// MagicHandy release, so "automatic" never means an unreviewed upstream build.
// A runtime left installed while chat uses Ollama or an external server is not
// downloaded again until managed mode is selected.
func managedRuntimeUpdateDue(settings config.Settings, status llm.ManagedLlamaRuntimeStatus) bool {
	return settings.UI.SetupCompleted && managedLLMSelected(settings.LLM) &&
		settings.UI.RuntimeUpdateMode != config.UpdateCheckManual &&
		status.BuildSupported && status.Installed && !status.Current &&
		(status.Backend == "cpu" || status.Backend == "cuda")
}

// startManagedRuntimeUpdate installs the pinned runtime with the backend the
// user already chose, reporting whether an update started. The previous
// runtime stays active until the verified replacement is installed.
func (s *Server) startManagedRuntimeUpdate(settings config.Settings) bool {
	if s.managedLLM == nil {
		return false
	}
	snapshot := s.managedLLM.Snapshot()
	if !managedRuntimeUpdateDue(settings, snapshot.Runtime) || managedRuntimeBuildInProgress(snapshot.Build) {
		return false
	}
	if err := s.closeLLM(); err != nil {
		s.logger.Warn("automatic llama.cpp update could not stop the runner", "error", err)
		return false
	}
	if _, err := s.runtimeUpdater(snapshot.Runtime.Backend); err != nil {
		s.logger.Warn("automatic llama.cpp update did not start", "error", err)
		return false
	}
	s.logger.Info("updating the managed llama.cpp runtime",
		"installed", snapshot.Runtime.Version, "pinned", llm.ManagedLlamaVersion, "backend", snapshot.Runtime.Backend)
	s.runtimeUpdateWG.Add(1)
	go s.autoloadAfterRuntimeUpdate()
	return true
}

// autoloadAfterRuntimeUpdate waits for the update to finish, then applies the
// saved load policy as startup would have.
func (s *Server) autoloadAfterRuntimeUpdate() {
	defer s.runtimeUpdateWG.Done()
	ticker := time.NewTicker(runtimeUpdatePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.lifecycleCtx.Done():
			return
		case <-ticker.C:
		}
		if managedRuntimeBuildInProgress(s.managedLLM.Snapshot().Build) {
			continue
		}
		settings, _ := s.store.Snapshot()
		s.startLLMAutoload(settings.LLM)
		return
	}
}
