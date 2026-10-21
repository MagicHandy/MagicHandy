package openaiauth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiagnosticRedactionIsLazyAndNeverReturnsStoredSecrets(t *testing.T) {
	dir := t.TempDir()
	m, err := Open(Options{DataDir: dir, LazyStorage: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	if _, err := m.RedactDiagnostics(t.Context(), []string{"a local failure"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "openai-private")); !os.IsNotExist(err) {
		t.Fatal("local failure initialized cloud storage")
	}
	const secret = "fixture/nonstandard+private"
	if err := m.SetAPIKey(t.Context(), secret); err != nil {
		t.Fatal(err)
	}
	values, err := m.RedactDiagnostics(t.Context(), []string{"prefix " + secret + " suffix", "fixture%2Fnonstandard%2Bprivate"})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range values {
		if !strings.Contains(value, "[redacted]") || strings.Contains(value, "fixture") {
			t.Fatal("credential not redacted")
		}
	}
}
