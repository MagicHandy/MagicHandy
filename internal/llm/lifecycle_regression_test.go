package llm

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type retiredLlamaTransport struct{}

func (retiredLlamaTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	body := `{"status":"ok"}`
	if r.URL.Path == "/v1/models" {
		body = `{"data":[{"id":"retired-model"}]}`
	}
	if r.URL.Path == "/v1/chat/completions" {
		body = "data: {\"choices\":[{\"delta\":{\"content\":\"{}\"}}]}\n\ndata: [DONE]\n\n"
	}
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
}

func TestClosedManagedProviderCannotStartAgain(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	dir := t.TempDir()
	model := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(model, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	count := filepath.Join(dir, "starts.txt")
	t.Setenv("MAGICHANDY_TEST_LLAMA_RUNNER", "1")
	t.Setenv("MAGICHANDY_TEST_LLAMA_RUNNER_COUNT", count)
	p, err := NewManagedLlamaCPPProvider(ManagedLlamaCPPOptions{HTTPProviderOptions: HTTPProviderOptions{BaseURL: fmt.Sprintf("http://127.0.0.1:%d", port), Model: "retired-model", Client: &http.Client{Transport: retiredLlamaTransport{}}}, RunnerPath: os.Args[0], ModelPath: model, ContextSize: 4096})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Unload(context.Background()) })
	if status := p.Load(context.Background()); !status.Available {
		t.Fatalf("load failed: %+v", status)
	}
	if n := waitForStartCount(t, count); n != 1 {
		t.Fatalf("initial helper starts=%d", n)
	}
	if err = p.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = p.StreamChat(context.Background(), ChatRequest{}, nil); err == nil {
		t.Fatal("closed provider accepted a new generation")
	}
	if status := p.Load(context.Background()); status.Available {
		t.Fatal("closed provider reloaded")
	}
	payload, err := os.ReadFile(count) // #nosec G304 -- count is a parent-owned temporary test fixture.
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(payload), "start") != 1 {
		t.Fatalf("retired runner restarted: %s", payload)
	}
}

func TestExistingDeadlineHonorsShorterBudget(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	child, finish := checkedRequestContext(parent, 10*time.Millisecond)
	defer finish()
	deadline, _ := child.Deadline()
	remaining := time.Until(deadline)
	if remaining > 20*time.Millisecond {
		t.Fatalf("shorter budget ignored: remaining=%s", remaining)
	}

}
