package chat

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

var synsualCommandPattern = regexp.MustCompile(`(?i)\b([a-z][a-z0-9]*)\(\)`)

type synsualCommandSpec struct {
	Regiao      string
	TipoBatida  string
	Velocidade  int
	Intensidade int
	AtrasoMS    int
	Physical    string
	StrokeRange [2]float64
}

var synsualCommandCatalog = map[string]synsualCommandSpec{
	"bjtip":               {Regiao: "cabeca", TipoBatida: "leve", Velocidade: 28, Intensidade: 35, AtrasoMS: 220, Physical: "oral", StrokeRange: [2]float64{0.74, 0.98}},
	"bjdeepthroat":        {Regiao: "cabeca", TipoBatida: "fluido", Velocidade: 42, Intensidade: 55, AtrasoMS: 170, Physical: "deepthroat", StrokeRange: [2]float64{0.68, 0.96}},
	"bjfull":              {Regiao: "meio_cabeca", TipoBatida: "fluido", Velocidade: 48, Intensidade: 58, AtrasoMS: 150, Physical: "oral", StrokeRange: [2]float64{0.45, 0.95}},
	"bjfinisher":          {Regiao: "cabeca", TipoBatida: "turbo", Velocidade: 88, Intensidade: 92, AtrasoMS: 80, Physical: "deepthroat", StrokeRange: [2]float64{0.55, 0.98}},
	"doggymod":            {Regiao: "meio_cabeca", TipoBatida: "moderado", Velocidade: 52, Intensidade: 60, AtrasoMS: 140, Physical: "riding", StrokeRange: [2]float64{0.18, 0.88}},
	"doggyintense":        {Regiao: "meio_cabeca", TipoBatida: "alto", Velocidade: 72, Intensidade: 78, AtrasoMS: 110, Physical: "riding", StrokeRange: [2]float64{0.12, 0.92}},
	"doggyfinisher":       {Regiao: "full", TipoBatida: "turbo", Velocidade: 92, Intensidade: 95, AtrasoMS: 70, Physical: "riding", StrokeRange: [2]float64{0.08, 0.96}},
	"shallowmis":          {Regiao: "meio", TipoBatida: "lento", Velocidade: 32, Intensidade: 40, AtrasoMS: 240, Physical: "riding", StrokeRange: [2]float64{0.58, 0.76}},
	"slowmis":             {Regiao: "full", TipoBatida: "lento", Velocidade: 38, Intensidade: 48, AtrasoMS: 210, Physical: "riding", StrokeRange: [2]float64{0.15, 0.9}},
	"fastmis":             {Regiao: "full", TipoBatida: "moderado", Velocidade: 68, Intensidade: 72, AtrasoMS: 120, Physical: "riding", StrokeRange: [2]float64{0.1, 0.94}},
	"finishermis":         {Regiao: "full", TipoBatida: "turbo", Velocidade: 90, Intensidade: 94, AtrasoMS: 75, Physical: "riding", StrokeRange: [2]float64{0.08, 0.96}},
	"cowshallow":          {Regiao: "meio_cabeca", TipoBatida: "lento", Velocidade: 34, Intensidade: 42, AtrasoMS: 230, Physical: "riding", StrokeRange: [2]float64{0.62, 0.82}},
	"cowfast":             {Regiao: "meio_cabeca", TipoBatida: "fluido", Velocidade: 62, Intensidade: 68, AtrasoMS: 125, Physical: "riding", StrokeRange: [2]float64{0.22, 0.9}},
	"cowintense":          {Regiao: "meio_cabeca", TipoBatida: "alto", Velocidade: 78, Intensidade: 82, AtrasoMS: 100, Physical: "riding", StrokeRange: [2]float64{0.15, 0.92}},
	"cowfinisher":         {Regiao: "meio_cabeca", TipoBatida: "turbo", Velocidade: 94, Intensidade: 96, AtrasoMS: 65, Physical: "riding", StrokeRange: [2]float64{0.1, 0.95}},
	"hjtease":             {Regiao: "meio_cabeca", TipoBatida: "leve", Velocidade: 30, Intensidade: 38, AtrasoMS: 250, Physical: "handjob", StrokeRange: [2]float64{0.38, 0.78}},
	"hjedge":              {Regiao: "meio_cabeca", TipoBatida: "lento", Velocidade: 46, Intensidade: 62, AtrasoMS: 190, Physical: "handjob", StrokeRange: [2]float64{0.42, 0.86}},
	"hjtip":               {Regiao: "cabeca", TipoBatida: "leve", Velocidade: 40, Intensidade: 50, AtrasoMS: 160, Physical: "handjob", StrokeRange: [2]float64{0.72, 0.96}},
	"hjbase":              {Regiao: "meio_base", TipoBatida: "moderado", Velocidade: 50, Intensidade: 58, AtrasoMS: 145, Physical: "handjob", StrokeRange: [2]float64{0.05, 0.42}},
	"hjfullslow":          {Regiao: "meio_cabeca", TipoBatida: "lento", Velocidade: 36, Intensidade: 46, AtrasoMS: 215, Physical: "handjob", StrokeRange: [2]float64{0.2, 0.88}},
	"hjfullintense":       {Regiao: "meio_cabeca", TipoBatida: "alto", Velocidade: 74, Intensidade: 80, AtrasoMS: 105, Physical: "handjob", StrokeRange: [2]float64{0.15, 0.9}},
	"hjmlk":               {Regiao: "meio_cabeca", TipoBatida: "fluido", Velocidade: 56, Intensidade: 70, AtrasoMS: 130, Physical: "handjob", StrokeRange: [2]float64{0.18, 0.86}},
	"hjfinisher":          {Regiao: "meio_cabeca", TipoBatida: "turbo", Velocidade: 91, Intensidade: 93, AtrasoMS: 72, Physical: "handjob", StrokeRange: [2]float64{0.12, 0.92}},
	"brattytease":         {Regiao: "meio_cabeca", TipoBatida: "leve", Velocidade: 26, Intensidade: 34, AtrasoMS: 260, Physical: "tease", StrokeRange: [2]float64{0.5, 0.78}},
	"slowcowgirlride":     {Regiao: "meio_cabeca", TipoBatida: "lento", Velocidade: 40, Intensidade: 50, AtrasoMS: 200, Physical: "riding", StrokeRange: [2]float64{0.25, 0.85}},
	"challengedeepthroat": {Regiao: "cabeca", TipoBatida: "fluido", Velocidade: 55, Intensidade: 65, AtrasoMS: 155, Physical: "deepthroat", StrokeRange: [2]float64{0.65, 0.97}},
	"boundhandjob":        {Regiao: "meio_cabeca", TipoBatida: "moderado", Velocidade: 44, Intensidade: 52, AtrasoMS: 175, Physical: "handjob", StrokeRange: [2]float64{0.3, 0.8}},
	"brattypussygrip":     {Regiao: "meio_cabeca", TipoBatida: "alto", Velocidade: 64, Intensidade: 74, AtrasoMS: 115, Physical: "riding", StrokeRange: [2]float64{0.35, 0.72}},
	"submissivefacefuck":  {Regiao: "cabeca", TipoBatida: "alto", Velocidade: 76, Intensidade: 84, AtrasoMS: 95, Physical: "deepthroat", StrokeRange: [2]float64{0.6, 0.98}},
	"teasingblowjob":      {Regiao: "cabeca", TipoBatida: "lento", Velocidade: 32, Intensidade: 40, AtrasoMS: 225, Physical: "oral", StrokeRange: [2]float64{0.7, 0.95}},
}

