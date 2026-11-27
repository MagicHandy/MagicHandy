package elevenlabs

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestParseAPIErrorClassifiesEveryResponseShape(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   Kind
	}{
		{"typed invalid key", 401, `{"detail":{"type":"authentication_error","code":"invalid_api_key","message":"Invalid API key"}}`, KindAuth},
		{"legacy invalid key", 401, `{"detail":{"status":"invalid_api_key","message":"Invalid API key"}}`, KindAuth},
		{"keyless filter", 401, `{"detail":{"type":"authentication_error","code":"unauthorized","status":"not_logged_in"}}`, KindAuth},
		{"typed missing scope", 403, `{"detail":{"type":"authorization_error","code":"insufficient_permissions"}}`, KindPermission},
		{"legacy missing scope", 401, `{"detail":{"status":"missing_permissions","message":"The API key you used is missing the permission voices_read to execute this operation."}}`, KindPermission},
		{"bare 401 naming a permission", 401, `{"detail":"missing the permission user_read"}`, KindPermission},
		{"free users", 403, `{"detail":{"type":"authorization_error","code":"voice_access_denied","message":"This voice is not available for free users."}}`, KindPlan},
		{"subscription", 403, `{"detail":{"type":"authorization_error","code":"subscription_required"}}`, KindPlan},
		{"free users payment", 402, `{"detail":{"status":"payment_required","message":"Free users cannot use library voices via the API. Please upgrade your subscription to use this voice."}}`, KindPlan},
		{"legacy quota", 401, `{"detail":{"status":"quota_exceeded","message":"This request exceeds your quota."}}`, KindQuota},
		{"credits", 402, `{"detail":{"type":"payment_required","code":"insufficient_credits"}}`, KindQuota},
		{"unusual activity", 401, `{"detail":{"status":"detected_unusual_activity","message":"Unusual activity detected. Free Tier usage disabled."}}`, KindUnusualActivity},
		{"system busy", 429, `{"detail":{"type":"rate_limit_error","code":"system_busy"}}`, KindRateLimited},
		{"legacy concurrency", 429, `{"detail":{"status":"too_many_concurrent_requests"}}`, KindRateLimited},
		{"voice not found", 404, `{"detail":{"type":"not_found","code":"voice_not_found"}}`, KindNotFound},
		{"legacy voice not found", 400, `{"detail":{"status":"voice_not_found","message":"A voice with voice_id was not found."}}`, KindNotFound},
		{"fastapi validation list", 422, `{"detail":[{"loc":["body","text"],"msg":"field required","type":"value_error.missing"}]}`, KindInvalid},
		{"text too long", 400, `{"detail":{"status":"max_character_limit_exceeded","message":"too long"}}`, KindInvalid},
		{"maintenance", 503, `{"detail":{"type":"service_unavailable","code":"maintenance"}}`, KindUnavailable},
		{"html gateway error", 502, `<html>Bad gateway</html>`, KindUnavailable},
		{"empty teapot", 418, ``, KindUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := parseAPIError(OperationSpeak, tc.status, http.Header{}, []byte(tc.body))
			if err.Kind() != tc.want {
				t.Fatalf("kind = %q, want %q (%+v)", err.Kind(), tc.want, err)
			}
			if err.UserMessage() == "" || err.Error() == "" {
				t.Fatal("every failure needs a message")
			}
		})
	}
}

func TestRetryableKinds(t *testing.T) {
	retryable := map[Kind]bool{KindRateLimited: true, KindUnavailable: true, KindNetwork: true}
	for _, kind := range []Kind{KindAuth, KindPermission, KindPlan, KindQuota, KindUnusualActivity,
		KindRateLimited, KindNotFound, KindInvalid, KindUnavailable, KindNetwork, KindUnknown} {
		if got := (&APIError{kind: kind}).Retryable(); got != retryable[kind] {
			t.Errorf("%s retryable = %v", kind, got)
		}
	}
}

func TestErrorTextCarriesFactsButStaysBounded(t *testing.T) {
	err := parseAPIError(OperationListVoices, 403, http.Header{"Request-Id": {"abc123"}},
		[]byte(`{"detail":{"type":"authorization_error","code":"insufficient_permissions","message":"`+strings.Repeat("x", 600)+`"}}`))
	text := err.Error()
	for _, want := range []string{"Voices (read)", "HTTP 403", "insufficient_permissions", "request abc123"} {
		if !strings.Contains(text, want) {
			t.Errorf("error text %q lacks %q", text, want)
		}
	}
	if len(err.Message) > 310 {
		t.Fatalf("message was not bounded: %d bytes", len(err.Message))
	}
}

func TestRetryAfterParsing(t *testing.T) {
	if got := parseRetryAfter("3"); got != 3*time.Second {
		t.Fatalf("seconds = %s", got)
	}
	if got := parseRetryAfter("soon"); got != 0 {
		t.Fatalf("garbage = %s", got)
	}
	future := time.Now().Add(5 * time.Second).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(future); got <= 0 || got > 6*time.Second {
		t.Fatalf("http date = %s", got)
	}
	if got := backoff(1, 30*time.Second); got != maxRetryAfter {
		t.Fatalf("retry-after must be capped, got %s", got)
	}
	for try := 1; try <= 6; try++ {
		if got := backoff(try, 0); got <= 0 || got > maxBackoff {
			t.Fatalf("backoff(%d) = %s", try, got)
		}
	}
}

func TestBoundedTextKeepsRunesWhole(t *testing.T) {
	value := strings.Repeat("é", 10)
	got := boundedText(value, 7)
	if !strings.HasSuffix(got, "…") || strings.ContainsRune(got, '�') || strings.Count(got, "é") != 3 {
		t.Fatalf("boundedText = %q", got)
	}
}
