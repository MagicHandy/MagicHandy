//go:build windows

package main

import (
	"testing"
	"time"
)

func TestInstallerShutdownEventStopsMatchingExecutable(t *testing.T) {
	requests, cleanup, err := listenForInstallerShutdown()
	if err != nil {
		t.Fatalf("listenForInstallerShutdown: %v", err)
	}

	type shutdownResult struct {
		requested bool
		err       error
	}
	result := make(chan shutdownResult, 1)
	go func() {
		requested, requestErr := requestInstallerShutdown(5 * time.Second)
		result <- shutdownResult{requested: requested, err: requestErr}
	}()

	select {
	case <-requests:
		cleanup()
	case <-time.After(2 * time.Second):
		cleanup()
		t.Fatal("shutdown event was not delivered")
	}
	got := <-result
	if got.err != nil {
		t.Fatalf("requestInstallerShutdown: %v", got.err)
	}
	if !got.requested {
		t.Fatal("requestInstallerShutdown did not report a running instance")
	}
}
