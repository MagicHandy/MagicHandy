// Package voice implements the optional voice worker boundary (ADR 0003):
// worker process lifecycle, the core-owned request queue, and status. The
// wire format lives in the leaf package internal/voice/protocol; this
// package re-exports it so the HTTP edge deals with one voice import.
// Voice model runtimes are separate processes; a missing or failed worker
// never affects chat, settings, motion, transport, or diagnostics.
package voice

import "github.com/mapledaemon/MagicHandy/internal/voice/protocol"

// Wire types, re-exported for core-side consumers.
type (
	// Role identifies which voice capability a worker provides.
	Role = protocol.Role
	// Request is the core → worker frame.
	Request = protocol.Request
	// Response is the worker → core frame.
	Response = protocol.Response
	// WorkerError is the structured error payload carried by error responses.
	WorkerError = protocol.WorkerError
	// TranscriptCandidate is one ASR hypothesis with its confidence.
	TranscriptCandidate = protocol.TranscriptCandidate
)

// ProtocolVersion is the protocol version this core speaks.
const ProtocolVersion = protocol.Version

// Worker roles.
const (
	RoleTTS = protocol.RoleTTS
	RoleASR = protocol.RoleASR
)

// Request frame types.
const (
	RequestHello      = protocol.RequestHello
	RequestHealth     = protocol.RequestHealth
	RequestLoad       = protocol.RequestLoad
	RequestUnload     = protocol.RequestUnload
	RequestSpeak      = protocol.RequestSpeak
	RequestTranscribe = protocol.RequestTranscribe
	RequestCancel     = protocol.RequestCancel
	RequestShutdown   = protocol.RequestShutdown
)

// Response frame types.
const (
	ResponseHello      = protocol.ResponseHello
	ResponseHealth     = protocol.ResponseHealth
	ResponseAudioChunk = protocol.ResponseAudioChunk
	ResponseTranscript = protocol.ResponseTranscript
	ResponseDone       = protocol.ResponseDone
	ResponseCanceled   = protocol.ResponseCanceled
	ResponseError      = protocol.ResponseError
)

// Structured worker error codes.
const (
	ErrorCodeProtocolMismatch  = protocol.ErrorCodeProtocolMismatch
	ErrorCodeInvalidRequest    = protocol.ErrorCodeInvalidRequest
	ErrorCodeModelNotLoaded    = protocol.ErrorCodeModelNotLoaded
	ErrorCodeMissingDependency = protocol.ErrorCodeMissingDependency
	ErrorCodeCanceled          = protocol.ErrorCodeCanceled
	ErrorCodeTimeout           = protocol.ErrorCodeTimeout
	ErrorCodeInternal          = protocol.ErrorCodeInternal
)

// Transcript rejection reasons.
const (
	RejectedNoSpeech      = protocol.RejectedNoSpeech
	RejectedLowConfidence = protocol.RejectedLowConfidence
)

// Model states reported by health responses.
const (
	ModelStateUnloaded = protocol.ModelStateUnloaded
	ModelStateLoading  = protocol.ModelStateLoading
	ModelStateReady    = protocol.ModelStateReady
)

// Roles lists the supported worker roles in display order.
func Roles() []Role { return protocol.Roles() }

// ParseRole validates a role string from the API edge.
func ParseRole(value string) (Role, error) { return protocol.ParseRole(value) }
