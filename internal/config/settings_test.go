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
	if settings.LLM.MaxOutputTokens != DefaultLLMMaxOutputTokens || settings.LLM.ReasoningMode != LLMReasoningOff {
		t.Fatalf("LLM generation defaults = %+v", settings.LLM)
	}
}

func TestBundledAPIApplicationIDUsesPublicV3ID(t *testing.T) {
	if BundledAPIApplicationID != "rQoTWeMPrklUYcfdSXYYhS_9z.jAVNwy" {
		t.Fatalf("bundled API application ID = %q, want public Handy API v3 ID", BundledAPIApplicationID)
	}
}

func TestEmptyDeveloperApplicationIDFallsBackToBundled(t *testing.T) {
	settings := DefaultSettings()
	settings.Device.APIApplicationIDSource = ApplicationIDSourceDeveloperOverride
	settings.Device.APIApplicationIDOverride = "  "

	normalized, err := NormalizeSettings(settings)
	if err != nil {
		t.Fatalf("NormalizeSettings: %v", err)
	}
	if normalized.Device.APIApplicationIDSource != ApplicationIDSourceBundled {
		t.Fatalf("app ID source = %q, want %q", normalized.Device.APIApplicationIDSource, ApplicationIDSourceBundled)
	}
	if normalized.Device.APIApplicationIDOverride != "" {
		t.Fatalf("app ID override = %q, want empty", normalized.Device.APIApplicationIDOverride)
	}
}

func TestLoadMissingSettingsUsesDefaults(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

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
	t.Cleanup(func() { _ = store.Close() })

	settings, _ := store.Snapshot()
	settings.Server.Port = 49720
	settings.Device.HSPDispatchOwner = DispatchOwnerIntiface
	settings.Device.IntifaceServerAddress = "wss://intiface.example.test/socket"
	settings.Device.APIApplicationIDSource = ApplicationIDSourceDeveloperOverride
	settings.Device.APIApplicationIDOverride = "dev-app"
	settings.Device.HandyConnectionKey = "secret"
	settings.LLM.OllamaModelsPath = `D:\Ollama\models`
	capabilities := LLMMotionCapabilities{Motion: true, Patterns: false, AreaFocus: true, ExperimentalPatterns: true}
	settings.LLM.MotionCapabilities = &capabilities
	settings.Media.LibraryPaths = []string{filepath.Join(dir, "videos")}
	settings.Diagnostics.Verbosity = DiagnosticsVerbosityDebug
	if _, err := store.Save(settings); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore reload: %v", err)
	}
	t.Cleanup(func() { _ = reloaded.Close() })
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
	if got.LLM.MotionCapabilities == nil || !got.LLM.MotionCapabilities.Motion || got.LLM.MotionCapabilities.Patterns || !got.LLM.MotionCapabilities.AreaFocus || !got.LLM.MotionCapabilities.ExperimentalPatterns {
		t.Fatalf("motion capabilities did not persist: %+v", got.LLM.MotionCapabilities)
	}
	if len(got.Media.LibraryPaths) != 1 || got.Media.LibraryPaths[0] != filepath.Join(dir, "videos") {
		t.Fatalf("media library paths = %v", got.Media.LibraryPaths)
	}
}

func TestMotionCapabilitiesDoNotAliasStoreState(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	settings, _ := store.Snapshot()
	capabilities := LLMMotionCapabilities{Motion: true, Patterns: false, AreaFocus: true, ExperimentalPatterns: true}
	settings.LLM.MotionCapabilities = &capabilities
	if _, err := store.Save(settings); err != nil {
		t.Fatalf("Save: %v", err)
	}

	capabilities.Patterns = true
	snapshot, _ := store.Snapshot()
	if snapshot.LLM.MotionCapabilities == nil || snapshot.LLM.MotionCapabilities.Patterns {
		t.Fatalf("saved motion capabilities aliased caller state: %+v", snapshot.LLM.MotionCapabilities)
	}
	snapshot.LLM.MotionCapabilities.Patterns = true
	fresh, _ := store.Snapshot()
	if fresh.LLM.MotionCapabilities.Patterns {
		t.Fatal("motion capabilities snapshot aliases the store")
	}
	public := fresh.Public()
	public.LLM.MotionCapabilities.Patterns = true
	fresh, _ = store.Snapshot()
	if fresh.LLM.MotionCapabilities.Patterns {
		t.Fatal("public motion capabilities alias the store")
	}
}

