package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/voice"
)

const elevenLabsFixtureKey = "sk_fixture0123456789abcdef"

// fakeElevenLabs answers the catalog endpoints. mode selects the key's
// behavior: "full", "restricted" (every read is a missing permission), or
// "invalid".
type fakeElevenLabs struct {
	mode       string
	voiceCalls atomic.Int32
	library    atomic.Value // last library query
	added      atomic.Value // last add path
}

func (f *fakeElevenLabs) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("xi-api-key")
		if key == "" || strings.Contains(r.URL.String(), key) {
			t.Errorf("key missing from header or present in URL: %s", r.URL)
		}
		if f.mode == "invalid" || (key != elevenLabsFixtureKey && key != "sk_typed0123456789") {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"detail":{"type":"authentication_error","code":"invalid_api_key","message":"Invalid API key"}}`)
			return
		}
		if f.mode == "restricted" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"detail":{"status":"missing_permissions","message":"The API key you used is missing the permission user_read to execute this operation."}}`)
			return
		}
		switch {
		case r.URL.Path == "/v1/user/subscription":
			_, _ = io.WriteString(w, `{"tier":"free","status":"free","character_count":250,"character_limit":10000,"voice_slots_used":0,"voice_limit":3}`)
		case r.URL.Path == "/v1/models":
			_, _ = io.WriteString(w, `[{"model_id":"eleven_multilingual_v2","name":"Multilingual v2","can_do_text_to_speech":true,"can_use_style":true,"can_use_speaker_boost":true,"maximum_text_length_per_request":10000,"model_rates":{"character_cost_multiplier":1}},{"model_id":"eleven_v4","name":"Eleven v4","can_do_text_to_speech":true},{"model_id":"eleven_v4_turbo","name":"Eleven v4 Turbo","can_do_text_to_speech":true},{"model_id":"eleven_multilingual_sts_v2","can_do_text_to_speech":false}]`)
		case r.URL.Path == "/v1/voices/"+config.DefaultElevenLabsVoiceID:
			_, _ = io.WriteString(w, `{"voice_id":"`+config.DefaultElevenLabsVoiceID+`","name":"Jessica","category":"premade","is_legacy":false,"labels":{"accent":"american"}}`)
		case strings.HasPrefix(r.URL.Path, "/v1/voices/") && r.Method == http.MethodGet:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"detail":{"type":"not_found","code":"voice_not_found"}}`)
		case r.URL.Path == "/v2/voices":
			f.voiceCalls.Add(1)
			_, _ = io.WriteString(w, `{"voices":[{"voice_id":"v1","name":"Ava","category":"premade","labels":{}}],"has_more":false}`)
		case r.URL.Path == "/v1/shared-voices":
			f.library.Store(r.URL.RawQuery)
			_, _ = io.WriteString(w, `{"voices":[{"public_owner_id":"owner1","voice_id":"lib1","name":"Velvet - Soft","free_users_allowed":true,"live_moderation_enabled":false}],"has_more":false}`)
		case strings.HasPrefix(r.URL.Path, "/v1/voices/add/") && r.Method == http.MethodPost:
			f.added.Store(r.URL.Path)
			_, _ = io.WriteString(w, `{"voice_id":"added1"}`)
		default:
			http.NotFound(w, r)
		}
	})
}

func newElevenLabsTestServer(t *testing.T, mode string, withKey bool) (*Server, *fakeElevenLabs) {
	t.Helper()
	fake := &fakeElevenLabs{mode: mode}
	upstream := httptest.NewServer(fake.handler(t))
	t.Cleanup(upstream.Close)
	server := newTestServer(t)
	server.elevenLabs.baseURL = upstream.URL
	if withKey {
		saveSettings(t, server.store, func(settings config.Settings) config.Settings {
			settings.Voice.ElevenLabsAPIKey = elevenLabsFixtureKey
			return settings
		})
	}
	return server, fake
}

func decodeBody(t *testing.T, res *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if strings.Contains(res.Body.String(), elevenLabsFixtureKey) {
		t.Fatalf("response revealed the API key: %s", res.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", res.Body.String(), err)
	}
	return body
}

func TestElevenLabsAccountReportsKeyPlanAndVoice(t *testing.T) {
	server, _ := newElevenLabsTestServer(t, "full", true)
	res := handyAPI(t, server, http.MethodGet, "/api/voice/elevenlabs/account", "")
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	body := decodeBody(t, res)
	subscription, _ := body["subscription"].(map[string]any)
	selected, _ := body["selected_voice"].(map[string]any)
	if body["key_status"] != "ok" || body["library_via_api"] != false || subscription["tier"] != "free" || selected["name"] != "Jessica" ||
		body["selected_model_problem"] != nil {
		t.Fatalf("account = %v", body)
	}
	permissions, _ := body["permissions"].(map[string]any)
	for _, scope := range []string{"user_read", "models_read", "voices_read"} {
		if probe, _ := permissions[scope].(map[string]any); probe["state"] != "ok" {
			t.Fatalf("%s = %v", scope, permissions[scope])
		}
	}
}

func TestElevenLabsAccountExplainsRestrictedAndInvalidKeys(t *testing.T) {
	restricted, _ := newElevenLabsTestServer(t, "restricted", true)
	body := decodeBody(t, handyAPI(t, restricted, http.MethodGet, "/api/voice/elevenlabs/account", ""))
	if body["key_status"] != "restricted" || !strings.Contains(body["message"].(string), "Speech may still work") {
		t.Fatalf("restricted key = %v", body)
	}

	invalid, _ := newElevenLabsTestServer(t, "invalid", true)
	body = decodeBody(t, handyAPI(t, invalid, http.MethodGet, "/api/voice/elevenlabs/account", ""))
	if body["key_status"] != "invalid" {
		t.Fatalf("invalid key = %v", body)
	}

	missing, _ := newElevenLabsTestServer(t, "full", false)
	body = decodeBody(t, handyAPI(t, missing, http.MethodGet, "/api/voice/elevenlabs/account", ""))
	if body["key_set"] != false || body["key_status"] != "missing" {
		t.Fatalf("missing key = %v", body)
	}
}

func TestElevenLabsAccountChecksATypedKeyWithoutSavingIt(t *testing.T) {
	server, _ := newElevenLabsTestServer(t, "full", false)
	res := handyAPI(t, server, http.MethodPost, "/api/voice/elevenlabs/account", `{"api_key":" sk_typed0123456789 ","voice_id":"unknownvoice"}`)
	body := decodeBody(t, res)
	if body["key_status"] != "ok" || body["selected_voice_problem"] == nil {
		t.Fatalf("typed key check = %v", body)
	}
	if strings.Contains(res.Body.String(), "sk_typed") {
		t.Fatal("the typed key must not be echoed")
	}
	settings := snapshotSettings(t, server)
	if settings.Voice.ElevenLabsAPIKey != "" {
		t.Fatal("checking a typed key must not save it")
	}
	if res := handyAPI(t, server, http.MethodPost, "/api/voice/elevenlabs/account", `{"api_key":"has space"}`); res.Code != http.StatusBadRequest {
		t.Fatalf("malformed key status = %d", res.Code)
	}
}

func TestElevenLabsCatalogEndpoints(t *testing.T) {
	server, fake := newElevenLabsTestServer(t, "full", true)

	models := decodeBody(t, handyAPI(t, server, http.MethodGet, "/api/voice/elevenlabs/models", ""))
	list, _ := models["models"].([]any)
	if len(list) != 3 {
		t.Fatalf("speech models = %v", models)
	}
	v4, _ := list[1].(map[string]any)
	supports, _ := v4["supports"].(map[string]any)
	if v4["model_id"] != "eleven_v4" || supports["style"] != false || supports["audio_tags"] != true {
		t.Fatalf("eleven_v4 view = %v", v4)
	}

	for range 2 {
		voices := decodeBody(t, handyAPI(t, server, http.MethodGet, "/api/voice/elevenlabs/voices?search=ava", ""))
		if entries, _ := voices["voices"].([]any); len(entries) != 1 {
			t.Fatalf("voices = %v", voices)
		}
	}
	if calls := fake.voiceCalls.Load(); calls != 1 {
		t.Fatalf("a repeated voice listing should be cached; upstream calls = %d", calls)
	}

	library := decodeBody(t, handyAPI(t, server, http.MethodGet, "/api/voice/elevenlabs/library?search=soft&gender=female", ""))
	query, _ := fake.library.Load().(string)
	if entries, _ := library["voices"].([]any); len(entries) != 1 ||
		!strings.Contains(query, "include_live_moderated=false") || !strings.Contains(query, "include_custom_rates=false") {
		t.Fatalf("library = %v, query %q", library, query)
	}

	added := decodeBody(t, handyAPI(t, server, http.MethodPost, "/api/voice/elevenlabs/library/add", `{"public_owner_id":"owner1","voice_id":"lib1","name":"Velvet"}`))
	if added["voice_id"] != "added1" || fake.added.Load() != "/v1/voices/add/owner1/lib1" {
		t.Fatalf("add = %v", added)
	}
	decodeBody(t, handyAPI(t, server, http.MethodGet, "/api/voice/elevenlabs/voices?search=ava", ""))
	if calls := fake.voiceCalls.Load(); calls != 2 {
		t.Fatalf("adding a voice should refresh the account voices; upstream calls = %d", calls)
	}
	if res := handyAPI(t, server, http.MethodPost, "/api/voice/elevenlabs/library/add", `{"public_owner_id":"../x","voice_id":"lib1"}`); res.Code != http.StatusBadRequest {
		t.Fatalf("unsafe owner ID status = %d", res.Code)
	}
}

func TestElevenLabsCatalogNeedsAKey(t *testing.T) {
	server, _ := newElevenLabsTestServer(t, "full", false)
	res := handyAPI(t, server, http.MethodGet, "/api/voice/elevenlabs/voices", "")
	if body := decodeBody(t, res); res.Code != http.StatusConflict || body["kind"] != "no_key" {
		t.Fatalf("no key = %d %v", res.Code, body)
	}
}

func TestElevenLabsPreviewURLAllowlist(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://storage.googleapis.com/eleven-public-prod/database/voices/a/b.mp3": true,
		"https://storage.googleapis.com/eleven-public-cdn/premade/voices/x.mp3":     true,
		"http://storage.googleapis.com/eleven-public-prod/a.mp3":                    false,
		"https://storage.googleapis.com/other-bucket/a.mp3":                         false,
		"https://evil.example/eleven-public-prod/a.mp3":                             false,
		"https://user@storage.googleapis.com/eleven-public-prod/a.mp3":              false,
		"https://storage.googleapis.com:8443/eleven-public-prod/a.mp3":              false,
		"https://storage.googleapis.com/eleven-public-prod/../secret":               false,
		"https://storage.googleapis.com/eleven-public-prod/%2e%2e/secret":           false,
		"": false,
	} {
		if _, ok := elevenLabsPreviewURL(raw); ok != want {
			t.Errorf("elevenLabsPreviewURL(%q) = %v, want %v", raw, ok, want)
		}
	}
}

func TestSpeakRepliesFromChatStartsTheSpeechWorker(t *testing.T) {
	server := newTestServer(t)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Voice.Enabled = true
		settings.Voice.SpeakReplies = false
		settings.Voice.TTSAutoLaunch = false
		settings.Voice.TTSProvider = config.VoiceProviderCustom
		settings.Voice.TTSWorkerPath = chatStubBinary(t)
		settings.Voice.TTSWorkerArgs = []string{"-role", "tts"}
		return settings
	})
	res := handyAPI(t, server, http.MethodPut, "/api/voice/preferences", `{"speak_replies":true}`)
	if res.Code != http.StatusOK {
		t.Fatalf("preferences status %d: %s", res.Code, res.Body.String())
	}
	waitForVoiceWorkerReady(t, server.voice.Worker(voice.RoleTTS))
}

func TestKeySaveIsNotBlockedByUnrelatedSaves(t *testing.T) {
	server := newTestServer(t)
	revision := func() string {
		body := decodeBody(t, handyAPI(t, server, http.MethodGet, "/api/settings", ""))
		value, _ := body["credential_revision"].(string)
		if !strings.HasPrefix(value, "c1-") {
			t.Fatalf("credential revision = %v", body["credential_revision"])
		}
		return value
	}
	keyPatch := func(revision, value string, wasSet bool) string {
		return `{"credential_revision":"` + revision + `","changes":{"voice":{"elevenlabs_api_key":"` + value +
			`"}},"expected":{"voice":{"elevenlabs_api_key":` + map[bool]string{true: "true", false: "false"}[wasSet] + `}}}`
	}
	formRevision := revision()
	if res := handyAPI(t, server, http.MethodPatch, "/api/settings",
		`{"changes":{"diagnostics":{"verbosity":"debug"}},"expected":{"diagnostics":{"verbosity":"normal"}}}`); res.Code != http.StatusOK {
		t.Fatalf("unrelated save = %d %s", res.Code, res.Body.String())
	}
	if res := handyAPI(t, server, http.MethodPatch, "/api/settings", keyPatch(formRevision, "sk_first0123456789", false)); res.Code != http.StatusOK {
		t.Fatalf("a key save after an unrelated save = %d %s", res.Code, res.Body.String())
	}
	// Another form replaced the key since this revision was read: conflict.
	if res := handyAPI(t, server, http.MethodPatch, "/api/settings", keyPatch(formRevision, "sk_stale0123456789", true)); res.Code != http.StatusConflict {
		t.Fatalf("a stale key edit = %d", res.Code)
	}
	if settings := snapshotSettings(t, server); settings.Voice.ElevenLabsAPIKey != "sk_first0123456789" {
		t.Fatal("a stale key edit changed the stored key")
	}
	if res := handyAPI(t, server, http.MethodPatch, "/api/settings", keyPatch(revision(), `  sk_latest0123456789\n`, true)); res.Code != http.StatusOK {
		t.Fatalf("a current key edit = %d %s", res.Code, res.Body.String())
	}
	if settings := snapshotSettings(t, server); settings.Voice.ElevenLabsAPIKey != "sk_latest0123456789" {
		t.Fatalf("a pasted key should be trimmed, got %q", settings.Voice.ElevenLabsAPIKey)
	}
}

func TestElevenLabsWorkerConfigPrefersTheBundledWorker(t *testing.T) {
	dir := t.TempDir()
	name := "voice-elevenlabs-worker"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bundled := filepath.Join(dir, name)
	if err := os.WriteFile(bundled, []byte("stub"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := config.DefaultSettings().Voice
	settings.Enabled = true
	settings.TTSProvider = config.VoiceTTSProviderElevenLabs
	settings.TTSWorkerPath = filepath.Join(dir, "left-over-custom-worker.exe")
	got := voiceManagerConfig(settings, filepath.Join(dir, "magichandy.exe"), t.TempDir())
	if got.TTS.Command != bundled || got.TTS.JobTimeout != elevenLabsJobTimeout {
		t.Fatalf("tts config = %+v", got.TTS)
	}
	if got.TTS.Env != nil {
		t.Fatal("no key means no key environment")
	}

	stability, speed := 0.35, 0.9
	settings.ElevenLabsDelivery = config.ElevenLabsDelivery{Stability: &stability, Speed: &speed, Language: "en"}
	got = voiceManagerConfig(settings, filepath.Join(dir, "magichandy.exe"), t.TempDir())
	args := strings.Join(got.TTS.Args, " ")
	if !strings.Contains(args, "-stability 0.35") || !strings.Contains(args, "-speed 0.9") ||
		!strings.Contains(args, "-language en") || strings.Contains(args, "-similarity") || strings.Contains(args, "-style") {
		t.Fatalf("delivery args = %q", args)
	}
}
