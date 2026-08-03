package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/voice"
)

func TestResolveWorkerBinaryOrder(t *testing.T) {
	root := t.TempDir()
	appDir := filepath.Join(root, "app")
	toolsDir := filepath.Join(root, "data", "tools")
	if err := os.MkdirAll(appDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(toolsDir, 0o750); err != nil {
		t.Fatal(err)
	}
	name := "voice-parakeet-worker"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	beside := filepath.Join(appDir, name)
	tool := filepath.Join(toolsDir, name)
	if err := os.WriteFile(beside, []byte("worker"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tool, []byte("worker"), 0o600); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(appDir, "magichandy.exe")
	if got := resolveWorkerBinary("explicit-worker", executable, filepath.Join(root, "data"), "voice-parakeet-worker"); got != "explicit-worker" {
		t.Fatalf("explicit resolution = %q", got)
	}
	if got := resolveWorkerBinary("", executable, filepath.Join(root, "data"), "voice-parakeet-worker"); got != beside {
		t.Fatalf("beside-app resolution = %q, want %q", got, beside)
	}
	if err := os.Remove(beside); err != nil {
		t.Fatal(err)
	}
	if got := resolveWorkerBinary("", executable, filepath.Join(root, "data"), "voice-parakeet-worker"); got != tool {
		t.Fatalf("tools resolution = %q, want %q", got, tool)
	}
}

func TestResolveFirstPartyWorkerBinaryFallsBackFromStaleOverride(t *testing.T) {
	root := t.TempDir()
	appDir := filepath.Join(root, "app")
	worker := managedTestFile(t, filepath.Join(appDir, workerBinaryName("voice-openai-tts-worker")))
	executable := filepath.Join(appDir, "magichandy.exe")
	stale := filepath.Join(root, "old-install", workerBinaryName("voice-openai-tts-worker"))

	if got := resolveFirstPartyWorkerBinary(stale, executable, "", "voice-openai-tts-worker"); got != worker {
		t.Fatalf("stale managed override resolved to %q, want bundled worker %q", got, worker)
	}
	explicit := managedTestFile(t, filepath.Join(root, "custom", workerBinaryName("voice-openai-tts-worker")))
	if got := resolveFirstPartyWorkerBinary(explicit, executable, "", "voice-openai-tts-worker"); got != explicit {
		t.Fatalf("valid managed override resolved to %q, want %q", got, explicit)
	}
}

func TestVoiceManagerConfigComposesExternalOpenAITTS(t *testing.T) {
	settings := config.DefaultSettings().Voice
	settings.Enabled = true
	settings.TTSProvider = config.VoiceTTSProviderOpenAICompat
	settings.TTSWorkerPath = `C:\\workers\\openai-tts.exe`
	settings.TTSBaseURL = "https://tts.example.test/api"
	settings.TTSModel = "voice-model"
	settings.TTSVoice = "speaker-a"
	settings.TTSResponseFormat = "mp3"
	settings.TTSHealthPath = "/ready"
	settings.OpenAITTSAPIKey = "private-bearer"

	got := voiceManagerConfig(settings, "", "")
	if got.TTS.Command != settings.TTSWorkerPath {
		t.Fatalf("external TTS command = %q", got.TTS.Command)
	}
	wantArgs := "-base-url|https://tts.example.test/api|-model|voice-model|-voice|speaker-a|-response-format|mp3|-health-path|/ready"
	if joined := strings.Join(got.TTS.Args, "|"); joined != wantArgs {
		t.Fatalf("external TTS args = %q, want %q", joined, wantArgs)
	}
	if got.TTS.Env["OPENAI_TTS_API_KEY"] != "private-bearer" ||
		strings.Contains(strings.Join(got.TTS.Args, " "), "private-bearer") {
		t.Fatalf("compatible TTS bearer must be environment-only: %+v", got.TTS)
	}
	if got.TTS.JobTimeout != 10*time.Minute {
		t.Fatalf("compatible TTS job timeout = %v", got.TTS.JobTimeout)
	}
}

func TestVoiceManagerConfigComposesManagedFasterQwen(t *testing.T) {
	root := t.TempDir()
	python := managedTestFile(t, filepath.Join(root, ".venv", managedPythonDirectory(), managedPythonName()))
	server := managedTestFile(t, filepath.Join(root, "source", "examples", "openai_server.py"))
	launcher := managedTestFile(t, filepath.Join(root, "magichandy-faster-qwen-server.py"))
	reference := managedTestFile(t, filepath.Join(root, "voice.wav"))
	model := managedFasterQwenSnapshot(t, root, config.DefaultFasterQwenModel, "abc123")

	settings := config.DefaultSettings().Voice
	settings.Enabled = true
	settings.TTSProvider = config.VoiceTTSProviderFasterQwen
	settings.TTSWorkerPath = "voice-openai-tts-worker"
	settings.TTSAutoLaunch = true
	settings.TTSModuleRoot = root
	settings.TTSModel = config.DefaultFasterQwenModel
	settings.TTSVoice = "default"
	settings.TTSReferenceWAV = reference
	settings.TTSReferenceText = "Exact words from the reference."
	settings.TTSLanguage = "English"
	settings.TTSDevice = config.TTSDeviceCUDA
	settings.TTSServerPort = 9015
	settings.OpenAITTSAPIKey = "stale-external-key"

	got := voiceManagerConfig(settings, "", t.TempDir())
	if got.TTS.Command != settings.TTSWorkerPath {
		t.Fatalf("managed Faster Qwen worker = %+v", got.TTS)
	}
	if got.TTS.Env["HF_HOME"] != filepath.Join(root, "model-cache") ||
		got.TTS.Env["HF_HUB_OFFLINE"] != "1" ||
		got.TTS.Env["TRANSFORMERS_OFFLINE"] != "1" ||
		got.TTS.Env["NUMBA_CACHE_DIR"] != filepath.Join(root, "runtime-cache", "numba") ||
		got.TTS.Env["OPENAI_TTS_API_KEY"] != "" {
		t.Fatalf("managed Faster Qwen environment = %+v", got.TTS.Env)
	}
	assertArgumentsContain(t, got.TTS.Args,
		[2]string{"-server-command", python},
		[2]string{"-server-dir", filepath.Join(root, "source")},
		[2]string{"-server-port", "9015"},
		[2]string{"-server-arg", launcher},
		[2]string{"-server-arg", server},
		[2]string{"-server-arg", "--model"},
		[2]string{"-server-arg", model},
		[2]string{"-server-arg", "--ref-audio"},
		[2]string{"-server-arg", reference},
		[2]string{"-server-arg", "--ref-text"},
		[2]string{"-server-arg", settings.TTSReferenceText},
		[2]string{"-server-arg", "--host"},
		[2]string{"-server-arg", "127.0.0.1"},
	)
	if got.TTS.JobTimeout != voiceModelLoadTimeout {
		t.Fatalf("managed Faster Qwen job timeout = %v", got.TTS.JobTimeout)
	}
	for _, runtimeControl := range []string{"-seed", "-randomize-seed", "-instruct"} {
		if slices.Contains(got.TTS.Args, runtimeControl) {
			t.Fatalf("per-request Qwen control %q leaked into process args: %+v", runtimeControl, got.TTS.Args)
		}
	}

	settings.TTSSeedMode = config.TTSSeedModeVaried
	settings.TTSTonePreset = config.TTSToneWarm
	settings.TTSSeed = 42
	updated := voiceManagerConfig(settings, "", t.TempDir())
	if !got.Equal(updated) {
		t.Fatalf("seed/tone edit changed Qwen process config:\nbefore=%+v\nafter=%+v", got.TTS, updated.TTS)
	}
	request := voiceSpeechRequest(settings, "Speak now.")
	if request.Seed == nil || *request.Seed != 42 || !request.RandomizeSeed ||
		request.Instruct != config.ResolveTTSTonePrompt(settings) {
		t.Fatalf("Qwen speech request controls = %+v", request)
	}

	settings.TTSReferenceText = ""
	if incomplete := voiceManagerConfig(settings, "", ""); incomplete.TTS.Command != "" {
		t.Fatalf("incomplete managed Faster Qwen module must not start: %+v", incomplete.TTS)
	}
}

func TestOpenAITTSInstructionIsScopedToFasterQwen(t *testing.T) {
	settings := config.DefaultSettings().Voice
	settings.Enabled = true
	settings.TTSProvider = config.VoiceTTSProviderOpenAICompat
	settings.TTSWorkerPath = "voice-openai-tts-worker"
	settings.TTSModel = "external-model"
	settings.TTSTonePreset = config.TTSToneWarm

	got := voiceManagerConfig(settings, "", "")
	if slices.Contains(got.TTS.Args, "-instruct") {
		t.Fatalf("generic compatible provider received Qwen instruction args: %+v", got.TTS.Args)
	}
}

func TestFasterQwenModelPathRejectsAmbiguousOrIncompleteCaches(t *testing.T) {
	root := t.TempDir()
	settings := config.DefaultSettings().Voice
	settings.TTSProvider = config.VoiceTTSProviderFasterQwen
	settings.TTSModuleRoot = root
	settings.TTSModel = config.DefaultFasterQwenModel
	repositoryCache := managedFasterQwenRepositoryCache(root, settings.TTSModel)

	managedFasterQwenModelFiles(t, filepath.Join(repositoryCache, "snapshots", "one"))
	got, err := fasterQwenModelPath(settings, "")
	if err != nil || got != filepath.Join(repositoryCache, "snapshots", "one") {
		t.Fatalf("single unreferenced model snapshot = %q, %v", got, err)
	}

	managedFasterQwenModelFiles(t, filepath.Join(repositoryCache, "snapshots", "two"))
	if _, err := fasterQwenModelPath(settings, ""); err == nil || !strings.Contains(err.Error(), "2 snapshots") {
		t.Fatalf("ambiguous model snapshots error = %v", err)
	}

	managedTestFile(t, filepath.Join(repositoryCache, "refs", "main"))
	if _, err := fasterQwenModelPath(settings, ""); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("incomplete referenced snapshot error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(repositoryCache, "refs", "main"), []byte(`..\escape`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fasterQwenModelPath(settings, ""); err == nil || !strings.Contains(err.Error(), "invalid refs/main") {
		t.Fatalf("unsafe model revision error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repositoryCache, "refs", "main"), make([]byte, (4<<10)+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fasterQwenModelPath(settings, ""); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("oversized model revision error = %v", err)
	}
}

func TestFasterQwenModelPathPrefersMaterializedModel(t *testing.T) {
	root := t.TempDir()
	settings := config.DefaultSettings().Voice
	settings.TTSProvider = config.VoiceTTSProviderFasterQwen
	settings.TTSModuleRoot = root
	settings.TTSModel = config.DefaultFasterQwenModel
	materialized := filepath.Join(root, "model")
	managedFasterQwenMaterializedModel(t, materialized, settings.TTSModel)

	got, err := fasterQwenModelPath(settings, "")
	if err != nil || got != materialized {
		t.Fatalf("materialized model = %q, %v; want %q", got, err, materialized)
	}

	settings.TTSModel = "fixture/different-model"
	if _, err := fasterQwenModelPath(settings, ""); err == nil || !strings.Contains(err.Error(), "belongs to") {
		t.Fatalf("mismatched materialized model error = %v", err)
	}

	managedFasterQwenMaterializedManifest(t, materialized, settings.TTSModel, false)
	managedTestFile(t, filepath.Join(materialized, fasterQwenMaterializedManifest))
	if _, err := fasterQwenModelPath(settings, ""); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("in-progress materialized model error = %v", err)
	}
}

func TestFasterQwenModelPathRetainsCompleteLegacyCacheDuringMaterialization(t *testing.T) {
	root := t.TempDir()
	settings := config.DefaultSettings().Voice
	settings.TTSProvider = config.VoiceTTSProviderFasterQwen
	settings.TTSModuleRoot = root
	settings.TTSModel = config.DefaultFasterQwenModel
	managedTestFile(t, filepath.Join(root, "model", "partial.txt"))
	legacy := managedFasterQwenSnapshot(t, root, settings.TTSModel, "abc123")

	got, err := fasterQwenModelPath(settings, "")
	if err != nil || got != legacy {
		t.Fatalf("legacy fallback = %q, %v; want %q", got, err, legacy)
	}
}

func TestValidateFasterQwenModelDirectoryRequiresIndexedShards(t *testing.T) {
	directory := t.TempDir()
	managedFasterQwenModelFiles(t, directory)
	if err := os.Remove(filepath.Join(directory, "model.safetensors")); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(directory, "model.safetensors.index.json")
	if err := os.WriteFile(indexPath, []byte(`{"weight_map":{"layer":"model-00001-of-00001.safetensors"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateFasterQwenModelDirectory(directory); err == nil || !strings.Contains(err.Error(), "shard is missing") {
		t.Fatalf("missing indexed shard error = %v", err)
	}
	managedTestFile(t, filepath.Join(directory, "model-00001-of-00001.safetensors"))
	if _, err := validateFasterQwenModelDirectory(directory); err != nil {
		t.Fatalf("complete indexed model: %v", err)
	}
	if err := os.WriteFile(indexPath, []byte(`{"weight_map":{"layer":"../escape.safetensors"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateFasterQwenModelDirectory(directory); err == nil || !strings.Contains(err.Error(), "unsafe shard path") {
		t.Fatalf("unsafe indexed shard error = %v", err)
	}
	for _, shard := range []string{"nested//shard.safetensors", "nested/./shard.safetensors", `nested\shard.safetensors`} {
		contents := []byte(`{"weight_map":{"layer":` + strconv.Quote(shard) + `}}`)
		if err := os.WriteFile(indexPath, contents, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := validateFasterQwenModelDirectory(directory); err == nil || !strings.Contains(err.Error(), "unsafe shard path") {
			t.Fatalf("unsafe indexed shard %q error = %v", shard, err)
		}
	}
	if err := os.WriteFile(indexPath, make([]byte, maxFasterQwenWeightIndexBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateFasterQwenModelDirectory(directory); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("oversized indexed shard map error = %v", err)
	}
}

func TestValidateFasterQwenMaterializedModelRejectsLinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	managedFasterQwenMaterializedModel(t, target, config.DefaultFasterQwenModel)
	link := filepath.Join(root, "model")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("model-directory symlinks unavailable: %v", err)
	}
	if _, err := validateFasterQwenMaterializedModelDirectory(link, config.DefaultFasterQwenModel); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("linked materialized model error = %v", err)
	}
}

func TestVoiceManagerConfigComposesManagedChatterbox(t *testing.T) {
	root := t.TempDir()
	python := managedTestFile(t, filepath.Join(root, ".venv", managedPythonDirectory(), managedPythonName()))
	server := managedTestFile(t, filepath.Join(root, "source", "server.py"))
	launcher := managedTestFile(t, filepath.Join(root, "magichandy-chatterbox-server.py"))
	configPath := managedTestFile(t, filepath.Join(root, "runtime", "config.yaml"))
	voicePath := managedTestFile(t, filepath.Join(root, "runtime", "voices", "sample.wav"))

	settings := config.DefaultSettings().Voice
	settings.Enabled = true
	settings.TTSProvider = config.VoiceTTSProviderChatterbox
	settings.TTSWorkerPath = "voice-openai-tts-worker"
	settings.TTSAutoLaunch = true
	settings.TTSModuleRoot = root
	settings.TTSBaseURL = "http://127.0.0.1:8992"
	settings.TTSModel = config.DefaultChatterboxModel
	settings.TTSVoice = "sample.wav"
	settings.TTSHealthPath = config.DefaultChatterboxHealthPath
	settings.TTSServerPort = 8992

	launch, ready := managedTTSCommand(settings, "")
	if !ready || launch.command != python || launch.directory != filepath.Dir(configPath) ||
		len(launch.args) != 2 || launch.args[0] != launcher || launch.args[1] != server {
		t.Fatalf("managed Chatterbox launch = %+v, ready=%v", launch, ready)
	}
	got := voiceManagerConfig(settings, "", "")
	assertArgumentsContain(t, got.TTS.Args,
		[2]string{"-server-command", python},
		[2]string{"-server-dir", filepath.Join(root, "runtime")},
		[2]string{"-server-port", "8992"},
		[2]string{"-health-ready-field", "loaded"},
		[2]string{"-server-arg", launcher},
		[2]string{"-server-arg", server},
	)
	if got.TTS.JobTimeout != voiceModelLoadTimeout {
		t.Fatalf("managed Chatterbox job timeout = %v", got.TTS.JobTimeout)
	}
	if err := os.Remove(voicePath); err != nil {
		t.Fatal(err)
	}
	if _, ready := managedTTSCommand(settings, ""); ready {
		t.Fatal("managed Chatterbox module with a missing selected voice reported ready")
	}
}

func TestManagedTTSAutoLaunchCanBeDisabled(t *testing.T) {
	settings := config.DefaultSettings().Voice
	settings.Enabled = true
	settings.TTSProvider = config.VoiceTTSProviderFasterQwen
	settings.TTSWorkerPath = "voice-openai-tts-worker"
	settings.TTSAutoLaunch = false
	settings.TTSBaseURL = "http://127.0.0.1:9777"
	settings.TTSModel = config.DefaultFasterQwenModel

	got := voiceManagerConfig(settings, "", "")
	if got.TTS.Command != settings.TTSWorkerPath {
		t.Fatalf("user-managed server adapter = %+v", got.TTS)
	}
	if strings.Contains(strings.Join(got.TTS.Args, "|"), "-server-command") {
		t.Fatalf("auto-launch off still composed a managed child: %+v", got.TTS.Args)
	}
}

func TestInspectTTSModuleSeparatesAdapterAndRuntime(t *testing.T) {
	root := t.TempDir()
	appDir := filepath.Join(root, "app")
	worker := managedTestFile(t, filepath.Join(appDir, workerBinaryName("voice-openai-tts-worker")))
	settings := config.DefaultSettings().Voice
	settings.TTSProvider = config.VoiceTTSProviderFasterQwen
	settings.TTSModuleRoot = filepath.Join(root, "module")
	settings.TTSModel = config.DefaultFasterQwenModel

	status := inspectTTSModule(settings, filepath.Join(appDir, "magichandy.exe"), "")
	if status.State != "incomplete" || !status.WorkerInstalled || status.RuntimeInstalled {
		t.Fatalf("adapter-only module status = %+v (worker %q)", status, worker)
	}

	managedTestFile(t, filepath.Join(settings.TTSModuleRoot, ".venv", managedPythonDirectory(), managedPythonName()))
	managedTestFile(t, filepath.Join(settings.TTSModuleRoot, "source", "examples", "openai_server.py"))
	managedTestFile(t, filepath.Join(settings.TTSModuleRoot, "magichandy-faster-qwen-server.py"))
	status = inspectTTSModule(settings, filepath.Join(appDir, "magichandy.exe"), "")
	if status.State != "incomplete" || status.Installed || status.RuntimeInstalled ||
		!strings.Contains(status.Message, "Rerun") {
		t.Fatalf("pre-model module status = %+v", status)
	}

	managedFasterQwenSnapshot(t, settings.TTSModuleRoot, settings.TTSModel, "abc123")
	status = inspectTTSModule(settings, filepath.Join(appDir, "magichandy.exe"), "")
	if status.State != "incomplete" || status.Installed || !status.RuntimeInstalled ||
		!strings.Contains(status.Message, "Voice settings") {
		t.Fatalf("pre-reference module status = %+v", status)
	}

	settings.TTSReferenceText = "Exact transcript."
	settings.TTSReferenceWAV = filepath.Join(root, "reference.wav")
	managedTestFile(t, settings.TTSReferenceWAV)
	status = inspectTTSModule(settings, filepath.Join(appDir, "magichandy.exe"), "")
	if status.State != "ready" || !status.Installed || !status.RuntimeInstalled {
		t.Fatalf("complete module status = %+v", status)
	}
}

func managedFasterQwenSnapshot(t *testing.T, root, repository, revision string) string {
	t.Helper()
	repositoryCache := managedFasterQwenRepositoryCache(root, repository)
	snapshot := filepath.Join(repositoryCache, "snapshots", revision)
	managedFasterQwenModelFiles(t, snapshot)
	ref := managedTestFile(t, filepath.Join(repositoryCache, "refs", "main"))
	if err := os.WriteFile(ref, []byte(revision+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func managedFasterQwenRepositoryCache(root, repository string) string {
	return filepath.Join(root, "model-cache", "hub", "models--"+strings.ReplaceAll(repository, "/", "--"))
}

func managedFasterQwenModelFiles(t *testing.T, directory string) {
	t.Helper()
	for _, path := range []string{
		filepath.Join(directory, "config.json"),
		filepath.Join(directory, "generation_config.json"),
		filepath.Join(directory, "merges.txt"),
		filepath.Join(directory, "preprocessor_config.json"),
		filepath.Join(directory, "tokenizer_config.json"),
		filepath.Join(directory, "vocab.json"),
		filepath.Join(directory, "model.safetensors"),
		filepath.Join(directory, "speech_tokenizer", "config.json"),
		filepath.Join(directory, "speech_tokenizer", "configuration.json"),
		filepath.Join(directory, "speech_tokenizer", "model.safetensors"),
		filepath.Join(directory, "speech_tokenizer", "preprocessor_config.json"),
	} {
		managedTestFile(t, path)
	}
}

func managedFasterQwenMaterializedModel(t *testing.T, directory, repository string) {
	t.Helper()
	managedFasterQwenModelFiles(t, directory)
	managedFasterQwenMaterializedManifest(t, directory, repository, true)
}

func managedFasterQwenMaterializedManifest(t *testing.T, directory, repository string, complete bool) {
	t.Helper()
	contents, err := json.Marshal(map[string]any{
		"schema_version": 1,
		"repository":     repository,
		"complete":       complete,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(directory), fasterQwenMaterializedManifest), contents, 0o600); err != nil {
		t.Fatal(err)
	}
}

func managedTestFile(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func managedPythonDirectory() string {
	if runtime.GOOS == "windows" {
		return "Scripts"
	}
	return "bin"
}

func managedPythonName() string {
	if runtime.GOOS == "windows" {
		return "python.exe"
	}
	return "python"
}

func workerBinaryName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func assertArgumentsContain(t *testing.T, arguments []string, pairs ...[2]string) {
	t.Helper()
	for _, pair := range pairs {
		key, value := pair[0], pair[1]
		found := false
		for position := 0; position+1 < len(arguments); position++ {
			if arguments[position] == key && arguments[position+1] == value {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("arguments %q do not contain %q followed by %q", arguments, key, value)
		}
	}
}

func TestAudioContentTypeSupportsCompatibleTTSFormats(t *testing.T) {
	for format, want := range map[string]string{
		"wav":  "audio/wav",
		"mp3":  "audio/mpeg",
		"opus": "audio/ogg",
		"aac":  "audio/aac",
		"flac": "audio/flac",
	} {
		if got := audioContentType(format); got != want {
			t.Errorf("audioContentType(%q) = %q, want %q", format, got, want)
		}
	}
}

func TestVoiceInputPreferencesPersistAndValidate(t *testing.T) {
	server := newTestServer(t)
	request := withController(httptest.NewRequest(http.MethodPut, "/api/voice/input-preferences", strings.NewReader(`{
		"input_mode":"hold",
		"input_sensitivity":72,
		"input_silence_ms":1300,
		"input_noise_suppression":false
	}`)))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("save input preferences = %d: %s", recorder.Code, recorder.Body.String())
	}
	saved, _ := server.store.Snapshot()
	if saved.Voice.InputMode != config.VoiceInputModeHold || saved.Voice.InputSensitivity != 72 ||
		saved.Voice.InputSilenceMillis != 1300 || saved.Voice.InputNoiseSuppress {
		t.Fatalf("saved input preferences = %+v", saved.Voice)
	}

	invalid := []string{
		`{"input_mode":"timed"}`,
		`{"input_sensitivity":0}`,
		`{"input_sensitivity":101}`,
		`{"input_silence_ms":0}`,
		`{"input_silence_ms":3001}`,
	}
	for _, body := range invalid {
		request = withController(httptest.NewRequest(http.MethodPut, "/api/voice/input-preferences", strings.NewReader(body)))
		recorder = httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("invalid input preferences %s = %d, want 400", body, recorder.Code)
		}
		unchanged, _ := server.store.Snapshot()
		if unchanged.Voice.InputMode != config.VoiceInputModeHold || unchanged.Voice.InputSensitivity != 72 ||
			unchanged.Voice.InputSilenceMillis != 1300 || unchanged.Voice.InputNoiseSuppress {
			t.Fatalf("invalid update %s changed preferences to %+v", body, unchanged.Voice)
		}
	}
}

func TestVoiceManagerConfigComposesFirstPartyProviders(t *testing.T) {
	settings := config.DefaultSettings().Voice
	settings.Enabled = true
	settings.TTSProvider = config.VoiceTTSProviderElevenLabs
	settings.TTSWorkerPath = `C:\workers\eleven.exe`
	settings.ElevenLabsVoiceID = "voice-id"
	settings.ElevenLabsModelID = "model-id"
	settings.ElevenLabsAPIKey = "private-key"
	settings.ASRProvider = config.VoiceASRProviderParakeet
	settings.ParakeetSource = config.ParakeetSourceCustom
	settings.ASRWorkerPath = `C:\workers\parakeet.exe`
	settings.ParakeetServerPath = `C:\parakeet\server.exe`
	settings.ParakeetModelPath = `C:\parakeet\model.gguf`
	settings.ParakeetServerPort = 9011

	got := voiceManagerConfig(settings, "", "")
	if got.TTS.Command != settings.TTSWorkerPath || strings.Join(got.TTS.Args, "|") != "-voice-id|voice-id|-model-id|model-id" {
		t.Fatalf("ElevenLabs composition = %+v", got.TTS)
	}
	if got.TTS.Env["ELEVENLABS_API_KEY"] != "private-key" || strings.Contains(strings.Join(got.TTS.Args, " "), "private-key") {
		t.Fatalf("ElevenLabs secret must be environment-only: %+v", got.TTS)
	}
	wantASR := `-server-path|C:\parakeet\server.exe|-server-model|C:\parakeet\model.gguf|-server-port|9011`
	if got.ASR.Command != settings.ASRWorkerPath || strings.Join(got.ASR.Args, "|") != wantASR {
		t.Fatalf("Parakeet composition = %+v, want args %q", got.ASR, wantASR)
	}
}

func TestVoiceManagerConfigUsesAppManagedParakeetAssets(t *testing.T) {
	dataDir := t.TempDir()
	serverPath, modelPath := parakeetAppPaths(dataDir)
	if err := os.MkdirAll(filepath.Dir(serverPath), 0o750); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{serverPath: "server", modelPath: "model"} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	settings := config.DefaultSettings().Voice
	settings.Enabled = true
	settings.ASRProvider = config.VoiceASRProviderParakeet
	settings.ASRWorkerPath = "app-worker"
	settings.ParakeetSource = config.ParakeetSourceApp

	got := voiceManagerConfig(settings, "", dataDir)
	wantArgs := strings.Join([]string{"-server-path", serverPath, "-server-model", modelPath, "-server-port", "8990"}, "|")
	if got.ASR.Command != "app-worker" || strings.Join(got.ASR.Args, "|") != wantArgs {
		t.Fatalf("app-managed Parakeet composition = %+v, want args %q", got.ASR, wantArgs)
	}
}

func TestVoiceManagerConfigDoesNotStartIncompleteCustomParakeet(t *testing.T) {
	settings := config.DefaultSettings().Voice
	settings.Enabled = true
	settings.ASRProvider = config.VoiceASRProviderParakeet
	settings.ParakeetSource = config.ParakeetSourceCustom
	settings.ASRWorkerPath = "custom-worker"
	settings.ParakeetServerPath = "server.exe"

	got := voiceManagerConfig(settings, "", "")
	if got.ASR.Command != "" || got.ASR.Args != nil {
		t.Fatalf("incomplete custom Parakeet must remain unconfigured: %+v", got.ASR)
	}
}

func TestInspectParakeetAppModuleSeparatesAdapterAndRuntime(t *testing.T) {
	root := t.TempDir()
	appDir := filepath.Join(root, "app")
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(appDir, 0o750); err != nil {
		t.Fatal(err)
	}
	workerName := "voice-parakeet-worker"
	if runtime.GOOS == "windows" {
		workerName += ".exe"
	}
	workerPath := filepath.Join(appDir, workerName)
	if err := os.WriteFile(workerPath, []byte("worker"), 0o600); err != nil {
		t.Fatal(err)
	}

	status := inspectParakeetAppModule("", filepath.Join(appDir, "magichandy.exe"), dataDir)
	if status.State != "incomplete" || !status.WorkerInstalled || status.RuntimeInstalled {
		t.Fatalf("adapter-only status = %+v", status)
	}
	serverPath, modelPath := parakeetAppPaths(dataDir)
	if err := os.MkdirAll(filepath.Dir(serverPath), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{serverPath, modelPath} {
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	status = inspectParakeetAppModule("", filepath.Join(appDir, "magichandy.exe"), dataDir)
	if !status.Installed || status.State != "ready" || !status.RuntimeInstalled {
		t.Fatalf("complete status = %+v", status)
	}
}

func TestVoiceManagerConfigPreservesCustomAndDisablesNone(t *testing.T) {
	settings := config.DefaultSettings().Voice
	settings.Enabled = true
	settings.TTSProvider = config.VoiceProviderCustom
	settings.TTSWorkerPath = "custom-tts"
	settings.TTSWorkerArgs = []string{"--unchanged", "value"}
	settings.ASRProvider = config.VoiceProviderNone
	settings.ASRWorkerPath = "hidden-custom-asr"
	got := voiceManagerConfig(settings, "", "")
	if !got.TTS.Enabled || got.TTS.Command != "custom-tts" || strings.Join(got.TTS.Args, "|") != "--unchanged|value" {
		t.Fatalf("custom behavior changed: %+v", got.TTS)
	}
	if got.ASR.Enabled || got.ASR.Command != "" {
		t.Fatalf("provider none must disable hidden command: %+v", got.ASR)
	}
}

func TestSilentTestWAVBase64ProducesValidPCMSilence(t *testing.T) {
	encoded := silentTestWAVBase64()
	audio, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode test WAV: %v", err)
	}
	if !hasCanonicalASRWAV(audio) {
		t.Fatal("canonical managed-ASR validator rejected the generated WAV")
	}
	if len(audio) < 44 || string(audio[0:4]) != "RIFF" || string(audio[8:12]) != "WAVE" || string(audio[12:16]) != "fmt " || string(audio[36:40]) != "data" {
		t.Fatalf("test payload is not a canonical WAV header")
	}
	if got := binary.LittleEndian.Uint32(audio[24:28]); got != 16000 {
		t.Fatalf("sample rate = %d, want 16000", got)
	}
	if got := binary.LittleEndian.Uint16(audio[22:24]); got != 1 {
		t.Fatalf("channels = %d, want 1", got)
	}
	if got := binary.LittleEndian.Uint16(audio[34:36]); got != 16 {
		t.Fatalf("bit depth = %d, want 16", got)
	}
	if got, want := int(binary.LittleEndian.Uint32(audio[40:44])), len(audio)-44; got != want {
		t.Fatalf("data length = %d, want %d", got, want)
	}
	for _, sample := range audio[44:] {
		if sample != 0 {
			t.Fatal("test WAV must contain silence")
		}
	}
}

func TestVoiceModelLoadUsesManagedStartupTimeout(t *testing.T) {
	if got := voiceModelActionTimeout(true); got != voiceModelLoadTimeout {
		t.Fatalf("load timeout = %s, want %s", got, voiceModelLoadTimeout)
	}
	if got := voiceModelActionTimeout(false); got != voiceHealthTimeout {
		t.Fatalf("unload timeout = %s, want %s", got, voiceHealthTimeout)
	}
}

func TestVoiceStatusDefaultsToDisabled(t *testing.T) {
	server := newTestServer(t)

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/voice/status", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var payload struct {
		Voice struct {
			Enabled         bool `json:"enabled"`
			ProtocolVersion int  `json:"protocol_version"`
			Workers         map[string]struct {
				State      string `json:"state"`
				Configured bool   `json:"configured"`
			} `json:"workers"`
			Modules map[string]voiceModuleStatus `json:"modules"`
		} `json:"voice"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode voice status: %v", err)
	}
	if payload.Voice.Enabled {
		t.Fatal("voice must be disabled by default")
	}
	if payload.Voice.ProtocolVersion != 1 {
		t.Fatalf("protocol_version = %d, want 1", payload.Voice.ProtocolVersion)
	}
	for _, role := range []string{"tts", "asr"} {
		worker, ok := payload.Voice.Workers[role]
		if !ok {
			t.Fatalf("voice status is missing the %s worker", role)
		}
		if worker.State != "disabled" {
			t.Fatalf("%s worker state = %q, want disabled", role, worker.State)
		}
	}
	if _, ok := payload.Voice.Modules["parakeet"]; !ok {
		t.Fatal("voice status is missing the app-managed Parakeet module")
	}
}

func TestVoiceStateAppearsInAppState(t *testing.T) {
	server := newTestServer(t)

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/state", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if _, ok := payload["voice"]; !ok {
		t.Fatal("/api/state must include the voice block")
	}
}

func TestVoiceWorkersAutoloadFromPersistedSettings(t *testing.T) {
	tests := []struct {
		name         string
		enabled      bool
		speakReplies bool
		wantTTS      voice.WorkerState
		wantASR      voice.WorkerState
	}{
		{name: "both roles", enabled: true, speakReplies: true, wantTTS: voice.StateRunning, wantASR: voice.StateRunning},
		{name: "voice input only", enabled: true, speakReplies: false, wantTTS: voice.StateStopped, wantASR: voice.StateRunning},
		{name: "voice disabled", enabled: false, speakReplies: true, wantTTS: voice.StateDisabled, wantASR: voice.StateDisabled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, err := config.OpenStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			settings, _ := store.Snapshot()
			settings.Voice.Enabled = test.enabled
			settings.Voice.SpeakReplies = test.speakReplies
			settings.Voice.TTSProvider = config.VoiceProviderCustom
			settings.Voice.TTSWorkerPath = chatStubBinary(t)
			settings.Voice.TTSWorkerArgs = []string{"-role", "tts"}
			settings.Voice.ASRProvider = config.VoiceProviderCustom
			settings.Voice.ASRWorkerPath = chatStubBinary(t)
			settings.Voice.ASRWorkerArgs = []string{"-role", "asr"}
			if _, err := store.Save(settings); err != nil {
				t.Fatal(err)
			}
			server := newTestServerWithStore(t, store, Runtime{})
			defer server.Close()

			deadline := time.Now().Add(5 * time.Second)
			for {
				tts := server.voice.Worker(voice.RoleTTS).Status()
				asr := server.voice.Worker(voice.RoleASR).Status()
				if tts.State == test.wantTTS && asr.State == test.wantASR &&
					(tts.State != voice.StateRunning || tts.ModelState == voice.ModelStateReady) &&
					(asr.State != voice.StateRunning || asr.ModelState == voice.ModelStateReady) {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("autoload states: tts=%+v asr=%+v", tts, asr)
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}

func TestVoiceSettingsTransitionRestartsAutoloadWorker(t *testing.T) {
	server := newTestServer(t)
	previous := snapshotSettings(t, server)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Voice.Enabled = true
		settings.Voice.SpeakReplies = true
		settings.Voice.TTSProvider = config.VoiceProviderCustom
		settings.Voice.TTSWorkerPath = chatStubBinary(t)
		settings.Voice.TTSWorkerArgs = []string{"-role", "tts"}
		return settings
	})
	next := snapshotSettings(t, server)
	server.applyVoiceSettingsTransition(previous, next)
	waitForVoiceWorkerReady(t, server.voice.Worker(voice.RoleTTS))

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	if err := server.voice.Worker(voice.RoleTTS).Stop(stopCtx); err != nil {
		t.Fatalf("stop first worker: %v", err)
	}
	cancel()

	previous = next
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Voice.TTSWorkerArgs = []string{"-role", "tts", "-start-loaded"}
		return settings
	})
	next = snapshotSettings(t, server)
	server.applyVoiceSettingsTransition(previous, next)
	waitForVoiceWorkerReady(t, server.voice.Worker(voice.RoleTTS))
}

func waitForVoiceWorkerReady(t *testing.T, worker *voice.Supervisor) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		status := worker.Status()
		if status.State == voice.StateRunning && status.ModelState == voice.ModelStateReady {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("voice worker did not become ready: %+v", status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestVoiceWorkerStartAutoLoadsModel(t *testing.T) {
	server := newTestServer(t)
	saveAndApplyVoiceSettings(t, server, func(settings config.Settings) config.Settings {
		settings.Voice.Enabled = true
		settings.Voice.TTSProvider = config.VoiceProviderCustom
		settings.Voice.TTSWorkerPath = chatStubBinary(t)
		settings.Voice.TTSWorkerArgs = []string{"-role", "tts"}
		return settings
	})

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, withController(httptest.NewRequest(http.MethodPost, "/api/voice/workers/tts/start", nil)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("start = %d: %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		LoadError string `json:"load_error"`
		Worker    struct {
			State      string `json:"state"`
			ModelState string `json:"model_state"`
		} `json:"worker"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	if payload.LoadError != "" {
		t.Fatalf("auto-load reported an error: %s", payload.LoadError)
	}
	if payload.Worker.State != "running" || payload.Worker.ModelState != "ready" {
		t.Fatalf("start must leave the worker running with the model ready, got state=%q model=%q", payload.Worker.State, payload.Worker.ModelState)
	}
}

func TestVoiceWorkerStartRequiresController(t *testing.T) {
	server := newTestServer(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/voice/workers/tts/start", nil)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden && recorder.Code != http.StatusConflict {
		t.Fatalf("start without controller = %d, want a controller rejection", recorder.Code)
	}
	if recorder.Code == http.StatusOK {
		t.Fatal("start must not succeed without the controller lease")
	}
}

func TestVoiceWorkerStartWhileDisabledFails(t *testing.T) {
	server := newTestServer(t)

	recorder := httptest.NewRecorder()
	request := withController(httptest.NewRequest(http.MethodPost, "/api/voice/workers/tts/start", nil))
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusConflict)
	}
	if !contains(recorder.Body.String(), "disabled") {
		t.Fatalf("error should say voice is disabled, got %s", recorder.Body.String())
	}
}

func TestVoiceWorkerStartWithoutCommandFails(t *testing.T) {
	server := newTestServer(t)
	saveAndApplyVoiceSettings(t, server, func(settings config.Settings) config.Settings {
		settings.Voice.Enabled = true
		settings.Voice.ASRProvider = config.VoiceProviderCustom
		return settings
	})

	recorder := httptest.NewRecorder()
	request := withController(httptest.NewRequest(http.MethodPost, "/api/voice/workers/asr/start", nil))
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusConflict)
	}
	if !contains(recorder.Body.String(), "configured") {
		t.Fatalf("error should say no worker is configured, got %s", recorder.Body.String())
	}

	// The unconfigured state must be visible, not an opaque failure.
	statusRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(statusRecorder, httptest.NewRequest(http.MethodGet, "/api/voice/status", nil))
	if !contains(statusRecorder.Body.String(), "not_configured") {
		t.Fatalf("voice status should report not_configured, got %s", statusRecorder.Body.String())
	}
}

func TestVoiceUnknownRoleIsNotFound(t *testing.T) {
	server := newTestServer(t)

	recorder := httptest.NewRecorder()
	request := withController(httptest.NewRequest(http.MethodPost, "/api/voice/workers/kazoo/start", nil))
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestVoiceUnknownRequestIsNotFound(t *testing.T) {
	server := newTestServer(t)

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/voice/requests/12345", nil))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestVoiceTranscriptionUsesASRQueueAndReturnsTranscript(t *testing.T) {
	server := newTestServer(t)
	saveAndApplyVoiceSettings(t, server, func(settings config.Settings) config.Settings {
		settings.Voice.Enabled = true
		settings.Voice.ASRProvider = config.VoiceProviderCustom
		settings.Voice.ASRWorkerPath = chatStubBinary(t)
		settings.Voice.ASRWorkerArgs = []string{"-role", "asr", "-start-loaded"}
		return settings
	})

	start := httptest.NewRecorder()
	server.Handler().ServeHTTP(start, withController(httptest.NewRequest(http.MethodPost, "/api/voice/workers/asr/start", nil)))
	if start.Code != http.StatusOK {
		t.Fatalf("start ASR = %d: %s", start.Code, start.Body.String())
	}

	body := `{"audio_b64":"` + strings.Repeat("A", 128*1024) + `","audio_format":"webm"}`
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, withController(httptest.NewRequest(http.MethodPost, "/api/voice/transcriptions", strings.NewReader(body))))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("transcribe = %d: %s", recorder.Code, recorder.Body.String())
	}
	var accepted struct {
		Request struct {
			ID string `json:"id"`
		} `json:"request"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(accepted.Request.ID, "asr-") {
		t.Fatalf("ASR request ID = %q, want role-prefixed ID", accepted.Request.ID)
	}

	snapshot := waitForVoiceRequestDone(t, server, accepted.Request.ID)
	if len(snapshot.Transcript) != 1 || snapshot.Transcript[0].Text != "stub transcript" {
		t.Fatalf("transcript = %+v", snapshot)
	}

	raw := httptest.NewRequest(http.MethodPost, "/api/voice/transcriptions", strings.NewReader("browser-audio"))
	raw.Header.Set("Content-Type", "audio/webm")
	rawRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(rawRecorder, withController(raw))
	if rawRecorder.Code != http.StatusAccepted {
		t.Fatalf("raw audio transcribe = %d: %s", rawRecorder.Code, rawRecorder.Body.String())
	}
	var rawAccepted struct {
		Request struct {
			ID string `json:"id"`
		} `json:"request"`
	}
	if err := json.Unmarshal(rawRecorder.Body.Bytes(), &rawAccepted); err != nil {
		t.Fatal(err)
	}
	waitForVoiceRequestDone(t, server, rawAccepted.Request.ID)
	entries, err := os.ReadDir(filepath.Join(server.voiceDataDir, "voice", "inputs"))
	if err != nil {
		t.Fatalf("read staged voice inputs: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			t.Fatalf("unexpected file in voice input root: %s", entry.Name())
		}
		files, err := os.ReadDir(filepath.Join(server.voiceDataDir, "voice", "inputs", entry.Name()))
		if err != nil {
			t.Fatalf("read voice input session: %v", err)
		}
		if len(files) != 0 {
			t.Fatalf("completed transcription retained staged audio: %+v", files)
		}
	}
}

func TestVoiceTranscriptionRejectsStaleStopSequence(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	callMotion(t, server, http.MethodPost, "/api/motion/stop", `{}`)

	request := withController(httptest.NewRequest(http.MethodPost, "/api/voice/transcriptions", strings.NewReader("audio")))
	request.Header.Set("Content-Type", "audio/webm")
	request.Header.Set(stopSequenceHeader, "0")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "invalidated") {
		t.Fatalf("stale transcription = %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestEmergencyStopInvalidatesTrackedTTSAudio(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	saveAndApplyVoiceSettings(t, server, func(settings config.Settings) config.Settings {
		settings.Voice.Enabled = true
		settings.Voice.TTSProvider = config.VoiceProviderCustom
		settings.Voice.TTSWorkerPath = chatStubBinary(t)
		settings.Voice.TTSWorkerArgs = []string{"-role", "tts", "-start-loaded"}
		return settings
	})
	start := httptest.NewRecorder()
	server.Handler().ServeHTTP(start, withController(httptest.NewRequest(http.MethodPost, "/api/voice/workers/tts/start", nil)))
	if start.Code != http.StatusOK {
		t.Fatalf("start TTS = %d: %s", start.Code, start.Body.String())
	}

	testRequest := withController(httptest.NewRequest(http.MethodPost, "/api/voice/workers/tts/test", strings.NewReader(`{"text":"hello","delay_ms":0}`)))
	testRequest.Header.Set("Content-Type", "application/json")
	testRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(testRecorder, testRequest)
	if testRecorder.Code != http.StatusAccepted {
		t.Fatalf("TTS test = %d: %s", testRecorder.Code, testRecorder.Body.String())
	}
	var accepted struct {
		Request struct {
			ID string `json:"id"`
		} `json:"request"`
	}
	if err := json.Unmarshal(testRecorder.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	waitForVoiceRequestDone(t, server, accepted.Request.ID)

	callMotion(t, server, http.MethodPost, "/api/motion/stop", `{}`)
	audioRecorder := httptest.NewRecorder()
	audioRequest := withController(httptest.NewRequest(http.MethodGet, "/api/voice/requests/"+accepted.Request.ID+"/audio", nil))
	server.Handler().ServeHTTP(audioRecorder, audioRequest)
	if audioRecorder.Code != http.StatusConflict {
		t.Fatalf("canceled TTS audio status = %d, want conflict: %s", audioRecorder.Code, audioRecorder.Body.String())
	}
}

func waitForVoiceRequestDone(t *testing.T, server *Server, id string) voice.RequestSnapshot {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		pending, ok := server.voice.Request(id)
		if !ok {
			t.Fatalf("voice request %q was not tracked", id)
		}
		snapshot := pending.Snapshot()
		if snapshot.State == voice.RequestStateDone {
			return snapshot
		}
		if time.Now().After(deadline) {
			t.Fatalf("voice request %q did not finish: %+v", id, snapshot)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestVoiceTranscriptionRejectsUnsupportedAudioFormat(t *testing.T) {
	server := newTestServer(t)
	body := strings.NewReader(`{"audio_b64":"AA==","audio_format":"mp3"}`)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, withController(httptest.NewRequest(http.MethodPost, "/api/voice/transcriptions", body)))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "webm, ogg, or wav") {
		t.Fatalf("unsupported format = %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestManagedParakeetTranscriptionRejectsCompressedOrFakeWAV(t *testing.T) {
	server := newTestServer(t)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Voice.ASRProvider = config.VoiceASRProviderParakeet
		return settings
	})
	forgedHeader := make([]byte, 46)
	copy(forgedHeader[0:4], "RIFF")
	copy(forgedHeader[8:12], "WAVE")
	copy(forgedHeader[12:16], "junk")
	oddPCM := make([]byte, 45)
	copy(oddPCM[0:4], "RIFF")
	binary.LittleEndian.PutUint32(oddPCM[4:8], 37)
	copy(oddPCM[8:12], "WAVE")
	copy(oddPCM[12:16], "fmt ")
	binary.LittleEndian.PutUint32(oddPCM[16:20], 16)
	binary.LittleEndian.PutUint16(oddPCM[20:22], 1)
	binary.LittleEndian.PutUint16(oddPCM[22:24], 1)
	binary.LittleEndian.PutUint32(oddPCM[24:28], 16000)
	binary.LittleEndian.PutUint32(oddPCM[28:32], 32000)
	binary.LittleEndian.PutUint16(oddPCM[32:34], 2)
	binary.LittleEndian.PutUint16(oddPCM[34:36], 16)
	copy(oddPCM[36:40], "data")
	binary.LittleEndian.PutUint32(oddPCM[40:44], 1)
	for name, body := range map[string]string{
		"webm":          `{"audio_b64":"AA==","audio_format":"webm"}`,
		"headerless":    `{"audio_b64":"ZmFrZS13YXYtYnl0ZXM=","audio_format":"wav"}`,
		"forged header": `{"audio_b64":"` + base64.StdEncoding.EncodeToString(forgedHeader) + `","audio_format":"wav"}`,
		"odd PCM":       `{"audio_b64":"` + base64.StdEncoding.EncodeToString(oddPCM) + `","audio_format":"wav"}`,
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			server.Handler().ServeHTTP(recorder, withController(httptest.NewRequest(http.MethodPost, "/api/voice/transcriptions", strings.NewReader(body))))
			if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "requires a WAV") {
				t.Fatalf("managed format rejection = %d: %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestVoiceSettingsRoundTripThroughAPI(t *testing.T) {
	server := newTestServer(t)

	body := `{
		"server": {"port": 49717},
		"device": {
			"hsp_dispatch_owner": "cloud_rest",
			"firmware_api_requirement": "firmware_v4_api_v3_required",
			"api_application_id_source": "bundled_app_id",
			"api_application_id_override": ""
		},
		"motion": {"speed_min_percent": 20, "speed_max_percent": 80, "stroke_min_percent": 0, "stroke_max_percent": 100, "reverse_direction": false, "style": "balanced"},
		"llm": {"provider": "llama_cpp", "llama_cpp_mode": "managed", "llama_cpp_base_url": "http://127.0.0.1:8080", "ollama_base_url": "http://127.0.0.1:11434", "model": "local-model", "prompt_set": "magichandy_motion_v1", "request_timeout_ms": 120000},
		"voice": {
			"enabled": true,
			"tts_provider": "openai_compatible",
			"tts_worker_path": "C:\\workers\\stub.exe",
			"tts_base_url": "http://127.0.0.1:9444/",
			"tts_model": "local-voice-model",
			"tts_voice": "speaker-a",
			"tts_response_format": "wav",
			"tts_health_path": "/ready",
			"tts_server_port": 9444,
			"tts_device": "cpu",
			"openai_tts_api_key": "private-rest-key"
		},
		"diagnostics": {"verbosity": "normal"},
		"clear_connection_key": false
	}`

	recorder := httptest.NewRecorder()
	request := withController(httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body)))
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("save settings = %d: %s", recorder.Code, recorder.Body.String())
	}

	settings := snapshotSettings(t, server)
	if !settings.Voice.Enabled {
		t.Fatal("voice enabled flag did not persist")
	}
	if settings.Voice.TTSWorkerPath != `C:\workers\stub.exe` {
		t.Fatalf("tts worker path = %q", settings.Voice.TTSWorkerPath)
	}
	if settings.Voice.TTSProvider != config.VoiceTTSProviderOpenAICompat ||
		settings.Voice.TTSBaseURL != "http://127.0.0.1:9444" ||
		settings.Voice.TTSModel != "local-voice-model" ||
		settings.Voice.TTSVoice != "speaker-a" {
		t.Fatalf("compatible TTS settings did not persist: %+v", settings.Voice)
	}
	if settings.Voice.OpenAITTSAPIKey != "private-rest-key" ||
		strings.Contains(recorder.Body.String(), "private-rest-key") {
		t.Fatalf("compatible TTS key was not stored privately: response=%s", recorder.Body.String())
	}

	// The saved-but-unstarted worker must show as stopped, never autostart.
	statusRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(statusRecorder, httptest.NewRequest(http.MethodGet, "/api/voice/status", nil))
	if !contains(statusRecorder.Body.String(), `"state":"stopped"`) {
		t.Fatalf("configured tts worker should be stopped, got %s", statusRecorder.Body.String())
	}
}

func snapshotSettings(t *testing.T, server *Server) config.Settings {
	t.Helper()
	settings, _ := server.store.Snapshot()
	return settings
}

func saveAndApplyVoiceSettings(t *testing.T, server *Server, mutate func(config.Settings) config.Settings) {
	t.Helper()
	previous := snapshotSettings(t, server)
	saveSettings(t, server.store, mutate)
	server.applyVoiceSettingsTransition(previous, snapshotSettings(t, server))
}