func TestMediaSettingsNormalizePathsAndRejectRelativeLocation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "videos")
	settings := DefaultSettings()
	settings.Media.LibraryPaths = []string{"  " + root + "  ", root, ""}

	normalized, err := NormalizeSettings(settings)
	if err != nil {
		t.Fatalf("NormalizeSettings: %v", err)
	}
	if len(normalized.Media.LibraryPaths) != 1 || normalized.Media.LibraryPaths[0] != filepath.Clean(root) {
		t.Fatalf("normalized paths = %v", normalized.Media.LibraryPaths)
	}
	public := normalized.Public()
	public.Media.LibraryPaths[0] = "changed"
	if normalized.Media.LibraryPaths[0] == "changed" {
		t.Fatal("public media paths alias private settings")
	}

	settings.Media.LibraryPaths = []string{"relative/videos"}
	if _, err := NormalizeSettings(settings); err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("relative path error = %v", err)
	}
}

func TestDefaultMediaSettingsPublishAnEmptyArray(t *testing.T) {
	normalized, err := NormalizeSettings(DefaultSettings())
	if err != nil {
		t.Fatalf("NormalizeSettings: %v", err)
	}
	public := normalized.Public()
	if public.Media.LibraryPaths == nil || len(public.Media.LibraryPaths) != 0 {
		t.Fatalf("default public media paths = %#v, want non-nil empty slice", public.Media.LibraryPaths)
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
	if settings.Motion.ApplyVideoSpeedLimit {
		t.Fatal("missing video speed-limit policy should default off")
	}
	if settings.LLM.Provider != LLMProviderLlamaCPP {
		t.Fatal("missing LLM settings were not defaulted")
	}
	if settings.LLM.MaxOutputTokens != DefaultLLMMaxOutputTokens || settings.LLM.ReasoningMode != LLMReasoningOff {
		t.Fatalf("missing LLM generation settings were not defaulted: %+v", settings.LLM)
	}
	if settings.Device.IntifaceServerAddress != DefaultIntifaceServerAddress {
		t.Fatalf("missing Intiface server address = %q, want %q", settings.Device.IntifaceServerAddress, DefaultIntifaceServerAddress)
	}
	if settings.Chat.StartupBehavior != ChatStartupPrevious || settings.Chat.KeepUnsavedOnExit {
		t.Fatalf("missing chat settings were not defaulted: %+v", settings.Chat)
	}
}

func TestOlderSettingsUpdatePreservesNewTuningAndParakeetSource(t *testing.T) {
	current := DefaultSettings()
	current.LLM.MaxOutputTokens = 1024
	current.LLM.ReasoningMode = LLMReasoningOff
	current.Voice.ParakeetSource = ParakeetSourceApp
	current.Voice.ParakeetServerPath = `C:\retained\server.exe`
	current.Voice.ParakeetModelPath = `C:\retained\model.gguf`
	current.Voice.InputMode = VoiceInputModeHold
	current.Voice.InputSensitivity = 68
	current.Voice.InputSilenceMillis = 1400
	current.Voice.InputNoiseSuppress = false
	current.Chat = ChatSettings{StartupBehavior: ChatStartupPrevious, KeepUnsavedOnExit: true}
	llmUpdate := LLMUpdateFromSettings(current.LLM)
	llmUpdate.MaxOutputTokens = nil
	llmUpdate.ReasoningMode = nil

	oldUpdate := SettingsUpdate{
		Server: current.Server,
		Device: DeviceUpdate{
			HSPDispatchOwner:       current.Device.HSPDispatchOwner,
			IntifaceServerAddress:  current.Device.IntifaceServerAddress,
			FirmwareAPIRequirement: current.Device.FirmwareAPIRequirement,
			APIApplicationIDSource: current.Device.APIApplicationIDSource,
		},
		Motion: current.Motion,
		LLM:    llmUpdate,
		Voice: VoiceUpdate{
			TTSProvider:        current.Voice.TTSProvider,
			ASRProvider:        current.Voice.ASRProvider,
			ParakeetServerPath: current.Voice.ParakeetServerPath,
			ParakeetModelPath:  current.Voice.ParakeetModelPath,
			ParakeetServerPort: current.Voice.ParakeetServerPort,
		},
		Diagnostics: current.Diagnostics,
	}
	encoded, err := json.Marshal(oldUpdate)
	if err != nil {
		t.Fatalf("marshal old update: %v", err)
	}
	if strings.Contains(string(encoded), "max_output_tokens") || strings.Contains(string(encoded), "reasoning_mode") || strings.Contains(string(encoded), "parakeet_source") || strings.Contains(string(encoded), "input_sensitivity") {
		t.Fatalf("old update unexpectedly contains new fields: %s", encoded)
	}
	var decoded SettingsUpdate
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode old update: %v", err)
	}
	next, err := current.ApplyUpdate(decoded)
	if err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}
	if next.LLM.MaxOutputTokens != 1024 || next.LLM.ReasoningMode != LLMReasoningOff {
		t.Fatalf("older update reset LLM tuning: %+v", next.LLM)
	}
	if next.Voice.ParakeetSource != ParakeetSourceApp {
		t.Fatalf("older update changed Parakeet source to %q", next.Voice.ParakeetSource)
	}
	if next.Voice.InputMode != VoiceInputModeHold || next.Voice.InputSensitivity != 68 ||
		next.Voice.InputSilenceMillis != 1400 || next.Voice.InputNoiseSuppress {
		t.Fatalf("older update reset voice input tuning: %+v", next.Voice)
	}
	if next.Chat != current.Chat {
		t.Fatalf("older update reset chat settings: %+v", next.Chat)
	}
}

