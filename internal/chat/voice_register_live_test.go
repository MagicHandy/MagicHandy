package chat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Drives the real composed prompt through a live local model and counts the
// register faults a user reported from a session: trailing participle chains
// ("..., letting every lingering touch stretch out"), abstract nouns standing in
// for anything physical, purple ornament ("the deliberate curve of my body"),
// repeated openings, and pet names.
//
// It is skipped unless a server is named, so it never runs in CI:
//
//	LLAMA=http://127.0.0.1:8489 go test ./internal/chat -run TestVoiceRegisterAgainstLiveModel -v
//	LLAMA=... EVAL_VOICE=intimate EVAL_LABEL=after go test ...
//
// Counts alone do not tell you whether the reply is any good, so read the
// printed replies too. Run it before and after a prompt change; a single run is
// noisy at temperature 0.8.
//
// The fixture persona is deliberately reserved and clipped. That is the case
// that exposes a voice level or reaction style overriding the persona's own
// temperament, because an effusive persona would mask the fault by agreeing
// with the effusive default (docs/chat-voice.md).
const reservedPersona = "A quiet, watchful man who says little and means all of it. He keeps his " +
	"voice low and level, deflects attention with a polite half-answer, and never raises it. " +
	"Patient to the point of stillness. Tone: restrained, spare, quietly intense."

var evalTurns = []string{
	"Fuck me.",
	"Slower please",
	"Don't stop",
	"Tell me what you're thinking",
	"Faster",
	"I'm close",
	"What do you like about this",
	"Describe me",
	"Keep going just like that",
	"Say something",
	"Harder",
	"How does that feel to you",
}

type llamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// loopbackEndpoint bounds the LLAMA environment variable to a plain HTTP
// loopback address before it is ever used to build a request. A local eval
// harness has no reason to reach anything else, and constraining it here keeps
// an operator-supplied string from becoming an arbitrary outbound request.
func loopbackEndpoint(t *testing.T, base string) string {
	t.Helper()
	parsed, err := url.Parse(strings.TrimSuffix(base, "/"))
	if err != nil {
		t.Fatalf("LLAMA is not a URL: %v", err)
	}
	if parsed.Scheme != "http" {
		t.Fatalf("LLAMA must be an http:// loopback URL, got scheme %q", parsed.Scheme)
	}
	host := parsed.Hostname()
	if host != "localhost" && !net.ParseIP(host).IsLoopback() {
		t.Fatalf("LLAMA must point at loopback, got host %q", host)
	}
	return "http://" + parsed.Host + "/v1/chat/completions"
}

func generate(t *testing.T, endpoint, system string, history []llamaMessage, user string) string {
	t.Helper()
	messages := append([]llamaMessage{{Role: "system", Content: system}}, history...)
	messages = append(messages, llamaMessage{Role: "user", Content: user})
	// Mirrors what the llama.cpp provider sends with reasoning_mode "off";
	// without it the model spends the whole budget in reasoning_content and
	// returns empty content.
	body, _ := json.Marshal(map[string]any{
		"messages": messages, "temperature": 0.8, "max_tokens": 300, "stream": false,
		"chat_template_kwargs": map[string]any{"enable_thinking": false},
	})
	// #nosec G704 -- endpoint is rebuilt from a validated loopback host above.
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 180 * time.Second}
	// #nosec G704 -- same validated loopback endpoint.
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	var decoded struct {
		Choices []struct {
			Message llamaMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Choices) == 0 {
		return ""
	}
	return decoded.Choices[0].Message.Content
}

// replyOf pulls the reply string out of the contract JSON the model returns.
func replyOf(raw string) string {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return strings.TrimSpace(raw)
	}
	var shape struct {
		Reply string `json:"reply"`
	}
	if json.Unmarshal([]byte(raw[start:end+1]), &shape) == nil && shape.Reply != "" {
		return shape.Reply
	}
	return strings.TrimSpace(raw)
}

