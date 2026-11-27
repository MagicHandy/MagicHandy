package elevenlabs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// VoiceSettings override the voice's stored settings for one request. Nil
// fields keep the voice's own value.
type VoiceSettings struct {
	Stability       *float64 `json:"stability,omitempty"`
	SimilarityBoost *float64 `json:"similarity_boost,omitempty"`
	Style           *float64 `json:"style,omitempty"`
	UseSpeakerBoost *bool    `json:"use_speaker_boost,omitempty"`
	Speed           *float64 `json:"speed,omitempty"`
}

// Empty reports whether no override is set.
func (s *VoiceSettings) Empty() bool {
	return s == nil || (s.Stability == nil && s.SimilarityBoost == nil && s.Style == nil &&
		s.UseSpeakerBoost == nil && s.Speed == nil)
}

// SpeechRequest is one text-to-speech call.
type SpeechRequest struct {
	Text               string
	ModelID            string
	OutputFormat       string
	LanguageCode       string
	VoiceSettings      *VoiceSettings
	PreviousText       string
	NextText           string
	PreviousRequestIDs []string
	Seed               *uint32
}

// Speech is a streaming audio response. Close it when done.
type Speech struct {
	Body          io.ReadCloser
	ContentType   string
	RequestID     string
	CharacterCost int
}

// StreamSpeech starts a streaming synthesis. Transient failures (rate limits,
// server errors, connection failures) are retried before any audio arrives;
// once the body is returned, a failure ends the stream.
func (c *Client) StreamSpeech(ctx context.Context, voiceID string, request SpeechRequest) (*Speech, error) {
	if !ValidIdentifier(voiceID) {
		return nil, &APIError{Operation: OperationSpeak, kind: KindInvalid, Message: "the voice ID is empty or contains characters ElevenLabs does not use"}
	}
	if strings.TrimSpace(request.Text) == "" {
		return nil, &APIError{Operation: OperationSpeak, kind: KindInvalid, Message: "there is no text to speak"}
	}
	if request.ModelID != "" && !ValidIdentifier(request.ModelID) {
		return nil, &APIError{Operation: OperationSpeak, kind: KindInvalid, Message: "the model ID contains characters ElevenLabs does not use"}
	}
	payload := map[string]any{"text": request.Text}
	if request.ModelID != "" {
		payload["model_id"] = request.ModelID
	}
	if request.LanguageCode != "" {
		payload["language_code"] = request.LanguageCode
	}
	if !request.VoiceSettings.Empty() {
		payload["voice_settings"] = request.VoiceSettings
	}
	if request.PreviousText != "" {
		payload["previous_text"] = request.PreviousText
	}
	if request.NextText != "" {
		payload["next_text"] = request.NextText
	}
	if len(request.PreviousRequestIDs) > 0 {
		ids := request.PreviousRequestIDs
		if len(ids) > 3 {
			ids = ids[len(ids)-3:]
		}
		payload["previous_request_ids"] = ids
	}
	if request.Seed != nil {
		payload["seed"] = *request.Seed
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	if request.OutputFormat != "" {
		query.Set("output_format", request.OutputFormat)
	}
	result, err := c.send(ctx, OperationSpeak, http.MethodPost, "/v1/text-to-speech/"+url.PathEscape(voiceID)+"/stream", query, body, "audio/*")
	if err != nil {
		return nil, err
	}
	header := result.response.Header
	cost, _ := strconv.Atoi(strings.TrimSpace(header.Get("character-cost")))
	return &Speech{
		Body:          newIdleBody(result.response.Body, c.idleTimeout, result.cancel),
		ContentType:   header.Get("Content-Type"),
		RequestID:     boundedText(firstHeader(header, "request-id", "x-request-id"), 80),
		CharacterCost: cost,
	}, nil
}

// ModelProfile is what MagicHandy knows about how to shape a request for a
// model: which voice settings it honors, whether it takes a language code,
// and how much text one request may carry.
type ModelProfile struct {
	ID           string
	MaxTextRunes int
	Stability    bool
	Similarity   bool
	Style        bool
	SpeakerBoost bool
	Speed        bool
	LanguageCode bool
	AudioTags    bool
	// StabilityPreset marks models whose own app offers stability as three
	// presets (creative 0, natural 0.5, robust 1); a UI may show it that way.
	StabilityPreset bool
	// RequestStitching marks models whose continuity across split requests
	// is documented through previous_request_ids; others get previous_text.
	RequestStitching bool
}

// defaultMaxTextRunes keeps an unknown model's requests modest; MagicHandy's
// speech text is bounded well below the smallest documented model limit.
const defaultMaxTextRunes = 4000

// ProfileFor describes modelID. Documented behavior comes first; when the
// account's live model list is available it narrows style, speaker boost and
// the text limit.
func ProfileFor(modelID string, live *Model) ModelProfile {
	profile := ModelProfile{ID: modelID, MaxTextRunes: defaultMaxTextRunes, Stability: true, Similarity: true}
	switch {
	case strings.HasPrefix(modelID, "eleven_v4"):
		// Eleven v4 and v4 Turbo take Stability and Similarity only; Style,
		// Speed and SSML are not available.
		profile.LanguageCode, profile.AudioTags = true, true
	case strings.HasPrefix(modelID, "eleven_v3"):
		profile.LanguageCode, profile.AudioTags, profile.StabilityPreset = true, true, true
		profile.MaxTextRunes = 5000
	case modelID == "eleven_multilingual_v2":
		// language_code is not supported for Multilingual v2.
		profile.Style, profile.SpeakerBoost, profile.Speed = true, true, true
		profile.MaxTextRunes, profile.RequestStitching = 10000, true
	case strings.HasPrefix(modelID, "eleven_flash_v2") || strings.HasPrefix(modelID, "eleven_turbo_v2"):
		profile.SpeakerBoost, profile.Speed, profile.LanguageCode = true, true, strings.HasSuffix(modelID, "_5")
		profile.MaxTextRunes, profile.RequestStitching = 30000, true
		if strings.HasSuffix(modelID, "_5") {
			profile.MaxTextRunes = 40000
		}
	}
	if live != nil && live.ID == modelID {
		profile.Style = profile.Style && live.CanUseStyle
		profile.SpeakerBoost = profile.SpeakerBoost && live.CanUseSpeakerBoost
		if live.MaxTextLength > 0 && live.MaxTextLength < profile.MaxTextRunes {
			profile.MaxTextRunes = live.MaxTextLength
		}
	}
	return profile
}

// Shape removes settings the model does not honor and clamps the rest to the
// documented ranges, so a saved setting never turns into a rejected request.
func (p ModelProfile) Shape(settings *VoiceSettings) *VoiceSettings {
	if settings.Empty() {
		return nil
	}
	shaped := &VoiceSettings{}
	if p.Stability && settings.Stability != nil {
		// Eleven v3 presents stability as three presets in ElevenLabs' own
		// app, but the API accepts any value in range (verified 2026-11-25),
		// so a saved value is sent as is.
		value := clamp01(*settings.Stability)
		shaped.Stability = &value
	}
	if p.Similarity && settings.SimilarityBoost != nil {
		value := clamp01(*settings.SimilarityBoost)
		shaped.SimilarityBoost = &value
	}
	if p.Style && settings.Style != nil {
		value := clamp01(*settings.Style)
		shaped.Style = &value
	}
	if p.SpeakerBoost && settings.UseSpeakerBoost != nil {
		value := *settings.UseSpeakerBoost
		shaped.UseSpeakerBoost = &value
	}
	if p.Speed && settings.Speed != nil {
		value := math.Min(1.2, math.Max(0.7, *settings.Speed))
		shaped.Speed = &value
	}
	if shaped.Empty() {
		return nil
	}
	return shaped
}

func clamp01(value float64) float64 {
	if math.IsNaN(value) {
		return 0.5
	}
	return math.Min(1, math.Max(0, value))
}

// SplitForSpeech breaks text into pieces of at most maxRunes, preferring
// paragraph and sentence boundaries, then clause boundaries, then spaces. It
// does not split inside a [bracketed] audio tag unless the tag alone is longer
// than maxRunes. Most replies fit in one piece, which keeps their delivery
// continuous.
func SplitForSpeech(text string, maxRunes int) []string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if text == "" {
		return nil
	}
	if maxRunes <= 0 {
		maxRunes = defaultMaxTextRunes
	}
	var pieces []string
	for utf8.RuneCountInString(text) > maxRunes {
		cut := splitPoint(text, maxRunes)
		piece := strings.TrimSpace(text[:cut])
		if piece != "" {
			pieces = append(pieces, piece)
		}
		text = strings.TrimSpace(text[cut:])
	}
	if text != "" {
		pieces = append(pieces, text)
	}
	return pieces
}