func TestNewChatStartupRejectsUnsavedDraftRetention(t *testing.T) {
	settings := DefaultSettings()
	settings.Chat = ChatSettings{StartupBehavior: ChatStartupNew, KeepUnsavedOnExit: true}
	if err := validateSettings(settings); err == nil || !strings.Contains(err.Error(), "cannot also retain") {
		t.Fatalf("chat startup conflict error = %v", err)
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
		LLM:         LLMUpdateFromSettings(current.LLM),
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
	t.Cleanup(func() { _ = store.Close() })
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
	t.Cleanup(func() { _ = store.Close() })

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
	t.Cleanup(func() { _ = reloaded.Close() })
	got, _ := reloaded.Snapshot()
	if got.Server.Port != 49723 {
		t.Fatalf("server port = %d, want 49723", got.Server.Port)
	}
}

func TestStoreSnapshotsOwnVoiceArgumentSlices(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	settings, _ := store.Snapshot()
	settings.Voice.TTSWorkerArgs = []string{"--role", "tts"}
	settings.Voice.ASRWorkerArgs = []string{"--role", "asr"}
	saved, err := store.Save(settings)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	saved.Voice.TTSWorkerArgs[0] = "mutated-save-result"
	snapshot, _ := store.Snapshot()
	snapshot.Voice.TTSWorkerArgs[0] = "mutated-snapshot"
	snapshot.Voice.ASRWorkerArgs[0] = "mutated-snapshot"
	public, _ := store.PublicSnapshot()
	public.Voice.TTSWorkerArgs[0] = "mutated-public-snapshot"

	unchanged, _ := store.Snapshot()
	if got := unchanged.Voice.TTSWorkerArgs[0]; got != "--role" {
		t.Fatalf("stored TTS arguments were mutated through a snapshot: %q", got)
	}
	if got := unchanged.Voice.ASRWorkerArgs[0]; got != "--role" {
		t.Fatalf("stored ASR arguments were mutated through a snapshot: %q", got)
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
	if defaults.Voice.ParakeetSource != ParakeetSourceApp {
		t.Fatalf("Parakeet source = %q, want %q", defaults.Voice.ParakeetSource, ParakeetSourceApp)
	}
	if defaults.Voice.NeuTTSSamplingMode != NeuTTSSamplingFixed || defaults.Voice.NeuTTSSamplerSeed != DefaultNeuTTSSamplerSeed {
		t.Fatalf("NeuTTS sampling defaults = %q/%d", defaults.Voice.NeuTTSSamplingMode, defaults.Voice.NeuTTSSamplerSeed)
	}
	if defaults.Voice.InputMode != VoiceInputModeHandsFree || defaults.Voice.InputSensitivity != DefaultVoiceInputSensitivity ||
		defaults.Voice.InputSilenceMillis != DefaultVoiceInputSilenceMillis || !defaults.Voice.InputNoiseSuppress {
		t.Fatalf("voice input defaults = %+v", defaults.Voice)
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

	invalid := defaults
	invalid.Voice.NeuTTSSamplingMode = "roulette"
	if _, err := NormalizeSettings(invalid); err == nil || !strings.Contains(err.Error(), "NeuTTS sampling mode") {
		t.Fatalf("invalid NeuTTS sampling mode error = %v", err)
	}
}

func TestVoiceSettingsRejectInvalidExternalASRURL(t *testing.T) {
	settings := DefaultSettings()
	settings.Voice.ASRProvider = VoiceASRProviderOpenAICompat
	settings.Voice.ASRBaseURL = "file:///tmp/asr"
	if _, err := NormalizeSettings(settings); err == nil || !strings.Contains(err.Error(), "ASR base URL") {
		t.Fatalf("invalid ASR URL error = %v", err)
	}

	settings.Voice.ASRBaseURL = "http://user:password@127.0.0.1:8990"
	if _, err := NormalizeSettings(settings); err == nil || !strings.Contains(err.Error(), "userinfo") {
		t.Fatalf("ASR URL with credentials error = %v", err)
	}
}

func TestLLMSettingsRejectUnsafeBaseURLs(t *testing.T) {
	for _, baseURL := range []string{
		"file:///tmp/ollama",
		"http://user:secret@127.0.0.1:11434",
		"http://127.0.0.1:11434?token=secret",
		"http://127.0.0.1:11434/#fragment",
	} {
		settings := DefaultSettings()
		settings.LLM.OllamaBaseURL = baseURL
		if _, err := NormalizeSettings(settings); err == nil {
			t.Fatalf("NormalizeSettings accepted Ollama URL %q", baseURL)
		}
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
	if settings.Voice.NeuTTSSamplingMode != NeuTTSSamplingFixed || settings.Voice.NeuTTSSamplerSeed != DefaultNeuTTSSamplerSeed {
		t.Fatalf("legacy NeuTTS sampling defaults = %+v", settings.Voice)
	}
}

func TestExistingParakeetPathsMigrateToCustomSource(t *testing.T) {
	data := []byte(`{
		"version":1,
		"voice":{"asr_provider":"parakeet_managed","parakeet_server_path":"C:\\parakeet\\server.exe","parakeet_model_path":"C:\\parakeet\\model.gguf"}
	}`)
	settings, _, err := loadSettingsFromBytes(data)
	if err != nil {
		t.Fatalf("loadSettingsFromBytes: %v", err)
	}
	if settings.Voice.ParakeetSource != ParakeetSourceCustom {
		t.Fatalf("Parakeet source = %q, want %q", settings.Voice.ParakeetSource, ParakeetSourceCustom)
	}
}

func TestVoiceProviderFieldsSurviveAHiddenProviderSave(t *testing.T) {
	current := DefaultSettings()
	parakeetSource := ParakeetSourceCustom
	neuTTSSamplingMode := NeuTTSSamplingRandom
	neuTTSSamplerSeed := uint32(27)
	update := SettingsUpdate{
		Server: current.Server,
		Device: DeviceUpdate{HSPDispatchOwner: current.Device.HSPDispatchOwner, FirmwareAPIRequirement: current.Device.FirmwareAPIRequirement, APIApplicationIDSource: current.Device.APIApplicationIDSource},
		Motion: current.Motion, LLM: LLMUpdateFromSettings(current.LLM), Diagnostics: current.Diagnostics,
		Voice: VoiceUpdate{
			Enabled: true, TTSProvider: VoiceProviderNone, ASRProvider: VoiceProviderNone,
			TTSWorkerPath: `C:\custom\tts.exe`, TTSWorkerArgs: []string{"--kept"},
			ASRWorkerPath: `C:\custom\asr.exe`, ASRWorkerArgs: []string{"--also-kept"},
			ElevenLabsVoiceID: "voice-123", ElevenLabsModelID: "model-456",
			ParakeetServerPath: `C:\parakeet\server.exe`, ParakeetModelPath: `C:\parakeet\model.gguf`, ParakeetServerPort: 9012,
			ParakeetSource: &parakeetSource,
			ASRBaseURL:     "http://127.0.0.1:7777/", ASRModel: "parakeet",
			NeuTTSRunnerPath: `C:\neutts\stream_pcm.exe`, NeuTTSReferenceWAV: `C:\voices\reference.wav`,
			NeuTTSReferenceCodes: `C:\voices\reference.npy`, NeuTTSReferenceText: "Reference transcript.", NeuTTSBackbone: "local/backbone",
			NeuTTSSamplingMode: &neuTTSSamplingMode, NeuTTSSamplerSeed: &neuTTSSamplerSeed,
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
	if next.Voice.ParakeetSource != ParakeetSourceCustom {
		t.Fatalf("hidden Parakeet source was discarded: %+v", next.Voice)
	}
	if next.Voice.ASRBaseURL != "http://127.0.0.1:7777" || len(next.Voice.TTSWorkerArgs) != 1 {
		t.Fatalf("hidden provider fields were not normalized losslessly: %+v", next.Voice)
	}
	if next.Voice.NeuTTSSamplingMode != NeuTTSSamplingRandom || next.Voice.NeuTTSSamplerSeed != neuTTSSamplerSeed {
		t.Fatalf("hidden NeuTTS sampling fields were discarded: %+v", next.Voice)
	}
}

func TestVoiceSettingsSurviveApplyUpdateAndReload(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	current, _ := store.Snapshot()
	inputMode := VoiceInputModeHold
	inputSensitivity := 72
	inputSilence := 1250
	noiseSuppression := false
	neuTTSSamplingMode := NeuTTSSamplingRandom
	neuTTSSamplerSeed := uint32(42)
	update := SettingsUpdate{
		Server: current.Server,
		Device: DeviceUpdate{HSPDispatchOwner: current.Device.HSPDispatchOwner, FirmwareAPIRequirement: current.Device.FirmwareAPIRequirement, APIApplicationIDSource: current.Device.APIApplicationIDSource},
		Motion: current.Motion,
		LLM:    LLMUpdateFromSettings(current.LLM),
		Voice: VoiceUpdate{
			Enabled: true, ASRWorkerPath: `C:\workers\stub.exe`, ASRWorkerArgs: []string{"-role", "asr"},
			InputMode: &inputMode, InputSensitivity: &inputSensitivity, InputSilenceMillis: &inputSilence,
			InputNoiseSuppress: &noiseSuppression,
			NeuTTSSamplingMode: &neuTTSSamplingMode, NeuTTSSamplerSeed: &neuTTSSamplerSeed,
		},
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
	t.Cleanup(func() { _ = reloaded.Close() })
	got, _ := reloaded.Snapshot()
	if !got.Voice.Enabled || got.Voice.ASRWorkerPath == "" || len(got.Voice.ASRWorkerArgs) != 2 {
		t.Fatalf("voice settings did not survive reload: %+v", got.Voice)
	}
	if got.Voice.InputMode != inputMode || got.Voice.InputSensitivity != inputSensitivity ||
		got.Voice.InputSilenceMillis != inputSilence || got.Voice.InputNoiseSuppress {
		t.Fatalf("voice input preferences did not survive reload: %+v", got.Voice)
	}
	if got.Voice.NeuTTSSamplingMode != neuTTSSamplingMode || got.Voice.NeuTTSSamplerSeed != neuTTSSamplerSeed {
		t.Fatalf("NeuTTS sampling preferences did not survive reload: %+v", got.Voice)
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
		LLM:         LLMUpdateFromSettings(settings.LLM),
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

func TestChatVoiceDefaultsPreservesAndValidates(t *testing.T) {
	// Payloads that predate the field resolve to the utility voice.
	settings := DefaultSettings()
	settings.LLM.ChatVoice = ""
	normalized, err := NormalizeSettings(settings)
	if err != nil {
		t.Fatalf("NormalizeSettings: %v", err)
	}
	if normalized.LLM.ChatVoice != LLMChatVoiceUtility {
		t.Fatalf("legacy payload voice = %q, want utility", normalized.LLM.ChatVoice)
	}

	// A saved explicit voice survives an update that omits the field, and an
	// update that carries the field replaces it.
	saved := normalized
	saved.LLM.ChatVoice = LLMChatVoiceExplicit
	update := SettingsUpdate{
		Server:      saved.Server,
		Device:      DeviceUpdate{HSPDispatchOwner: saved.Device.HSPDispatchOwner, FirmwareAPIRequirement: saved.Device.FirmwareAPIRequirement, APIApplicationIDSource: saved.Device.APIApplicationIDSource},
		Motion:      saved.Motion,
		LLM:         LLMUpdateFromSettings(saved.LLM),
		Diagnostics: saved.Diagnostics,
	}
	update.LLM.ChatVoice = nil
	next, err := saved.ApplyUpdate(update)
	if err != nil {
		t.Fatalf("ApplyUpdate omit: %v", err)
	}
	if next.LLM.ChatVoice != LLMChatVoiceExplicit {
		t.Fatalf("omitted field clobbered saved voice: %q", next.LLM.ChatVoice)
	}
	warm := " Warm "
	update.LLM.ChatVoice = &warm
	next, err = saved.ApplyUpdate(update)
	if err != nil {
		t.Fatalf("ApplyUpdate replace: %v", err)
	}
	if next.LLM.ChatVoice != LLMChatVoiceWarm {
		t.Fatalf("voice replace = %q, want normalized warm", next.LLM.ChatVoice)
	}

	// Unknown values are rejected, not silently coerced.
	invalid := "spicy"
	update.LLM.ChatVoice = &invalid
	if _, err := saved.ApplyUpdate(update); err == nil {
		t.Fatal("unknown chat voice must be rejected")
	}
}

func TestLLMChatProfileSettingsNormalizePreserveAndValidate(t *testing.T) {
	settings := DefaultSettings()
	settings.LLM.UserAnatomy = ""
	normalized, err := NormalizeSettings(settings)
	if err != nil {
		t.Fatalf("NormalizeSettings: %v", err)
	}
	if normalized.LLM.UserAnatomy != LLMUserAnatomyPenis {
		t.Fatalf("legacy anatomy = %q, want %q", normalized.LLM.UserAnatomy, LLMUserAnatomyPenis)
	}

	saved := normalized
	saved.LLM.UserAnatomy = LLMUserAnatomyVagina
	saved.LLM.CustomAnatomy = "saved wording"
	saved.LLM.PersonaDescription = "saved persona"
	update := SettingsUpdate{
		Server:      saved.Server,
		Device:      DeviceUpdate{HSPDispatchOwner: saved.Device.HSPDispatchOwner, FirmwareAPIRequirement: saved.Device.FirmwareAPIRequirement, APIApplicationIDSource: saved.Device.APIApplicationIDSource},
		Motion:      saved.Motion,
		LLM:         LLMUpdateFromSettings(saved.LLM),
		Diagnostics: saved.Diagnostics,
	}
	update.LLM.UserAnatomy = nil
	update.LLM.CustomAnatomy = nil
	update.LLM.PersonaDescription = nil
	next, err := saved.ApplyUpdate(update)
	if err != nil {
		t.Fatalf("ApplyUpdate omitted profile: %v", err)
	}
	if next.LLM.UserAnatomy != LLMUserAnatomyVagina || next.LLM.CustomAnatomy != "saved wording" || next.LLM.PersonaDescription != "saved persona" {
		t.Fatalf("omitted profile fields were not preserved: %+v", next.LLM)
	}

	custom := " Custom "
	customWording := "  my\n custom\twording  "
	persona := "  An energetic\n and passionate partner  "
	update.LLM.UserAnatomy = &custom
	update.LLM.CustomAnatomy = &customWording
	update.LLM.PersonaDescription = &persona
	next, err = saved.ApplyUpdate(update)
	if err != nil {
		t.Fatalf("ApplyUpdate profile: %v", err)
	}
	if next.LLM.UserAnatomy != LLMUserAnatomyCustom || next.LLM.CustomAnatomy != "my custom wording" || next.LLM.PersonaDescription != "An energetic and passionate partner" {
		t.Fatalf("normalized profile = %+v", next.LLM)
	}

	unknown := "other"
	update.LLM.UserAnatomy = &unknown
	if _, err := saved.ApplyUpdate(update); err == nil {
		t.Fatal("unknown user anatomy must be rejected")
	}
	update.LLM.UserAnatomy = &custom
	tooLong := strings.Repeat("界", MaxLLMCustomAnatomyChars+1)
	update.LLM.CustomAnatomy = &tooLong
	if _, err := saved.ApplyUpdate(update); err == nil {
		t.Fatal("overlong custom anatomy must be rejected by character count")
	}
	validCustom := "wording"
	tooLong = strings.Repeat("p", MaxLLMPersonaDescriptionChars+1)
	update.LLM.CustomAnatomy = &validCustom
	update.LLM.PersonaDescription = &tooLong
	if _, err := saved.ApplyUpdate(update); err == nil {
		t.Fatal("overlong persona must be rejected")
	}

	options := DefaultSettings().Public().Options.LLMUserAnatomies
	if len(options) != 3 || options[0] != LLMUserAnatomyPenis || options[2] != LLMUserAnatomyCustom {
		t.Fatalf("user anatomy options = %v", options)
	}
}

func TestLegacySettingsWithoutAnatomyLoadNeutral(t *testing.T) {
	encoded, err := json.Marshal(DefaultSettings())
	if err != nil {
		t.Fatalf("marshal defaults: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("decode defaults: %v", err)
	}
	llmSettings := document["llm"].(map[string]any)
	delete(llmSettings, "user_anatomy")
	delete(llmSettings, "custom_anatomy")
	delete(llmSettings, "persona_description")
	encoded, err = json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal legacy settings: %v", err)
	}

	loaded, _, err := loadSettingsFromBytes(encoded)
	if err != nil {
		t.Fatalf("load legacy settings: %v", err)
	}
	if loaded.LLM.UserAnatomy != LLMUserAnatomyCustom || loaded.LLM.CustomAnatomy != "" {
		t.Fatalf("legacy anatomy = %q/%q, want neutral custom state", loaded.LLM.UserAnatomy, loaded.LLM.CustomAnatomy)
	}
}
