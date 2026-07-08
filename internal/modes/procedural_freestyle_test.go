package modes

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestNextProceduralFreestyleSegmentMapsStyle(t *testing.T) {
	planner := NewPlanner(99)
	settings := styleSettings(config.MotionStyleIntense)

	segment := NextProceduralFreestyleSegment(planner, settings)
	if segment.Physics.Velocidade < 60 {
		t.Fatalf("intense velocidade = %d, want higher band", segment.Physics.Velocidade)
	}
	allowed := map[string]struct{}{
		"cabeca":      {},
		"meio_cabeca": {},
		"meio":        {},
	}
	if _, ok := allowed[segment.Physics.Regiao]; !ok {
		t.Fatalf("intense regiao = %q, want cabeca/meio_cabeca/meio", segment.Physics.Regiao)
	}
	if segment.Physics.TipoBatida != "alto" && segment.Physics.TipoBatida != "moderado" {
		t.Fatalf("intense tipo_batida = %q", segment.Physics.TipoBatida)
	}
	if segment.DurationMillis < minSegmentMillis {
		t.Fatalf("duration = %d, want >= %d", segment.DurationMillis, minSegmentMillis)
	}
}

func TestNextProceduralFreestyleSegmentGentleIsCalmer(t *testing.T) {
	intense := NextProceduralFreestyleSegment(NewPlanner(1), styleSettings(config.MotionStyleIntense))
	gentle := NextProceduralFreestyleSegment(NewPlanner(1), styleSettings(config.MotionStyleGentle))

	if gentle.Physics.Velocidade >= intense.Physics.Velocidade {
		t.Fatalf("gentle velocidade = %d, intense = %d, want gentle lower", gentle.Physics.Velocidade, intense.Physics.Velocidade)
	}
	allowed := map[string]struct{}{"meio": {}, "meio_cabeca": {}}
	if _, ok := allowed[gentle.Physics.Regiao]; !ok {
		t.Fatalf("gentle regiao = %q, want meio or meio_cabeca", gentle.Physics.Regiao)
	}
}
