package httpapi

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestVoiceUpdateArgumentsPreserveSelection(t *testing.T) {
	for _, module := range setupVoiceModules {
		t.Run(module.ID, func(t *testing.T) {
			home := t.TempDir()
			previous := config.DefaultSettings().Voice
			previous.TTSProvider = module.Provider
			previous.TTSModuleRoot = filepath.Join(home, "runtimes", strings.Repeat("a", 32))
			previous.TTSModel = "custom/model"
			previous.TTSVoice = "custom.wav"
			previous.TTSLanguage = "es"
			previous.SpeakReplies = true
			previous.TTSReferenceWAV = filepath.Join(t.TempDir(), "reference.wav")
			voicePath := filepath.Join(previous.TTSModuleRoot, "runtime", "voices", previous.TTSVoice)
			managedTestFile(t, voicePath)
			module.Port = 9123
			root, args, err := voiceInstallArguments("installer.ps1", "data", "wrong-default", module, setupVoiceInstallRequest{
				Device: "cuda", AutoLaunch: true, updateFrom: &previous,
			})
			if err != nil {
				t.Fatal(err)
			}
			if root != home {
				t.Fatalf("update root = %s, want module home %s", root, home)
			}
			for flag, value := range map[string]string{"-InstallRoot": home, "-Port": "9123", "-Device": "cuda", "-Model": previous.TTSModel, "-Voice": previous.TTSVoice, "-Language": previous.TTSLanguage} {
				index := slices.Index(args, flag)
				if index < 0 || index+1 >= len(args) || args[index+1] != value {
					t.Fatalf("%s lost in %v", flag, args)
				}
			}
			for _, flag := range []string{"-Update", "-AutoLaunch", "-SpeakReplies", "-SkipAppConfiguration"} {
				if !slices.Contains(args, flag) {
					t.Fatalf("missing %s", flag)
				}
			}
			index := slices.Index(args, "-ReferenceWav")
			if module.ID == "chatterbox" {
				if index < 0 || args[index+1] != voicePath {
					t.Fatal("selected Chatterbox voice was not copied")
				}
			} else if index >= 0 {
				t.Fatal("Qwen reference belongs in app settings, not the install command")
			}
		})
	}
}

func TestVoiceUpdateDoesNotInventModuleHome(t *testing.T) {
	for _, root := range []string{filepath.Join("custom", "tts"), filepath.Join("custom", "runtimes", "user-folder")} {
		if voiceModuleHome(root) != root {
			t.Fatalf("changed legacy/custom root %s", root)
		}
	}
	previous := config.DefaultSettings().Voice
	previous.TTSProvider, previous.TTSModuleRoot, previous.TTSVoice = config.VoiceTTSProviderChatterbox, t.TempDir(), "missing.wav"
	_, _, err := voiceInstallArguments("installer.ps1", "data", "unused", setupVoiceModules[1], setupVoiceInstallRequest{updateFrom: &previous})
	if err == nil {
		t.Fatal("update with missing selected voice should fail before running installer")
	}
}

func TestApplyVoiceModuleUpdatePreservesAllOtherSettings(t *testing.T) {
	server := newTestServer(t)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Voice.TTSProvider = config.VoiceTTSProviderFasterQwen
		settings.Voice.TTSModuleRoot = t.TempDir()
		settings.Voice.TTSModel = "Qwen/Qwen3-TTS-12Hz-1.7B-Base"
		settings.Voice.TTSServerPort = 9123
		settings.Voice.TTSReferenceWAV = filepath.Join(t.TempDir(), "reference.wav")
		settings.Voice.TTSReferenceText = "My exact reference transcript."
		settings.Voice.OpenAITTSAPIKey = "fixture-secret"
		settings.Voice.TTSSeed = 123
		settings.Voice.TTSLanguage = "es"
		return settings
	})
	before, _ := server.store.Snapshot()
	previous := before.Voice
	root := t.TempDir()
	if err := server.applyInstalledVoiceModule(t.Context(), setupVoiceInstallResult{Root: root, updateFrom: &previous}); err != nil {
		t.Fatal(err)
	}
	after, _ := server.store.Snapshot()
	want := before
	want.Voice.TTSModuleRoot = root
	if !reflect.DeepEqual(after, want) {
		t.Fatal("module update changed settings other than the runtime root")
	}
	// A user's enablement/reference changes while preparing are retained too.
	current := previous
	current.Enabled, current.SpeakReplies = true, true
	current.TTSReferenceText = "New transcript entered during preparation."
	updated, err := applyVoiceModuleUpdate(current, setupVoiceInstallResult{Root: root, updateFrom: &previous})
	current.TTSModuleRoot = root
	if err != nil || !reflect.DeepEqual(updated, current) {
		t.Fatal("concurrent unrelated voice edits were lost")
	}
}

func TestVoiceUpdateRejectsChangedSelection(t *testing.T) {
	previous := config.DefaultSettings().Voice
	previous.TTSProvider = config.VoiceTTSProviderFasterQwen
	previous.TTSModuleRoot = t.TempDir()
	for _, mutate := range []func(*config.VoiceSettings){
		func(s *config.VoiceSettings) { s.TTSProvider = config.VoiceTTSProviderChatterbox },
		func(s *config.VoiceSettings) { s.TTSModuleRoot = "another runtime" },
		func(s *config.VoiceSettings) { s.TTSModel = "another/model" },
		func(s *config.VoiceSettings) { s.TTSVoice = "another.wav" },
		func(s *config.VoiceSettings) { s.TTSDevice = "changed" },
		func(s *config.VoiceSettings) { s.TTSServerPort++ },
	} {
		current := previous
		mutate(&current)
		got, err := applyVoiceModuleUpdate(current, setupVoiceInstallResult{Root: "candidate", updateFrom: &previous})
		if err == nil || !reflect.DeepEqual(got, current) {
			t.Fatal("update overwrote a changed TTS selection")
		}
	}
}