// splitPoint returns a byte offset within the first maxRunes runes of text.
func splitPoint(text string, maxRunes int) int {
	limit := len(text)
	count := 0
	for offset := range text {
		if count == maxRunes {
			limit = offset
			break
		}
		count++
	}
	window := text[:limit]
	inTag := tagOpenAt(window)
	best := func(match func(previous, current rune) bool) int {
		position := -1
		depth := 0
		var previous rune
		for offset, current := range window {
			switch current {
			case '[':
				depth++
			case ']':
				if depth > 0 {
					depth--
				}
			}
			if depth == 0 && match(previous, current) {
				position = offset + utf8.RuneLen(current)
			}
			previous = current
		}
		return position
	}
	minimum := limit / 3
	for _, match := range []func(previous, current rune) bool{
		func(previous, current rune) bool { return current == '\n' && previous == '\n' },
		func(previous, current rune) bool { return unicode.IsSpace(current) && isSentenceEnd(previous) },
		func(previous, current rune) bool { return unicode.IsSpace(current) && isClauseEnd(previous) },
		func(_, current rune) bool { return unicode.IsSpace(current) },
	} {
		if position := best(match); position > minimum {
			return position
		}
	}
	if inTag > 0 {
		return inTag
	}
	return limit
}

// tagOpenAt returns the offset of an unclosed '[' in window, or -1.
func tagOpenAt(window string) int {
	open := -1
	for offset, r := range window {
		switch r {
		case '[':
			open = offset
		case ']':
			open = -1
		}
	}
	return open
}

func isSentenceEnd(r rune) bool {
	switch r {
	case '.', '!', '?', '…', '。', '！', '？':
		return true
	}
	return false
}

func isClauseEnd(r rune) bool {
	switch r {
	case ',', ';', ':', '—', '–', '、', '，', '；':
		return true
	}
	return false
}

// ErrNoAudio reports a stream that ended without any bytes.
var ErrNoAudio = errors.New("ElevenLabs returned no audio")
