package startup

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProfileLockAndStaleReadiness(t *testing.T) {
	root := t.TempDir()
	first, err := Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(filepath.Join(root, ".")); !errors.Is(err, ErrRunning) {
		t.Fatalf("second owner: %v", err)
	}
	other, err := Acquire(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	other.Close()
	if err := first.Publish("http://127.0.0.1:1", ""); err != nil {
		t.Fatal(err)
	}
	// Closing the OS handle models process death: a stale record must not grant
	// ownership or survive acquisition by the next process.
	if err := first.file.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if _, err := os.Stat(filepath.Join(root, "startup", "instance.json")); !os.IsNotExist(err) {
		t.Fatalf("stale readiness still present: %v", err)
	}
}

func TestExistingInstanceRequiresFreshProof(t *testing.T) {
	root := t.TempDir()
	instance, err := Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()
	server := httptest.NewServer(instance.Handler(http.NotFoundHandler()))
	defer server.Close()
	if err := instance.Publish(server.URL, ""); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	existing, err := FindExisting(ctx, root)
	if err != nil || existing.URL != server.URL || existing.PID != os.Getpid() {
		t.Fatalf("discovery=%+v err=%v", existing, err)
	}

	var captured string
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.URL.String() + r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"instance_id":"` + existing.ID + `","proof":"wrong"}`))
	}))
	defer foreign.Close()
	if err := instance.Publish(foreign.URL, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyExisting(t.Context(), root, foreign.Client()); err == nil {
		t.Fatal("accepted unrelated service")
	}
	if strings.Contains(captured, instance.info.Token) {
		t.Fatal("private identity token was transmitted")
	}
}

func TestIdentityNeverExposesStateToForeignHostsOrPeers(t *testing.T) {
	instance, err := Acquire(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()
	if err := instance.Publish("http://127.0.0.1:49717", ""); err != nil {
		t.Fatal(err)
	}
	for _, peer := range []string{"192.168.1.2:5000", "127.0.0.1:5000"} {
		for _, host := range []string{"evil.example:49717", "127.0.0.1:49717"} {
			r := httptest.NewRequest(http.MethodGet, "http://"+host+identityPath+"?challenge="+randomHex(), nil)
			r.RemoteAddr = peer
			w := httptest.NewRecorder()
			instance.Handler(http.NotFoundHandler()).ServeHTTP(w, r)
			allowed := strings.HasPrefix(peer, "127.") && host == "127.0.0.1:49717"
			if (w.Code == http.StatusOK) != allowed {
				t.Fatalf("peer=%s host=%s status=%d", peer, host, w.Code)
			}
			if strings.Contains(w.Body.String(), instance.info.Token) {
				t.Fatal("response exposed identity token")
			}
		}
	}
}

func TestDiscoveryRejectsRedirectsAndNonlocalRecords(t *testing.T) {
	root := t.TempDir()
	instance, err := Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://192.0.2.1/", http.StatusFound)
	}))
	defer redirect.Close()
	if err := instance.Publish(redirect.URL, ""); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	if _, err := FindExisting(ctx, root); err == nil {
		t.Fatal("accepted redirect")
	}
	for _, address := range []string{"http://example.test:80", "http://127.0.0.1:1@evil.test:80", "https://127.0.0.1:443", "http://127.0.0.1:80/path"} {
		if _, err := localOrigin(address); err == nil {
			t.Fatalf("accepted unsafe origin %q", address)
		}
	}
}

func TestHostedReadinessFailsDiscoveryWithoutWaitingForStartup(t *testing.T) {
	root := t.TempDir()
	instance, err := Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()
	if err := instance.Publish("https://127.0.0.1:49717", ""); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	started := time.Now()
	if _, err := FindExisting(ctx, root); !errors.Is(err, errNotDiscoverable) || time.Since(started) > time.Second {
		t.Fatalf("hosted discovery did not fail promptly: %v", err)
	}
}
