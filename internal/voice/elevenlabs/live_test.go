//go:build liveeval

package elevenlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

// TestLiveElevenLabs checks the client against the real API with the key in
// ELEVENLABS_API_KEY. It spends a few hundred credits on short samples and
// writes what it observed to ELEVENLABS_LIVE_REPORT (JSON) when set. Run it
// with: go test -tags liveeval -run TestLiveElevenLabs -v ./internal/voice/elevenlabs/
func TestLiveElevenLabs(t *testing.T) {
	key := os.Getenv("ELEVENLABS_API_KEY")
	if key == "" {
		t.Skip("ELEVENLABS_API_KEY is not set")
	}
	client, err := New(Options{APIKey: key})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	report := map[string]any{}
	defer func() {
		path := os.Getenv("ELEVENLABS_LIVE_REPORT")
		if path == "" {
			return
		}
		data, _ := json.MarshalIndent(report, "", "  ")
		if bytes.Contains(data, []byte(key)) {
			t.Fatal("the report would contain the key")
		}
		_ = os.WriteFile(path, data, 0o600)
	}()

	subscription, err := client.Subscription(ctx)
	if err != nil {
		t.Fatalf("subscription: %v", err)
	}
	report["subscription"] = map[string]any{"tier": subscription.Tier, "status": subscription.Status,
		"character_count": subscription.CharacterCount, "character_limit": subscription.CharacterLimit,
		"voice_slots_used": subscription.VoiceSlotsUsed, "voice_limit": subscription.VoiceLimit}
	t.Logf("plan %q: %d of %d credits used", subscription.Tier, subscription.CharacterCount, subscription.CharacterLimit)

	models, err := client.Models(ctx)
	if err != nil {
		t.Fatalf("models: %v", err)
	}
	var speechModels []map[string]any
	for _, model := range models {
		if !model.CanDoTextToSpeech {
			continue
		}
		entry := map[string]any{"id": model.ID, "name": model.Name, "max_text": model.MaxTextLength,
			"style": model.CanUseStyle, "speaker_boost": model.CanUseSpeakerBoost, "alpha": model.RequiresAlphaAccess,
			"cost_factor": model.TokenCostFactor, "languages": len(model.Languages), "concurrency_group": model.ConcurrencyGroup}
		if model.Rates != nil {
			entry["character_cost_multiplier"] = model.Rates.CharacterCostMultiplier
		}
		speechModels = append(speechModels, entry)
		t.Logf("model %-28s max=%-6d style=%-5v boost=%-5v alpha=%v", model.ID, model.MaxTextLength, model.CanUseStyle, model.CanUseSpeakerBoost, model.RequiresAlphaAccess)
	}
	report["speech_models"] = speechModels

	defaults, err := client.Voices(ctx, VoiceQuery{VoiceType: "default", PageSize: 100})
	if err != nil {
		t.Fatalf("default voices: %v", err)
	}
	var defaultVoices []map[string]any
	for _, voice := range defaults.Voices {
		defaultVoices = append(defaultVoices, map[string]any{"id": voice.ID, "name": voice.Name, "category": voice.Category,
			"legacy": voice.IsLegacy, "labels": voice.Labels, "tiers": voice.AvailableForTiers, "description": voice.Description})
	}
	report["default_voices"] = defaultVoices
	t.Logf("%d default voices", len(defaultVoices))

	rachel, rachelErr := client.Voice(ctx, "21m00Tcm4TlvDq8ikWAM")
	report["rachel_lookup"] = describe(rachelErr, map[string]any{"name": rachel.Name, "legacy": rachel.IsLegacy, "category": rachel.Category})

	library := map[string]any{}
	for _, search := range []string{"", "soft", "warm", "whisper", "asmr", "sultry", "seductive", "breathy", "intimate", "playful", "deep", "calm"} {
		page, err := client.SharedVoices(ctx, LibraryQuery{Search: search, PageSize: 100, Sort: "trending", Language: "en"})
		if err != nil {
			library[search] = describe(err, nil)
			continue
		}
		free, descriptives, useCases := 0, map[string]int{}, map[string]int{}
		for _, voice := range page.Voices {
			if voice.FreeUsersAllowed {
				free++
			}
			descriptives[voice.Descriptive]++
			useCases[voice.UseCase]++
		}
		library[search] = map[string]any{"count": len(page.Voices), "has_more": page.HasMore, "free_users_allowed": free,
			"descriptives": topCounts(descriptives, 12), "use_cases": topCounts(useCases, 8), "sample": sampleNames(page.Voices, 6)}
	}
	report["library"] = library

	// A missing permission: this key has no History access.
	historyErr := rawStatus(ctx, client, http.MethodGet, "/v1/history?page_size=1", nil)
	report["missing_permission_history"] = historyErr
	t.Logf("history without permission: %v", historyErr)

	// Empty text: does validation answer before spending anything?
	report["empty_text"] = rawStatus(ctx, client, http.MethodPost, "/v1/text-to-speech/21m00Tcm4TlvDq8ikWAM/stream?output_format=pcm_24000", []byte(`{"text":"","model_id":"eleven_multilingual_v2"}`))

	voiceID := firstUsableDefault(defaults.Voices)
	report["sample_voice"] = voiceID
	sample := "I'm right here with you. Take a slow breath, and tell me what you'd like."
	latency := map[string]any{}
	for _, model := range []string{"eleven_multilingual_v2", "eleven_flash_v2_5", "eleven_v3", "eleven_v4", "eleven_v4_turbo"} {
		latency[model] = timeSpeech(ctx, client, voiceID, SpeechRequest{Text: sample, ModelID: model, OutputFormat: "pcm_24000"})
		t.Logf("%s: %v", model, latency[model])
	}
	report["latency"] = latency

	style, speed, stability := 0.4, 0.9, 0.42
	report["v4_with_style_and_speed"] = timeSpeech(ctx, client, voiceID, SpeechRequest{Text: "Hi.", ModelID: "eleven_v4", OutputFormat: "pcm_24000",
		VoiceSettings: &VoiceSettings{Style: &style, Speed: &speed}})
	report["v3_with_continuous_stability"] = timeSpeech(ctx, client, voiceID, SpeechRequest{Text: "Hi.", ModelID: "eleven_v3", OutputFormat: "pcm_24000",
		VoiceSettings: &VoiceSettings{Stability: &stability}})
	report["v4_with_previous_request_ids"] = timeSpeech(ctx, client, voiceID, SpeechRequest{Text: "Hi.", ModelID: "eleven_v4", OutputFormat: "pcm_24000",
		PreviousRequestIDs: []string{"00000000000000000000"}})

	if page, err := client.SharedVoices(ctx, LibraryQuery{PageSize: 30, Sort: "trending"}); err == nil {
		for _, voice := range page.Voices {
			if voice.FreeUsersAllowed && !voice.LiveModerationEnabled {
				report["library_voice_on_this_plan"] = timeSpeech(ctx, client, voice.ID, SpeechRequest{Text: "Hi.", ModelID: "eleven_multilingual_v2", OutputFormat: "pcm_24000"})
				break
			}
		}
	}
}

