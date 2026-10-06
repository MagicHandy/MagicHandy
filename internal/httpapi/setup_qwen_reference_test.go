package httpapi

import (
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestReferenceWAV(t *testing.T, seconds int) string {
	t.Helper()
	const rate = 16000 * 2
	audio := make([]byte, 44+seconds*rate)
	copy(audio, "RIFF")
	binary.LittleEndian.PutUint32(audio[4:], uint32(len(audio)-8)) //nolint:gosec // small fixed test fixture.
	copy(audio[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(audio[16:], 16)
	binary.LittleEndian.PutUint16(audio[20:], 1)
	binary.LittleEndian.PutUint16(audio[22:], 1)
	binary.LittleEndian.PutUint32(audio[24:], 16000)
	binary.LittleEndian.PutUint32(audio[28:], rate)
	binary.LittleEndian.PutUint16(audio[32:], 2)
	binary.LittleEndian.PutUint16(audio[34:], 16)
	copy(audio[36:], "data")
	binary.LittleEndian.PutUint32(audio[40:], uint32(seconds*rate)) //nolint:gosec // small fixed test fixture.
	path := filepath.Join(t.TempDir(), "sample.wav")
	if err := os.WriteFile(path, audio, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSetupQwenReferenceChecksBoundedWAVAndTranscript(t *testing.T) {
	valid := setupTestReferenceWAV(t, 3)
	if duration, err := checkSetupQwenWAV(valid); err != nil || duration != 3000 {
		t.Fatalf("duration=%d err=%v", duration, err)
	}
	for _, ref := range []*setupQwenReference{
		{WAV: valid, Transcript: " "},
		{WAV: valid, Transcript: strings.Repeat("x", 8193)},
		{WAV: "relative.wav", Transcript: "test"},
		{WAV: filepath.Join(t.TempDir(), "missing.wav"), Transcript: "test"},
		{WAV: setupTestReferenceWAV(t, 0), Transcript: "test"},
		{WAV: setupTestReferenceWAV(t, 31), Transcript: "test"},
	} {
		if _, err := normalizeSetupQwenReference(ref); err == nil {
			t.Fatalf("invalid reference accepted: %+v", ref)
		}
	}
	audio, err := os.ReadFile(valid) // #nosec G304 -- WAV fixture created in this test's t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []func([]byte){
		func(a []byte) { copy(a, "text") },
		func(a []byte) { binary.LittleEndian.PutUint32(a[40:], ^uint32(0)) },
		func(a []byte) { binary.LittleEndian.PutUint16(a[22:], 0) },
		func(a []byte) { binary.LittleEndian.PutUint16(a[32:], 0) },
	} {
		broken := append([]byte(nil), audio...)
		mutation(broken)
		if _, err := setupQwenWAVDuration(broken); err == nil {
			t.Fatal("malformed WAV accepted")
		}
	}
}

func TestSetupQwenReferenceCheckNeedsControllerAndDoesNotSave(t *testing.T) {
	server := newTestServer(t)
	path := setupTestReferenceWAV(t, 3)
	for _, controlled := range []bool{false, true} {
		recorder := httptest.NewRecorder()
		request := jsonAPIRequest(t, http.MethodPost, "/api/setup/qwen/reference/check", map[string]any{"wav": path})
		if controlled {
			request = withController(request)
		}
		server.Handler().ServeHTTP(recorder, request)
		if controlled && recorder.Code != http.StatusOK {
			t.Fatalf("check=%d %s", recorder.Code, recorder.Body.String())
		}
		if !controlled && recorder.Code != http.StatusConflict {
			t.Fatalf("unguarded check=%d", recorder.Code)
		}
	}
	settings, _ := server.store.Snapshot()
	if settings.Voice.TTSReferenceWAV != "" || settings.Voice.Enabled {
		t.Fatal("checking a file changed voice settings")
	}
}

func TestSetupQwenReferenceEnablesSpeechOnlyWithValidatedInstallReference(t *testing.T) {
	for _, configure := range []bool{false, true} {
		server := newTestServer(t)
		module, err := findSetupVoiceModule("faster-qwen3-tts")
		if err != nil {
			t.Fatal(err)
		}
		result := setupVoiceInstallResult{Module: module, Device: "cuda", Enable: true, AutoLaunch: false}
		if configure {
			result.Reference = &setupQwenReference{WAV: setupTestReferenceWAV(t, 3), Transcript: "Exact sample words."}
		}
		// A missing test runtime may be reported, but the saved settings still
		// prove the backend's gate. No synthesis, microphone or motion runs.
		_ = server.applyInstalledVoiceModule(context.Background(), result)
		settings, _ := server.store.Snapshot()
		if settings.Voice.Enabled != configure || settings.Voice.SpeakReplies != configure {
			t.Fatalf("reference=%v settings=%+v", configure, settings.Voice)
		}
		if configure && settings.Voice.TTSReferenceText != "Exact sample words." {
			t.Fatal("reference was not saved")
		}
	}
}

func TestSetupQwenChangedReferenceCannotReplaceExistingSettings(t *testing.T) {
	server := newTestServer(t)
	before, _ := server.store.Snapshot()
	module, _ := findSetupVoiceModule("faster-qwen3-tts")
	result := setupVoiceInstallResult{Module: module, Enable: true, Reference: &setupQwenReference{WAV: filepath.Join(t.TempDir(), "deleted.wav"), Transcript: "test"}}
	if err := server.applyInstalledVoiceModule(context.Background(), result); err == nil {
		t.Fatal("missing reference accepted after installation")
	}
	after, _ := server.store.Snapshot()
	if after.Voice.TTSProvider != before.Voice.TTSProvider || after.Voice.Enabled != before.Voice.Enabled {
		t.Fatal("invalid reference changed the existing provider")
	}
}
