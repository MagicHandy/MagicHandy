package elevenlabs

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Kind groups ElevenLabs failures by what the person can do about them. The
// API reports a typed detail object ({type, code, message, status,
// request_id, param}); older responses carry only the legacy status field, and
// a few endpoints answer with FastAPI's list-shaped validation detail. Kind is
// derived from the most specific signal available so every shape classifies.
type Kind string

const (
	// KindAuth means the key is missing, malformed, revoked, or wrong.
	KindAuth Kind = "auth"
	// KindPermission means a restricted key lacks the scope an endpoint needs.
	KindPermission Kind = "permission"
	// KindPlan means the account's plan does not include the voice, model or feature.
	KindPlan Kind = "plan"
	// KindQuota means the account has no credits left for this request.
	KindQuota Kind = "quota"
	// KindUnusualActivity is the free-tier abuse block (often a VPN or shared IP).
	KindUnusualActivity Kind = "unusual_activity"
	// KindRateLimited covers request-rate, concurrency and system-busy refusals.
	KindRateLimited Kind = "rate_limited"
	// KindNotFound means the voice, model or other resource does not exist.
	KindNotFound Kind = "not_found"
	// KindInvalid means ElevenLabs rejected the request as malformed.
	KindInvalid Kind = "invalid"
	// KindUnavailable means an ElevenLabs server failure or maintenance window.
	KindUnavailable Kind = "unavailable"
	// KindNetwork means no HTTP response arrived (DNS, TLS, reset, timeout).
	KindNetwork Kind = "network"
	// KindUnknown is any response the classifier does not recognize.
	KindUnknown Kind = "unknown"
)

// APIError is one failed ElevenLabs call. It never contains the API key: the
// key travels only in a request header, and the parsed fields come from the
// response body, which ElevenLabs does not echo credentials into.
type APIError struct {
	StatusCode int
	Type       string
	Code       string
	Status     string
	Message    string
	RequestID  string
	Param      string
	RetryAfter time.Duration
	// Operation names what was being attempted, for messages ("list voices").
	Operation string
	kind      Kind
	cause     error
}

func (e *APIError) Error() string {
	if e == nil {
		return "ElevenLabs request failed"
	}
	var b strings.Builder
	b.WriteString(e.UserMessage())
	detail := strings.TrimSpace(e.Message)
	if detail != "" && !strings.Contains(b.String(), detail) {
		b.WriteString(" ElevenLabs said: ")
		b.WriteString(strings.TrimSuffix(detail, "."))
		b.WriteString(".")
	}
	var facts []string
	if e.StatusCode != 0 {
		facts = append(facts, "HTTP "+strconv.Itoa(e.StatusCode))
	}
	if code := e.code(); code != "" {
		facts = append(facts, code)
	}
	if e.RequestID != "" {
		facts = append(facts, "request "+e.RequestID)
	}
	if len(facts) > 0 {
		b.WriteString(" (")
		b.WriteString(strings.Join(facts, ", "))
		b.WriteString(")")
	}
	return b.String()
}

// Unwrap exposes a transport-level cause such as a context cancellation.
func (e *APIError) Unwrap() error { return e.cause }

// Kind reports the failure class.
func (e *APIError) Kind() Kind {
	if e == nil {
		return KindUnknown
	}
	return e.kind
}

// Retryable reports whether the same request may succeed if sent again
// shortly. Quota, plan and permission failures need the person to act first.
func (e *APIError) Retryable() bool {
	switch e.Kind() {
	case KindRateLimited, KindUnavailable, KindNetwork:
		return true
	default:
		return false
	}
}

func (e *APIError) code() string {
	if e.Code != "" {
		return e.Code
	}
	return e.Status
}

