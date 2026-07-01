package transport

import (
	"errors"
	"fmt"
	"strings"
)

const (
	cloudRESTName = "handy_cloud_rest"

	cloudPathStrokeWindow = "slider/stroke"
	cloudPathHSPSetup     = "hsp/setup"
	cloudPathHSPAdd       = "hsp/add"
	cloudPathHSPPlay      = "hsp/play"
	cloudPathHSPStop      = "hsp/stop"
	cloudPathHSPState     = "hsp/state"
	cloudPathHSPEvents    = "sse"
)

// CloudPrerequisites describes the Cloud REST prerequisites needed before live dispatch.
type CloudPrerequisites struct {
	ApplicationID string
	ConnectionKey string
	FirmwareMajor int
	APIMajor      int
	HSPAvailable  bool
}

// CloudBuildOptions controls physical transport-boundary mapping.
type CloudBuildOptions struct {
	ReverseDirection bool
}

// CloudRESTBuilder shapes HSP Cloud REST requests without sending them.
type CloudRESTBuilder struct {
	auth    CloudAuthMetadata
	options CloudBuildOptions
}

// CloudAuthMetadata is safe to serialize; the private connection key is omitted.
type CloudAuthMetadata struct {
	ApplicationID    string `json:"application_id"`
	ConnectionKey    string `json:"-"`
	ConnectionKeySet bool   `json:"connection_key_set"`
	FirmwareMajor    int    `json:"firmware_major"`
	APIMajor         int    `json:"api_major"`
}

// CloudRequest is a safe, deterministic request shape for Cloud REST dispatch.
type CloudRequest struct {
	Transport string            `json:"transport"`
	Operation string            `json:"operation"`
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Auth      CloudAuthMetadata `json:"auth"`
	Body      any               `json:"body,omitempty"`
}

// CloudHSPPoint is the Cloud REST HSP timed-point payload shape.
type CloudHSPPoint struct {
	X int   `json:"x"`
	T int64 `json:"t"`
}

// HSPUnavailableError reports failed v4/API v3/HSP prerequisites without fallback.
type HSPUnavailableError struct {
	Code       string `json:"code"`
	Field      string `json:"field,omitempty"`
	Message    string `json:"message"`
	Action     string `json:"action"`
	NoFallback bool   `json:"no_fallback"`
}

// Error returns the human-readable HSP prerequisite error.
func (e HSPUnavailableError) Error() string {
	return e.Message
}

// NewCloudRESTBuilder validates prerequisites and returns a pure request shaper.
func NewCloudRESTBuilder(prerequisites CloudPrerequisites, options CloudBuildOptions) (*CloudRESTBuilder, error) {
	auth, err := BuildCloudAuthMetadata(prerequisites)
	if err != nil {
		return nil, err
	}
	return &CloudRESTBuilder{
		auth:    auth,
		options: options,
	}, nil
}

// BuildCloudAuthMetadata validates firmware v4/API v3/HSP auth metadata.
func BuildCloudAuthMetadata(prerequisites CloudPrerequisites) (CloudAuthMetadata, error) {
	appID := strings.TrimSpace(prerequisites.ApplicationID)
	key := strings.TrimSpace(prerequisites.ConnectionKey)

	switch {
	case appID == "":
		return CloudAuthMetadata{}, hspUnavailable("missing_application_id", "application_id", "API v3 Application ID is required")
	case strings.ContainsAny(appID, " \t\r\n"):
		return CloudAuthMetadata{}, hspUnavailable("invalid_application_id", "application_id", "API v3 Application ID is malformed")
	case key == "":
		return CloudAuthMetadata{}, hspUnavailable("missing_connection_key", "connection_key", "Handy connection key is required")
	case strings.ContainsAny(key, " \t\r\n") || len(key) < 8:
		return CloudAuthMetadata{}, hspUnavailable("malformed_connection_key", "connection_key", "Handy connection key is malformed")
	case prerequisites.FirmwareMajor != 4:
		return CloudAuthMetadata{}, hspUnavailable("firmware_v4_required", "firmware_major", "Handy firmware v4 is required for HSP")
	case prerequisites.APIMajor != 3:
		return CloudAuthMetadata{}, hspUnavailable("api_v3_required", "api_major", "Handy API v3 is required for HSP")
	case !prerequisites.HSPAvailable:
		return CloudAuthMetadata{}, hspUnavailable("hsp_unavailable", "hsp", "HSP is unavailable for this device/API state")
	}

	return CloudAuthMetadata{
		ApplicationID:    appID,
		ConnectionKey:    key,
		ConnectionKeySet: true,
		FirmwareMajor:    prerequisites.FirmwareMajor,
		APIMajor:         prerequisites.APIMajor,
	}, nil
}

