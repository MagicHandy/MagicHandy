package chatauto

// Humor values for the auto session scene.
type Humor string

const (
	HumorDesejando Humor = "desejando"
	HumorTesao     Humor = "tesao"
	HumorIntensa   Humor = "intensa"
	HumorDominatrix Humor = "dominatrix"
)

// Pose values for the auto session scene.
type Pose string

const (
	PoseHandjob     Pose = "handjob"
	PoseOral        Pose = "oral"
	PoseCavalgando  Pose = "cavalgando"
	PoseDeepthroat  Pose = "deepthroat"
)

// Intent is the AI roteiro for one autonomous scene beat.
type Intent struct {
	Humor            Humor `json:"humor"`
	Posicao          Pose  `json:"posicao"`
	Intensidade      int   `json:"intensidade"`
	IntensidadeMin   int   `json:"intensidade_min,omitempty"`
	IntensidadeMax   int   `json:"intensidade_max,omitempty"`
	Velocidade       int   `json:"velocidade,omitempty"`
	DuracaoSegundos  int   `json:"duracao_segundos,omitempty"`
}

// Response is the strict JSON shape for one auto turn.
type Response struct {
	Reply   string `json:"reply"`
	AutoDom Intent `json:"autodom"`
}

// MotionChoice is the procedural motion the auto session is playing.
type MotionChoice struct {
	Action      string `json:"action"`
	Velocidade  int    `json:"velocidade"`
	Intensidade int    `json:"intensidade"`
	Regiao      string `json:"regiao"`
	TipoBatida  string `json:"tipo_batida"`
	AtrasoMS    int    `json:"atraso_ms"`
}

// State is the live auto-session UI state.
type State struct {
	Active          bool         `json:"active"`
	Stamina         float64      `json:"stamina"`
	Humor           Humor        `json:"humor"`
	SpiceLevel      SpiceLevel   `json:"spice_level,omitempty"`
	Posicao         Pose         `json:"posicao"`
	SceneIntensidade int         `json:"scene_intensidade,omitempty"`
	MoodProgress    float64      `json:"mood_progress"`
	Motion          MotionChoice `json:"motion"`
	LastReply       string       `json:"last_reply,omitempty"`
	ReplyPartial    string  `json:"reply_partial,omitempty"`
	SegmentEndsInMS int64   `json:"segment_ends_in_ms,omitempty"`
	LLMBusy         bool    `json:"llm_busy"`
	Error           string  `json:"error,omitempty"`
}

// NewInitialState returns the default auto session state.
func NewInitialState() State {
	return State{
		Stamina: 100,
		Humor:   HumorDesejando,
		Posicao: PoseHandjob,
	}
}
