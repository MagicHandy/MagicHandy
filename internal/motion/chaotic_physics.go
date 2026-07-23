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
	// StrokeRangeMin/Max are optional normalized 0..1 bounds from scene director.
	StrokeRangeMin float64
	StrokeRangeMax float64
	// Action carries director action for stroke profile selection (riding, deepthroat, …).
	Action string
	// StrokeProfile overrides action-based profile when explicitly set.
	StrokeProfile StrokeProfile
}
