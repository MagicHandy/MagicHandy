package elevenlabs

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"
)

func TestVoicesSendsOnlyRecognizedFilters(t *testing.T) {
	var query url.Values
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		_, _ = io.WriteString(w, `{"voices":[{"voice_id":"v1","name":"Ava","category":"premade","labels":{"accent":"american","age":30},"is_legacy":true,"preview_url":"https://storage.googleapis.com/eleven-public-prod/a.mp3"}],"has_more":true,"next_page_token":"tok"}`)
	}), Options{})
	page, err := client.Voices(t.Context(), VoiceQuery{Search: "  soft   warm ", VoiceType: "bogus", Category: "premade", PageSize: 500, Sort: "name"})
	if err != nil {
		t.Fatal(err)
	}
	if query.Get("search") != "soft warm" || query.Has("voice_type") || query.Get("category") != "premade" ||
		query.Get("page_size") != "100" || query.Get("sort_direction") != "asc" {
		t.Fatalf("query = %v", query)
	}
	if len(page.Voices) != 1 || !page.HasMore || page.NextPageToken != "tok" {
		t.Fatalf("page = %+v", page)
	}
	voice := page.Voices[0]
	if voice.Labels["accent"] != "american" || voice.Labels["age"] != "30" || !voice.IsLegacy {
		t.Fatalf("voice = %+v", voice)
	}
}

func TestLibraryExcludesLiveModeratedVoicesByDefault(t *testing.T) {
	var query url.Values
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		_, _ = io.WriteString(w, `{"voices":[{"public_owner_id":"owner1","voice_id":"lib1","name":"Velvet - Soft","free_users_allowed":false,"live_moderation_enabled":false,"rate":2}],"has_more":false}`)
	}), Options{})
	page, err := client.SharedVoices(t.Context(), LibraryQuery{Search: "breathy", Gender: "female", Sort: "trending", Page: 2})
	if err != nil {
		t.Fatal(err)
	}
	if query.Get("include_live_moderated") != "false" || query.Get("include_custom_rates") != "false" ||
		query.Get("sort") != "trending" || query.Get("page") != "2" || query.Get("gender") != "female" {
		t.Fatalf("query = %v", query)
	}
	if len(page.Voices) != 1 || page.Voices[0].FreeUsersAllowed || page.Voices[0].Rate == nil || *page.Voices[0].Rate != 2 {
		t.Fatalf("page = %+v", page)
	}
}

func TestAddSharedVoiceValidatesAndReturnsTheNewID(t *testing.T) {
	var path string
	var body map[string]any
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = io.WriteString(w, `{"voice_id":"added123"}`)
	}), Options{})
	if _, err := client.AddSharedVoice(t.Context(), "owner/../x", "lib1", "Name"); err == nil {
		t.Fatal("unsafe owner ID must be rejected before any request")
	}
	id, err := client.AddSharedVoice(t.Context(), "owner1", "lib1", "  Velvet   - Soft  ")
	if err != nil || id != "added123" {
		t.Fatalf("id = %q, %v", id, err)
	}
	if path != "/v1/voices/add/owner1/lib1" || body["new_name"] != "Velvet - Soft" {
		t.Fatalf("path %q body %v", path, body)
	}
}

func TestSubscriptionReportsTheFreeTier(t *testing.T) {
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"tier":"free","status":"free","character_count":1200,"character_limit":10000,"voice_limit":3}`)
	}), Options{})
	subscription, err := client.Subscription(t.Context())
	if err != nil || !subscription.FreeTier() || subscription.CharacterLimit != 10000 {
		t.Fatalf("subscription = %+v, %v", subscription, err)
	}
}

func TestValidIdentifier(t *testing.T) {
	for value, want := range map[string]bool{
		"21m00Tcm4TlvDq8ikWAM": true, "eleven_v4": true, "eleven-x": true,
		"": false, "a/b": false, "a b": false, "../": false, "x?y": false,
	} {
		if ValidIdentifier(value) != want {
			t.Errorf("ValidIdentifier(%q) != %v", value, want)
		}
	}
}
