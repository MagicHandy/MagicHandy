package httpapi

import (
	"encoding/base64"
	"encoding/binary"
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

func TestNeuTTSReferencePreparationAndPrivatePreview(t *testing.T) {
	server := newTestServer(t)
	directory := t.TempDir()
	source := filepath.Join(directory, "reference.npy")
	header := "{'descr': '<i4', 'fortran_order': False, 'shape': (3,), }          \n"
	npy := make([]byte, 10+len(header)+12)
	copy(npy, []byte("\x93NUMPY"))
	npy[6] = 1
	binary.LittleEndian.PutUint16(npy[8:10], uint16(len(header))) // #nosec G115 -- fixed test header is below 64 KiB.
	copy(npy[10:], header)
	for index, value := range []uint32{3, 5, 8} {
		binary.LittleEndian.PutUint32(npy[10+len(header)+index*4:], value)
	}
	if err := os.WriteFile(source, npy, 0o600); err != nil {
		t.Fatal(err)
	}
	wav, err := base64.StdEncoding.DecodeString(silentTestWAVBase64())
	if err != nil {
		t.Fatal(err)
	}
	wavPath := filepath.Join(directory, "reference.wav")
	if err := os.WriteFile(wavPath, wav, 0o600); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"source_path": source, "reference_wav": wavPath})
	request := withController(httptest.NewRequest(http.MethodPost, "/api/voice/neutts/references", strings.NewReader(string(body))))
	prepareLocalPathPickerRequest(request)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("prepare status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Reference struct {
			ID        string `json:"id"`
			CodesPath string `json:"codes_path"`
			AudioPath string `json:"audio_path"`
		} `json:"reference"`
		PreviewURL string `json:"preview_url"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Reference.ID == "" || response.PreviewURL == "" || !strings.HasPrefix(response.Reference.CodesPath, server.voiceDataDir) || !strings.HasPrefix(response.Reference.AudioPath, server.voiceDataDir) {
		t.Fatalf("prepare response = %+v", response)
	}

	request = withController(httptest.NewRequest(http.MethodGet, response.PreviewURL, nil))
	prepareLocalPathPickerRequest(request)
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "audio/wav" || !strings.HasPrefix(recorder.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("preview response = %d, headers=%v", recorder.Code, recorder.Header())
	}
	if !strings.HasPrefix(recorder.Body.String(), "RIFF") {
		t.Fatal("preview did not return the managed WAV")
	}

	request = httptest.NewRequest(http.MethodGet, response.PreviewURL, nil)
	prepareLocalPathPickerRequest(request)
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("preview without controller = %d, want 409", recorder.Code)
	}
}

func TestNeuTTSReferencePreparationRejectsRemoteHostPaths(t *testing.T) {
	server := newTestServer(t)
	request := withController(httptest.NewRequest(http.MethodPost, "/api/voice/neutts/references", strings.NewReader(`{"source_path":"C:\\private\\voice.pt"}`)))
	request.Header.Set("Content-Type", "application/json")
	request.Host = "127.0.0.1:49717"
	request.RemoteAddr = "192.0.2.10:54321"
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("remote reference preparation = %d, want 403: %s", recorder.Code, recorder.Body.String())
	}
}

func TestNeuTTSReferenceGenerationRejectsUntrustedManagedRuntime(t *testing.T) {
	server := newTestServer(t)
	runner, _, _ := installTestAppManagedNeuTTSRuntime(t, server.voiceDataDir)
	if err := os.WriteFile(filepath.Join(filepath.Dir(runner), "runtime.json"), []byte(`{"schema_version":2,"source_commit":"untrusted"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if neuttsReferenceEncoderInstalled(server.voiceDataDir) {
		t.Fatal("reference encoder reported installed with an untrusted manifest")
	}

	wav, err := base64.StdEncoding.DecodeString(silentTestWAVBase64())
	if err != nil {
		t.Fatal(err)
	}
	wavPath := filepath.Join(t.TempDir(), "reference.wav")
	if err := os.WriteFile(wavPath, wav, 0o600); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]string{
		"reference_wav": wavPath,
		"transcript":    "Exact transcript.",
	})
	if err != nil {
		t.Fatal(err)
	}
	request := withController(httptest.NewRequest(http.MethodPost, "/api/voice/neutts/references", strings.NewReader(string(body))))
	prepareLocalPathPickerRequest(request)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("reference generation status = %d, want 422: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "app-managed NeuCodec reference encoder is unavailable") {
		t.Fatalf("reference generation error did not identify the untrusted runtime: %s", recorder.Body.String())
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

func TestVoiceManagerConfigRequiresCompleteNeuTTSRuntime(t *testing.T) {
	root := t.TempDir()
	installTestNeuTTSBackbone(t)
	adapter := filepath.Join(root, "voice-neutts-worker.exe")
	runner := filepath.Join(root, "stream_pcm.exe")
	codes := filepath.Join(root, "reference.npy")
	for _, path := range []string{adapter, runner, codes} {
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "models"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "models", "neucodec_decoder.safetensors"), []byte("decoder"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := config.DefaultSettings().Voice
	settings.Enabled = true
	settings.TTSProvider = config.VoiceTTSProviderNeuTTSAir
	settings.TTSWorkerPath = adapter
	settings.NeuTTSRunnerPath = runner
	settings.NeuTTSReferenceCodes = codes

	if got := voiceManagerConfig(settings, "", ""); got.TTS.Command != "" {
		t.Fatalf("NeuTTS without a transcript must remain unconfigured: %+v", got.TTS)
	}
	settings.NeuTTSReferenceText = "Exact reference transcript."
	got := voiceManagerConfig(settings, "", "")
	if got.TTS.Command != adapter {
		t.Fatalf("complete NeuTTS command = %q", got.TTS.Command)
	}
	wantArgs := strings.Join([]string{"-runner", runner, "-ref-text", settings.NeuTTSReferenceText, "-backbone", settings.NeuTTSBackbone, "-ref-codes", codes}, "|")
	if strings.Join(got.TTS.Args, "|") != wantArgs {
		t.Fatalf("NeuTTS args = %q, want %q", strings.Join(got.TTS.Args, "|"), wantArgs)
	}
}

func TestInspectNeuTTSModuleSeparatesAdapterAndRuntime(t *testing.T) {
	root := t.TempDir()
	installTestNeuTTSBackbone(t)
	appDir := filepath.Join(root, "app")
	if err := os.MkdirAll(appDir, 0o750); err != nil {
		t.Fatal(err)
	}
	workerName := "voice-neutts-worker"
	if runtime.GOOS == "windows" {
		workerName += ".exe"
	}
	if err := os.WriteFile(filepath.Join(appDir, workerName), []byte("worker"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := config.DefaultSettings().Voice
	status := inspectNeuTTSModule(settings, true, false, "")
	if status.State != "incomplete" || !status.WorkerInstalled || status.RuntimeInstalled || !strings.Contains(status.Message, "skipping llama.cpp also skips NeuTTS") {
		t.Fatalf("adapter-only NeuTTS status = %+v", status)
	}

	settings.NeuTTSRunnerPath = filepath.Join(root, "stream_pcm.exe")
	settings.NeuTTSReferenceCodes = filepath.Join(root, "reference.npy")
	settings.NeuTTSReferenceText = "Exact transcript."
	for _, path := range []string{settings.NeuTTSRunnerPath, settings.NeuTTSReferenceCodes} {
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "models"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "models", "neucodec_decoder.safetensors"), []byte("decoder"), 0o600); err != nil {
		t.Fatal(err)
	}
	status = inspectNeuTTSModule(settings, true, true, "")
	if status.State != "ready" || !status.Installed || !status.RuntimeInstalled {
		t.Fatalf("complete NeuTTS status = %+v", status)
	}
}

func installTestNeuTTSBackbone(t *testing.T) {
	installTestCachedBackbone(t, config.DefaultNeuTTSBackbone, "neutts-air-Q4_0.gguf")
}

func TestNeuTTSBackboneCacheDiscoversPersistedCustomRepo(t *testing.T) {
	installTestCachedBackbone(t, "example/custom-neutts", "custom-q8.gguf")
	if !neuttsBackboneCached("example/custom-neutts", "") {
		t.Fatal("custom persisted backbone cache was not discovered")
	}
	if neuttsBackboneCached(`..\escape/repo`, "") {
		t.Fatal("invalid repository identifiers must not escape the cache root")
	}
}

func installTestAppManagedNeuTTSRuntime(t *testing.T, dataDir string) (string, string, []byte) {
	t.Helper()
	runner, hfHome := neuttsAppPaths(dataDir)
	encoder, encoderModel := neuttsEncoderPaths(dataDir)
	if err := os.MkdirAll(filepath.Join(filepath.Dir(runner), "models"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(encoderModel), 0o750); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		runner: "runner",
		filepath.Join(filepath.Dir(runner), "models", "neucodec_decoder.safetensors"): "decoder",
		encoder:                "encoder",
		encoderModel:           "encoder model",
		encoderModel + ".data": "encoder weights",
		filepath.Join(filepath.Dir(runner), "DirectML.dll"): "directml",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runnerHash, runnerOK := fileSHA256(runner)
	decoderHash, decoderOK := fileSHA256(filepath.Join(filepath.Dir(runner), "models", "neucodec_decoder.safetensors"))
	encoderHash, encoderOK := fileSHA256(encoder)
	directMLHash, directMLOK := fileSHA256(filepath.Join(filepath.Dir(runner), "DirectML.dll"))
	if !runnerOK || !decoderOK || !encoderOK || !directMLOK {
		t.Fatal("could not hash app-managed NeuTTS fixtures")
	}
	manifest, err := json.Marshal(map[string]any{
		"schema_version":            2,
		"source_commit":             managedNeuTTSSource,
		"rust_toolchain":            managedNeuTTSRust,
		"backbone_revision":         managedNeuTTSBackbone,
		"codec_revision":            managedNeuTTSCodec,
		"runner_sha256":             runnerHash,
		"decoder_sha256":            decoderHash,
		"backbone_sha256":           managedNeuTTSBackboneSHA,
		"codec_checkpoint_sha256":   managedNeuTTSCodecSHA,
		"encoder_revision":          managedNeuTTSEncoder,
		"encoder_sha256":            encoderHash,
		"encoder_model_sha256":      managedNeuTTSEncoderSHA,
		"encoder_model_data_sha256": managedNeuTTSWeightsSHA,
		"directml_sha256":           directMLHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(runner), "runtime.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	installTestCachedBackboneRevisionAt(t, hfHome, config.DefaultNeuTTSBackbone, managedNeuTTSBackbone, "neutts-air-Q4_0.gguf")
	return runner, hfHome, manifest
}

func TestVoiceManagerConfigUsesAppManagedNeuTTSRuntime(t *testing.T) {
	dataDir := t.TempDir()
	runner, hfHome, manifest := installTestAppManagedNeuTTSRuntime(t, dataDir)
	realFileSHA256 := managedNeuTTSFileSHA256
	hashCalls := 0
	managedNeuTTSFileSHA256 = func(path string) (string, bool) {
		hashCalls++
		switch filepath.Base(path) {
		case "neutts-air-Q4_0.gguf":
			return managedNeuTTSBackboneSHA, true
		case "distill_neucodec_encoder.onnx":
			return managedNeuTTSEncoderSHA, true
		case "distill_neucodec_encoder.onnx.data":
			return managedNeuTTSWeightsSHA, true
		}
		return realFileSHA256(path)
	}
	t.Cleanup(func() { managedNeuTTSFileSHA256 = realFileSHA256 })
	root := t.TempDir()
	adapter := filepath.Join(root, "voice-neutts-worker.exe")
	codes := filepath.Join(root, "reference.npy")
	for _, path := range []string{adapter, codes} {
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	settings := config.DefaultSettings().Voice
	settings.Enabled = true
	settings.TTSProvider = config.VoiceTTSProviderNeuTTSAir
	settings.TTSWorkerPath = adapter
	settings.NeuTTSReferenceCodes = codes
	settings.NeuTTSReferenceText = "Exact transcript."

	assertAppManagedNeuTTSStartup(t, settings, dataDir, runner, hfHome, &hashCalls)
	assertAppManagedNeuTTSBackboneValidation(t, &settings, dataDir, hfHome)
	assertAppManagedNeuTTSManifestValidation(t, settings, dataDir, runner, manifest)
}

func assertAppManagedNeuTTSStartup(t *testing.T, settings config.VoiceSettings, dataDir, runner, hfHome string, hashCalls *int) {
	t.Helper()
	got := voiceManagerConfig(settings, "", dataDir)
	if *hashCalls != 0 {
		t.Fatalf("startup configuration hashed managed runtime files %d times", *hashCalls)
	}
	if got.TTS.Command != settings.TTSWorkerPath || got.TTS.Env["HF_HOME"] != hfHome {
		t.Fatalf("app-managed NeuTTS config = %+v", got.TTS)
	}
	if len(got.TTS.Args) < 2 || got.TTS.Args[1] != runner {
		t.Fatalf("app-managed NeuTTS runner args = %q, want %q", got.TTS.Args, runner)
	}
	status := inspectNeuTTSModule(settings, true, true, dataDir)
	if status.State != "ready" || !status.RuntimeInstalled || !status.Installed {
		t.Fatalf("app-managed NeuTTS status = %+v", status)
	}
	if !managedNeuTTSManifestReady(dataDir, true) {
		t.Fatal("explicit managed runtime integrity verification failed")
	}
}

func assertAppManagedNeuTTSBackboneValidation(t *testing.T, settings *config.VoiceSettings, dataDir, hfHome string) {
	t.Helper()
	installTestCachedBackboneAt(t, hfHome, "example/custom-neutts", "custom.gguf")
	settings.NeuTTSBackbone = "example/custom-neutts"
	if got := voiceManagerConfig(*settings, "", dataDir); got.TTS.Command != "" {
		t.Fatalf("app-managed NeuTTS must not substitute a custom backbone: %+v", got.TTS)
	}
	settings.NeuTTSBackbone = config.DefaultNeuTTSBackbone
	backboneRef := filepath.Join(hfHome, "hub", "models--neuphonic--neutts-air-q4-gguf", "refs", "main")
	if err := os.WriteFile(backboneRef, []byte("unexpected-revision"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := voiceManagerConfig(*settings, "", dataDir); got.TTS.Command != "" {
		t.Fatalf("app-managed NeuTTS must reject an unexpected backbone revision: %+v", got.TTS)
	}
	if err := os.WriteFile(backboneRef, []byte(managedNeuTTSBackbone), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertAppManagedNeuTTSManifestValidation(t *testing.T, settings config.VoiceSettings, dataDir, runner string, manifest []byte) {
	t.Helper()
	if err := os.WriteFile(runner, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if managedNeuTTSManifestReady(dataDir, true) {
		t.Fatal("explicit integrity verification accepted changed runner bytes")
	}
	if got := voiceManagerConfig(settings, "", dataDir); got.TTS.Command == "" {
		t.Fatalf("startup should trust the installer-published manifest without re-hashing large artifacts: %+v", got.TTS)
	}
	if err := os.WriteFile(runner, []byte("runner"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest = []byte(strings.Replace(string(manifest), managedNeuTTSSource, "untrusted-source", 1))
	if err := os.WriteFile(filepath.Join(filepath.Dir(runner), "runtime.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := voiceManagerConfig(settings, "", dataDir); got.TTS.Command != "" {
		t.Fatalf("app-managed NeuTTS must reject an unexpected manifest: %+v", got.TTS)
	}
}

func installTestCachedBackbone(t *testing.T, repo, filename string) {
	t.Helper()
	hfHome := t.TempDir()
	t.Setenv("HF_HOME", hfHome)
	installTestCachedBackboneAt(t, hfHome, repo, filename)
}

func installTestCachedBackboneAt(t *testing.T, hfHome, repo, filename string) {
	t.Helper()
	installTestCachedBackboneRevisionAt(t, hfHome, repo, "test-revision", filename)
}

func installTestCachedBackboneRevisionAt(t *testing.T, hfHome, repo, revision, filename string) {
	t.Helper()
	repoRoot := filepath.Join(hfHome, "hub", "models--"+strings.ReplaceAll(repo, "/", "--"))
	if err := os.MkdirAll(filepath.Join(repoRoot, "refs"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "refs", "main"), []byte(revision), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(repoRoot, "snapshots", revision)
	if err := os.MkdirAll(snapshot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, filename), []byte("gguf"), 0o600); err != nil {
		t.Fatal(err)
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

func TestVoiceWorkerStartAutoLoadsModel(t *testing.T) {
	server := newTestServer(t)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Voice.Enabled = true
		settings.Voice.TTSProvider = config.VoiceProviderCustom
		settings.Voice.TTSWorkerPath = chatStubBinary(t)
		settings.Voice.TTSWorkerArgs = []string{"-role", "tts"}
		return settings
	})
	server.applyVoiceSettingsTransition(snapshotSettings(t, server))

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
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Voice.Enabled = true
		settings.Voice.ASRProvider = config.VoiceProviderCustom
		return settings
	})
	server.applyVoiceSettingsTransition(snapshotSettings(t, server))

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
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Voice.Enabled = true
		settings.Voice.ASRProvider = config.VoiceProviderCustom
		settings.Voice.ASRWorkerPath = chatStubBinary(t)
		settings.Voice.ASRWorkerArgs = []string{"-role", "asr", "-start-loaded"}
		return settings
	})
	server.applyVoiceSettingsTransition(snapshotSettings(t, server))

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
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Voice.Enabled = true
		settings.Voice.TTSProvider = config.VoiceProviderCustom
		settings.Voice.TTSWorkerPath = chatStubBinary(t)
		settings.Voice.TTSWorkerArgs = []string{"-role", "tts", "-start-loaded"}
		return settings
	})
	server.applyVoiceSettingsTransition(snapshotSettings(t, server))
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
		"voice": {"enabled": true, "tts_worker_path": "C:\\workers\\stub.exe", "tts_worker_args": ["-role", "tts"]},
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
	if len(settings.Voice.TTSWorkerArgs) != 2 {
		t.Fatalf("tts worker args = %v", settings.Voice.TTSWorkerArgs)
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
