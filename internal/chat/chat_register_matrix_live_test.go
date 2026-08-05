package chat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

// Persona x model register matrix, scored against a live llama.cpp server.
// Skipped unless LLAMACPP names one, so it never runs in CI:
//
//	LLAMACPP=http://127.0.0.1:8792 MODELS=name PERSONAS=all go test ./internal/chat -run TestMatrix -v
//
// Two families of metric, and both matter. FAULTS are the corny/broken register
// users reported. QUALITY guards the opposite failure: a prompt can score well
// on faults by making replies short, cold, euphemised, or refusing, and that is
// a worse product. A change is an improvement only if faults fall and quality
// does not -- explicitness in particular must hold, since sanitising the voice
// is a regression however clean the fault counts look.
//
// The four personas are deliberately spread: a dominant woman, a reserved man, a
// submissive woman at the non-explicit Intimate level, and a deliberately crude
// man. Rook exists to catch sanitisation -- a prompt change that makes him
// polite has broken something, and no fault metric would show it.
type evalPersona struct {
	key   string
	name  string
	desc  string
	voice VoiceLevel
	style ReactionStyle
}

var evalPersonas = []evalPersona{
	{
		key: "mara", name: "Mara", voice: VoiceExplicit, style: StyleDominant,
		desc: "Mara is thirty-four, sharp-tongued and entirely unembarrassed. She runs a bar and " +
			"runs the people in it. She decides what happens and says so in plain words, teases " +
			"without cruelty, and finds hesitation funny rather than off-putting. Tone: confident, " +
			"warm, filthy when she wants to be.",
	},
	{
		key: "elias", name: "Elias", voice: VoiceExplicit, style: StyleNeutral,
		desc: "Elias is quiet and watchful, a restorer of old furniture who works with his hands and " +
			"says little. He notices everything and comments on almost none of it. When he does " +
			"speak he is direct and unhurried, never loud. Tone: reserved, dry, patient.",
	},
	{
		key: "sofia", name: "Sofia", voice: VoiceIntimate, style: StyleSubmissive,
		desc: "Sofia is soft-spoken and devoted, happiest when she is useful to someone she likes. " +
			"She asks before she acts and lights up at approval. She is shy about saying what she " +
			"wants but says it anyway. Tone: gentle, eager, a little breathless.",
	},
	{
		key: "rook", name: "Rook", voice: VoiceExplicit, style: StyleDominant,
		desc: "Rook is a blunt, crude dockworker with no interest in being polite about sex. He " +
			"swears constantly, says exactly what he wants in the coarsest available words, and " +
			"has no patience for delicacy or romance. Tone: rough, vulgar, impatient.",
	},
}

var matrixTurns = []string{
	"Fuck me.",
	"Slower please",
	"Tell me what you want to do to me",
	"Don't stop",
	"Describe what you're doing",
	"I'm close",
	"Talk dirty to me",
	"How does that feel to you",
}

type omsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func matrixGenerate(t *testing.T, endpoint, system string, history []omsg, user string) string {
	t.Helper()
	messages := append([]omsg{{Role: "system", Content: system}}, history...)
	messages = append(messages, omsg{Role: "user", Content: user})
	// Mirrors internal/llm/llama_cpp.go StreamChat: the OpenAI-compatible
	// endpoint with response_format json_object and thinking disabled. The
	// json_object constraint materially changes generation, so an eval that omits
	// it is not measuring what the app actually gets.
	body, _ := json.Marshal(map[string]any{
		"messages": messages, "stream": false,
		"temperature": 0.8, "max_tokens": 300,
		"response_format":      map[string]any{"type": "json_object"},
		"chat_template_kwargs": map[string]any{"enable_thinking": false},
	})
	// #nosec G704 -- endpoint is rebuilt from a validated loopback host by
	// loopbackEndpoint, shared with the register harness in this package.
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 300 * time.Second}
	// #nosec G704 -- same validated loopback endpoint.
	response, err := client.Do(request)
	if err != nil {
		t.Logf("generate failed: %v", err)
		return ""
	}
	defer func() { _ = response.Body.Close() }()
	var decoded struct {
		Choices []struct {
			Message omsg `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return ""
	}
	if len(decoded.Choices) == 0 {
		return ""
	}
	return decoded.Choices[0].Message.Content
}

var (
	// The reported fault is the reply TRAILING OFF into a decorative participle
	// ("..., moving with every ounce of patience I have."), not participles as
	// such. A mid-sentence participial clause that continues into a subordinate
	// clause is ordinary English -- "gripping it tight while I slide up and down"
	// is good writing, and a metric that punishes it would push the models toward
	// clipped, stilted prose. So: only the final sentence counts, and only when
	// nothing follows the participle to turn it into a real clause.
	mTrailTail   = regexp.MustCompile(`(?i),\s+\w+ing\b([^.!?]*)[.!?]?\s*$`)
	mSubordinate = regexp.MustCompile(`(?i)\b(while|until|so|as|because|and|that|when|if|but|before|after)\b`)
	mPet         = regexp.MustCompile(`(?i)(\b(darling|sweetheart|baby)\b|\bmy (love|sweet|dear|pet)\b)`)
	mAbstract    = regexp.MustCompile(`(?i)\b(warmth|rhythm|harmony|hum|intent|sensation|connection|tension|moment|world|everything|forever)\b`)
	mOrnament    = regexp.MustCompile(`(?i)(\bthe\s+(\w+\s+)?(curve|curves|arch|line|lines|shape|contour|swell|planes|expanse)\s+of\b|\b(exquisite|delicate|divine|sublime|velvet|silken|molten|intoxicating|deliberate)\b)`)
	// Quality / anti-sanitization.
	mExplicit  = regexp.MustCompile(`(?i)\b(cock|dick|cunt|pussy|clit|fuck|fucking|balls|tits|ass|nipples?)\b`)
	mRefusal   = regexp.MustCompile(`(?i)(\bI (can't|cannot|won't be able)\b|as an AI|I'm not able|I am not able|let's keep (it|things)|maybe we should|I'd rather not)`)
	mAssistant = regexp.MustCompile(`(?i)(let me know|how can I help|feel free|is there anything|I'm here to help)`)
	mConcrete  = regexp.MustCompile(`(?i)\b(hand|hands|mouth|tongue|lips|breath|skin|hips|thighs|throat|fingers|palm|weight|teeth|jaw|grip)\b`)
	// Syntactic monotony. The two-word-opening count badly under-measures this:
	// a persona can score rep 2 while every single sentence is still "I <verb>".
	// That is the failure mode the "each sentence states one thing and stops"
	// rule introduced, and it reads as flat and repetitive rather than corny.
	mIStart = regexp.MustCompile(`(?i)^\s*(I|I'm|I'll|I've|I'd)\b`)
)

// sentenceOpeners splits a reply into sentences and reports how many there are
// and how many begin with a first-person pronoun.
func sentenceOpeners(reply string) (total, firstPerson int) {
	for _, s := range regexp.MustCompile(`[.!?]+`).Split(reply, -1) {
		if strings.TrimSpace(s) == "" {
			continue
		}
		total++
		if mIStart.MatchString(s) {
			firstPerson++
		}
	}
	return total, firstPerson
}

type score struct {
	n                                             int
	participle, pet, refusal, assistant           int
	abstract, ornament, explicit, concrete, words int
	sentences, iSentences, iReplies               int
	openings                                      map[string]int
	vocab                                         map[string]bool
}

// trailsOffIntoParticiple reports the actual reported tic: the reply's LAST
// sentence ending in a bare participial phrase that adds nothing after it.
func trailsOffIntoParticiple(reply string) bool {
	sentences := regexp.MustCompile(`[.!?]+`).Split(strings.TrimSpace(reply), -1)
	last := ""
	for _, s := range sentences {
		if strings.TrimSpace(s) != "" {
			last = s
		}
	}
	match := mTrailTail.FindStringSubmatch(last)
	if match == nil {
		return false
	}
	// A subordinator turns the participle into a real clause rather than a
	// decorative trail-off, and that is ordinary good English.
	return !mSubordinate.MatchString(match[1])
}

func newScore() *score {
	return &score{openings: map[string]int{}, vocab: map[string]bool{}}
}

func (s *score) add(reply string) {
	if strings.TrimSpace(reply) == "" {
		return
	}
	s.n++
	if trailsOffIntoParticiple(reply) {
		s.participle++
	}
	if mPet.MatchString(reply) {
		s.pet++
	}
	if mRefusal.MatchString(reply) {
		s.refusal++
	}
	if mAssistant.MatchString(reply) {
		s.assistant++
	}
	if mExplicit.MatchString(reply) {
		s.explicit++
	}
	if mIStart.MatchString(reply) {
		s.iReplies++
	}
	total, firstPerson := sentenceOpeners(reply)
	s.sentences += total
	s.iSentences += firstPerson
	s.abstract += len(mAbstract.FindAllString(reply, -1))
	s.ornament += len(mOrnament.FindAllString(reply, -1))
	s.concrete += len(mConcrete.FindAllString(reply, -1))
	words := strings.Fields(reply)
	s.words += len(words)
	for _, w := range words {
		s.vocab[strings.ToLower(strings.Trim(w, ".,!?;:\"'"))] = true
	}
	if len(words) >= 2 {
		s.openings[strings.ToLower(words[0]+" "+words[1])]++
	}
}

// topRepeats names the actual repeated openings, because "rep 11" does not tell
// you whether the model is stuck on one phrase or mildly repetitive everywhere.
func (s *score) topRepeats() string {
	type kv struct {
		k string
		n int
	}
	var list []kv
	for k, n := range s.openings {
		if n > 1 {
			list = append(list, kv{k, n})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].n > list[j].n })
	var parts []string
	for i, item := range list {
		if i >= 5 {
			break
		}
		parts = append(parts, fmt.Sprintf("%q x%d", item.k, item.n))
	}
	return strings.Join(parts, "  ")
}

func (s *score) repeats() int {
	total := 0
	for _, c := range s.openings {
		if c > 1 {
			total += c - 1
		}
	}
	return total
}

func (s *score) pct(v int) float64 {
	if s.n == 0 {
		return 0
	}
	return 100 * float64(v) / float64(s.n)
}

func (s *score) line(label string) string {
	if s.n == 0 {
		return fmt.Sprintf("%-28s  NO OUTPUT", label)
	}
	iSent := 0.0
	if s.sentences > 0 {
		iSent = 100 * float64(s.iSentences) / float64(s.sentences)
	}
	return fmt.Sprintf("%-26s n=%2d | FAULTS part %3.0f%% pet %3.0f%% abst %.1f orn %.1f rep %2d I-open %3.0f%% I-sent %3.0f%% | QUALITY expl %3.0f%% conc %.1f words %3.0f refus %3.0f%% asst %3.0f%% vocab %d",
		label, s.n,
		s.pct(s.participle), s.pct(s.pet),
		float64(s.abstract)/float64(s.n), float64(s.ornament)/float64(s.n), s.repeats(),
		s.pct(s.iReplies), iSent,
		s.pct(s.explicit), float64(s.concrete)/float64(s.n),
		float64(s.words)/float64(s.n),
		s.pct(s.refusal), s.pct(s.assistant), len(s.vocab))
}

func runPersona(t *testing.T, endpoint string, p evalPersona, show bool) *score {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	caps := FullCapabilities()
	caps.Voice = p.voice
	caps.Style = p.style
	caps.MoodTracking = true
	ctx := &ConversationContext{
		PersonaName: p.name, PersonaDescription: p.desc,
		UserAnatomy: "penis", CurrentMood: "Intimate",
	}
	sc := newScore()
	var history []omsg
	for _, turn := range matrixTurns {
		system := composeSystem(set, nil, defaultPatternChoices(), caps, nil, ctx)
		raw := matrixGenerate(t, endpoint, system, history, turn)
		reply := replyOf(raw)
		sc.add(reply)
		if show && reply != "" {
			t.Logf("    [%s] %q -> %s", p.key, turn, truncate(reply, 200))
		}
		history = append(history, omsg{Role: "user", Content: turn}, omsg{Role: "assistant", Content: reply})
		if len(history) > 6 {
			history = history[len(history)-6:]
		}
		ctx.RecentAssistantReplies = append(ctx.RecentAssistantReplies, reply)
		if len(ctx.RecentAssistantReplies) > maxRecentAssistantReplies {
			ctx.RecentAssistantReplies = ctx.RecentAssistantReplies[1:]
		}
	}
	return sc
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func TestMatrix(t *testing.T) {
	base := os.Getenv("LLAMACPP")
	if base == "" {
		t.Skip("set LLAMACPP to the llama.cpp server base URL")
	}
	endpoint := loopbackEndpoint(t, base)
	models := strings.Split(os.Getenv("MODELS"), ",")
	wanted := os.Getenv("PERSONAS")
	show := os.Getenv("SHOW") != ""
	label := os.Getenv("LABEL")

	fmt.Printf("\n########## %s ##########\n", label)
	agg := map[string]*score{}
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		fmt.Printf("\n=== %s\n", model)
		total := newScore()
		for _, p := range evalPersonas {
			if wanted != "" && wanted != "all" && !strings.Contains(wanted, p.key) {
				continue
			}
			sc := runPersona(t, endpoint, p, show)
			fmt.Println("  " + sc.line(p.key+"/"+string(p.voice)))
			if r := sc.topRepeats(); r != "" {
				fmt.Println("      repeats: " + r)
			}
			for _, r := range []*score{total} {
				r.n += sc.n
				r.participle += sc.participle
				r.pet += sc.pet
				r.refusal += sc.refusal
				r.assistant += sc.assistant
				r.abstract += sc.abstract
				r.ornament += sc.ornament
				r.explicit += sc.explicit
				r.concrete += sc.concrete
				r.words += sc.words
				r.sentences += sc.sentences
				r.iSentences += sc.iSentences
				r.iReplies += sc.iReplies
				for k, v := range sc.openings {
					r.openings[k] += v
				}
				for k := range sc.vocab {
					r.vocab[k] = true
				}
			}
		}
		fmt.Println("  " + total.line("ALL"))
		agg[model] = total
	}
	keys := make([]string, 0, len(agg))
	for k := range agg {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Printf("\n--- %s summary ---\n", label)
	for _, k := range keys {
		fmt.Println(agg[k].line(shortModel(k)))
	}
}

func shortModel(m string) string {
	if i := strings.Index(m, "/"); i >= 0 {
		m = m[i+1:]
	}
	if len(m) > 26 {
		m = m[:26]
	}
	return m
}
