package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultSettingsIncludesPhaseTwoFields(t *testing.T) {
	settings := DefaultSettings()

	if settings.Version != CurrentSettingsVersion {
		t.Fatalf("version = %d, want %d", settings.Version, CurrentSettingsVersion)
	}
	if settings.Server.Port != DefaultServerPort {
		t.Fatalf("server port = %d, want %d", settings.Server.Port, DefaultServerPort)
	}
	if settings.Device.HSPDispatchOwner != DispatchOwnerCloudREST {
		t.Fatalf("dispatch owner = %q, want %q", settings.Device.HSPDispatchOwner, DispatchOwnerCloudREST)
	}
	if settings.Device.IntifaceServerAddress != DefaultIntifaceServerAddress {
		t.Fatalf("Intiface server address = %q, want %q", settings.Device.IntifaceServerAddress, DefaultIntifaceServerAddress)
	}
	if settings.Device.FirmwareAPIRequirement != FirmwareAPIRequirementRequired {
		t.Fatalf("firmware requirement = %q, want %q", settings.Device.FirmwareAPIRequirement, FirmwareAPIRequirementRequired)
	}
	if settings.Device.APIApplicationIDSource != ApplicationIDSourceBundled {
		t.Fatalf("app ID source = %q, want %q", settings.Device.APIApplicationIDSource, ApplicationIDSourceBundled)
	}
	if settings.LLM.Provider != LLMProviderLlamaCPP {
		t.Fatalf("LLM provider = %q, want %q", settings.LLM.Provider, LLMProviderLlamaCPP)
	}
	if settings.LLM.LlamaCPPMode != LlamaCPPModeManaged {
		t.Fatalf("llama.cpp mode = %q, want %q", settings.LLM.LlamaCPPMode, LlamaCPPModeManaged)
	}
	if settings.LLM.LlamaCPPBaseURL != DefaultLlamaCPPBaseURL {
		t.Fatalf("llama.cpp URL = %q, want %q", settings.LLM.LlamaCPPBaseURL, DefaultLlamaCPPBaseURL)
	}
	if settings.LLM.OllamaBaseURL != DefaultOllamaBaseURL {
		t.Fatalf("Ollama URL = %q, want %q", settings.LLM.OllamaBaseURL, DefaultOllamaBaseURL)
	}
	if settings.LLM.PromptSet != PromptSetMagicHandyMotionV1 {
		t.Fatalf("prompt set = %q, want %q", settings.LLM.PromptSet, PromptSetMagicHandyMotionV1)
	}
}

func TestBundledAPIApplicationIDUsesPublicV3ID(t *testing.T) {
	if BundledAPIApplicationID != "rQoTWeMPrklUYcfdSXYYhS_9z.jAVNwy" {
		t.Fatalf("bundled API application ID = %q, want public Handy API v3 ID", BundledAPIApplicationID)
	}
}

func TestLoadMissingSettingsUsesDefaults(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}

	settings, status := store.Snapshot()
	if !status.UsingDefaults {
		t.Fatal("missing settings file did not use defaults")
	}
	if settings.Server.Port != DefaultServerPort {
		t.Fatalf("server port = %d, want %d", settings.Server.Port, DefaultServerPort)
	}
}

func TestSaveAndLoadSettings(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}

	settings, _ := store.Snapshot()
	settings.Server.Port = 49720
	settings.Device.HSPDispatchOwner = DispatchOwnerIntiface
	settings.Device.IntifaceServerAddress = "wss://intiface.example.test/socket"
	settings.Device.APIApplicationIDSource = ApplicationIDSourceDeveloperOverride
	settings.Device.APIApplicationIDOverride = "dev-app"
	settings.Device.HandyConnectionKey = "secret"
	settings.LLM.OllamaModelsPath = `D:\Ollama\models`
	settings.Diagnostics.Verbosity = DiagnosticsVerbosityDebug
	if _, err := store.Save(settings); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore reload: %v", err)
	}
	got, status := reloaded.Snapshot()
	if status.Source != loadSourceSQLite {
		t.Fatalf("source = %q, want %q", status.Source, loadSourceSQLite)
	}
	if got.Server.Port != 49720 {
		t.Fatalf("server port = %d, want 49720", got.Server.Port)
	}
	if got.Device.HandyConnectionKey != "secret" {
		t.Fatal("connection key did not persist")
	}
	if got.Device.HSPDispatchOwner != DispatchOwnerIntiface || got.Device.IntifaceServerAddress != "wss://intiface.example.test/socket" {
		t.Fatalf("Intiface settings did not persist: %+v", got.Device)
	}
	if got.LLM.OllamaModelsPath != `D:\Ollama\models` {
		t.Fatalf("Ollama models path = %q", got.LLM.OllamaModelsPath)
	}
}

