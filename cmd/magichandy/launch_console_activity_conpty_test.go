//go:build windows && consoleharness

package main

import (
	"io"
	"net/http"
	"testing"
	"time"
)

func verifyConsoleActivityAfterPolling(t *testing.T, run *pseudoConsoleRun, baseURL string) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{}}
	defer client.CloseIdleConnections()
	// More requests than the old shared 400-record history could hold. These
	// run against the isolated simulated app and cannot command a device.
	for range 450 {
		consoleProbeRequest(t, client, baseURL+"/healthz", http.StatusOK)
	}
	start := len(run.output())
	run.press("d")
	waitForConsoleTextAfter(t, run, start, "GET /healthz 200")
	start = len(run.output())
	run.press("d")
	// Check newly painted output, not an old startup line in the capture.
	waitForConsoleTextAfter(t, run, start, "Server starting")
	start = len(run.output())
	consoleProbeRequest(t, client, baseURL+"/api/console-harness-missing", http.StatusNotFound)
	waitForConsoleTextAfter(t, run, start, "GET /api/console-harness-missing 404")
}

func waitForConsoleTextAfter(t *testing.T, run *pseudoConsoleRun, start int, text string) {
	t.Helper()
	run.waitUntil(t, text, func() string {
		return terminalSequence.ReplaceAllString(run.output()[start:], "")
	})
}

func consoleProbeRequest(t *testing.T, client *http.Client, url string, wantStatus int) {
	t.Helper()
	response, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	_, readErr := io.Copy(io.Discard, response.Body)
	closeErr := response.Body.Close()
	if response.StatusCode != wantStatus || readErr != nil || closeErr != nil {
		t.Fatalf("console probe: status=%d read=%v close=%v", response.StatusCode, readErr, closeErr)
	}
}
