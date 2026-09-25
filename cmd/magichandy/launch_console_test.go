package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/netaccess"
)

// Redirected output, as in scripts and tests, keeps the structured JSON logs.
func TestLaunchConsoleKeepsJSONLogsWhenOutputIsRedirected(t *testing.T) {
	var stdout, stderr bytes.Buffer
	for _, mode := range []string{consoleAuto, consolePlain} {
		consoleUI, err := startLaunchConsole(mode, false, &stdout, &stderr)
		if err != nil || consoleUI != nil {
			t.Fatalf("mode %s: console = %v, %v; want plain logs", mode, consoleUI, err)
		}
	}
	consoleUI, _ := startLaunchConsole(consoleAuto, false, &stdout, &stderr)
	consoleUI.logger(&stderr, 0).Info("server starting")
	if !strings.Contains(stderr.String(), `"msg":"server starting"`) {
		t.Fatalf("plain logs are not JSON: %q", stderr.String())
	}
	if consoleUI.finish(errors.New("failed")) {
		t.Fatal("plain logs claimed to show an error")
	}
	if _, err := startLaunchConsole("fancy", false, &stdout, &stderr); err == nil {
		t.Fatal("an unknown console mode was accepted")
	}
}

func TestRunRejectsUnknownConsoleMode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"-data-dir", t.TempDir(), "-console", "fancy"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "unknown console mode") {
		t.Fatalf("run error = %v", err)
	}
}

func TestAccessDescriptionUsesSetupTerms(t *testing.T) {
	policy := func(mode, scope string) *netaccess.Policy {
		return &netaccess.Policy{Config: netaccess.Config{Mode: mode, Scope: scope}}
	}
	for _, tc := range []struct {
		security serverSecurity
		want     string
	}{
		{serverSecurity{}, "Local only (this computer)"},
		{serverSecurity{NetworkPolicy: policy(netaccess.Local, "")}, "Local only (this computer)"},
		{serverSecurity{AuthenticationRequired: true}, "Local only (this computer); sign-in required"},
		{serverSecurity{NetworkPolicy: policy(netaccess.DirectHTTPS, "lan"), TLSConfig: &tls.Config{}, AuthenticationRequired: true}, "LAN + local, over HTTPS; sign-in required"},
		{serverSecurity{NetworkPolicy: policy(netaccess.DirectHTTPS, "public"), TLSConfig: &tls.Config{}}, "Public, over HTTPS"},
		{serverSecurity{NetworkPolicy: policy(netaccess.TrustedProxy, "")}, "Through a trusted proxy"},
		{serverSecurity{TLSConfig: &tls.Config{}}, "Network, over HTTPS"},
	} {
		if got := accessDescription(tc.security); got != tc.want {
			t.Fatalf("accessDescription(%+v) = %q, want %q", tc.security, got, tc.want)
		}
	}
}

// The console says the app is running only once the listener is bound; a
// port that is already in use fails before that.
func TestServeHTTPReportsReadyOnlyAfterBinding(t *testing.T) {
	server := newHTTPServer("127.0.0.1:0", http.NotFoundHandler(), nil)
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- serveHTTP(server, func() { close(ready) }) }()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("ready was not reported")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("serveHTTP = %v, want ErrServerClosed", err)
	}

	blocked := newHTTPServer("127.0.0.1:-1", http.NotFoundHandler(), nil)
	if err := serveHTTP(blocked, func() { t.Fatal("ready reported without a listener") }); err == nil {
		t.Fatal("an invalid listen address served")
	}
}
