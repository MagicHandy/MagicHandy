package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

type voiceModuleUpdate struct {
	Available bool      `json:"available"`
	Supported bool      `json:"supported"`
	ID        string    `json:"id,omitempty"`
	Module    string    `json:"module,omitempty"`
	Busy      bool      `json:"busy,omitempty"`
	Job       *setupJob `json:"job,omitempty"`
}

// Keep startup/poll checks local and bounded. One cached entry prevents every
// browser state poll from re-reading the adapter files. Activation changes root
// and therefore invalidates the key; an explicit update always checks afresh.
type voiceModuleUpdateCache struct {
	mu      sync.Mutex
	key     string
	expires time.Time
	value   voiceModuleUpdate
}

func (c *voiceModuleUpdateCache) inspect(settings config.VoiceSettings, executablePath, dataDir string, force bool) voiceModuleUpdate {
	root := ttsModuleRoot(settings, dataDir)
	key := settings.TTSProvider + "\x00" + root + "\x00" + executablePath
	c.mu.Lock()
	defer c.mu.Unlock()
	if !force && key == c.key && time.Now().Before(c.expires) {
		return c.value
	}
	c.key, c.expires = key, time.Now().Add(30*time.Second)
	c.value = inspectVoiceModuleUpdate(settings.TTSProvider, root, executablePath)
	return c.value
}

func inspectVoiceModuleUpdate(provider, root, executablePath string) voiceModuleUpdate {
	var module setupVoiceModule
	for _, candidate := range setupVoiceModules {
		if candidate.Provider == provider {
			module = candidate
		}
	}
	if module.ID == "" || root == "" {
		return voiceModuleUpdate{}
	}
	launcher := "faster-qwen-server.py"
	if module.ID == "chatterbox" {
		launcher = "chatterbox-server.py"
	}
	installedLauncher := filepath.Join(root, "magichandy-"+launcher)
	if !isRegularFile(installedLauncher) && !isRegularFile(filepath.Join(root, "module-state.json")) {
		return voiceModuleUpdate{} // A fresh installation is not an update.
	}
	script, err := resolveTTSInstallerScript(executablePath)
	if err != nil {
		return voiceModuleUpdate{}
	}
	update := voiceModuleUpdate{Supported: runtime.GOOS == "windows" && runtime.GOARCH == "amd64", Module: module.ID}
	digest := sha256.New()
	_, _ = fmt.Fprintf(digest, "%s\x00%s\x00%s\x00", module.ID, root, module.SourceRevision)
	for _, name := range []string{launcher, "tts_stream.py"} {
		expected, err := readFileLimited(filepath.Join(filepath.Dir(script), "tts", name), 1<<20)
		if err != nil || len(expected) == 0 {
			return voiceModuleUpdate{} // Do not offer an unavailable bundled update.
		}
		expected = bytes.ReplaceAll(expected, []byte("\r\n"), []byte("\n"))
		_, _ = digest.Write(expected)
		installed := filepath.Join(root, name)
		if name == launcher {
			installed = installedLauncher
		}
		actual, err := readFileLimited(installed, 1<<20)
		if err != nil || !bytes.Equal(expected, bytes.ReplaceAll(actual, []byte("\r\n"), []byte("\n"))) {
			update.Available = true
		}
	}
	var state struct {
		SourceRevision string `json:"source_revision"`
	}
	manifest, err := readFileLimited(filepath.Join(root, "module-state.json"), 32<<10)
	if err != nil || json.Unmarshal(manifest, &state) != nil || state.SourceRevision != module.SourceRevision {
		update.Available = true
	}
	update.ID = fmt.Sprintf("%x", digest.Sum(nil))
	return update
}

func (s *Server) ttsModuleUpdate(settings config.VoiceSettings, force bool) voiceModuleUpdate {
	update := s.voiceModuleUpdates.inspect(settings, s.voiceExecutable, s.voiceDataDir, force)
	if s.setup != nil {
		if job := s.setup.Snapshot(); job != nil {
			update.Busy = job.Status == setupJobQueued || job.Status == setupJobRunning
			if job.Kind == "tts_update" && job.Module == update.Module {
				job.Output, job.Steps = "", nil
				update.Job = job
			}
		}
	}
	return update
}

func (s *Server) handleTTSModuleUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var request struct {
		UpdateID string `json:"update_id"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	settings, _ := s.store.Snapshot()
	update := s.ttsModuleUpdate(settings.Voice, true)
	if !update.Available || !update.Supported || request.UpdateID == "" || request.UpdateID != update.ID {
		writeError(w, http.StatusConflict, errors.New("the TTS module update changed or is unavailable; refresh Voice settings"))
		return
	}
	job, err := s.setup.StartVoiceUpdate(settings.Voice)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"installation": job})
}

func voiceModuleHome(root string) string {
	name := filepath.Base(root)
	if len(name) == 32 && strings.Trim(name, "0123456789abcdef") == "" && filepath.Base(filepath.Dir(root)) == "runtimes" {
		return filepath.Dir(filepath.Dir(root))
	}
	return root
}

func (s *Server) handleTTSModuleUpdateCancel(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var request struct {
		JobID string `json:"job_id"`
	}
	if err := decodeJSON(r, &request); err != nil || request.JobID == "" {
		writeError(w, http.StatusBadRequest, errors.New("a TTS update job ID is required"))
		return
	}
	job, err := s.setup.Cancel(request.JobID)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"installation": job})
}