func TestOpenStoreImportsLegacySettingsFile(t *testing.T) {
	dir := t.TempDir()
	legacy := DefaultSettings()
	legacy.Server.Port = 49725
	legacy.Device.HandyConnectionKey = "secret-import-key"
	legacy.Diagnostics.Verbosity = DiagnosticsVerbosityDebug
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy settings: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, settingsFileName), data, 0o600); err != nil {
		t.Fatalf("write legacy settings: %v", err)
	}

	store, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	got, status := store.Snapshot()
	if !status.Imported || status.Source != loadSourceImport {
		t.Fatalf("status = %+v, want imported legacy settings", status)
	}
	if got.Server.Port != 49725 || got.Device.HandyConnectionKey != "secret-import-key" {
		t.Fatalf("settings = %+v, want imported port and private key", got)
	}
	public, _ := store.PublicSnapshot()
	publicData, err := json.Marshal(public)
	if err != nil {
		t.Fatalf("marshal public settings: %v", err)
	}
	if strings.Contains(string(publicData), "secret-import-key") {
		t.Fatal("imported connection key leaked through public settings")
	}
	if _, err := os.Stat(filepath.Join(dir, settingsFileName)); !os.IsNotExist(err) {
		t.Fatalf("legacy settings path stat = %v, want renamed away", err)
	}
	if _, err := os.Stat(filepath.Join(dir, settingsFileName+".migrated")); err != nil {
		t.Fatalf("archived legacy settings missing: %v", err)
	}
}

func TestMissingFieldsAreDefaulted(t *testing.T) {
	raw := []byte(`{"version":1,"server":{"port":49721}}`)

	settings, migrated, err := loadSettingsFromBytes(raw)
	if err != nil {
		t.Fatalf("loadSettingsFromBytes: %v", err)
	}
	if migrated {
		t.Fatal("current settings unexpectedly migrated")
	}
	if settings.Server.Port != 49721 {
		t.Fatalf("server port = %d, want 49721", settings.Server.Port)
	}
	if settings.Motion.SpeedMaxPercent != DefaultSettings().Motion.SpeedMaxPercent {
		t.Fatal("missing motion settings were not defaulted")
	}
	if settings.LLM.Provider != LLMProviderLlamaCPP {
		t.Fatal("missing LLM settings were not defaulted")
	}
	if settings.Device.IntifaceServerAddress != DefaultIntifaceServerAddress {
		t.Fatalf("missing Intiface server address = %q, want %q", settings.Device.IntifaceServerAddress, DefaultIntifaceServerAddress)
	}
}