var (
	participleTail = regexp.MustCompile(`(?i),\s+\w+ing\s`)
	// Vocative uses only: a bare \blove\b also matches "I love seeing you", which
	// is not the fault being measured.
	petName      = regexp.MustCompile(`(?i)(\b(darling|sweetheart|baby)\b|\bmy (love|sweet|dear|pet)\b)`)
	abstractNoun = regexp.MustCompile(`(?i)\b(warmth|rhythm|harmony|hum|intent|sensation|connection|tension|moment|world|everything|forever)\b`)
	// The user's quoted example of the fault: "deliberate curve of my body".
	// Geometry-noun-of-possessive and admiring adjectives are the two shapes the
	// purple register takes, so they are measured separately from abstraction.
	ornamentOf = regexp.MustCompile(`(?i)\bthe\s+(\w+\s+)?(curve|curves|arch|line|lines|shape|contour|contours|swell|planes|expanse)\s+of\b`)
	purpleAdj  = regexp.MustCompile(`(?i)\b(exquisite|delicate|divine|perfect|beautiful|glorious|sublime|velvet|silken|molten|electric|intoxicating|deliberate)\b`)
)

func scoreReplies(label string, replies []string) {
	participles, pets, abstracts := 0, 0, 0
	ornaments := 0
	openings := map[string]int{}
	for _, reply := range replies {
		if participleTail.MatchString(reply) {
			participles++
		}
		if petName.MatchString(reply) {
			pets++
		}
		abstracts += len(abstractNoun.FindAllString(reply, -1))
		ornaments += len(ornamentOf.FindAllString(reply, -1)) + len(purpleAdj.FindAllString(reply, -1))
		words := strings.Fields(reply)
		if len(words) >= 2 {
			openings[strings.ToLower(words[0]+" "+words[1])]++
		}
	}
	repeatedOpenings := 0
	for _, count := range openings {
		if count > 1 {
			repeatedOpenings += count - 1
		}
	}
	fmt.Printf("\n== %s (n=%d)\n", label, len(replies))
	fmt.Printf("   trailing participle clause : %d (%.0f%%)\n", participles, 100*float64(participles)/float64(len(replies)))
	fmt.Printf("   pet name                   : %d (%.0f%%)\n", pets, 100*float64(pets)/float64(len(replies)))
	fmt.Printf("   abstract nouns             : %d total, %.1f per reply\n", abstracts, float64(abstracts)/float64(len(replies)))
	fmt.Printf("   purple ornament            : %d total, %.1f per reply\n", ornaments, float64(ornaments)/float64(len(replies)))
	fmt.Printf("   repeated 2-word openings   : %d\n", repeatedOpenings)
	for _, reply := range replies {
		fmt.Printf("   - %s\n", reply)
	}
}

func TestVoiceRegisterAgainstLiveModel(t *testing.T) {
	base := os.Getenv("LLAMA")
	if base == "" {
		t.Skip("set LLAMA to a local llama.cpp base URL to score the live reply register")
	}
	endpoint := loopbackEndpoint(t, base)
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	capabilities := FullCapabilities()
	capabilities.Voice = VoiceWarm
	if level := os.Getenv("EVAL_VOICE"); level != "" {
		capabilities.Voice = VoiceLevel(level)
	}
	capabilities.Style = StyleSubmissive
	capabilities.MoodTracking = true
	context := &ConversationContext{
		PersonaName:        "Ash",
		PersonaDescription: reservedPersona,
		UserAnatomy:        "penis",
		CurrentMood:        "Intimate",
	}

	var replies []string
	var history []llamaMessage
	for _, turn := range evalTurns {
		system := composeSystem(set, nil, defaultPatternChoices(), capabilities, nil, context)
		reply := replyOf(generate(t, endpoint, system, history, turn))
		replies = append(replies, reply)
		history = append(history,
			llamaMessage{Role: "user", Content: turn},
			llamaMessage{Role: "assistant", Content: reply})
		if len(history) > 6 {
			history = history[len(history)-6:]
		}
		context.RecentAssistantReplies = append(context.RecentAssistantReplies, reply)
		if len(context.RecentAssistantReplies) > maxRecentAssistantReplies {
			context.RecentAssistantReplies = context.RecentAssistantReplies[1:]
		}
	}
	scoreReplies(fmt.Sprintf("%s %s/submissive/reserved", os.Getenv("EVAL_LABEL"), capabilities.Voice), replies)
}
