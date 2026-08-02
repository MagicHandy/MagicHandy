package config

import (
	"reflect"
	"testing"
)

func TestSetupCompletionDefaultsOnlyForExistingDocuments(t *testing.T) {
	legacyCurrent, migrated, err := loadSettingsFromBytes([]byte(
		`{"version":2,"ui":{"locale":"en","theme":"steel-azure"},"voice":{"tts_provider":"none","asr_provider":"none"}}`,
	))
	if err != nil {
		t.Fatalf("load current document without setup field: %v", err)
	}
	if migrated {
		t.Fatal("additive setup completion should not require a schema migration")
	}
	if !legacyCurrent.UI.SetupCompleted {
		t.Fatal("existing settings document was incorrectly sent through first-run setup")
	}
	if legacyCurrent.UI.UpdateCheckMode != UpdateCheckAutomatic {
		t.Fatalf("existing settings update mode = %q, want automatic", legacyCurrent.UI.UpdateCheckMode)
	}
	if !reflect.DeepEqual(legacyCurrent.UI.NotificationCategories, DefaultNotificationCategories) {
		t.Fatalf("existing settings notification categories = %v, want %v", legacyCurrent.UI.NotificationCategories, DefaultNotificationCategories)
	}

	explicitFresh, migrated, err := loadSettingsFromBytes([]byte(
		`{"version":2,"ui":{"locale":"en","theme":"steel-azure","setup_completed":false},"voice":{"tts_provider":"none","asr_provider":"none"}}`,
	))
	if err != nil {
		t.Fatalf("load explicit incomplete setup: %v", err)
	}
	if migrated {
		t.Fatal("explicit setup state should not require migration")
	}
	if explicitFresh.UI.SetupCompleted {
		t.Fatal("explicit incomplete setup state was not preserved")
	}
	if explicitFresh.UI.UpdateCheckMode != UpdateCheckAutomatic {
		t.Fatalf("fresh settings update mode = %q, want automatic", explicitFresh.UI.UpdateCheckMode)
	}
}

func TestUpdateCheckModePersistsAndSurvivesOlderUIWrites(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	settings, _ := store.Snapshot()
	settings.UI.UpdateCheckMode = UpdateCheckManual
	if _, err := store.Save(settings); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reloaded, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore reload: %v", err)
	}
	t.Cleanup(func() { _ = reloaded.Close() })
	current, _ := reloaded.Snapshot()
	if current.UI.UpdateCheckMode != UpdateCheckManual {
		t.Fatalf("reloaded update check mode = %q, want %q", current.UI.UpdateCheckMode, UpdateCheckManual)
	}

	oldUpdate := SettingsUpdate{
		Server: current.Server,
		UI:     &UISettings{Locale: current.UI.Locale},
		Device: DeviceUpdate{
			HSPDispatchOwner:       current.Device.HSPDispatchOwner,
			IntifaceServerAddress:  current.Device.IntifaceServerAddress,
			FirmwareAPIRequirement: current.Device.FirmwareAPIRequirement,
			APIApplicationIDSource: current.Device.APIApplicationIDSource,
		},
		Motion:      current.Motion,
		LLM:         LLMUpdateFromSettings(current.LLM),
		Voice:       VoiceUpdate{},
		Diagnostics: current.Diagnostics,
	}
	next, err := current.ApplyUpdate(oldUpdate)
	if err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}
	if next.UI.UpdateCheckMode != UpdateCheckManual {
		t.Fatalf("older UI update reset update check mode to %q", next.UI.UpdateCheckMode)
	}
	if !reflect.DeepEqual(next.UI.NotificationCategories, DefaultNotificationCategories) {
		t.Fatalf("older UI update reset notification categories to %v", next.UI.NotificationCategories)
	}
}

func TestNotificationCategoriesPersistIncludingExplicitNone(t *testing.T) {
	current := DefaultSettings()
	update := SettingsUpdate{
		Server: current.Server,
		UI: &UISettings{
			Locale:                 current.UI.Locale,
			Theme:                  current.UI.Theme,
			SetupCompleted:         current.UI.SetupCompleted,
			UpdateCheckMode:        current.UI.UpdateCheckMode,
			NotificationCategories: []string{},
		},
		Device: DeviceUpdate{
			HSPDispatchOwner:       current.Device.HSPDispatchOwner,
			IntifaceServerAddress:  current.Device.IntifaceServerAddress,
			FirmwareAPIRequirement: current.Device.FirmwareAPIRequirement,
			APIApplicationIDSource: current.Device.APIApplicationIDSource,
		},
		Motion:      current.Motion,
		LLM:         LLMUpdateFromSettings(current.LLM),
		Voice:       VoiceUpdate{},
		Diagnostics: current.Diagnostics,
	}
	next, err := current.ApplyUpdate(update)
	if err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}
	if next.UI.NotificationCategories == nil || len(next.UI.NotificationCategories) != 0 {
		t.Fatalf("explicit empty notification categories were not preserved: %#v", next.UI.NotificationCategories)
	}

	next.UI.NotificationCategories = []string{"library", "library"}
	if _, err := NormalizeSettings(next); err == nil {
		t.Fatal("duplicate notification categories were accepted")
	}
}
