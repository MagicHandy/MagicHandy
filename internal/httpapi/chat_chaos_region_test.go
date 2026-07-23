package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestChatChaosRegionChangeRestartsPlayerWithCabecaEnvelope(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
	})
	t.Cleanup(server.Close)

	settings, _ := server.store.Snapshot()
	settings.Motion.MotionGenerationMode = config.MotionGenerationModeProcedural
	settings.Motion.HardwareSafetyLock = false

	ctx := context.Background()
	server.chatChaos.mu.Lock()
	server.chatChaos.generation = 1
	generation := server.chatChaos.generation
	server.chatChaos.mu.Unlock()

	meio := &chat.MotionCommand{
		Action:      chat.MotionActionStart,
		Velocidade:  90,
		Intensidade: 70,
		Regiao:      "meio",
		TipoBatida:  "very_fast",
	}
	if err := server.playChatChaoticMotion(ctx, meio, settings, generation); err != nil {
		t.Fatal(err)
	}
	waitForHSPPlay(t, fake, 2*time.Second)

	addsBefore := countHSPAdds(fake.Commands())
	cabeca := &chat.MotionCommand{
		Action:      chat.MotionActionTarget,
		Velocidade:  90,
		Intensidade: 70,
		Regiao:      "cabeca",
		TipoBatida:  "very_fast",
	}
	if err := server.playChatChaoticMotion(ctx, cabeca, settings, generation); err != nil {
		t.Fatal(err)
	}
	waitForHSPAdds(t, fake, addsBefore+1, 2*time.Second)

	batch := newestHSPAddPointsFromFake(fake, addsBefore)
	if len(batch) == 0 {
		t.Fatal("expected cabeca dispatch HSP batch")
	}
	minPos, maxPos := batch[0].PositionPercent, batch[0].PositionPercent
	for _, point := range batch {
		if point.PositionPercent < minPos {
			minPos = point.PositionPercent
		}
		if point.PositionPercent > maxPos {
			maxPos = point.PositionPercent
		}
	}
	if minPos < 65 || maxPos < 90 {
		t.Fatalf("cabeca dispatch batch %d..%d, want ~70..100", minPos, maxPos)
	}

	plays := countHSPPlays(fake.Commands())
	if plays < 2 {
		t.Fatalf("hsp_plays = %d, want region-change restart (>=2)", plays)
	}
}

func waitForHSPPlay(t *testing.T, fake *transport.Fake, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if countHSPPlays(fake.Commands()) > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for HSP play")
}

func waitForHSPAdds(t *testing.T, fake *transport.Fake, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if countHSPAdds(fake.Commands()) >= want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d HSP adds (got %d)", want, countHSPAdds(fake.Commands()))
}

func countHSPAdds(commands []transport.Command) int {
	count := 0
	for _, command := range commands {
		if command.Kind == transport.CommandKindHSPAdd {
			count++
		}
	}
	return count
}

func newestHSPAddPointsFromFake(fake *transport.Fake, skipAdds int) []transport.TimedPoint {
	commands := fake.Commands()
	var points []transport.TimedPoint
	seen := 0
	for _, command := range commands {
		if command.Kind != transport.CommandKindHSPAdd || command.HSPAdd == nil {
			continue
		}
		if seen < skipAdds {
			seen++
			continue
		}
		points = command.HSPAdd.Points
	}
	return points
}

func TestRegionChangeSessionCabecaVeryFastBounds(t *testing.T) {
	meio := buildChaosSessionForDurationFromPosition(
		motion.ChaoticPhysics{Regiao: "meio", TipoBatida: "very_fast", Velocidade: 90},
		config.DefaultSettings().Motion,
		false,
		nil,
		2_500,
		-1,
	)
	cabeca := buildChaosSessionForDurationFromPosition(
		motion.ChaoticPhysics{Regiao: "cabeca", TipoBatida: "very_fast", Velocidade: 90},
		config.DefaultSettings().Motion,
		false,
		nil,
		2_500,
		-1,
	)
	_ = meio
	minPos, maxPos := cabeca.Points[0].PositionPercent, cabeca.Points[0].PositionPercent
	for _, point := range cabeca.Points {
		if point.PositionPercent < minPos {
			minPos = point.PositionPercent
		}
		if point.PositionPercent > maxPos {
			maxPos = point.PositionPercent
		}
	}
	if minPos > 75 || maxPos < 90 {
		t.Fatalf("cabeca very_fast session %d..%d, want ~70..100", minPos, maxPos)
	}
}