func describe(err error, ok map[string]any) map[string]any {
	if err == nil {
		if ok == nil {
			ok = map[string]any{}
		}
		ok["ok"] = true
		return ok
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return map[string]any{"ok": false, "status": apiErr.StatusCode, "kind": apiErr.Kind(), "type": apiErr.Type,
			"code": apiErr.Code, "legacy_status": apiErr.Status, "message": apiErr.Message}
	}
	return map[string]any{"ok": false, "error": err.Error()}
}

func rawStatus(ctx context.Context, client *Client, method, path string, body []byte) map[string]any {
	result, err := client.send(ctx, "probe", method, strings.Split(path, "?")[0], nil, body, "")
	if strings.Contains(path, "?") {
		// Re-send with the query when one was given.
		if result.response != nil {
			_ = result.response.Body.Close()
			result.cancel()
		}
		request, _ := http.NewRequestWithContext(ctx, method, client.baseURL+path, bytes.NewReader(body))
		request.Header.Set("xi-api-key", client.apiKey)
		request.Header.Set("Content-Type", "application/json")
		response, doErr := client.http.Do(request)
		if doErr != nil {
			return map[string]any{"error": client.Redact(doErr.Error())}
		}
		defer func() { _ = response.Body.Close() }()
		data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		if response.StatusCode < 300 {
			return map[string]any{"status": response.StatusCode, "bytes": len(data)}
		}
		parsed := parseAPIError("probe", response.StatusCode, response.Header, data)
		return describe(parsed, nil)
	}
	if err != nil {
		return describe(err, nil)
	}
	defer result.cancel()
	defer func() { _ = result.response.Body.Close() }()
	data, _ := io.ReadAll(io.LimitReader(result.response.Body, 1<<20))
	return map[string]any{"status": result.response.StatusCode, "bytes": len(data)}
}

func timeSpeech(ctx context.Context, client *Client, voiceID string, request SpeechRequest) map[string]any {
	start := time.Now()
	speech, err := client.StreamSpeech(ctx, voiceID, request)
	if err != nil {
		return describe(err, nil)
	}
	defer func() { _ = speech.Body.Close() }()
	buffer := make([]byte, 32*1024)
	var first time.Duration
	total := 0
	for {
		n, readErr := speech.Body.Read(buffer)
		if n > 0 && first == 0 {
			first = time.Since(start)
		}
		total += n
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				return map[string]any{"ok": false, "error": readErr.Error(), "bytes": total}
			}
			break
		}
	}
	return map[string]any{"ok": true, "first_audio_ms": first.Milliseconds(), "total_ms": time.Since(start).Milliseconds(),
		"bytes": total, "even": total%2 == 0, "audio_seconds": float64(total) / 48000, "content_type": speech.ContentType,
		"request_id_present": speech.RequestID != "", "character_cost": speech.CharacterCost}
}

func firstUsableDefault(voices []Voice) string {
	for _, voice := range voices {
		if !voice.IsLegacy && ValidIdentifier(voice.ID) {
			return voice.ID
		}
	}
	if len(voices) > 0 {
		return voices[0].ID
	}
	return "21m00Tcm4TlvDq8ikWAM"
}

func topCounts(counts map[string]int, limit int) []string {
	type pair struct {
		key   string
		count int
	}
	var pairs []pair
	for key, count := range counts {
		pairs = append(pairs, pair{key, count})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].count > pairs[j].count })
	var out []string
	for index, entry := range pairs {
		if index >= limit {
			break
		}
		out = append(out, entry.key+"="+itoa(entry.count))
	}
	return out
}

func sampleNames(voices []LibraryVoice, limit int) []string {
	var names []string
	for index, voice := range voices {
		if index >= limit {
			break
		}
		names = append(names, voice.Name)
	}
	return names
}

func itoa(value int) string {
	data, _ := json.Marshal(value)
	return string(data)
}
