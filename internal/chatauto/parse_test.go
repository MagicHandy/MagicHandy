package chatauto_test

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chatauto"
)

func TestParseResponse(t *testing.T) {
	raw := `{"autodom":{"humor":"tesao","posicao":"oral","intensidade_min":3,"intensidade_max":8},"reply":"Vem mais perto."}`
	response, err := chatauto.ParseResponse(raw)
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	if response.Reply != "Vem mais perto." {
		t.Fatalf("reply = %q", response.Reply)
	}
	if response.AutoDom.Humor != chatauto.HumorTesao {
		t.Fatalf("humor = %q", response.AutoDom.Humor)
	}
	if response.AutoDom.IntensidadeMin != 3 || response.AutoDom.IntensidadeMax != 8 {
		t.Fatalf("intensidade range = %d-%d", response.AutoDom.IntensidadeMin, response.AutoDom.IntensidadeMax)
	}
	if response.AutoDom.DuracaoSegundos != 60 {
		t.Fatalf("duracao = %d, want procedural max 60", response.AutoDom.DuracaoSegundos)
	}
}
