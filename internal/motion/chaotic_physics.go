package motion

// ChaoticPhysics is the procedural motion parameter set shared by chat and freestyle.
type ChaoticPhysics struct {
	Velocidade  int
	Intensidade int
	Regiao      string
	TipoBatida  string
	// AtrasoMS is the smoothing delay between curve points (ms). 160 = fluid mouse
	// feel, 1 = turbo/vibrate. Zero lets tipo_batida pick the default.
	AtrasoMS int
}
