package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

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
