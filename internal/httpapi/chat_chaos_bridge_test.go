package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestChatChaosBridgeFillerAppendsWithoutRestart(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
	})
	t.Cleanup(server.Close)

	settings, _ := server.store.Snapshot()
	settings.Motion.MotionGenerationMode = config.MotionGenerationModeProcedural
	settings.Motion.HardwareSafetyLock = false

	command := &chat.MotionCommand{
		Action:      chat.MotionActionStart,
		Velocidade:  50,
		Intensidade: 60,
		Regiao:      "meio",
		TipoBatida:  "fluido",
	}

	ctx := context.Background()
	server.chatChaos.mu.Lock()
	server.chatChaos.generation = 1
	generation := server.chatChaos.generation
	server.chatChaos.mu.Unlock()

	if err := server.playChatChaoticMotion(ctx, command, settings, generation); err != nil {
		t.Fatal(err)
	}

	waitDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(waitDeadline) {
		if countHSPPlays(fake.Commands()) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	initialPlays := countHSPPlays(fake.Commands())
	if initialPlays == 0 {
		t.Fatal("expected initial HSP play")
	}

	extended := false
	bridgeDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(bridgeDeadline) {
		server.chatChaosMaybeLoopBridge(ctx, generation, settings)
		server.chatChaos.mu.Lock()
		activePlayer := server.chatChaos.player
		server.chatChaos.mu.Unlock()
		if activePlayer != nil && activePlayer.TimelineEndMS() > 15_000 {
			extended = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !extended {
		t.Fatal("bridge filler did not extend timeline")
	}

	if plays := countHSPPlays(fake.Commands()); plays != initialPlays {
		t.Fatalf("hsp_plays = %d, want %d (append-only bridge)", plays, initialPlays)
	}

	server.chatChaos.mu.Lock()
	activePlayer := server.chatChaos.player
	server.chatChaos.mu.Unlock()
	if activePlayer == nil || !activePlayer.Running() {
		t.Fatal("player should still be running after bridge filler")
	}
}

func countHSPPlays(commands []transport.Command) int {
	count := 0
	for _, command := range commands {
		if command.Kind == transport.CommandKindHSPPlay {
			count++
		}
	}
	return count
}

func TestChatSendStopHaltsChaosPlayer(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
	})
	t.Cleanup(server.Close)

	settings, _ := server.store.Snapshot()
	settings.Motion.MotionGenerationMode = config.MotionGenerationModeProcedural
	settings.Motion.HardwareSafetyLock = false

	command := &chat.MotionCommand{
		Action:      chat.MotionActionStart,
		Velocidade:  50,
		Intensidade: 60,
		Regiao:      "meio",
		TipoBatida:  "fluido",
	}
	ctx := context.Background()
	server.chatChaos.mu.Lock()
	server.chatChaos.generation = 1
	generation := server.chatChaos.generation
	server.chatChaos.mu.Unlock()
	if err := server.playChatChaoticMotion(ctx, command, settings, generation); err != nil {
		t.Fatal(err)
	}
	server.chatChaos.mu.Lock()
	playerBefore := server.chatChaos.player
	server.chatChaos.mu.Unlock()
	if playerBefore == nil || !playerBefore.Running() {
		t.Fatal("expected chaos player running before stop")
	}

	recorder := httptest.NewRecorder()
	stopBody := `{"text":"stop"}`
	request := withController(httptest.NewRequest(http.MethodPost, "/api/chat/send", strings.NewReader(stopBody)))
	request.Header.Set("Content-Type", "application/json")
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("stop status %d: %s", recorder.Code, recorder.Body.String())
	}
	server.chatChaos.mu.Lock()
	playerAfter := server.chatChaos.player
	server.chatChaos.mu.Unlock()
	if playerAfter != nil && playerAfter.Running() {
		t.Fatal("chaos player still running after chat stop")
	}
}
