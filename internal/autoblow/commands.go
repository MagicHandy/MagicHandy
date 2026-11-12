package autoblow

import (
	"context"
	"math"
	"net/http"
)

// UltraState is the Autoblow Ultra's reported state.
type UltraState struct {
	OperationalMode       string  `json:"operationalMode"`
	LocalScript           int     `json:"localScript"`
	LocalScriptSpeed      int     `json:"localScriptSpeed"`
	MotorTemperature      float64 `json:"motorTemperature"`
	OscillatorTargetSpeed float64 `json:"oscillatorTargetSpeed"`
	OscillatorLowPoint    float64 `json:"oscillatorLowPoint"`
	OscillatorHighPoint   float64 `json:"oscillatorHighPoint"`
	SyncScriptToken       string  `json:"syncScriptToken"`
}

// VacuGlideState is the VacuGlide's reported state.
type VacuGlideState struct {
	OperationalMode  string  `json:"operationalMode"`
	LocalScript      int     `json:"localScript"`
	TargetSpeed      float64 `json:"targetSpeed"`
	StrokePlusValve  bool    `json:"strokePlusValve"`
	StrokeMinusValve bool    `json:"strokeMinusValve"`
	SyncScriptToken  string  `json:"syncScriptToken"`
}

// Valves of the VacuGlide's cap. Opening one moves air into or out of the
// closed sleeve: stroke plus makes the stroke longer and tighter, stroke minus
// shorter and looser (ADR 0069 records how this was established).
const (
	ValveStrokePlus  = "stroke-plus"
	ValveStrokeMinus = "stroke-minus"
)

// Operational modes the owners act on.
const (
	ModeOnlineConnected    = "ONLINE_CONNECTED"
	ModeGoTo               = "GO_TO"
	ModeOscillatorPlaying  = "OSCILLATOR_PLAYING"
	ModeOscillatorPaused   = "OSCILLATOR_PAUSED"
	ModeTargetSpeedPlaying = "TARGET_SPEED_PLAYING"
	ModeTargetSpeedPaused  = "TARGET_SPEED_PAUSED"
)

// UltraState reads the Ultra's state.
func (c *Client) UltraState(ctx context.Context) (UltraState, error) {
	var state UltraState
	return state, c.device(ctx, http.MethodGet, "state", nil, &state)
}

// Goto moves the Ultra to a position at a speed, both percent. The cloud
// answers once it has sent the command, not when the move ends.
func (c *Client) Goto(ctx context.Context, position, speed float64) error {
	body := struct {
		Position int `json:"position"`
		Speed    int `json:"speed"`
	}{Position: percent(position, 0), Speed: percent(speed, 1)}
	return c.device(ctx, http.MethodPut, "goto", body, nil)
}

// Oscillate strokes between minY and maxY at a speed, all percent, at least
// 10 apart.
func (c *Client) Oscillate(ctx context.Context, speed, minY, maxY float64) (UltraState, error) {
	body := struct {
		Speed int `json:"speed"`
		MinY  int `json:"minY"`
		MaxY  int `json:"maxY"`
	}{Speed: percent(speed, 1), MinY: percent(minY, 0), MaxY: percent(maxY, 0)}
	var state UltraState
	return state, c.device(ctx, http.MethodPut, "oscillate", body, &state)
}

// StopUltra stops the Ultra whatever it is doing.
func (c *Client) StopUltra(ctx context.Context) (UltraState, error) {
	var state UltraState
	return state, c.device(ctx, http.MethodPut, "oscillate/stop", nil, &state)
}

// VacuGlideState reads the VacuGlide's state.
func (c *Client) VacuGlideState(ctx context.Context) (VacuGlideState, error) {
	var state VacuGlideState
	return state, c.device(ctx, http.MethodGet, "state", nil, &state)
}

// TargetSpeed runs the VacuGlide's motor at a constant speed, in percent.
func (c *Client) TargetSpeed(ctx context.Context, speed float64) (VacuGlideState, error) {
	body := struct {
		TargetSpeed int `json:"targetSpeed"`
	}{TargetSpeed: percent(speed, 0)}
	var state VacuGlideState
	return state, c.device(ctx, http.MethodPut, "target-speed", body, &state)
}

// StopVacuGlide stops the motor whatever is playing.
func (c *Client) StopVacuGlide(ctx context.Context) (VacuGlideState, error) {
	var state VacuGlideState
	return state, c.device(ctx, http.MethodPut, "target-speed/stop", nil, &state)
}

// Valve opens or closes one of the VacuGlide's stroke valves.
func (c *Client) Valve(ctx context.Context, valve string, open bool) (VacuGlideState, error) {
	body := struct {
		ValveState bool `json:"valveState"`
	}{ValveState: open}
	var state VacuGlideState
	return state, c.device(ctx, http.MethodPut, "valve/"+valve, body, &state)
}

// percent rounds to a whole percent within [minimum, 100].
func percent(value, minimum float64) int {
	if math.IsNaN(value) {
		return int(minimum)
	}
	return int(math.Round(math.Max(minimum, math.Min(100, value))))
}
