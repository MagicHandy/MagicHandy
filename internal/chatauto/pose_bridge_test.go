package chatauto

import "testing"

func TestApplyStaminaForBridgeUsesDrainOnly(t *testing.T) {
	intent := Intent{Humor: HumorTesao, Intensidade: 8, DuracaoSegundos: 10}
	drained := ApplyDrain(98, intent)
	afterBridge, _ := ApplyStaminaForBridge(98, PoseHandjob, intent)
	if afterBridge != drained {
		t.Fatalf("bridge = %v, drain = %v; bridge should match time-based drain", afterBridge, drained)
	}
}

func TestApplyRoteiroStaminaCommitPreservesStamina(t *testing.T) {
	intent := Intent{Humor: HumorTesao, Intensidade: 5, DuracaoSegundos: 100, Posicao: PoseHandjob}
	after, _ := ApplyRoteiroStaminaCommit(72, PoseHandjob, intent)
	if after != 72 {
		t.Fatalf("roteiro commit should not batch-drain, got %v", after)
	}
}
