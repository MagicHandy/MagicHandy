package elevenlabs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Model is one entry from GET /v1/models.
type Model struct {
	ID                  string     `json:"model_id"`
	Name                string     `json:"name"`
	Description         string     `json:"description"`
	CanDoTextToSpeech   bool       `json:"can_do_text_to_speech"`
	CanUseStyle         bool       `json:"can_use_style"`
	CanUseSpeakerBoost  bool       `json:"can_use_speaker_boost"`
	RequiresAlphaAccess bool       `json:"requires_alpha_access"`
	MaxTextLength       int        `json:"maximum_text_length_per_request"`
	TokenCostFactor     float64    `json:"token_cost_factor"`
	ConcurrencyGroup    string     `json:"concurrency_group"`
	Languages           []Language `json:"languages"`
	Rates               *struct {
		CharacterCostMultiplier float64 `json:"character_cost_multiplier"`
	} `json:"model_rates,omitempty"`
}

// Language is a model language.
type Language struct {
	ID   string `json:"language_id"`
	Name string `json:"name"`
}

// Models lists the models available to the key's workspace.
func (c *Client) Models(ctx context.Context) ([]Model, error) {
	var models []Model
	if err := c.doJSON(ctx, OperationListModels, http.MethodGet, "/v1/models", nil, nil, &models); err != nil {
		return nil, err
	}
	return models, nil
}

// Labels holds a voice's descriptive labels. Values are normally strings; any
// other JSON value is kept as its literal text instead of failing the decode.
type Labels map[string]string

// UnmarshalJSON tolerates non-string label values.
func (l *Labels) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	labels := make(Labels, len(raw))
	for key, value := range raw {
		var text string
		if json.Unmarshal(value, &text) != nil {
			text = strings.TrimSpace(string(value))
		}
		labels[key] = text
	}
	*l = labels
	return nil
}

// VerifiedLanguage records a language and model a voice was verified on.
type VerifiedLanguage struct {
	Language   string `json:"language"`
	ModelID    string `json:"model_id"`
	Accent     string `json:"accent,omitempty"`
	Locale     string `json:"locale,omitempty"`
	PreviewURL string `json:"preview_url,omitempty"`
}

// Voice is a voice available to the account (GET /v2/voices).
type Voice struct {
	ID                string             `json:"voice_id"`
	Name              string             `json:"name"`
	Category          string             `json:"category"`
	Description       string             `json:"description"`
	PreviewURL        string             `json:"preview_url"`
	Labels            Labels             `json:"labels"`
	IsLegacy          bool               `json:"is_legacy"`
	IsOwner           *bool              `json:"is_owner"`
	AvailableForTiers []string           `json:"available_for_tiers"`
	VerifiedLanguages []VerifiedLanguage `json:"verified_languages"`
	Settings          *VoiceSettings     `json:"settings"`
}

// VoiceQuery filters the account's voices.
type VoiceQuery struct {
	Search    string
	VoiceType string // personal, community, default, workspace, non-default, saved
	Category  string // premade, cloned, generated, professional
	PageSize  int
	PageToken string
	Sort      string // name or created_at_unix
}

// VoicePage is one page of account voices.
type VoicePage struct {
	Voices        []Voice `json:"voices"`
	HasMore       bool    `json:"has_more"`
	NextPageToken string  `json:"next_page_token"`
	TotalCount    int     `json:"total_count"`
}

// Voices lists voices the account can use, including the default voices.
func (c *Client) Voices(ctx context.Context, query VoiceQuery) (VoicePage, error) {
	values := url.Values{}
	setText(values, "search", query.Search, 100)
	if oneOf(query.VoiceType, "personal", "community", "default", "workspace", "non-default", "non-community", "saved") {
		values.Set("voice_type", query.VoiceType)
	}
	if oneOf(query.Category, "premade", "cloned", "generated", "professional") {
		values.Set("category", query.Category)
	}
	if oneOf(query.Sort, "name", "created_at_unix") {
		values.Set("sort", query.Sort)
		if query.Sort == "name" {
			values.Set("sort_direction", "asc")
		}
	}
	values.Set("page_size", strconv.Itoa(clampPageSize(query.PageSize, 30)))
	setText(values, "next_page_token", query.PageToken, 512)
	values.Set("include_total_count", "false")
	var page VoicePage
	if err := c.doJSON(ctx, OperationListVoices, http.MethodGet, "/v2/voices", values, nil, &page); err != nil {
		return VoicePage{}, err
	}
	return page, nil
}

// Voice looks up one voice by ID.
func (c *Client) Voice(ctx context.Context, voiceID string) (Voice, error) {
	if !ValidIdentifier(voiceID) {
		return Voice{}, errors.New("the ElevenLabs voice ID is not valid")
	}
	var voice Voice
	if err := c.doJSON(ctx, OperationGetVoice, http.MethodGet, "/v1/voices/"+url.PathEscape(voiceID), nil, nil, &voice); err != nil {
		return Voice{}, err
	}
	return voice, nil
}

// LibraryVoice is a shared voice from the Voice Library.
type LibraryVoice struct {
	PublicOwnerID         string             `json:"public_owner_id"`
	ID                    string             `json:"voice_id"`
	Name                  string             `json:"name"`
	Description           string             `json:"description"`
	Accent                string             `json:"accent"`
	Gender                string             `json:"gender"`
	Age                   string             `json:"age"`
	Descriptive           string             `json:"descriptive"`
	UseCase               string             `json:"use_case"`
	Category              string             `json:"category"`
	Language              string             `json:"language"`
	Locale                string             `json:"locale"`
	PreviewURL            string             `json:"preview_url"`
	FreeUsersAllowed      bool               `json:"free_users_allowed"`
	LiveModerationEnabled bool               `json:"live_moderation_enabled"`
	Featured              bool               `json:"featured"`
	Rate                  *float64           `json:"rate"`
	NoticePeriod          *int               `json:"notice_period"`
	ClonedByCount         int64              `json:"cloned_by_count"`
	UsageCharacters1y     int64              `json:"usage_character_count_1y"`
	IsAddedByUser         *bool              `json:"is_added_by_user"`
	VerifiedLanguages     []VerifiedLanguage `json:"verified_languages"`
}