// SynsualCommandInstructions is appended to Synsual-style personas in code.
const SynsualCommandInstructions = `
IMPORTANT: Include exactly ONE command at the end of each reply when the scene is sexual or playful-physical.
Use plain text for dialogue (no JSON). Put the command on its own line, e.g. bjtip()

Available commands:
bjtip(), bjdeepthroat(), bjfull(), bjfinisher(),
doggymod(), doggyintense(), doggyfinisher(),
shallowmis(), slowmis(), fastmis(), finishermis(),
cowshallow(), cowfast(), cowintense(), cowfinisher(),
hjtease(), hjedge(), hjtip(), hjbase(), hjfullslow(), hjfullintense(), hjmlk(), hjfinisher(),
brattyTease(), slowCowgirlRide(), challengeDeepThroat(), boundHandjob(),
brattyPussyGrip(), submissiveFaceFuck(), teasingBlowjob()

Match command intensity to the scene. Vary commands across turns. Pure greetings may omit a command.`

// ParseSynsualAssistantResponse converts free-text LLM output with command() suffix into AssistantResponse.
func ParseSynsualAssistantResponse(raw string) (AssistantResponse, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return AssistantResponse{}, fmt.Errorf("synsual response is empty")
	}

	commandName, reply, found := extractSynsualCommand(trimmed)
	if !found {
		return AssistantResponse{
			Reply:  trimmed,
			Motion: &MotionCommand{Action: MotionActionNone},
		}, nil
	}

	spec, ok := synsualCommandCatalog[strings.ToLower(commandName)]
	if !ok {
		return AssistantResponse{}, fmt.Errorf("unknown synsual command %q", commandName)
	}

	motion := synsualCommandToMotion(spec)
	if err := validateProceduralMotion(motion); err != nil {
		return AssistantResponse{}, err
	}

	return AssistantResponse{
		Reply:  strings.TrimSpace(reply),
		Motion: motion,
	}, nil
}

func synsualCommandToMotion(spec synsualCommandSpec) *MotionCommand {
	return &MotionCommand{
		Action:         MotionActionStart,
		PhysicalAction: spec.Physical,
		Velocidade:     spec.Velocidade,
		Intensidade:    spec.Intensidade,
		Regiao:         spec.Regiao,
		TipoBatida:     spec.TipoBatida,
		AtrasoMS:       spec.AtrasoMS,
		StrokeRange:    []float64{spec.StrokeRange[0], spec.StrokeRange[1]},
	}
}

func extractSynsualCommand(raw string) (command string, reply string, found bool) {
	matches := synsualCommandPattern.FindAllStringSubmatchIndex(raw, -1)
	if len(matches) == 0 {
		return "", raw, false
	}
	last := matches[len(matches)-1]
	command = raw[last[2]:last[3]]
	reply = strings.TrimSpace(raw[:last[0]] + raw[last[1]:])
	reply = strings.TrimSpace(strings.TrimRight(reply, "\n"))
	return command, reply, true
}

func IsSynsualMotionMode(mode string) bool {
	return normalizeMotionGenerationMode(mode) == config.MotionGenerationModeSynsual
}

// UsesProceduralMotionMode reports whether chat motion should flow through procedural physics.
func UsesProceduralMotionMode(mode string) bool {
	return config.UsesProceduralMotionGeneration(mode)
}
