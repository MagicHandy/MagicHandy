package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/voice/elevenlabs"
)

// The ElevenLabs catalog endpoints back the Settings voice picker: account
// and key status, the account's voices, Voice Library search, adding a
// library voice, the speech models, and voice previews. They use the saved
// key (or, for the account check, a key typed into the form but not saved
// yet), never return it, and are limited to clients that may configure the
// host. Synthesis itself stays in the worker process (ADR 0003).

const (
	elevenLabsUpstreamSlots = 2
	elevenLabsModelsTTL     = time.Hour
	elevenLabsVoicesTTL     = 2 * time.Minute
	elevenLabsPreviewBytes  = 4 << 20
	maxElevenLabsKeyBytes   = 256
)

type elevenLabsRuntime struct {
	once    sync.Once
	baseURL string // tests point this at a fake before first use
	slots   chan struct{}
	client  *http.Client
	preview *http.Client

	mu     sync.Mutex
	models map[string]elevenLabsCachedModels
	voices map[string]elevenLabsCachedVoices
}

type elevenLabsCachedModels struct {
	at     time.Time
	models []elevenlabs.Model
}

type elevenLabsCachedVoices struct {
	at   time.Time
	page elevenlabs.VoicePage
}

func (s *Server) elevenLabsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/voice/elevenlabs/account", s.handleElevenLabsAccount)
	mux.HandleFunc("POST /api/voice/elevenlabs/account", s.handleElevenLabsAccount)
	mux.HandleFunc("GET /api/voice/elevenlabs/models", s.handleElevenLabsModels)
	mux.HandleFunc("GET /api/voice/elevenlabs/voices", s.handleElevenLabsVoices)
	mux.HandleFunc("GET /api/voice/elevenlabs/library", s.handleElevenLabsLibrary)
	mux.HandleFunc("POST /api/voice/elevenlabs/library/add", s.handleElevenLabsLibraryAdd)
	mux.HandleFunc("GET /api/voice/elevenlabs/preview", s.handleElevenLabsPreview)
}

func (s *Server) elevenLabsRuntime() *elevenLabsRuntime {
	rt := &s.elevenLabs
	rt.once.Do(func() {
		if rt.baseURL == "" {
			rt.baseURL = elevenlabs.DefaultBaseURL
		}
		rt.slots = make(chan struct{}, elevenLabsUpstreamSlots)
		rt.client = elevenlabs.NewHTTPClient()
		rt.preview = elevenlabs.NewHTTPClient()
		rt.models = map[string]elevenLabsCachedModels{}
		rt.voices = map[string]elevenLabsCachedVoices{}
	})
	return rt
}

// acquire bounds concurrent upstream calls so the picker cannot exhaust the
// account's concurrency limit while the worker is speaking.
func (rt *elevenLabsRuntime) acquire(ctx context.Context) (func(), error) {
	select {
	case rt.slots <- struct{}{}:
		return func() { <-rt.slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (rt *elevenLabsRuntime) newClient(key string) (*elevenlabs.Client, error) {
	return elevenlabs.New(elevenlabs.Options{BaseURL: rt.baseURL, APIKey: key, HTTPClient: rt.client})
}

// keyFingerprint names a key in memory-only caches without retaining it.
func keyFingerprint(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:8])
}

// elevenLabsClient authorizes the request and builds a client for the saved
// key. It writes the response and returns false when the caller must stop.
func (s *Server) elevenLabsClient(w http.ResponseWriter, r *http.Request) (*elevenlabs.Client, string, bool) {
	if !s.capabilities(r).ConfigureHost {
		writeError(w, http.StatusForbidden, errors.New("administrator access is required"))
		return nil, "", false
	}
	settings, _ := s.store.Snapshot()
	key := settings.Voice.ElevenLabsAPIKey
	if key == "" {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "Add an ElevenLabs API key in Settings → Voice first.",
			"kind":  "no_key",
		})
		return nil, "", false
	}
	client, err := s.elevenLabsRuntime().newClient(key)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return nil, "", false
	}
	return client, keyFingerprint(key), true
}

func writeElevenLabsError(w http.ResponseWriter, r *http.Request, err error) {
	if r.Context().Err() != nil {
		return
	}
	if apiErr, ok := elevenlabs.AsAPIError(err); ok {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error":   apiErr.Error(),
			"kind":    apiErr.Kind(),
			"message": apiErr.UserMessage(),
		})
		return
	}
	writeError(w, http.StatusBadGateway, err)
}