// BuildStrokeWindow shapes the HSP stroke-window Cloud REST request.
func (b *CloudRESTBuilder) BuildStrokeWindow(command StrokeWindowCommand) (CloudRequest, error) {
	if err := validateStrokeWindow(command); err != nil {
		return CloudRequest{}, err
	}

	return CloudRequest{
		Transport: cloudRESTName,
		Operation: string(CommandKindStrokeWindow),
		Method:    "PUT",
		Path:      cloudPathStrokeWindow,
		Auth:      b.auth,
		Body: cloudStrokeWindowBody{
			Min: percentFraction(command.MinPercent),
			Max: percentFraction(command.MaxPercent),
		},
	}, nil
}

// BuildHSPSetup shapes a Cloud REST request that prepares an HSP stream.
func (b *CloudRESTBuilder) BuildHSPSetup(command HSPSetupCommand) (CloudRequest, error) {
	if command.StreamID == 0 {
		return CloudRequest{}, errors.New("HSP setup stream ID must be positive")
	}
	return CloudRequest{
		Transport: cloudRESTName,
		Operation: string(CommandKindHSPSetup),
		Method:    "PUT",
		Path:      cloudPathHSPSetup,
		Auth:      b.auth,
		Body:      cloudHSPSetupBody(command),
	}, nil
}

// BuildHSPAdd shapes a Cloud REST request that appends HSP timed points.
func (b *CloudRESTBuilder) BuildHSPAdd(command HSPAddCommand) (CloudRequest, error) {
	return b.buildHSPAdd(command, true, len(command.Points))
}

func (b *CloudRESTBuilder) buildHSPAdd(command HSPAddCommand, flush bool, tailPointStreamIndex int) (CloudRequest, error) {
	streamID, err := cleanStreamID(command.StreamID)
	if err != nil {
		return CloudRequest{}, err
	}
	if len(command.Points) == 0 {
		return CloudRequest{}, errors.New("HSP add requires at least one point")
	}
	if tailPointStreamIndex < len(command.Points) {
		return CloudRequest{}, errors.New("HSP tail point stream index must cover the appended points")
	}

	points := make([]CloudHSPPoint, len(command.Points))
	for index, point := range command.Points {
		if point.PositionPercent < 0 || point.PositionPercent > 100 {
			return CloudRequest{}, fmt.Errorf("HSP point %d x must be between 0 and 100", index)
		}
		if point.TimeMillis < 0 {
			return CloudRequest{}, fmt.Errorf("HSP point %d t must be non-negative", index)
		}

		x := point.PositionPercent
		if b.options.ReverseDirection {
			x = 100 - x
		}
		points[index] = CloudHSPPoint{
			X: x,
			T: point.TimeMillis,
		}
	}

	return CloudRequest{
		Transport: cloudRESTName,
		Operation: string(CommandKindHSPAdd),
		Method:    "PUT",
		Path:      cloudPathHSPAdd,
		Auth:      b.auth,
		Body: cloudHSPAddBody{
			StreamID:             streamID,
			Points:               points,
			Flush:                flush,
			TailPointStreamIndex: tailPointStreamIndex,
		},
	}, nil
}