// UserMessage is one actionable sentence for Settings and the voice log.
func (e *APIError) UserMessage() string {
	if e == nil {
		return "ElevenLabs request failed."
	}
	operation := e.Operation
	if operation == "" {
		operation = "this request"
	}
	switch e.kind {
	case KindAuth:
		return "ElevenLabs didn't accept the API key. Check or replace it in Settings → Voice."
	case KindPermission:
		if scope := permissionForOperation(e.Operation); scope != "" {
			return fmt.Sprintf("This ElevenLabs API key isn't allowed to %s. Give the key the %s permission in ElevenLabs, or use an unrestricted key.", operation, scope)
		}
		return fmt.Sprintf("This ElevenLabs API key isn't allowed to %s. Check the key's permissions in ElevenLabs.", operation)
	case KindPlan:
		if e.mentionsFreeUsers() {
			return "This voice isn't available to free ElevenLabs accounts through the API. Choose one of your own or a default voice, or upgrade the ElevenLabs plan."
		}
		return fmt.Sprintf("Your ElevenLabs plan doesn't include what's needed to %s.", operation)
	case KindQuota:
		return "Your ElevenLabs credits are used up for this billing period. Add credits or wait for the monthly reset."
	case KindUnusualActivity:
		return "ElevenLabs paused free-tier use from this connection because it detected unusual activity, which often comes from a VPN or a shared IP address. A paid plan or a different network avoids it."
	case KindRateLimited:
		return "ElevenLabs is busy or this account reached its limit of simultaneous requests. Try again in a moment."
	case KindNotFound:
		lowered := strings.ToLower(e.code() + " " + e.Message)
		switch {
		case strings.Contains(lowered, "voice"):
			return "ElevenLabs couldn't find the selected voice in this account. Choose a voice in Settings → Voice."
		case strings.Contains(lowered, "model"):
			return "ElevenLabs doesn't offer the selected model to this account. Choose another model in Settings → Voice."
		}
		return fmt.Sprintf("ElevenLabs couldn't find what was needed to %s.", operation)
	case KindInvalid:
		return fmt.Sprintf("ElevenLabs rejected the request to %s.", operation)
	case KindUnavailable:
		return "ElevenLabs is temporarily unavailable. Try again shortly."
	case KindNetwork:
		return "MagicHandy couldn't reach ElevenLabs. Check the internet connection."
	default:
		return fmt.Sprintf("ElevenLabs couldn't %s.", operation)
	}
}

func (e *APIError) mentionsFreeUsers() bool {
	lowered := strings.ToLower(e.code() + " " + e.Message)
	return strings.Contains(lowered, "free user") || strings.Contains(lowered, "free_user") ||
		strings.Contains(lowered, "free tier") || strings.Contains(lowered, "free-tier")
}

// permissionForOperation names the restricted-key permission an operation needs,
// using the labels ElevenLabs shows when a key is created.
func permissionForOperation(operation string) string {
	switch operation {
	case OperationSpeak:
		return "Text to Speech"
	case OperationListVoices, OperationGetVoice, OperationSearchLibrary:
		return "Voices (read)"
	case OperationAddVoice:
		return "Voices (write) and Add voice from Voice Library"
	case OperationListModels:
		return "Models (read)"
	case OperationSubscription:
		return "User (read)"
	default:
		return ""
	}
}

// Operation labels used in messages; they read naturally after "allowed to".
const (
	OperationSpeak         = "generate speech"
	OperationListVoices    = "list your voices"
	OperationGetVoice      = "look up the voice"
	OperationSearchLibrary = "search the Voice Library"
	OperationAddVoice      = "add a Voice Library voice to your account"
	OperationListModels    = "list models"
	OperationSubscription  = "read the plan and credit usage"
	OperationPreview       = "load a voice preview"
)

// IsKind reports whether err is an APIError of the given kind.
func IsKind(err error, kind Kind) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Kind() == kind
}

// AsAPIError extracts an APIError from err.
func AsAPIError(err error) (*APIError, bool) {
	var apiErr *APIError
	ok := errors.As(err, &apiErr)
	return apiErr, ok
}

// parseAPIError builds an APIError from a non-2xx response body.
func parseAPIError(operation string, statusCode int, header http.Header, body []byte) *APIError {
	apiErr := &APIError{StatusCode: statusCode, Operation: operation}
	var envelope struct {
		Detail json.RawMessage `json:"detail"`
	}
	if json.Unmarshal(body, &envelope) == nil && len(envelope.Detail) > 0 {
		var detail struct {
			Type      string `json:"type"`
			Code      string `json:"code"`
			Message   string `json:"message"`
			Status    string `json:"status"`
			RequestID string `json:"request_id"`
			Param     string `json:"param"`
		}
		var text string
		var list []struct {
			Msg string `json:"msg"`
		}
		switch {
		case json.Unmarshal(envelope.Detail, &detail) == nil:
			apiErr.Type, apiErr.Code, apiErr.Message = detail.Type, detail.Code, detail.Message
			apiErr.Status, apiErr.RequestID, apiErr.Param = detail.Status, detail.RequestID, detail.Param
		case json.Unmarshal(envelope.Detail, &text) == nil:
			apiErr.Message = text
		case json.Unmarshal(envelope.Detail, &list) == nil && len(list) > 0:
			messages := make([]string, 0, len(list))
			for _, item := range list {
				if strings.TrimSpace(item.Msg) != "" {
					messages = append(messages, strings.TrimSpace(item.Msg))
				}
			}
			apiErr.Message = strings.Join(messages, "; ")
		}
	}
	apiErr.Message = boundedText(apiErr.Message, 300)
	apiErr.RequestID = boundedText(apiErr.RequestID, 80)
	if apiErr.RequestID == "" {
		apiErr.RequestID = boundedText(firstHeader(header, "request-id", "x-request-id"), 80)
	}
	apiErr.RetryAfter = parseRetryAfter(header.Get("Retry-After"))
	apiErr.kind = classify(statusCode, apiErr.Type, apiErr.Code, apiErr.Status, apiErr.Message)
	return apiErr
}