type elevenLabsAccountRequest struct {
	APIKey  string `json:"api_key"`
	VoiceID string `json:"voice_id"`
}

type elevenLabsProbe struct {
	State   string `json:"state"` // ok, denied, failed
	Kind    string `json:"kind,omitempty"`
	Message string `json:"message,omitempty"`
}

func probeOf(err error) elevenLabsProbe {
	if err == nil {
		return elevenLabsProbe{State: "ok"}
	}
	apiErr, ok := elevenlabs.AsAPIError(err)
	if !ok {
		return elevenLabsProbe{State: "failed", Message: "ElevenLabs request failed."}
	}
	if apiErr.Kind() == elevenlabs.KindPermission {
		return elevenLabsProbe{State: "denied", Kind: string(apiErr.Kind()), Message: apiErr.UserMessage()}
	}
	return elevenLabsProbe{State: "failed", Kind: string(apiErr.Kind()), Message: apiErr.UserMessage()}
}

// handleElevenLabsAccount reports whether the key works, what it may do, the
// plan and credit usage, and whether the selected voice is in the account.
// It spends no credits. POST accepts a key typed into the form but not saved
// yet, plus the form's voice ID; neither is stored.
func (s *Server) handleElevenLabsAccount(w http.ResponseWriter, r *http.Request) {
	if !s.capabilities(r).ConfigureHost {
		writeError(w, http.StatusForbidden, errors.New("administrator access is required"))
		return
	}
	settings, _ := s.store.Snapshot()
	key, voiceID, err := elevenLabsAccountTarget(r, settings.Voice.ElevenLabsAPIKey, settings.Voice.ElevenLabsVoiceID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if key == "" {
		writeJSON(w, http.StatusOK, map[string]any{"key_set": false, "key_status": "missing"})
		return
	}
	rt := s.elevenLabsRuntime()
	client, err := rt.newClient(key)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	probes := rt.probeAccount(r.Context(), client, keyFingerprint(key), voiceID)
	if r.Context().Err() != nil {
		return
	}
	writeJSON(w, http.StatusOK, probes.response(voiceID, settings.Voice.ElevenLabsModelID))
}

// elevenLabsAccountTarget returns the key and voice to check: the saved ones,
// or for POST the ones typed into the form.
func elevenLabsAccountTarget(r *http.Request, savedKey, savedVoice string) (string, string, error) {
	if r.Method != http.MethodPost {
		return savedKey, savedVoice, nil
	}
	var body elevenLabsAccountRequest
	if err := decodeJSON(r, &body); err != nil {
		return "", "", err
	}
	key, voiceID := savedKey, savedVoice
	if typed := strings.TrimSpace(body.APIKey); typed != "" {
		if len(typed) > maxElevenLabsKeyBytes || strings.ContainsAny(typed, " \t\r\n") {
			return "", "", errors.New("that doesn't look like an ElevenLabs API key")
		}
		key = typed
	}
	if typed := strings.TrimSpace(body.VoiceID); typed != "" {
		voiceID = typed
	}
	return key, voiceID, nil
}

type elevenLabsAccountProbes struct {
	subscription elevenlabs.Subscription
	subErr       error
	models       []elevenlabs.Model
	modelsErr    error
	voice        elevenlabs.Voice
	voiceErr     error
}

// probeAccount runs the three read-only checks concurrently, inside the
// upstream slots.
func (rt *elevenLabsRuntime) probeAccount(ctx context.Context, client *elevenlabs.Client, fingerprint, voiceID string) elevenLabsAccountProbes {
	var probes elevenLabsAccountProbes
	var wg sync.WaitGroup
	run := func(call func(context.Context)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := rt.acquire(ctx)
			if err != nil {
				return
			}
			defer release()
			call(ctx)
		}()
	}
	run(func(ctx context.Context) { probes.subscription, probes.subErr = client.Subscription(ctx) })
	run(func(ctx context.Context) { probes.models, probes.modelsErr = rt.cachedModels(ctx, client, fingerprint) })
	run(func(ctx context.Context) {
		if elevenlabs.ValidIdentifier(voiceID) {
			probes.voice, probes.voiceErr = client.Voice(ctx, voiceID)
			return
		}
		_, probes.voiceErr = client.Voices(ctx, elevenlabs.VoiceQuery{PageSize: 1})
	})
	wg.Wait()
	return probes
}