// BuildHSPPlay shapes a Cloud REST request that starts or resumes an HSP stream.
func (b *CloudRESTBuilder) BuildHSPPlay(command HSPPlayCommand) (CloudRequest, error) {
	streamID, err := cleanStreamID(command.StreamID)
	if err != nil {
		return CloudRequest{}, err
	}
	if command.StartTimeMillis < 0 {
		return CloudRequest{}, errors.New("HSP play start time must be non-negative")
	}

	return CloudRequest{
		Transport: cloudRESTName,
		Operation: string(CommandKindHSPPlay),
		Method:    "PUT",
		Path:      cloudPathHSPPlay,
		Auth:      b.auth,
		Body: cloudHSPPlayBody{
			StreamID:         streamID,
			StartTimeMillis:  command.StartTimeMillis,
			ServerTimeMillis: command.ServerTimeMillis,
			PlaybackRate:     1,
			PauseOnStarving:  true,
			Loop:             false,
		},
	}, nil
}

// BuildStop shapes a Cloud REST stop request without exposing free-form reasons.
func (b *CloudRESTBuilder) BuildStop() CloudRequest {
	return CloudRequest{
		Transport: cloudRESTName,
		Operation: string(CommandKindStop),
		Method:    "PUT",
		Path:      cloudPathHSPStop,
		Auth:      b.auth,
		Body:      cloudStopBody{},
	}
}

// BuildConnectionCheck shapes a Cloud REST request that checks HSP availability.
func (b *CloudRESTBuilder) BuildConnectionCheck() CloudRequest {
	return CloudRequest{
		Transport: cloudRESTName,
		Operation: string(CommandKindConnectionCheck),
		Method:    "GET",
		Path:      cloudPathHSPState,
		Auth:      b.auth,
	}
}

// BuildHSPState shapes a Cloud REST request that reads HSP state.
func (b *CloudRESTBuilder) BuildHSPState() CloudRequest {
	return CloudRequest{
		Transport: cloudRESTName,
		Operation: string(CommandKindHSPState),
		Method:    "GET",
		Path:      cloudPathHSPState,
		Auth:      b.auth,
	}
}

// BuildHSPEvents shapes a Cloud REST request that opens the HSP event stream.
func (b *CloudRESTBuilder) BuildHSPEvents() CloudRequest {
	return CloudRequest{
		Transport: cloudRESTName,
		Operation: string(CommandKindHSPEvents),
		Method:    "GET",
		Path:      cloudPathHSPEvents,
		Auth:      b.auth,
	}
}

type cloudStrokeWindowBody struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

type cloudHSPSetupBody struct {
	StreamID uint32 `json:"stream_id"`
}

type cloudHSPAddBody struct {
	StreamID             string          `json:"-"`
	Points               []CloudHSPPoint `json:"points"`
	Flush                bool            `json:"flush"`
	TailPointStreamIndex int             `json:"tail_point_stream_index"`
}

type cloudHSPPlayBody struct {
	StreamID         string  `json:"-"`
	StartTimeMillis  int64   `json:"start_time"`
	ServerTimeMillis int64   `json:"server_time,omitempty"`
	PlaybackRate     float64 `json:"playback_rate"`
	PauseOnStarving  bool    `json:"pause_on_starving"`
	Loop             bool    `json:"loop"`
}

type cloudStopBody struct{}

func percentFraction(value int) float64 {
	return float64(value) / 100
}

func validateStrokeWindow(command StrokeWindowCommand) error {
	if command.MinPercent < 0 || command.MaxPercent > 100 {
		return errors.New("stroke window must stay within 0..100")
	}
	if command.MinPercent >= command.MaxPercent {
		return errors.New("stroke window minimum must be lower than maximum")
	}
	return nil
}

func cleanStreamID(streamID string) (string, error) {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return "", errors.New("HSP stream ID is required")
	}
	if strings.ContainsAny(streamID, " \t\r\n") {
		return "", errors.New("HSP stream ID cannot contain whitespace")
	}
	return streamID, nil
}

func hspUnavailable(code string, field string, message string) HSPUnavailableError {
	return HSPUnavailableError{
		Code:       code,
		Field:      field,
		Message:    message,
		Action:     "Fix firmware v4/API v3/HSP settings; MagicHandy has no legacy fallback transport.",
		NoFallback: true,
	}
}
