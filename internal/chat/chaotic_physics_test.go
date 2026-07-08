package chat

import "testing"

func TestNormalizeChaoticPhysicsDefaults(t *testing.T) {
	command := &MotionCommand{
		Velocidade:  0,
		Intensidade: 0,
		Regiao:      "  INVALIDO ",
		TipoBatida:  "foo",
	}
	NormalizeChaoticPhysics(command)

	if command.Regiao != defaultChaoticRegiao {
		t.Fatalf("regiao = %q", command.Regiao)
	}
	if command.TipoBatida != defaultChaoticTipoBatida {
		t.Fatalf("tipo_batida = %q", command.TipoBatida)
	}
	if command.Velocidade != defaultChaoticVelocity {
		t.Fatalf("velocidade = %d", command.Velocidade)
	}
	if command.Intensidade != defaultChaoticIntensity {
		t.Fatalf("intensidade = %d", command.Intensidade)
	}
}

func TestDecodeMotionIntensidadeInteger(t *testing.T) {
	physics, legacy, err := decodeMotionIntensidade([]byte(`60`))
	if err != nil {
		t.Fatalf("decode int: %v", err)
	}
	if physics != 60 || legacy != "" {
		t.Fatalf("physics=%d legacy=%q", physics, legacy)
	}
}

func TestDecodeMotionIntensidadeLegacyString(t *testing.T) {
	physics, legacy, err := decodeMotionIntensidade([]byte(`"media"`))
	if err != nil {
		t.Fatalf("decode string: %v", err)
	}
	if physics != 0 || legacy != "media" {
		t.Fatalf("physics=%d legacy=%q", physics, legacy)
	}
}

func TestNormalizeChaoticRegiaoAliases(t *testing.T) {
	cases := map[string]string{
		"cabeça": "cabeca",
		"HEAD":   "cabeca",
		"topo":   "cabeca",
		"shaft":  "meio",
		"fundo":  "base",
	}
	for input, want := range cases {
		if got := normalizeChaoticRegiao(input); got != want {
			t.Fatalf("%q => %q, want %q", input, got, want)
		}
	}
}
