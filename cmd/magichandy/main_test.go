package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestApplicationRuntimeMotionSimulationIsExplicit(t *testing.T) {
	production := applicationRuntime(false)
	if production.MotionTransport != nil || production.MotionSimulated {
		t.Fatal("production runtime unexpectedly bypasses the selected device owner")
	}

	review := applicationRuntime(true)
	if !review.MotionSimulated {
		t.Fatal("review runtime does not disclose motion simulation")
	}
	if review.MotionTransport == nil || review.MotionTransport.Diagnostics().Name != "fake_handy" {
		t.Fatalf("review motion transport = %+v, want fake_handy", review.MotionTransport)
	}
	if review.Transport.Diagnostics().Name != "fake_handy" {
		t.Fatalf("review diagnostics transport = %+v", review.Transport.Diagnostics())
	}
}

func TestRunConfiguresLanguagesWithoutStartingServer(t *testing.T) {
	dataDir := t.TempDir()
	seed, err := config.OpenStore(dataDir)
	if err != nil {
		t.Fatalf("OpenStore seed: %v", err)
	}
	settings, _ := seed.Snapshot()
	settings.Server.Port = 51234
	if _, err := seed.Save(settings); err != nil {
		t.Fatalf("Save seed: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("Close seed: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err = run([]string{
		"-data-dir", dataDir,
		"-set-ui-locale", config.LocaleJapanese,
		"-set-chat-locale", config.LocaleSpanish,
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run language configuration: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "ui=ja chat=es") {
		t.Fatalf("stdout = %q", stdout.String())
	}

	store, err := config.OpenStore(dataDir)
	if err != nil {
		t.Fatalf("OpenStore verify: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close verify store: %v", err)
		}
	})
	got, _ := store.Snapshot()
	if got.UI.Locale != config.LocaleJapanese {
		t.Fatalf("UI locale = %q, want %q", got.UI.Locale, config.LocaleJapanese)
	}
	if got.LLM.PromptSet != config.PromptSetMagicHandyMotionV1ES {
		t.Fatalf("prompt set = %q, want %q", got.LLM.PromptSet, config.PromptSetMagicHandyMotionV1ES)
	}
	if got.Server.Port != 51234 {
		t.Fatalf("server port = %d, want preserved 51234", got.Server.Port)
	}
	if got.UI.SetupCompleted {
		t.Fatal("language-only bootstrap should leave guided setup incomplete")
	}
}

func TestRunLanguageConfigurationCanCompleteUnattendedSetup(t *testing.T) {
	dataDir := t.TempDir()
	err := run([]string{
		"-data-dir", dataDir,
		"-set-ui-locale", config.LocaleEnglish,
		"-set-chat-locale", config.LocaleEnglish,
		"-complete-setup",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run completed language configuration: %v", err)
	}

	store, err := config.OpenStore(dataDir)
	if err != nil {
		t.Fatalf("OpenStore verify: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	settings, _ := store.Snapshot()
	if !settings.UI.SetupCompleted {
		t.Fatal("unattended setup was not marked complete")
	}
}

func TestRunLanguageConfigurationRefusesRecoveredDefaults(t *testing.T) {
	dataDir := t.TempDir()
	seed, err := config.OpenStore(dataDir)
	if err != nil {
		t.Fatalf("OpenStore seed: %v", err)
	}
	const invalidDocument = "{broken"
	if _, err := seed.Datastore().SQL().Exec(`
		INSERT INTO settings(id, document, updated_at)
		VALUES('current', ?, 'fixture')
		ON CONFLICT(id) DO UPDATE SET document = excluded.document
	`, invalidDocument); err != nil {
		_ = seed.Close()
		t.Fatalf("seed invalid settings: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("Close seed: %v", err)
	}

	err = run([]string{
		"-data-dir", dataDir,
		"-set-ui-locale", config.LocaleJapanese,
		"-set-chat-locale", config.LocaleSpanish,
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "recovered defaults are active") {
		t.Fatalf("run error = %v, want recovered-default refusal", err)
	}

	store, err := config.OpenStore(dataDir)
	if err != nil {
		t.Fatalf("OpenStore verify: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	settings, status := store.Snapshot()
	if !status.Recovered || !status.UsingDefaults {
		t.Fatalf("status = %+v, want recovery to remain active", status)
	}
	if settings.UI.Locale != config.LocaleEnglish {
		t.Fatalf("UI locale = %q, want recovered default %q", settings.UI.Locale, config.LocaleEnglish)
	}
	var activeRows int
	if err := store.Datastore().SQL().QueryRow(`SELECT COUNT(*) FROM settings`).Scan(&activeRows); err != nil {
		t.Fatalf("count active settings: %v", err)
	}
	if activeRows != 0 {
		t.Fatalf("active settings rows = %d, want 0", activeRows)
	}
}

func TestRunLanguageConfigurationRequiresTwoSupportedLocales(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing chat", args: []string{"-set-ui-locale", "es"}, want: "must be provided together"},
		{name: "invalid UI", args: []string{"-set-ui-locale", "fr", "-set-chat-locale", "es"}, want: "unsupported UI locale"},
		{name: "invalid chat", args: []string{"-set-ui-locale", "es", "-set-chat-locale", "fr"}, want: "unsupported chat locale"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"-data-dir", t.TempDir()}, test.args...)
			err := run(args, &bytes.Buffer{}, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("run error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestRunConfiguresScriptedTTSWithoutStartingServer(t *testing.T) {
	dataDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{
		"-data-dir", dataDir,
		"-configure-tts-module", config.VoiceTTSProviderFasterQwen,
		"-tts-module-root", `C:\MagicHandy\voice\faster-qwen3-tts`,
		"-tts-base-url", "http://127.0.0.1:9015",
		"-tts-model", config.DefaultFasterQwenModel,
		"-tts-voice", "default",
		"-tts-language", "English",
		"-tts-device", config.TTSDeviceCUDA,
		"-tts-server-port", "9015",
		"-tts-auto-launch",
		"-tts-speak-replies",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run TTS configuration: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "provider=faster_qwen3_tts auto_launch=true") {
		t.Fatalf("stdout = %q", stdout.String())
	}

	store, err := config.OpenStore(dataDir)
	if err != nil {
		t.Fatalf("OpenStore verify: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	settings, _ := store.Snapshot()
	if settings.Voice.TTSProvider != config.VoiceTTSProviderFasterQwen ||
		settings.Voice.TTSServerPort != 9015 ||
		settings.Voice.TTSDevice != config.TTSDeviceCUDA ||
		!settings.Voice.TTSAutoLaunch ||
		!settings.Voice.SpeakReplies {
		t.Fatalf("saved TTS settings = %+v", settings.Voice)
	}
}

func TestRunConfiguresFasterQwenBeforeGUIReference(t *testing.T) {
	dataDir := t.TempDir()
	err := run([]string{
		"-data-dir", dataDir,
		"-configure-tts-module", config.VoiceTTSProviderFasterQwen,
		"-tts-module-root", `C:\MagicHandy\voice\faster-qwen3-tts`,
		"-tts-device", config.TTSDeviceCUDA,
		"-tts-server-port", "8991",
		"-tts-auto-launch",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run pre-reference TTS configuration: %v", err)
	}

	store, err := config.OpenStore(dataDir)
	if err != nil {
		t.Fatalf("OpenStore verify: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	settings, _ := store.Snapshot()
	if settings.Voice.TTSProvider != config.VoiceTTSProviderFasterQwen ||
		settings.Voice.TTSReferenceWAV != "" ||
		settings.Voice.TTSReferenceText != "" ||
		!settings.Voice.TTSAutoLaunch {
		t.Fatalf("saved pre-reference TTS settings = %+v", settings.Voice)
	}
}

func TestRunTTSModuleUpdatePreservesGUIReference(t *testing.T) {
	dataDir := t.TempDir()
	initialArgs := []string{
		"-data-dir", dataDir,
		"-configure-tts-module", config.VoiceTTSProviderFasterQwen,
		"-tts-module-root", `C:\MagicHandy\voice\faster-qwen3-tts`,
		"-tts-device", config.TTSDeviceCUDA,
		"-tts-server-port", "8991",
	}
	if err := run(initialArgs, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run initial TTS configuration: %v", err)
	}

	store, err := config.OpenStore(dataDir)
	if err != nil {
		t.Fatalf("OpenStore for GUI update: %v", err)
	}
	_, _, err = store.Update(func(settings config.Settings) (config.Settings, error) {
		settings.Voice.TTSReferenceWAV = `C:\voices\sample.wav`
		settings.Voice.TTSReferenceText = "Exact reference transcript."
		settings.Voice.TTSWorkerPath = `C:\old-install\voice-openai-tts-worker.exe`
		settings.Voice.TTSWorkerArgs = []string{"--legacy"}
		return settings, nil
	})
	if closeErr := store.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("save GUI reference: %v", err)
	}

	updateArgs := []string{
		"-data-dir", dataDir,
		"-configure-tts-module", config.VoiceTTSProviderFasterQwen,
		"-tts-module-root", `C:\MagicHandy\voice\faster-qwen3-tts`,
		"-tts-device", config.TTSDeviceCUDA,
		"-tts-server-port", "8993",
	}
	if err := run(updateArgs, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run reference-independent TTS update: %v", err)
	}

	store, err = config.OpenStore(dataDir)
	if err != nil {
		t.Fatalf("OpenStore verify: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	settings, _ := store.Snapshot()
	if settings.Voice.TTSReferenceWAV != `C:\voices\sample.wav` ||
		settings.Voice.TTSReferenceText != "Exact reference transcript." ||
		settings.Voice.TTSServerPort != 8993 ||
		settings.Voice.TTSWorkerPath != "" || len(settings.Voice.TTSWorkerArgs) != 0 {
		t.Fatalf("saved TTS settings after update = %+v", settings.Voice)
	}
}

func TestRunRejectsUnsupportedFasterQwenCPUConfiguration(t *testing.T) {
	err := run([]string{
		"-data-dir", t.TempDir(),
		"-configure-tts-module", config.VoiceTTSProviderFasterQwen,
		"-tts-module-root", `C:\MagicHandy\voice\faster-qwen3-tts`,
		"-tts-device", config.TTSDeviceCPU,
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "NVIDIA CUDA") {
		t.Fatalf("run error = %v, want Faster Qwen CUDA validation", err)
	}
}

func TestRunConfiguresChatterboxDefaults(t *testing.T) {
	dataDir := t.TempDir()
	err := run([]string{
		"-data-dir", dataDir,
		"-configure-tts-module", config.VoiceTTSProviderChatterbox,
		"-tts-module-root", `C:\MagicHandy\voice\chatterbox-tts`,
		"-tts-server-port", "8992",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run Chatterbox configuration: %v", err)
	}

	store, err := config.OpenStore(dataDir)
	if err != nil {
		t.Fatalf("OpenStore verify: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	settings, _ := store.Snapshot()
	if settings.Voice.TTSModel != config.DefaultChatterboxModel ||
		settings.Voice.TTSVoice != config.DefaultChatterboxVoice ||
		settings.Voice.TTSHealthPath != config.DefaultChatterboxHealthPath {
		t.Fatalf("saved Chatterbox defaults = %+v", settings.Voice)
	}
}

func TestValidateListenAddressRequiresAuthenticatedHTTPSForRemoteBinding(t *testing.T) {
	for _, address := range []string{
		"127.0.0.1:8080",
		"127.12.34.56:8080",
		"localhost:8080",
		"[::1]:8080",
	} {
		if err := validateListenAddress(address, false, false); err != nil {
			t.Errorf("validateListenAddress(%q) = %v, want accepted loopback", address, err)
		}
	}
	for _, address := range []string{
		"0.0.0.0:8080",
		"[::]:8080",
		"magic-handy.local:8080",
	} {
		if err := validateListenAddress(address, true, true); err == nil {
			t.Errorf("validateListenAddress(%q) = nil, want ambiguous binding rejection", address)
		}
	}
	if err := validateListenAddress("192.168.1.20:8080", false, true); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("remote HTTP error = %v, want HTTPS requirement", err)
	}
	if err := validateListenAddress("192.168.1.20:8080", true, false); err == nil || !strings.Contains(err.Error(), "account") {
		t.Fatalf("remote unauthenticated error = %v, want account requirement", err)
	}
	if err := validateListenAddress("192.168.1.20:8080", true, true); err != nil {
		t.Fatalf("authenticated remote HTTPS error = %v", err)
	}
	if err := validateListenAddress("8.8.8.8:8080", true, true); err == nil || !strings.Contains(err.Error(), "internet-facing") {
		t.Fatalf("public address error = %v, want internet-facing rejection", err)
	}
}