func networkError(operation string, err error) *APIError {
	return &APIError{Operation: operation, kind: KindNetwork, cause: err}
}

// signalRules map code/status substrings to a Kind, most specific first: an
// unusual-activity block or a quota refusal outranks the HTTP status it
// arrives with.
var signalRules = []struct {
	kind    Kind
	signals []string
}{
	{KindUnusualActivity, []string{"detected_unusual_activity", "unusual_activity"}},
	{KindQuota, []string{"quota_exceeded", "insufficient_credits", "credits_exceeded", "character_limit_reached"}},
	{KindPermission, []string{"missing_permissions", "insufficient_permissions", "missing_permission"}},
	{KindPlan, []string{"subscription_required", "feature_not_available", "paid_plan_required", "free_users_not_allowed",
		"voice_not_allowed", "voice_access_denied", "model_access_denied", "upgrade_required"}},
	{KindAuth, []string{"invalid_api_key", "missing_api_key", "invalid_authorization_header", "not_logged_in",
		"sign_in_required", "unauthorized", "needs_authorization"}},
	{KindRateLimited, []string{"rate_limit_exceeded", "concurrent_limit_exceeded", "too_many_concurrent_requests", "system_busy"}},
	{KindNotFound, []string{"_not_found"}},
	{KindUnavailable, []string{"service_unavailable", "maintenance", "internal_error"}},
}

// typeKinds maps the typed error's type field.
var typeKinds = map[string]Kind{
	"authentication_error": KindAuth,
	"authorization_error":  KindPermission,
	"payment_required":     KindQuota,
	"rate_limit_error":     KindRateLimited,
	"not_found":            KindNotFound,
	"validation_error":     KindInvalid,
	"invalid_request":      KindInvalid,
	"internal_error":       KindUnavailable,
	"service_unavailable":  KindUnavailable,
}

// statusKinds is the last resort when the body carries no usable signal.
var statusKinds = map[int]Kind{
	http.StatusUnauthorized:        KindAuth,
	http.StatusPaymentRequired:     KindQuota,
	http.StatusForbidden:           KindPermission,
	http.StatusNotFound:            KindNotFound,
	http.StatusTooManyRequests:     KindRateLimited,
	http.StatusBadRequest:          KindInvalid,
	http.StatusUnprocessableEntity: KindInvalid,
}

// classify maps the API's signals onto Kind, most specific first.
func classify(statusCode int, errType, code, status, message string) Kind {
	signals := strings.ToLower(code + " " + status)
	lowered := strings.ToLower(message)
	if strings.Contains(lowered, "unusual activity") {
		return KindUnusualActivity
	}
	for _, rule := range signalRules {
		for _, signal := range rule.signals {
			if strings.Contains(signals, signal) {
				return rule.kind
			}
		}
	}
	// A refusal that names the plan is about the plan, whatever its type or
	// status says ("This voice is not available for free users").
	planWording := strings.Contains(lowered, "free user") || strings.Contains(lowered, "subscription") ||
		strings.Contains(lowered, "upgrade")
	if kind, ok := typeKinds[strings.ToLower(errType)]; ok {
		if planWording && (kind == KindPermission || kind == KindQuota) {
			return KindPlan
		}
		return kind
	}
	if kind, ok := statusKinds[statusCode]; ok {
		switch {
		case planWording && (kind == KindPermission || kind == KindQuota):
			return KindPlan
		case kind == KindAuth && strings.Contains(lowered, "permission"):
			return KindPermission
		}
		return kind
	}
	if statusCode >= 500 {
		return KindUnavailable
	}
	return KindUnknown
}

func parseRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		if wait := time.Until(when); wait > 0 {
			return wait
		}
	}
	return 0
}

func firstHeader(header http.Header, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(header.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func boundedText(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= limit {
		return value
	}
	cut := value[:limit]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + "…"
}
