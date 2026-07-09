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
	settings.Device.APIApplicationIDSource = ApplicationIDSourceDeveloperOverride
	settings.Device.APIApplicationIDOverride = "dev-app"
	settings.Device.HandyConnectionKey = "secret"
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

	data, err := json.Marshal(settings.Public())
	if err != nil {
		t.Fatalf("marshal public settings: %v", err)
	}
	if string(data) == "" || json.Valid(data) != true {
		t.Fatal("public settings did not marshal to valid JSON")
	}
	if containsString(string(data), "secret") {
		t.Fatal("public settings leaked connection key")
	}
	if !settings.Public().Device.ConnectionKeySet {
		t.Fatal("public settings did not indicate configured connection key")
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
