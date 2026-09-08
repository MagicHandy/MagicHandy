package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func voiceUpdateFixture(t *testing.T, module setupVoiceModule) (string, string) {
	t.Helper()
	root, bundle := t.TempDir(), t.TempDir()
	launcher := "faster-qwen-server.py"
	if module.ID == "chatterbox" {
		launcher = "chatterbox-server.py"
	}
	files := map[string]string{
		filepath.Join(bundle, "scripts", "install-tts-module.ps1"): "# installer",
		filepath.Join(bundle, "scripts", "tts", launcher):          "# current adapter\n",
		filepath.Join(bundle, "scripts", "tts", "tts_stream.py"):   "# current stream helper\n",
		filepath.Join(root, "magichandy-"+launcher):                "# current adapter\r\n",
		filepath.Join(root, "tts_stream.py"):                       "# current stream helper\r\n",
		filepath.Join(root, "module-state.json"):                   `{"source_revision":"` + module.SourceRevision + `"}`,
	}
	for path, contents := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root, filepath.Join(bundle, "magichandy.exe")
}

func TestVoiceModuleUpdateDetection(t *testing.T) {
	for _, module := range setupVoiceModules {
		t.Run(module.ID, func(t *testing.T) {
			root, executable := voiceUpdateFixture(t, module)
			current := inspectVoiceModuleUpdate(module.Provider, root, executable)
			if current.Available || current.ID == "" || current.Module != module.ID {
				t.Fatalf("current module (including CRLF) = %+v", current)
			}
			if current.Supported != (runtime.GOOS == "windows" && runtime.GOARCH == "amd64") {
				t.Fatal("update support does not match installer platform")
			}
			for _, file := range []string{"tts_stream.py", "module-state.json"} {
				path := filepath.Join(root, file)
				original, err := os.ReadFile(path) // #nosec G304 -- fixed fixture filenames beneath t.TempDir.
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("outdated"), 0o600); err != nil {
					t.Fatal(err)
				}
				update := inspectVoiceModuleUpdate(module.Provider, root, executable)
				if !update.Available || update.ID != current.ID {
					t.Fatalf("outdated %s = %+v", file, update)
				}
				if err := os.WriteFile(path, original, 0o600); err != nil { // #nosec G703 -- restores the same test-owned fixture file.
					t.Fatal(err)
				}
			}
			if err := os.Remove(filepath.Join(root, "tts_stream.py")); err != nil {
				t.Fatal(err)
			}
			if update := inspectVoiceModuleUpdate(module.Provider, root, executable); !update.Available {
				t.Fatal("legacy module without streaming helper was not offered an update")
			}
			if err := os.Remove(filepath.Join(filepath.Dir(executable), "scripts", "tts", "tts_stream.py")); err != nil {
				t.Fatal(err)
			}
			if update := inspectVoiceModuleUpdate(module.Provider, root, executable); update.Available {
				t.Fatal("offered an update without the bundled payload")
			}
		})
	}
}

func TestVoiceModuleUpdateIgnoresFreshAndExternalProviders(t *testing.T) {
	root, executable := voiceUpdateFixture(t, setupVoiceModules[0])
	for _, test := range []struct{ provider, root string }{
		{config.VoiceTTSProviderFasterQwen, t.TempDir()},
		{config.VoiceTTSProviderOpenAICompat, root},
		{config.VoiceProviderNone, root},
	} {
		if update := inspectVoiceModuleUpdate(test.provider, test.root, executable); update.Available || update.ID != "" {
			t.Fatalf("unexpected update for %s: %+v", test.provider, update)
		}
	}
}

func TestVoiceModuleUpdateCacheForceAndSelectionChange(t *testing.T) {
	module := setupVoiceModules[0]
	root, executable := voiceUpdateFixture(t, module)
	settings := config.DefaultSettings().Voice
	settings.TTSProvider, settings.TTSModuleRoot = module.Provider, root
	var cache voiceModuleUpdateCache
	if cache.inspect(settings, executable, "", false).Available {
		t.Fatal("current fixture needs update")
	}
	if err := os.Remove(filepath.Join(root, "tts_stream.py")); err != nil {
		t.Fatal(err)
	}
	if cache.inspect(settings, executable, "", false).Available {
		t.Fatal("state polling bypassed cache")
	}
	if !cache.inspect(settings, executable, "", true).Available {
		t.Fatal("explicit update did not recheck disk")
	}
	settings.TTSModuleRoot, _ = voiceUpdateFixture(t, module)
	if cache.inspect(settings, executable, "", false).Available {
		t.Fatal("new selected root reused stale result")
	}
}

