package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestLoggerWritesStructuredJSON(t *testing.T) {
	var out bytes.Buffer

	logger := New(&out, slog.LevelDebug)
	logger.Info("core ready", "addr", "127.0.0.1:49717")

	var entry map[string]any
	if err := json.Unmarshal(out.Bytes(), &entry); err != nil {
		t.Fatalf("log output is not JSON: %v", err)
	}

	if entry["msg"] != "core ready" {
		t.Fatalf("msg = %v, want core ready", entry["msg"])
	}
	if entry["service"] != serviceName {
		t.Fatalf("service = %v, want %s", entry["service"], serviceName)
	}
	if entry["addr"] != "127.0.0.1:49717" {
		t.Fatalf("addr = %v, want 127.0.0.1:49717", entry["addr"])
	}
}

func TestParseLevelRejectsUnknownLevel(t *testing.T) {
	if _, err := ParseLevel("verbose"); err == nil {
		t.Fatal("ParseLevel accepted an unknown level")
	}
}
