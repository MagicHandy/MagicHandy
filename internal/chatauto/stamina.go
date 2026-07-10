package chatauto

// ApplyDrain reduces stamina from one played segment.
func ApplyDrain(stamina float64, intent Intent) float64 {
	cost := float64(intent.Intensidade) * float64(intent.DuracaoSegundos) * 0.08
	next := stamina - cost
	if next < 0 {
		return 0
	}
	return next
}

// ApplyRecover adds stamina after gentle segments.
func ApplyRecover(stamina float64, intent Intent) float64 {
	bonus := 0.0
	if intent.Humor == HumorDesejando {
		bonus += 4
	}
	if intent.Posicao == PoseOral && intent.Intensidade <= 3 {
		bonus += 6
	}
	next := stamina + bonus
	if next > 100 {
		return 100
	}
	return next
}