func (p elevenLabsAccountProbes) response(voiceID, modelID string) map[string]any {
	permissions := map[string]elevenLabsProbe{
		"user_read":   probeOf(p.subErr),
		"models_read": probeOf(p.modelsErr),
		"voices_read": probeOf(p.voiceErr),
	}
	if elevenlabs.IsKind(p.voiceErr, elevenlabs.KindNotFound) {
		permissions["voices_read"] = elevenLabsProbe{State: "ok"}
	}
	status, message := elevenLabsKeyStatus(p.subErr, p.modelsErr, p.voiceErr)
	response := map[string]any{
		"key_set":     true,
		"key_status":  status,
		"permissions": permissions,
		"checked_at":  time.Now().UTC().Format(time.RFC3339),
	}
	if message != "" {
		response["message"] = message
	}
	if p.subErr == nil {
		response["subscription"] = map[string]any{
			"tier":            p.subscription.Tier,
			"status":          p.subscription.Status,
			"character_count": p.subscription.CharacterCount,
			"character_limit": p.subscription.CharacterLimit,
			"next_reset_unix": p.subscription.NextCharacterCountResetUnix,
			"voice_slots":     p.subscription.VoiceSlotsUsed,
			"voice_limit":     p.subscription.VoiceLimit,
		}
		response["library_via_api"] = !p.subscription.FreeTier()
	}
	if elevenlabs.ValidIdentifier(voiceID) {
		if p.voiceErr == nil {
			response["selected_voice"] = p.voice
		} else if elevenlabs.IsKind(p.voiceErr, elevenlabs.KindNotFound) {
			response["selected_voice_problem"] = "This voice isn't in your ElevenLabs account. Choose a voice below."
		}
	}
	if _, ok := speechModel(p.models, modelID); p.modelsErr == nil && !ok && modelID != "" {
		response["selected_model_problem"] = fmt.Sprintf("ElevenLabs doesn't offer %s to this account. Choose another model.", modelID)
	}
	return response
}

// elevenLabsKeyStatus summarizes the probes. A key restricted to speech
// fails every catalog probe with a permission error; that key may still
// speak, so it reads "restricted", not "invalid".
func elevenLabsKeyStatus(errs ...error) (string, string) {
	allDenied, anyOK := true, false
	var first *elevenlabs.APIError
	for _, err := range errs {
		if err == nil {
			anyOK = true
			allDenied = false
			continue
		}
		apiErr, ok := elevenlabs.AsAPIError(err)
		if !ok {
			allDenied = false
			continue
		}
		switch apiErr.Kind() {
		case elevenlabs.KindAuth:
			return "invalid", apiErr.UserMessage()
		case elevenlabs.KindUnusualActivity:
			return "unusual_activity", apiErr.UserMessage()
		case elevenlabs.KindPermission:
		case elevenlabs.KindNotFound:
			anyOK = true
			allDenied = false
			continue
		default:
			allDenied = false
		}
		if first == nil {
			first = apiErr
		}
	}
	switch {
	case anyOK:
		return "ok", ""
	case allDenied:
		return "restricted", "This key can't read the account, models or voices. Speech may still work; give the key read permissions to browse voices here."
	case first != nil && first.Kind() == elevenlabs.KindNetwork:
		return "unreachable", first.UserMessage()
	case first != nil:
		return "error", first.UserMessage()
	default:
		return "error", "ElevenLabs could not be checked."
	}
}

func (rt *elevenLabsRuntime) cachedModels(ctx context.Context, client *elevenlabs.Client, fingerprint string) ([]elevenlabs.Model, error) {
	rt.mu.Lock()
	cached, ok := rt.models[fingerprint]
	rt.mu.Unlock()
	if ok && time.Since(cached.at) < elevenLabsModelsTTL {
		return cached.models, nil
	}
	models, err := client.Models(ctx)
	if err != nil {
		return nil, err
	}
	rt.mu.Lock()
	rt.models[fingerprint] = elevenLabsCachedModels{at: time.Now(), models: models}
	rt.mu.Unlock()
	return models, nil
}

func speechModel(models []elevenlabs.Model, id string) (elevenlabs.Model, bool) {
	for _, model := range models {
		if model.ID == id && model.CanDoTextToSpeech {
			return model, true
		}
	}
	return elevenlabs.Model{}, false
}

type elevenLabsModelView struct {
	ID             string                `json:"model_id"`
	Name           string                `json:"name"`
	Description    string                `json:"description"`
	MaxTextLength  int                   `json:"max_text_length"`
	CostMultiplier float64               `json:"cost_multiplier,omitempty"`
	Languages      []elevenlabs.Language `json:"languages,omitempty"`
	Alpha          bool                  `json:"requires_alpha_access,omitempty"`
	Supports       map[string]bool       `json:"supports"`
}