func TestTTSModuleUpdateRequiresControllerAndRejectsStaleUpdate(t *testing.T) {
	server := newTestServer(t)
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, httptest.NewRequest(method, "/api/voice/module/update", strings.NewReader(`{}`)))
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "controller") {
			t.Fatalf("%s without controller: %d", method, response.Code)
		}
	}
	root, executable := voiceUpdateFixture(t, setupVoiceModules[0])
	server.voiceExecutable = executable
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Voice.TTSProvider, settings.Voice.TTSModuleRoot = config.VoiceTTSProviderFasterQwen, root
		return settings
	})
	if err := os.Remove(filepath.Join(root, "tts_stream.py")); err != nil {
		t.Fatal(err)
	}
	settings, _ := server.store.Snapshot()
	update := server.ttsModuleUpdate(settings.Voice, false)
	if !update.Available {
		t.Fatal("fixture should offer update")
	}
	// A repaired install between display and click must not start a redundant installer.
	if err := os.WriteFile(filepath.Join(root, "tts_stream.py"), []byte("# current stream helper\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"stale", update.ID} {
		body, _ := json.Marshal(map[string]string{"update_id": id})
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, withController(httptest.NewRequest(http.MethodPost, "/api/voice/module/update", strings.NewReader(string(body)))))
		if response.Code != http.StatusConflict {
			t.Fatalf("stale update: %d %s", response.Code, response.Body.String())
		}
	}
	if server.setup.Snapshot() != nil {
		t.Fatal("stale update launched an installer")
	}
}

func TestTTSModuleUpdateStagesAndActivatesThroughHTTP(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skip("managed installer uses Windows PowerShell")
	}
	server := newTestServer(t)
	module := setupVoiceModules[1]
	home, executable := voiceUpdateFixture(t, module)
	server.voiceExecutable, server.setup.executablePath = executable, executable
	// A fixture installer performs no network access or Python execution. The
	// actual handler, job process, candidate checks and settings activation run.
	script := filepath.Join(filepath.Dir(executable), "scripts", "install-tts-module.ps1")
	if err := os.WriteFile(script, []byte("Write-Output 'Fixture runtime prepared.'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	managedTestFile(t, filepath.Join(filepath.Dir(executable), "voice-openai-tts-worker.exe"))
	candidate := filepath.Join(home, "runtimes", strings.Repeat("b", 32))
	for path, content := range map[string]string{
		filepath.Join(candidate, ".venv", "Scripts", "python.exe"):  "fixture",
		filepath.Join(candidate, "source", "server.py"):             "# fixture",
		filepath.Join(candidate, "runtime", "config.yaml"):          "# fixture",
		filepath.Join(candidate, "runtime", "voices", "Emily.wav"):  "fixture voice",
		filepath.Join(home, "runtime", "voices", "Emily.wav"):       "fixture voice",
		filepath.Join(candidate, "magichandy-chatterbox-server.py"): "# current adapter\n",
		filepath.Join(candidate, "tts_stream.py"):                   "# current stream helper\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	manifest, _ := json.Marshal(map[string]any{"schema_version": 2, "module": module.ID, "provider": module.Provider, "model": config.DefaultChatterboxModel, "voice": "Emily.wav", "source_revision": module.SourceRevision})
	if err := os.WriteFile(filepath.Join(candidate, "module-state.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	index, _ := json.Marshal(map[string]any{"schema_version": 2, "runtime_root": candidate})
	if err := os.WriteFile(filepath.Join(home, "candidate-state.json"), index, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(home, "tts_stream.py")); err != nil {
		t.Fatal(err)
	}
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Voice.TTSProvider, settings.Voice.TTSModuleRoot = module.Provider, home
		settings.Voice.TTSDevice = config.TTSDeviceCPU
		return settings
	})
	before, _ := server.store.Snapshot()
	update := server.ttsModuleUpdate(before.Voice, true)
	body, _ := json.Marshal(map[string]string{"update_id": update.ID})
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, withController(httptest.NewRequest(http.MethodPost, "/api/voice/module/update", strings.NewReader(string(body)))))
	if response.Code != http.StatusAccepted {
		t.Fatalf("start update: %d %s", response.Code, response.Body.String())
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		job := server.setup.Snapshot()
		if job.Status == setupJobComplete {
			break
		}
		if job.Status == setupJobFailed {
			t.Fatalf("update failed: %s", job.Message)
		}
		time.Sleep(20 * time.Millisecond)
	}
	after, _ := server.store.Snapshot()
	result := server.ttsModuleUpdate(after.Voice, false)
	if after.Voice.TTSModuleRoot != candidate || result.Available || result.Job == nil || result.Job.Status != setupJobComplete {
		t.Fatalf("candidate was not activated: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(home, "magichandy-chatterbox-server.py")); err != nil {
		t.Fatal("previous runtime was removed")
	}
}

func TestTTSModuleUpdateCancelCannotCancelReplacementJob(t *testing.T) {
	server := newTestServer(t)
	ctx, job, err := server.setup.reserveJob("tts_update", "chatterbox", "cpu", "queued")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		id     string
		status int
	}{{"old-job", http.StatusConflict}, {job.ID, http.StatusOK}} {
		body, _ := json.Marshal(map[string]string{"job_id": test.id})
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, withController(httptest.NewRequest(http.MethodDelete, "/api/voice/module/update", strings.NewReader(string(body)))))
		if response.Code != test.status {
			t.Fatalf("cancel %q: %d %s", test.id, response.Code, response.Body.String())
		}
		if (ctx.Err() != nil) != (test.id == job.ID) {
			t.Fatal("cancellation affected the wrong job")
		}
	}
}
