package httpapi

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
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
	if _, err := s.installManagedRuntime(s.lifecycleCtx, snapshot.Runtime.Backend); err != nil {
		s.logger.Warn("automatic llama.cpp update did not start", "error", err)
		return false
	}
	s.logger.Info("updating the managed llama.cpp runtime",
		"installed", snapshot.Runtime.Version, "pinned", llm.ManagedLlamaVersion, "backend", snapshot.Runtime.Backend)
	return true
}

var errRunnerRetirement = errors.New("the active LLM runner could not be stopped before rebuilding")

func (s *Server) installManagedRuntime(ctx context.Context, backend string) (llm.ManagedLlamaRuntimeBuild, error) {
	s.runtimeUpdateMu.Lock()
	defer s.runtimeUpdateMu.Unlock()
	var empty llm.ManagedLlamaRuntimeBuild
	if err := s.lifecycleCtx.Err(); err != nil {
		return empty, err
	}
	snapshot := s.managedLLM.Snapshot()
	backend = strings.ToLower(strings.TrimSpace(backend))
	if backend == "" {
		backend = "auto"
	}
	if !slices.Contains(snapshot.Runtime.SupportedBackends, backend) {
		return empty, errors.New("managed llama.cpp backend must be auto, cpu, or cuda")
	}
	if !snapshot.Runtime.BuildSupported {
		return empty, errors.New("managed llama.cpp installation is currently supported on Windows/amd64 only")
	}
	if s.runtimeUpdateDone != nil || managedRuntimeBuildInProgress(snapshot.Build) {
		return empty, errors.New("a managed llama.cpp installation is already running")
	}
	finish := s.llmRequests.beginChange()
	s.stopLLMAutoload()
	if err := s.llmRequests.waitIdle(ctx); err != nil {
		finish()
		return empty, err
	}
	if err := s.closeLLM(); err != nil {
		finish()
		return empty, fmt.Errorf("%w: %w", errRunnerRetirement, err)
	}
	build, err := s.runtimeUpdater(backend)
	if err != nil {
		finish()
		settings, _ := s.store.Snapshot()
		s.startLLMAutoload(settings.LLM)
		return empty, err
	}
	s.runtimeUpdateDone = make(chan struct{})
	go s.autoloadAfterRuntimeUpdate(finish)
	return build, nil
}

// autoloadAfterRuntimeUpdate waits for the update to finish, then applies the
// saved load policy as startup would have.
func (s *Server) autoloadAfterRuntimeUpdate(finish func()) {
	defer func() {
		s.runtimeUpdateMu.Lock()
		finish()
		close(s.runtimeUpdateDone)
		s.runtimeUpdateDone = nil
		s.runtimeUpdateMu.Unlock()
	}()
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
