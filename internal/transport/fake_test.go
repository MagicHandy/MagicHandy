package transport

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestFakeRecordsCommandShape(t *testing.T) {
	fake := newGoldenFake()
	recordGoldenCommands(t, fake)

	got, err := json.MarshalIndent(fake.Commands(), "", "  ")
	if err != nil {
		t.Fatalf("marshal commands: %v", err)
	}
	got = append(got, '\n')

	want, err := os.ReadFile("testdata/fake_commands.golden.json")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	want = []byte(strings.ReplaceAll(string(want), "\r\n", "\n"))
	if string(got) != string(want) {
		t.Fatalf("command shape mismatch\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestFakeDiagnosticsTrackLastCommandResult(t *testing.T) {
	fake := newGoldenFake()
	recordGoldenCommands(t, fake)

	diagnostics := fake.Diagnostics()
	if diagnostics.Name != fakeTransportName {
		t.Fatalf("name = %q, want %q", diagnostics.Name, fakeTransportName)
	}
	if diagnostics.CommandCount != 4 {
		t.Fatalf("command count = %d, want 4", diagnostics.CommandCount)
	}
	if diagnostics.PlaybackState != "idle" {
		t.Fatalf("playback state = %q, want idle", diagnostics.PlaybackState)
	}
	if diagnostics.LastCommand == nil || diagnostics.LastCommand.Kind != CommandKindStop {
		t.Fatalf("last command = %+v, want stop", diagnostics.LastCommand)
	}
	if diagnostics.LastResult == nil || !diagnostics.LastResult.OK {
		t.Fatalf("last result = %+v, want OK", diagnostics.LastResult)
	}
	if diagnostics.LastLatencyMillis != 12 {
		t.Fatalf("latency = %d, want 12", diagnostics.LastLatencyMillis)
	}
}

func TestDiagnosticsDoNotExposeSecrets(t *testing.T) {
	fake := newGoldenFake()
	_, err := fake.Stop(context.Background(), StopCommand{Reason: "secret-key-123"})
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}

	diagnostics, err := json.Marshal(fake.Diagnostics())
	if err != nil {
		t.Fatalf("marshal diagnostics: %v", err)
	}
	if strings.Contains(string(diagnostics), "secret-key-123") {
		t.Fatal("diagnostics leaked secret-like stop reason")
	}
}

func TestFakeRecordsContextCancellation(t *testing.T) {
	fake := newGoldenFake()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := fake.Stop(ctx, StopCommand{Reason: "cancelled"})
	if err == nil {
		t.Fatal("Stop succeeded with a cancelled context")
	}
	if result.OK {
		t.Fatal("cancelled command returned an OK result")
	}
	if fake.Diagnostics().LastError == "" {
		t.Fatal("cancelled command did not update diagnostics error")
	}
}

func newGoldenFake() *Fake {
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	return NewFake(WithClock(func() time.Time { return now }), WithLatency(12*time.Millisecond))
}

func recordGoldenCommands(t *testing.T, fake *Fake) {
	t.Helper()

	ctx := context.Background()
	if _, err := fake.SetStrokeWindow(ctx, StrokeWindowCommand{MinPercent: 10, MaxPercent: 90}); err != nil {
		t.Fatalf("SetStrokeWindow: %v", err)
	}
	if _, err := fake.AppendPoints(ctx, AppendPointsCommand{
		StreamID: "trace-test",
		Points: []TimedPoint{
			{PositionPercent: 20, TimeMillis: 0},
			{PositionPercent: 80, TimeMillis: 250},
		},
	}); err != nil {
		t.Fatalf("AppendPoints: %v", err)
	}
	if _, err := fake.Play(ctx, PlayCommand{StreamID: "trace-test"}); err != nil {
		t.Fatalf("Play: %v", err)
	}
	if _, err := fake.Stop(ctx, StopCommand{Reason: "test complete"}); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}