// handleElevenLabsModels lists the speech models with the settings each one
// honors, so the form only offers controls that take effect.
func (s *Server) handleElevenLabsModels(w http.ResponseWriter, r *http.Request) {
	client, fingerprint, ok := s.elevenLabsClient(w, r)
	if !ok {
		return
	}
	rt := s.elevenLabsRuntime()
	release, err := rt.acquire(r.Context())
	if err != nil {
		return
	}
	defer release()
	models, err := rt.cachedModels(r.Context(), client, fingerprint)
	if err != nil {
		writeElevenLabsError(w, r, err)
		return
	}
	views := make([]elevenLabsModelView, 0, len(models))
	for _, model := range models {
		if !model.CanDoTextToSpeech {
			continue
		}
		profile := elevenlabs.ProfileFor(model.ID, &model)
		view := elevenLabsModelView{
			ID: model.ID, Name: model.Name, Description: model.Description,
			MaxTextLength: profile.MaxTextRunes, Languages: model.Languages, Alpha: model.RequiresAlphaAccess,
			Supports: map[string]bool{
				"stability": profile.Stability, "similarity": profile.Similarity, "style": profile.Style,
				"speaker_boost": profile.SpeakerBoost, "speed": profile.Speed,
				"language_code": profile.LanguageCode, "audio_tags": profile.AudioTags,
			},
		}
		if model.Rates != nil {
			view.CostMultiplier = model.Rates.CharacterCostMultiplier
		}
		views = append(views, view)
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": views})
}

// handleElevenLabsVoices lists the account's voices, including the defaults.
func (s *Server) handleElevenLabsVoices(w http.ResponseWriter, r *http.Request) {
	client, fingerprint, ok := s.elevenLabsClient(w, r)
	if !ok {
		return
	}
	values := r.URL.Query()
	query := elevenlabs.VoiceQuery{
		Search:    values.Get("search"),
		VoiceType: values.Get("type"),
		Category:  values.Get("category"),
		PageToken: values.Get("page_token"),
		PageSize:  queryInt(values, "page_size"),
		Sort:      values.Get("sort"),
	}
	rt := s.elevenLabsRuntime()
	cacheKey := fingerprint + "\x00" + values.Encode()
	rt.mu.Lock()
	cached, hit := rt.voices[cacheKey]
	rt.mu.Unlock()
	if hit && time.Since(cached.at) < elevenLabsVoicesTTL {
		writeJSON(w, http.StatusOK, cached.page)
		return
	}
	release, err := rt.acquire(r.Context())
	if err != nil {
		return
	}
	defer release()
	page, err := client.Voices(r.Context(), query)
	if err != nil {
		writeElevenLabsError(w, r, err)
		return
	}
	rt.mu.Lock()
	if len(rt.voices) > 64 {
		rt.voices = map[string]elevenLabsCachedVoices{}
	}
	rt.voices[cacheKey] = elevenLabsCachedVoices{at: time.Now(), page: page}
	rt.mu.Unlock()
	writeJSON(w, http.StatusOK, page)
}

// handleElevenLabsLibrary searches the Voice Library. Live-moderated voices
// are excluded unless include_live_moderated=true is passed.
func (s *Server) handleElevenLabsLibrary(w http.ResponseWriter, r *http.Request) {
	client, _, ok := s.elevenLabsClient(w, r)
	if !ok {
		return
	}
	values := r.URL.Query()
	query := elevenlabs.LibraryQuery{
		Search:               values.Get("search"),
		Category:             values.Get("category"),
		Gender:               values.Get("gender"),
		Age:                  values.Get("age"),
		Accent:               values.Get("accent"),
		Language:             values.Get("language"),
		UseCases:             values.Get("use_cases"),
		Descriptives:         values.Get("descriptives"),
		Sort:                 values.Get("sort"),
		Featured:             values.Get("featured") == "true",
		IncludeLiveModerated: values.Get("include_live_moderated") == "true",
		IncludeCustomRates:   values.Get("include_custom_rates") == "true",
		Page:                 queryInt(values, "page"),
		PageSize:             queryInt(values, "page_size"),
	}
	rt := s.elevenLabsRuntime()
	release, err := rt.acquire(r.Context())
	if err != nil {
		return
	}
	defer release()
	page, err := client.SharedVoices(r.Context(), query)
	if err != nil {
		writeElevenLabsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handleElevenLabsLibraryAdd saves a Voice Library voice to the account so it
// can be selected. It changes the ElevenLabs account, so it needs control.
func (s *Server) handleElevenLabsLibraryAdd(w http.ResponseWriter, r *http.Request) {
	if !s.capabilities(r).ConfigureHost {
		writeError(w, http.StatusForbidden, errors.New("administrator access is required"))
		return
	}
	if !s.requireController(w, r) {
		return
	}
	var body struct {
		PublicOwnerID string `json:"public_owner_id"`
		VoiceID       string `json:"voice_id"`
		Name          string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	client, fingerprint, ok := s.elevenLabsClient(w, r)
	if !ok {
		return
	}
	rt := s.elevenLabsRuntime()
	release, err := rt.acquire(r.Context())
	if err != nil {
		return
	}
	defer release()
	voiceID, err := client.AddSharedVoice(r.Context(), body.PublicOwnerID, body.VoiceID, body.Name)
	if err != nil {
		if _, isAPI := elevenlabs.AsAPIError(err); !isAPI && r.Context().Err() == nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeElevenLabsError(w, r, err)
		return
	}
	rt.mu.Lock()
	for cacheKey := range rt.voices {
		if strings.HasPrefix(cacheKey, fingerprint+"\x00") {
			delete(rt.voices, cacheKey)
		}
	}
	rt.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"voice_id": voiceID})
}

// elevenLabsPreviewURL accepts only ElevenLabs' public preview storage, so the
// proxy cannot be pointed at arbitrary hosts.
func elevenLabsPreviewURL(raw string) (*url.URL, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" || parsed.Fragment != "" {
		return nil, false
	}
	if !strings.EqualFold(parsed.Hostname(), "storage.googleapis.com") {
		return nil, false
	}
	if !strings.HasPrefix(parsed.EscapedPath(), "/eleven-public-prod/") && !strings.HasPrefix(parsed.EscapedPath(), "/eleven-public-cdn/") {
		return nil, false
	}
	if strings.Contains(parsed.Path, "..") || len(raw) > 1024 {
		return nil, false
	}
	// Rebuild from the fixed scheme and host; only the validated path (and
	// the query some preview links carry) comes from the input.
	return &url.URL{Scheme: "https", Host: "storage.googleapis.com", Path: parsed.Path, RawPath: parsed.RawPath, RawQuery: parsed.RawQuery}, true
}

// handleElevenLabsPreview streams a voice preview through the app origin, so
// the browser plays it with the same audio path as speech and never contacts
// third-party storage directly. Previews are public files; no key is sent.
func (s *Server) handleElevenLabsPreview(w http.ResponseWriter, r *http.Request) {
	if !s.capabilities(r).ConfigureHost {
		writeError(w, http.StatusForbidden, errors.New("administrator access is required"))
		return
	}
	target, ok := elevenLabsPreviewURL(r.URL.Query().Get("url"))
	if !ok {
		writeError(w, http.StatusBadRequest, errors.New("that isn't an ElevenLabs voice preview"))
		return
	}
	rt := s.elevenLabsRuntime()
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	// #nosec G704 -- the URL is rebuilt on the fixed https://storage.googleapis.com
	// host and limited to ElevenLabs' public preview buckets by elevenLabsPreviewURL.
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// #nosec G704 -- same fixed-host preview URL; redirects are never followed.
	response, err := rt.preview.Do(request)
	if err != nil {
		if r.Context().Err() == nil {
			writeError(w, http.StatusBadGateway, errors.New("the voice preview could not be loaded"))
		}
		return
	}
	defer func() { _ = response.Body.Close() }()
	mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if response.StatusCode != http.StatusOK || (!strings.HasPrefix(mediaType, "audio/") && mediaType != "application/octet-stream") {
		writeError(w, http.StatusBadGateway, errors.New("the voice preview is unavailable"))
		return
	}
	if mediaType == "application/octet-stream" {
		mediaType = "audio/mpeg"
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, elevenLabsPreviewBytes+1))
	if err != nil || len(data) == 0 || len(data) > elevenLabsPreviewBytes {
		writeError(w, http.StatusBadGateway, errors.New("the voice preview could not be loaded"))
		return
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}

func queryInt(values url.Values, key string) int {
	value, err := strconv.Atoi(strings.TrimSpace(values.Get(key)))
	if err != nil || value < 0 {
		return 0
	}
	return value
}
