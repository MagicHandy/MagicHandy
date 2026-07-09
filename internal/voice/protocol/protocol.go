// Package protocol defines the versioned voice worker wire format (ADR
// 0003): NDJSON frames exchanged between the core and a worker process over
// stdio. Workers depend only on this wire format — the package is a leaf so
// worker implementations (including the stub) never import core internals.
package protocol

import "fmt"

// Version is the worker protocol version this core speaks. Workers
// answering hello with a different version are rejected with a clear error,
// never silently degraded.
const Version = 1

// Role identifies which voice capability a worker provides.
type Role string

const (
	// RoleTTS is a text-to-speech worker.
	RoleTTS Role = "tts"
	// RoleASR is a speech-recognition worker.
	RoleASR Role = "asr"
)

// Roles lists the supported worker roles in display order.
func Roles() []Role {
	return []Role{RoleTTS, RoleASR}
}

// ParseRole validates a role string from the API edge.
func ParseRole(value string) (Role, error) {
	switch Role(value) {
	case RoleTTS, RoleASR:
		return Role(value), nil
	default:
		return "", fmt.Errorf("unknown voice worker role %q", value)
	}
}

// Request types sent core → worker. One JSON object per line on stdin.
const (
	RequestHello      = "hello"
	RequestHealth     = "health"
	RequestLoad       = "load"
	RequestUnload     = "unload"
	RequestSpeak      = "speak"
	RequestTranscribe = "transcribe"
	RequestCancel     = "cancel"
	RequestShutdown   = "shutdown"
)

// Response types sent worker → core. One JSON object per line on stdout.
const (
	ResponseHello      = "hello"
	ResponseHealth     = "health"
	ResponseAudioChunk = "audio_chunk"
	ResponseTranscript = "transcript"
	ResponseDone       = "done"
	ResponseCanceled   = "canceled"
	ResponseError      = "error"
)

// Structured worker error codes (ADR 0003: structured error payloads).
const (
	ErrorCodeProtocolMismatch  = "protocol_mismatch"
	ErrorCodeInvalidRequest    = "invalid_request"
	ErrorCodeModelNotLoaded    = "model_not_loaded"
	ErrorCodeMissingDependency = "missing_dependency"
	ErrorCodeCanceled          = "canceled"
	ErrorCodeTimeout           = "timeout"
	ErrorCodeInternal          = "internal"
)

// Transcript rejection reasons. A rejected transcript never enters chat.
const (
	RejectedNoSpeech      = "no_speech"
	RejectedLowConfidence = "low_confidence"
)

// Model states reported by health responses.
const (
	ModelStateUnloaded = "unloaded"
	ModelStateLoading  = "loading"
	ModelStateReady    = "ready"
)

// Request is the core → worker frame. Exactly one type per frame; unused
// fields stay empty. Workers must ignore unknown fields for forward
// compatibility; version negotiation happens once at hello.
type Request struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`

	// hello
	ProtocolVersion int `json:"protocol_version,omitempty"`

	// cancel
	TargetID string `json:"target_id,omitempty"`

	// speak
	Text  string `json:"text,omitempty"`
	Voice string `json:"voice,omitempty"`

	// transcribe; stubs and tests inline small payloads, real providers get a
	// file/stream reference instead of megabytes of base64 on one line
	AudioB64    string `json:"audio_b64,omitempty"`
	AudioRef    string `json:"audio_ref,omitempty"`
	AudioFormat string `json:"audio_format,omitempty"`

	// stub/testing knob: minimum processing time, used to exercise
	// cancellation and timeout paths without a real model
	DelayMillis int `json:"delay_ms,omitempty"`
}

// WorkerError is the structured error payload carried by error responses.
type WorkerError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable,omitempty"`
}

func (e *WorkerError) Error() string {
	if e == nil {
		return "voice worker error"
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// TranscriptCandidate is one ASR hypothesis with its confidence.
type TranscriptCandidate struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
}

// Response is the worker → core frame. RequestID links it to the request it
// answers; hello/health/done/canceled/error are terminal for that request,
// audio_chunk and transcript are results (transcript is terminal, audio
// chunks end with done).
type Response struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id,omitempty"`

	// hello
	ProtocolVersion int      `json:"protocol_version,omitempty"`
	Provider        string   `json:"provider,omitempty"`
	ProviderVersion string   `json:"provider_version,omitempty"`
	Role            Role     `json:"role,omitempty"`
	Capabilities    []string `json:"capabilities,omitempty"`

	// health
	ModelState string `json:"model_state,omitempty"`
	QueueDepth int    `json:"queue_depth,omitempty"`

	// audio_chunk
	Seq         int    `json:"seq,omitempty"`
	AudioB64    string `json:"audio_b64,omitempty"`
	AudioFormat string `json:"audio_format,omitempty"`

	// transcript
	Candidates []TranscriptCandidate `json:"candidates,omitempty"`
	Rejected   string                `json:"rejected,omitempty"`

	// error
	Error *WorkerError `json:"error,omitempty"`
}

// Terminal reports whether this response ends its request. Unary requests
// (hello, health, load, unload) are answered by hello/health frames;
// streaming requests end with done, canceled, error, or transcript.
func (r Response) Terminal() bool {
	switch r.Type {
	case ResponseHello, ResponseHealth, ResponseDone, ResponseCanceled, ResponseError, ResponseTranscript:
		return true
	default:
		return false
	}
}
