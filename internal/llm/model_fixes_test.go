package llm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// An unmeasured Gemma 4 template shaped like the older E4B builds' templates:
// a turn/channel format whose generation prompt never closes the thought
// channel when thinking is off.
const gemma4TemplateWithoutClose = "{{- bos_token -}}{%- for message in messages -%}" +
	"{{- '<|turn>' + message['role'] + '\\n' + message['content'] + '<turn|>\\n' -}}{%- endfor -%}" +
	"{%- if add_generation_prompt -%}{{- '<|turn>model\\n' -}}{%- endif -%}" +
	"{#- thinking uses <|channel>thought -#}"

// The same template after Google's later fix.
const gemma4TemplateWithClose = gemma4TemplateWithoutClose +
	"{%- if not enable_thinking -%}{{- '<|channel>thought\\n<channel|>' -}}{%- endif -%}"

func importFixtureModel(t *testing.T, manager *ModelManager, options ggufFixtureOptions) ModelRecord {
	t.Helper()
	source := filepath.Join(t.TempDir(), "fixture.gguf")
	if err := os.WriteFile(source, testGGUFData(t, options), 0o600); err != nil {
		t.Fatal(err)
	}
	job, err := manager.StartGGUFImport(source, "Fixture")
	if err != nil {
		t.Fatalf("StartGGUFImport: %v", err)
	}
	job = waitForImport(t, manager, job.ID)
	if job.Status != ImportStatusComplete {
		t.Fatalf("import = %+v", job)
	}
	record, err := manager.Model(context.Background(), job.ModelID)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func openTestModelManager(t *testing.T, dataDir string) *ModelManager {
	t.Helper()
	manager, err := OpenModelManager(dataDir)
	if err != nil {
		t.Fatalf("OpenModelManager: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	return manager
}

func TestKnownTemplateIsFixedAutomatically(t *testing.T) {
	digest := sha256.Sum256([]byte(gemma4TemplateWithoutClose))
	key := hex.EncodeToString(digest[:])
	knownTemplateFixes[key] = TemplateFixGemma4CloseThinking
	t.Cleanup(func() { delete(knownTemplateFixes, key) })

	manager := openTestModelManager(t, t.TempDir())
	record := importFixtureModel(t, manager, ggufFixtureOptions{architecture: "gemma4", chatTemplate: gemma4TemplateWithoutClose})
	if record.Architecture != "gemma4" || record.TemplateFix != TemplateFixGemma4CloseThinking ||
		record.TemplateFixSource != TemplateFixSourceKnown || record.TemplateFixOffer != "" {
		t.Fatalf("record = %+v, want the built-in fix", record)
	}
	if _, err := manager.SetTemplateFix(context.Background(), record.ID, false); !errors.Is(err, ErrNoTemplateFix) {
		t.Fatalf("SetTemplateFix(off) on a built-in fix = %v, want ErrNoTemplateFix", err)
	}
}

func TestUnmeasuredGemma4TemplateIsOfferedAndRemembered(t *testing.T) {
	dataDir := t.TempDir()
	manager := openTestModelManager(t, dataDir)
	record := importFixtureModel(t, manager, ggufFixtureOptions{architecture: "gemma4", chatTemplate: gemma4TemplateWithoutClose})
	if record.TemplateFix != "" || record.TemplateFixOffer != TemplateFixGemma4CloseThinking {
		t.Fatalf("record = %+v, want an offered fix only", record)
	}

	enabled, err := manager.SetTemplateFix(context.Background(), record.ID, true)
	if err != nil {
		t.Fatalf("SetTemplateFix(on): %v", err)
	}
	if enabled.TemplateFix != TemplateFixGemma4CloseThinking || enabled.TemplateFixSource != TemplateFixSourceUser || enabled.TemplateFixOffer != "" {
		t.Fatalf("enabled = %+v", enabled)
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestModelManager(t, dataDir)
	models, err := reopened.List(context.Background())
	if err != nil || len(models) != 1 || models[0].TemplateFixSource != TemplateFixSourceUser {
		t.Fatalf("reopened models = %+v, %v; want the choice remembered", models, err)
	}

	disabled, err := reopened.SetTemplateFix(context.Background(), record.ID, false)
	if err != nil {
		t.Fatalf("SetTemplateFix(off): %v", err)
	}
	if disabled.TemplateFix != "" || disabled.TemplateFixOffer != TemplateFixGemma4CloseThinking {
		t.Fatalf("disabled = %+v, want the offer back", disabled)
	}
}

func TestTemplateFixIsOnlyOfferedForTheRecognizedDefect(t *testing.T) {
	cases := []struct {
		name    string
		options ggufFixtureOptions
		offer   string
	}{
		{"fixed Gemma 4 template", ggufFixtureOptions{architecture: "gemma4", chatTemplate: gemma4TemplateWithClose}, ""},
		{"Gemma 4 without a template", ggufFixtureOptions{architecture: "gemma4"}, TemplateFixGemma4CloseThinking},
		{"other architecture", ggufFixtureOptions{architecture: "llama", chatTemplate: gemma4TemplateWithoutClose}, ""},
		{"non-channel Gemma 4 template", ggufFixtureOptions{architecture: "gemma4", chatTemplate: "{{ messages }}"}, ""},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			manager := openTestModelManager(t, t.TempDir())
			record := importFixtureModel(t, manager, test.options)
			if record.TemplateFix != "" || record.TemplateFixOffer != test.offer {
				t.Fatalf("record = %+v, want offer %q", record, test.offer)
			}
			if test.offer == "" {
				if _, err := manager.SetTemplateFix(context.Background(), record.ID, true); !errors.Is(err, ErrNoTemplateFix) {
					t.Fatalf("SetTemplateFix = %v, want ErrNoTemplateFix", err)
				}
			}
		})
	}
}

func TestTemplateFixFileInstallsTheEmbeddedTemplate(t *testing.T) {
	dataDir := t.TempDir()
	manager := openTestModelManager(t, dataDir)
	path, err := manager.TemplateFixFile(TemplateFixGemma4CloseThinking)
	if err != nil {
		t.Fatalf("TemplateFixFile: %v", err)
	}
	if filepath.Dir(path) != filepath.Join(dataDir, "models", "templates") {
		t.Fatalf("template path %q is outside the model store", path)
	}
	assertTemplateFile := func() {
		t.Helper()
		written, readErr := os.ReadFile(path) // #nosec G304 -- temp fixture path.
		if readErr != nil || !bytes.Equal(written, gemma4CloseThinkingTemplate) {
			t.Fatalf("template file differs from the embedded template: %v", readErr)
		}
	}
	assertTemplateFile()
	if err := os.WriteFile(path, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if again, err := manager.TemplateFixFile(TemplateFixGemma4CloseThinking); err != nil || again != path {
		t.Fatalf("TemplateFixFile again = %q, %v", again, err)
	}
	assertTemplateFile()
	if _, err := manager.TemplateFixFile("unknown-fix"); err == nil {
		t.Fatal("TemplateFixFile accepted an unknown fix")
	}
}

// A startup autoload and a chat request can install the template at the same
// moment. Without serialization, Windows refuses one rename with "Access is
// denied" and that load fails.
func TestTemplateFixFileToleratesConcurrentLoads(t *testing.T) {
	for round := range 20 {
		manager := openTestModelManager(t, t.TempDir())
		var wg sync.WaitGroup
		errs := make(chan error, 6)
		for range 6 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := manager.TemplateFixFile(TemplateFixGemma4CloseThinking); err != nil {
					errs <- err
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatalf("round %d: concurrent template install failed: %v", round, err)
		}
	}
}

func TestEmbeddedFixTemplateClosesThinking(t *testing.T) {
	template := string(gemma4CloseThinkingTemplate)
	if !strings.Contains(template, `<|channel>thought\n<channel|>`) || !strings.Contains(template, "if not enable_thinking") {
		t.Fatal("embedded Gemma 4 fix template does not close the thought channel when thinking is off")
	}
	for digest, fix := range knownTemplateFixes {
		if len(digest) != 64 {
			t.Fatalf("known template digest %q is not a SHA-256", digest)
		}
		if _, ok := templateFixTemplates[fix]; !ok {
			t.Fatalf("known template %s names fix %q without a template", digest, fix)
		}
	}
}

func TestManagedLlamaCPPPassesTemplateFixAndReportsOffload(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(modelPath, []byte("test model"), 0o600); err != nil {
		t.Fatal(err)
	}
	templatePath := filepath.Join(dir, "fix.jinja")
	argsPath := filepath.Join(dir, "args.txt")
	t.Setenv("MAGICHANDY_TEST_LLAMA_RUNNER", "1")
	t.Setenv("MAGICHANDY_TEST_LLAMA_RUNNER_ARGS", argsPath)
	t.Setenv("MAGICHANDY_TEST_LLAMA_RUNNER_OUTPUT", "load_tensors: offloaded 41/43 layers to GPU")

	provider, err := NewManagedLlamaCPPProvider(ManagedLlamaCPPOptions{
		HTTPProviderOptions: HTTPProviderOptions{BaseURL: "http://127.0.0.1:18099", Model: "local-model"},
		RunnerPath:          os.Args[0],
		ModelPath:           modelPath,
		ContextSize:         32768,
		ChatTemplateFile:    templatePath,
	})
	if err != nil {
		t.Fatalf("NewManagedLlamaCPPProvider: %v", err)
	}
	t.Cleanup(func() { provider.Unload(context.Background()) })
	if err := provider.ensureStarted(); err != nil {
		t.Fatalf("ensureStarted: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for provider.LoadReport().TotalLayers == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if report := provider.LoadReport(); report.OffloadedLayers != 41 || report.TotalLayers != 43 || report.FullyOffloaded() {
		t.Fatalf("load report = %+v, want 41/43 partial", report)
	}
	arguments, err := os.ReadFile(argsPath) // #nosec G304 -- temp fixture path.
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(arguments), fmt.Sprintf("--chat-template-file\n%s", templatePath)) {
		t.Fatalf("runner arguments %q do not load the template fix", arguments)
	}
}

func TestLoadReportWatcherReadsSplitLines(t *testing.T) {
	var watcher loadReportWatcher
	for _, chunk := range []string{"llama_model_load: ", "done\nload_tensors: offl", "oaded 49/49 layers to GPU\r\n", "srv listening\n"} {
		_, _ = watcher.Write([]byte(chunk))
	}
	if report := watcher.Report(); report.OffloadedLayers != 49 || report.TotalLayers != 49 || !report.FullyOffloaded() {
		t.Fatalf("report = %+v, want 49/49", report)
	}
	watcher.Reset()
	if report := watcher.Report(); report.TotalLayers != 0 || report.FullyOffloaded() {
		t.Fatalf("report after reset = %+v", report)
	}
	_, _ = watcher.Write(bytes.Repeat([]byte("x"), 3*loadReportLineLimit))
	if len(watcher.partial) > loadReportLineLimit {
		t.Fatalf("partial line grew to %d bytes", len(watcher.partial))
	}
}
