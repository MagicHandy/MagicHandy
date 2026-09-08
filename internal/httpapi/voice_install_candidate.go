package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Voice runtime paths are immutable after installation. A tiny home index is
// changed only after settings point at a verified candidate, so a failed update
// never modifies or replaces the Python environment of the running worker.
func voiceRuntimeFromIndex(home, name string) (string, error) {
	path := filepath.Join(home, name)
	if !isRegularFileWithoutLinks(home, path) {
		return "", errors.New("voice runtime index is missing or unsafe")
	}
	data, err := readFileLimited(path, 32<<10)
	if err != nil {
		return "", err
	}
	var index struct {
		SchemaVersion int    `json:"schema_version"`
		RuntimeRoot   string `json:"runtime_root"`
	}
	if err := json.Unmarshal(data, &index); err != nil {
		return "", err
	}
	if index.SchemaVersion != 2 || index.RuntimeRoot == "" {
		return "", errors.New("voice runtime index does not identify a candidate")
	}
	runtimes := filepath.Join(home, "runtimes")
	relative, err := filepath.Rel(runtimes, index.RuntimeRoot)
	if err != nil || len(relative) != 32 || strings.ContainsAny(relative, `/\:.`) {
		return "", errors.New("voice candidate path is outside its runtime directory")
	}
	for _, char := range relative {
		if !strings.ContainsRune("0123456789abcdef", char) {
			return "", errors.New("voice candidate path is invalid")
		}
	}
	if !isRegularFileWithoutLinks(home, filepath.Join(index.RuntimeRoot, "module-state.json")) {
		return "", errors.New("voice candidate manifest is missing or unsafe")
	}
	return filepath.Clean(index.RuntimeRoot), nil
}

func activateVoiceInstallCandidate(home, candidate string) error {
	root, err := voiceRuntimeFromIndex(home, "candidate-state.json")
	if err != nil {
		return err
	}
	if root != filepath.Clean(candidate) {
		return errors.New("voice install candidate changed before activation")
	}
	// #nosec G703 -- both names are fixed files in the app-owned module home.
	if err := os.Rename(filepath.Join(home, "candidate-state.json"), filepath.Join(home, "module-state.json")); err != nil {
		return fmt.Errorf("voice settings were applied but the runtime index could not be promoted: %w", err)
	}
	return nil
}

func installedTTSRuntimeRoot(home string) string {
	if root, err := voiceRuntimeFromIndex(home, "module-state.json"); err == nil {
		return root
	}
	// Older installs and explicit version paths already are runtime roots.
	return home
}
