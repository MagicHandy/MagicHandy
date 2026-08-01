package httpapi

import (
	"errors"
	"net/http"
	"path/filepath"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

type managedLLMDuplicateSnapshot struct {
	Managed    bool                       `json:"managed"`
	RunnerName string                     `json:"runner_name,omitempty"`
	Processes  []llm.ManagedRunnerProcess `json:"processes"`
}

func (s *Server) handleManagedLLMDuplicates(w http.ResponseWriter, _ *http.Request) {
	snapshot, err := s.managedLLMDuplicates()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) handleTerminateManagedLLMDuplicates(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body struct {
		PIDs []int `json:"pids"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(body.PIDs) == 0 || len(body.PIDs) > 32 {
		writeError(w, http.StatusBadRequest, errors.New("one to 32 managed runner process IDs are required"))
		return
	}

	settings, _ := s.store.Snapshot()
	runnerPath, ownedPID, err := s.managedLLMRunner(settings.LLM)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	s.stopLLMAutoload()
	defer s.startLLMAutoload(settings.LLM)
	seen := make(map[int]struct{}, len(body.PIDs))
	for _, pid := range body.PIDs {
		if pid <= 0 || pid == ownedPID {
			writeError(w, http.StatusConflict, errors.New("refusing to terminate the process owned by this MagicHandy instance"))
			return
		}
		if _, duplicate := seen[pid]; duplicate {
			continue
		}
		seen[pid] = struct{}{}
		if err := llm.TerminateManagedRunnerProcess(runnerPath, pid); err != nil {
			writeError(w, http.StatusConflict, err)
			return
		}
	}

	snapshot, err := s.managedLLMDuplicates()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) managedLLMDuplicates() (managedLLMDuplicateSnapshot, error) {
	settings, _ := s.store.Snapshot()
	snapshot := managedLLMDuplicateSnapshot{Processes: []llm.ManagedRunnerProcess{}}
	if settings.LLM.Provider != config.LLMProviderLlamaCPP || settings.LLM.LlamaCPPMode != config.LlamaCPPModeManaged {
		return snapshot, nil
	}
	runnerPath, ownedPID, err := s.managedLLMRunner(settings.LLM)
	if err != nil {
		return snapshot, nil
	}
	processes, err := llm.FindManagedRunnerProcesses(runnerPath, ownedPID)
	if err != nil {
		return snapshot, err
	}
	snapshot.Managed = true
	snapshot.RunnerName = filepath.Base(runnerPath)
	snapshot.Processes = processes
	return snapshot, nil
}

func (s *Server) managedLLMRunner(settings config.LLMSettings) (string, int, error) {
	if settings.Provider != config.LLMProviderLlamaCPP || settings.LlamaCPPMode != config.LlamaCPPModeManaged {
		return "", 0, errors.New("managed llama.cpp is not selected")
	}
	runtimeStatus := s.managedLLM.Snapshot().Runtime
	if !runtimeStatus.Installed || runtimeStatus.RunnerPath == "" {
		return "", 0, errors.New("managed llama.cpp runtime is not installed")
	}
	return runtimeStatus.RunnerPath, s.ownedManagedLLMPID(), nil
}

func (s *Server) ownedManagedLLMPID() int {
	s.llm.mu.Lock()
	defer s.llm.mu.Unlock()
	provider, ok := s.llm.cached.(interface{ ProcessID() int })
	if !ok {
		return 0
	}
	return provider.ProcessID()
}
