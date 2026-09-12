package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// The installer emits operation labels separately from stdout/arguments, so
// actionable failure context can survive a restart without persisting raw logs.
// Only app-defined labels are accepted from a child process's output.
var voiceInstallStages = map[string]bool{
	"NVIDIA driver verification":                          true,
	"Python environment creation":                         true,
	"Python voice runtime verification":                   true,
	"Faster Qwen3-TTS dependency installation":            true,
	"Chatterbox dependency installation":                  true,
	"Pinned Chatterbox engine installation":               true,
	"Chatterbox ONNX protobuf compatibility installation": true,
	"Hugging Face client installation":                    true,
	"Hugging Face client verification":                    true,
	"Hugging Face model download":                         true,
	"Python dependency compatibility check":               true,
	"Source clone":                                        true,
	"Pinned source fetch":                                 true,
	"Pinned source checkout":                              true,
}

func (m *setupManager) voiceInstallFailure(id string, err error) error {
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.job == nil || m.job.ID != id {
		return err
	}
	lines := strings.Split(m.job.Output, "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(lines[index])
		payload, ok := strings.CutPrefix(line, "MAGICHANDY_SETUP_FAILURE:")
		if !ok {
			continue
		}
		var failure struct {
			Stage    string `json:"stage"`
			ExitCode int    `json:"exit_code"`
		}
		if json.Unmarshal([]byte(payload), &failure) != nil || !voiceInstallStages[failure.Stage] || failure.ExitCode == 0 {
			continue
		}
		return fmt.Errorf("%s failed (exit %d); see installation output for details: %w", failure.Stage, failure.ExitCode, err)
	}
	return err
}