func TestSettingsLoaderAcceptsUTF8BOM(t *testing.T) {
	raw := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"version":1,"server":{"port":49724}}`)...)

	settings, migrated, err := loadSettingsFromBytes(raw)
	if err != nil {
		t.Fatalf("loadSettingsFromBytes: %v", err)
	}
	if migrated {
		t.Fatal("current settings unexpectedly migrated")
	}
	if settings.Server.Port != 49724 {
		t.Fatalf("server port = %d, want 49724", settings.Server.Port)
	}
}

func TestMigrationHookPromotesVersionZero(t *testing.T) {
	settings := DefaultSettings()
	settings.Version = 0

	migrated, changed, err := MigrateSettings(settings, 0)
	if err != nil {
		t.Fatalf("MigrateSettings: %v", err)
	}
	if !changed {
		t.Fatal("version zero settings did not report migration")
	}
	if migrated.Version != CurrentSettingsVersion {
		t.Fatalf("version = %d, want %d", migrated.Version, CurrentSettingsVersion)
	}
}

func TestCorruptSettingsRecoverToDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, settingsFileName), []byte("{broken"), 0o600); err != nil {
		t.Fatalf("write corrupt settings: %v", err)
	}

	store, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}

	settings, status := store.Snapshot()
	if !status.Recovered || !status.UsingDefaults {
		t.Fatalf("status = %+v, want recovered defaults", status)
	}
	if settings.Server.Port != DefaultServerPort {
		t.Fatalf("server port = %d, want %d", settings.Server.Port, DefaultServerPort)
	}
}

func TestPublicSettingsRedactsConnectionKey(t *testing.T) {
	settings := DefaultSettings()
	settings.Device.HandyConnectionKey = "secret"
	settings.Device.HSPDispatchOwner = DispatchOwnerIntiface
	settings.Device.IntifaceServerAddress = "wss://intiface.example.test/socket"

	public := settings.Public()
	data, err := json.Marshal(public)
	if err != nil {
		t.Fatalf("marshal public settings: %v", err)
	}
	if string(data) == "" || json.Valid(data) != true {
		t.Fatal("public settings did not marshal to valid JSON")
	}
	if containsString(string(data), "secret") {
		t.Fatal("public settings leaked connection key")
	}
	if !public.Device.ConnectionKeySet {
		t.Fatal("public settings did not indicate configured connection key")
	}
	if public.Device.IntifaceServerAddress != settings.Device.IntifaceServerAddress {
		t.Fatalf("public Intiface server address = %q, want %q", public.Device.IntifaceServerAddress, settings.Device.IntifaceServerAddress)
	}
	if !containsString(string(data), `"hsp_dispatch_owner":"intiface"`) {
		t.Fatal("public settings did not preserve the hsp_dispatch_owner JSON key")
	}
	if !oneOf(DispatchOwnerIntiface, public.Options.HSPDispatchOwners...) {
		t.Fatal("public dispatch owner options did not include Intiface")
	}
}

func TestIntifaceSettingsApplyUpdate(t *testing.T) {
	current := DefaultSettings()
	update := SettingsUpdate{
		Server: current.Server,
		Device: DeviceUpdate{
			HSPDispatchOwner:       DispatchOwnerIntiface,
			IntifaceServerAddress:  "  wss://intiface.example.test/socket  ",
			FirmwareAPIRequirement: current.Device.FirmwareAPIRequirement,
			APIApplicationIDSource: current.Device.APIApplicationIDSource,
		},
		Motion:      current.Motion,
		LLM:         current.LLM,
		Diagnostics: current.Diagnostics,
	}

	next, err := current.ApplyUpdate(update)
	if err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}
	if next.Device.HSPDispatchOwner != DispatchOwnerIntiface {
		t.Fatalf("dispatch owner = %q, want %q", next.Device.HSPDispatchOwner, DispatchOwnerIntiface)
	}
	if next.Device.IntifaceServerAddress != "wss://intiface.example.test/socket" {
		t.Fatalf("Intiface server address = %q, want trimmed update", next.Device.IntifaceServerAddress)
	}

	update.Device.IntifaceServerAddress = ""
	next, err = current.ApplyUpdate(update)
	if err != nil {
		t.Fatalf("ApplyUpdate missing address: %v", err)
	}
	if next.Device.IntifaceServerAddress != DefaultIntifaceServerAddress {
		t.Fatalf("missing update address = %q, want default %q", next.Device.IntifaceServerAddress, DefaultIntifaceServerAddress)
	}
}

func TestLegacyVersionOneSettingsDefaultIntifaceAddressWithoutVersionBump(t *testing.T) {
	raw := []byte(`{"version":1,"device":{"hsp_dispatch_owner":"cloud_rest","firmware_api_requirement":"firmware_v4_api_v3_required","api_application_id_source":"bundled_app_id"}}`)

	settings, migrated, err := loadSettingsFromBytes(raw)
	if err != nil {
		t.Fatalf("loadSettingsFromBytes: %v", err)
	}
	if migrated {
		t.Fatal("additive Intiface address default unexpectedly migrated version-1 settings")
	}
	if settings.Version != 1 {
		t.Fatalf("version = %d, want 1", settings.Version)
	}
	if settings.Device.IntifaceServerAddress != DefaultIntifaceServerAddress {
		t.Fatalf("Intiface server address = %q, want %q", settings.Device.IntifaceServerAddress, DefaultIntifaceServerAddress)
	}
}

func TestIntifaceServerAddressValidation(t *testing.T) {
	valid := []string{
		"ws://127.0.0.1:12345",
		"wss://intiface.example.test",
		"ws://[::1]:12345/socket",
	}
	for _, address := range valid {
		t.Run("valid_"+address, func(t *testing.T) {
			settings := DefaultSettings()
			settings.Device.IntifaceServerAddress = address
			if _, err := NormalizeSettings(settings); err != nil {
				t.Fatalf("NormalizeSettings(%q): %v", address, err)
			}
		})
	}

	invalid := []struct {
		name    string
		address string
		want    string
	}{
		{name: "relative", address: "localhost/socket", want: "absolute ws or wss URL with a host"},
		{name: "missing host", address: "ws:///socket", want: "absolute ws or wss URL with a host"},
		{name: "wrong scheme", address: "http://127.0.0.1:12345", want: "scheme must be ws or wss"},
		{name: "userinfo", address: "ws://user@127.0.0.1:12345", want: "must not include userinfo"},
		{name: "query", address: "ws://127.0.0.1:12345?token=value", want: "must not include a query"},
		{name: "empty query", address: "ws://127.0.0.1:12345?", want: "must not include a query"},
		{name: "fragment", address: "ws://127.0.0.1:12345#fragment", want: "must not include a fragment"},
		{name: "empty fragment", address: "ws://127.0.0.1:12345#", want: "must not include a fragment"},
		{name: "malformed", address: "ws://127.0.0.1/%zz", want: "must be a valid URL"},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			settings := DefaultSettings()
			settings.Device.IntifaceServerAddress = test.address
			_, err := NormalizeSettings(settings)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NormalizeSettings(%q) error = %v, want containing %q", test.address, err, test.want)
			}
		})
	}
}

func TestDispatchOwnerValidationIncludesIntiface(t *testing.T) {
	settings := DefaultSettings()
	settings.Device.HSPDispatchOwner = DispatchOwnerIntiface
	if _, err := NormalizeSettings(settings); err != nil {
		t.Fatalf("NormalizeSettings Intiface owner: %v", err)
	}

	settings.Device.HSPDispatchOwner = "unknown"
	_, err := NormalizeSettings(settings)
	if err == nil || !strings.Contains(err.Error(), `unknown dispatch owner "unknown"`) {
		t.Fatalf("NormalizeSettings unknown owner error = %v", err)
	}
}

func TestSaveWritesSQLiteDatastore(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	settings, _ := store.Snapshot()
	settings.Server.Port = 49722
	if _, err := store.Save(settings); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := os.Stat(store.Path()); err != nil {
		t.Fatalf("settings datastore missing after save: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".settings-*.tmp"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary settings files left behind: %v", matches)
	}
}

func TestSaveReplacesExistingSettingsFile(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}

	settings, _ := store.Snapshot()
	settings.Server.Port = 49722
	if _, err := store.Save(settings); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	settings.Server.Port = 49723
	if _, err := store.Save(settings); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	reloaded, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore reload: %v", err)
	}
	got, _ := reloaded.Snapshot()
	if got.Server.Port != 49723 {
		t.Fatalf("server port = %d, want 49723", got.Server.Port)
	}
}

func containsString(value string, fragment string) bool {
	return strings.Contains(value, fragment)
}

func TestVoiceSettingsDefaultOffAndNormalized(t *testing.T) {
	defaults := DefaultSettings()
	if defaults.Voice.Enabled {
		t.Fatal("voice must default to disabled")
	}
	if defaults.Voice.TTSWorkerPath != "" || defaults.Voice.ASRWorkerPath != "" {
		t.Fatal("voice worker paths must default to empty")
	}
	if defaults.Voice.TTSProvider != VoiceProviderNone || defaults.Voice.ASRProvider != VoiceProviderNone {
		t.Fatalf("voice providers must default to none: %+v", defaults.Voice)
	}

	settings := defaults
	settings.Voice = VoiceSettings{
		Enabled:       true,
		TTSWorkerPath: `  C:\workers\stub.exe  `,
		TTSWorkerArgs: []string{" -role ", "tts", "  "},
	}
	normalized, err := NormalizeSettings(settings)
	if err != nil {
		t.Fatalf("NormalizeSettings: %v", err)
	}
	if normalized.Voice.TTSWorkerPath != `C:\workers\stub.exe` {
		t.Fatalf("tts worker path = %q, want trimmed", normalized.Voice.TTSWorkerPath)
	}
	if len(normalized.Voice.TTSWorkerArgs) != 2 {
		t.Fatalf("tts worker args = %v, want blank entries dropped", normalized.Voice.TTSWorkerArgs)
	}
}

func TestLegacyVoiceCommandsMigrateToCustomWithoutChangingArguments(t *testing.T) {
	data := []byte(`{
		"version":1,
		"voice":{"enabled":true,"tts_worker_path":"C:\\legacy\\tts.exe","tts_worker_args":["-role","tts"],"asr_worker_path":"C:\\legacy\\asr.exe","asr_worker_args":["-role","asr"]}
	}`)
	settings, migrated, err := loadSettingsFromBytes(data)
	if err != nil {
		t.Fatalf("loadSettingsFromBytes: %v", err)
	}
	if migrated {
		t.Fatal("additive version-1 provider defaults must not bump the schema version")
	}
	if settings.Voice.TTSProvider != VoiceProviderCustom || settings.Voice.ASRProvider != VoiceProviderCustom {
		t.Fatalf("legacy commands were not classified as custom: %+v", settings.Voice)
	}
	if settings.Voice.TTSWorkerPath != `C:\legacy\tts.exe` || strings.Join(settings.Voice.ASRWorkerArgs, "|") != "-role|asr" {
		t.Fatalf("legacy command changed during migration: %+v", settings.Voice)
	}
}

func TestVoiceProviderFieldsSurviveAHiddenProviderSave(t *testing.T) {
	current := DefaultSettings()
	update := SettingsUpdate{
		Server: current.Server,
		Device: DeviceUpdate{HSPDispatchOwner: current.Device.HSPDispatchOwner, FirmwareAPIRequirement: current.Device.FirmwareAPIRequirement, APIApplicationIDSource: current.Device.APIApplicationIDSource},
		Motion: current.Motion, LLM: current.LLM, Diagnostics: current.Diagnostics,
		Voice: VoiceUpdate{
			Enabled: true, TTSProvider: VoiceProviderNone, ASRProvider: VoiceProviderNone,
			TTSWorkerPath: `C:\custom\tts.exe`, TTSWorkerArgs: []string{"--kept"},
			ASRWorkerPath: `C:\custom\asr.exe`, ASRWorkerArgs: []string{"--also-kept"},
			ElevenLabsVoiceID: "voice-123", ElevenLabsModelID: "model-456",
			ParakeetServerPath: `C:\parakeet\server.exe`, ParakeetModelPath: `C:\parakeet\model.gguf`, ParakeetServerPort: 9012,
			ASRBaseURL: "http://127.0.0.1:7777/", ASRModel: "parakeet",
			NeuTTSRunnerPath: `C:\neutts\stream_pcm.exe`, NeuTTSReferenceWAV: `C:\voices\reference.wav`,
			NeuTTSReferenceCodes: `C:\voices\reference.npy`, NeuTTSReferenceText: "Reference transcript.", NeuTTSBackbone: "local/backbone",
		},
	}
	next, err := current.ApplyUpdate(update)
	if err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}
	if next.Voice.TTSProvider != VoiceProviderNone || next.Voice.ASRProvider != VoiceProviderNone {
		t.Fatalf("active selections changed: %+v", next.Voice)
	}
	if next.Voice.ElevenLabsVoiceID != "voice-123" || next.Voice.ParakeetServerPort != 9012 || next.Voice.NeuTTSReferenceCodes != `C:\voices\reference.npy` {
		t.Fatalf("hidden provider fields were discarded: %+v", next.Voice)
	}
	if next.Voice.ASRBaseURL != "http://127.0.0.1:7777" || len(next.Voice.TTSWorkerArgs) != 1 {
		t.Fatalf("hidden provider fields were not normalized losslessly: %+v", next.Voice)
	}
}

func TestVoiceSettingsSurviveApplyUpdateAndReload(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}

	current, _ := store.Snapshot()
	update := SettingsUpdate{
		Server:      current.Server,
		Device:      DeviceUpdate{HSPDispatchOwner: current.Device.HSPDispatchOwner, FirmwareAPIRequirement: current.Device.FirmwareAPIRequirement, APIApplicationIDSource: current.Device.APIApplicationIDSource},
		Motion:      current.Motion,
		LLM:         current.LLM,
		Voice:       VoiceUpdate{Enabled: true, ASRWorkerPath: `C:\workers\stub.exe`, ASRWorkerArgs: []string{"-role", "asr"}},
		Diagnostics: current.Diagnostics,
	}
	next, err := current.ApplyUpdate(update)
	if err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}
	if _, err := store.Save(next); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore reload: %v", err)
	}
	got, _ := reloaded.Snapshot()
	if !got.Voice.Enabled || got.Voice.ASRWorkerPath == "" || len(got.Voice.ASRWorkerArgs) != 2 {
		t.Fatalf("voice settings did not survive reload: %+v", got.Voice)
	}
}

func TestElevenLabsKeyIsRedactedAndWriteOnly(t *testing.T) {
	settings := DefaultSettings()
	settings.Voice.ElevenLabsAPIKey = "el-secret"

	public := settings.Public()
	if !public.Voice.ElevenLabsKeySet {
		t.Fatal("public view must report the key as set")
	}
	encoded, err := json.Marshal(public)
	if err != nil {
		t.Fatalf("marshal public settings: %v", err)
	}
	if strings.Contains(string(encoded), "el-secret") {
		t.Fatal("the ElevenLabs key leaked into the public settings view")
	}

	// An update without the key keeps the stored secret.
	update := SettingsUpdate{
		Server:      settings.Server,
		Device:      DeviceUpdate{HSPDispatchOwner: settings.Device.HSPDispatchOwner, FirmwareAPIRequirement: settings.Device.FirmwareAPIRequirement, APIApplicationIDSource: settings.Device.APIApplicationIDSource},
		Motion:      settings.Motion,
		LLM:         settings.LLM,
		Voice:       VoiceUpdate{Enabled: true},
		Diagnostics: settings.Diagnostics,
	}
	next, err := settings.ApplyUpdate(update)
	if err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}
	if next.Voice.ElevenLabsAPIKey != "el-secret" {
		t.Fatalf("stored key must survive a keyless update, got %q", next.Voice.ElevenLabsAPIKey)
	}

	// Replacing and clearing both work.
	replacement := " el-new "
	update.Voice.ElevenLabsAPIKey = &replacement
	next, err = settings.ApplyUpdate(update)
	if err != nil {
		t.Fatalf("ApplyUpdate replace: %v", err)
	}
	if next.Voice.ElevenLabsAPIKey != "el-new" {
		t.Fatalf("key replace = %q, want trimmed el-new", next.Voice.ElevenLabsAPIKey)
	}
	update.Voice.ElevenLabsAPIKey = nil
	update.Voice.ClearElevenLabsKey = true
	next, err = settings.ApplyUpdate(update)
	if err != nil {
		t.Fatalf("ApplyUpdate clear: %v", err)
	}
	if next.Voice.ElevenLabsAPIKey != "" {
		t.Fatalf("key clear left %q", next.Voice.ElevenLabsAPIKey)
	}
}