// LibraryQuery filters the Voice Library.
type LibraryQuery struct {
	Search       string
	Category     string // professional, famous, high_quality
	Gender       string
	Age          string
	Accent       string
	Language     string
	UseCases     string
	Descriptives string
	Sort         string // created_date, usage_character_count_1y, trending, cloned_by_count
	Featured     bool
	// IncludeLiveModerated defaults to false: live-moderated voices check
	// every request's text against the owner's prohibited categories and add
	// latency, which suits few companion-style conversations.
	IncludeLiveModerated bool
	// IncludeCustomRates defaults to false: those voices charge a credit
	// multiplier, which would make every spoken reply cost more.
	IncludeCustomRates bool
	Page               int
	PageSize           int
}

// LibraryPage is one page of Voice Library results.
type LibraryPage struct {
	Voices  []LibraryVoice `json:"voices"`
	HasMore bool           `json:"has_more"`
}

// SharedVoices searches the Voice Library.
func (c *Client) SharedVoices(ctx context.Context, query LibraryQuery) (LibraryPage, error) {
	values := url.Values{}
	setText(values, "search", query.Search, 100)
	if oneOf(query.Category, "professional", "famous", "high_quality") {
		values.Set("category", query.Category)
	}
	setText(values, "gender", query.Gender, 40)
	setText(values, "age", query.Age, 40)
	setText(values, "accent", query.Accent, 60)
	setText(values, "language", query.Language, 20)
	setText(values, "use_cases", query.UseCases, 60)
	setText(values, "descriptives", query.Descriptives, 60)
	if oneOf(query.Sort, "created_date", "usage_character_count_1y", "trending", "cloned_by_count") {
		values.Set("sort", query.Sort)
	}
	if query.Featured {
		values.Set("featured", "true")
	}
	values.Set("include_live_moderated", strconv.FormatBool(query.IncludeLiveModerated))
	values.Set("include_custom_rates", strconv.FormatBool(query.IncludeCustomRates))
	values.Set("page_size", strconv.Itoa(clampPageSize(query.PageSize, 30)))
	if query.Page > 0 {
		values.Set("page", strconv.Itoa(min(query.Page, 1000)))
	}
	var page LibraryPage
	if err := c.doJSON(ctx, OperationSearchLibrary, http.MethodGet, "/v1/shared-voices", values, nil, &page); err != nil {
		return LibraryPage{}, err
	}
	return page, nil
}

// AddSharedVoice saves a Voice Library voice to the account and returns the
// voice ID to synthesize with.
func (c *Client) AddSharedVoice(ctx context.Context, publicOwnerID, voiceID, name string) (string, error) {
	if !ValidIdentifier(publicOwnerID) || !ValidIdentifier(voiceID) {
		return "", errors.New("the Voice Library voice reference is not valid")
	}
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		name = voiceID
	}
	if len([]rune(name)) > 100 {
		name = string([]rune(name)[:100])
	}
	body, err := json.Marshal(map[string]any{"new_name": name})
	if err != nil {
		return "", err
	}
	var result struct {
		VoiceID string `json:"voice_id"`
	}
	path := fmt.Sprintf("/v1/voices/add/%s/%s", url.PathEscape(publicOwnerID), url.PathEscape(voiceID))
	if err := c.doJSON(ctx, OperationAddVoice, http.MethodPost, path, nil, body, &result); err != nil {
		return "", err
	}
	if !ValidIdentifier(result.VoiceID) {
		return "", &APIError{Operation: OperationAddVoice, kind: KindUnknown, Message: "the response did not include a voice ID"}
	}
	return result.VoiceID, nil
}

// Subscription is the account's plan and credit usage.
type Subscription struct {
	Tier                        string `json:"tier"`
	Status                      string `json:"status"`
	CharacterCount              int64  `json:"character_count"`
	CharacterLimit              int64  `json:"character_limit"`
	NextCharacterCountResetUnix *int64 `json:"next_character_count_reset_unix"`
	VoiceSlotsUsed              int    `json:"voice_slots_used"`
	VoiceLimit                  int    `json:"voice_limit"`
	CanExtendCharacterLimit     bool   `json:"can_extend_character_limit"`
	CanUseInstantVoiceCloning   bool   `json:"can_use_instant_voice_cloning"`
}

// FreeTier reports whether the subscription is the free plan.
func (s Subscription) FreeTier() bool { return strings.EqualFold(strings.TrimSpace(s.Tier), "free") }

// Subscription reads the plan and credit usage.
func (c *Client) Subscription(ctx context.Context) (Subscription, error) {
	var subscription Subscription
	if err := c.doJSON(ctx, OperationSubscription, http.MethodGet, "/v1/user/subscription", nil, nil, &subscription); err != nil {
		return Subscription{}, err
	}
	return subscription, nil
}

func setText(values url.Values, key, value string, limit int) {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return
	}
	if len([]rune(value)) > limit {
		value = string([]rune(value)[:limit])
	}
	values.Set(key, value)
}

func clampPageSize(size, fallback int) int {
	if size <= 0 {
		return fallback
	}
	return min(size, 100)
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
