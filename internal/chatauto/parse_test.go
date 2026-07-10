package chatauto_test

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chatauto"
)

func TestParseResponse(t *testing.T) {
	raw := `{"autodom":{"humor":"tesao","posicao":"oral","intensidade":5,"duracao_segundos":20},"reply":"Vem mais perto."}`
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
	if response.AutoDom.DuracaoSegundos != 45 {
		t.Fatalf("duracao = %d", response.AutoDom.DuracaoSegundos)
	}
}
