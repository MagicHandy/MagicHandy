//go:build magichandy_labs

package main

// Inert evaluation: the production provider/parser/compiler, no engine or transport.
import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/openaiauth"
)

type probeTransport struct{ effort, tier string }

func (p probeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/responses") {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		_ = r.Body.Close()
		if p.effort != "" {
			body["reasoning"] = map[string]string{"effort": p.effort}
		}
		if p.tier != "" {
			body["service_tier"] = p.tier
		}
		data, _ := json.Marshal(body)
		r.Body = io.NopCloser(bytes.NewReader(data))
		r.ContentLength = int64(len(data))
	}
	return http.DefaultTransport.RoundTrip(r)
}
func main() {
	phase := flag.String("phase", "steering", "screen, suite, or steering")
	models := flag.String("models", "gpt-6-sol/low", "comma-separated model/effort/tier variants; consumes ChatGPT plan usage")
	repeats := flag.Int("repeats", 1, "repeats per case")
	output := flag.String("output", ".scratch/cloud-motion-review.json", "report")
	steering := flag.Bool("steering", false, "clarify hard reach versus focus preferences")
	hostedGuidance := flag.Bool("hosted-guidance", true, "use final hosted production guidance")
	localModel := flag.String("local-model", "", "installed Ollama model for a local baseline")
	dataDir := flag.String("data-dir", "", "existing signed-in app data directory; credentials are never copied")
	flag.Parse()
	if *phase != "screen" && *phase != "suite" && *phase != "steering" {
		panic("phase must be screen, suite, or steering")
	}
	if *repeats < 1 || *repeats > 20 {
		panic("repeats must be between 1 and 20")
	}
	if *localModel != "" {
		*models = "local"
	}
	ctx := context.Background()
	var token llm.TokenSource
	var err error
	if *localModel == "" {
		if *dataDir == "" {
			panic("provide -data-dir for an existing signed-in profile, or -local-model")
		}
		manager, openErr := openaiauth.Open(openaiauth.Options{DataDir: *dataDir})
		must(openErr)
		defer manager.Close()
		token, err = manager.ActiveTokenSource(ctx)
		must(err)
	}
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent = 10
	limits.SpeedMaxPercent = 80
	rows := []map[string]any{}
	save := func() {
		data, _ := json.MarshalIndent(map[string]any{"turns": rows}, "", "  ")
		must(os.WriteFile(*output, data, 0600))
	}
	for _, variant := range strings.Split(*models, ",") {
		parts := strings.Split(variant, "/")
		model := parts[0]
		effort, tier := "", ""
		if len(parts) > 1 && parts[1] != "default" {
			effort = parts[1]
		}
		if len(parts) > 2 {
			tier = parts[2]
		}
		var p llm.Provider
		if *localModel != "" {
			model = *localModel
			variant = model
			p, err = llm.NewOllamaProvider(llm.HTTPProviderOptions{BaseURL: "http://127.0.0.1:11434", Model: model, Timeout: 90 * time.Second})
		} else {
			p, err = llm.NewOpenAIResponsesProvider(llm.OpenAIOptions{PlanUsage: true, Model: model, Token: token, Timeout: 45 * time.Second, Client: &http.Client{Transport: probeTransport{effort, tier}}})
		}
		must(err)
		for repeat := 0; repeat < *repeats; repeat++ {
			methods := []string{"creative_v2"}
			if *phase == "suite" {
				methods = []string{"creative_v2", "layered", "stroke_ends", "groove", "plain_words", "sequence", "adaptive"}
			}
			for _, method := range methods {
				current := initialScore(method, limits)
				cases := reviewCases(*phase, method)
				history := []llm.Message{}
				for index, message := range cases {
					prompt := chat.LLMLabPrompts()[method]
					if *hostedGuidance && *localModel == "" {
						prompt = chat.HostedLLMLabPrompts()[method]
					}
					if *steering {
						prompt = "Resolve requested reach before choosing texture. A request that confines ALL motion to a region requires the outer range to bound that region; focus alone cannot exclude broad strokes when mixed reach remains enabled. Inside a restricted range, broad and shorter strokes must both stay inside its endpoints. Do not confuse absolute slider positions with band-relative focus placement. Verify that the numeric edits actually realize the promised reach, copying only unrelated controls.\n\n" + prompt
					}
					trial := chat.RunLLMLab(ctx, p, model, method, prompt, message, current, limits, history, true)
					if method == "adaptive" {
						trial = adaptive(ctx, p, model, message, current, limits, history)
					}
					compiled := false
					summary := motion.PerceptualSummary{}
					if trial.Valid {
						target, e := motion.FlowTarget(trial.After, limits)
						compiled = e == nil
						if e != nil {
							trial.Error = e.Error()
							trial.Valid = false
						} else {
							review := motion.ReviewMotionOutput(target, limits)
							summary = review.Summary
							if review.Error != "" {
								trial.Error = review.Error
								trial.Valid = false
								compiled = false
							}
						}
					}
					data, _ := json.Marshal(trial)
					row := map[string]any{}
					_ = json.Unmarshal(data, &row)
					row["variant"] = variant
					row["repeat"] = repeat
					row["case"] = index
					row["compiled"] = compiled
					intent := reviewIntent(*phase, method, index, trial, current, summary)
					row["output"] = summary
					row["intent_pass"] = intent
					row["steering"] = *steering
					row["expected_recipe"] = fmt.Sprintf("%s case %d", *phase, index)
					rows = append(rows, row)
					save()
					fmt.Printf("%s r%d %s/%d valid=%v compiled=%v ms=%d error=%s\n", variant, repeat, method, index, trial.Valid, compiled, trial.ElapsedMillis, trial.Error)
					if trial.Valid {
						current = trial.After
					}
					history = append(history, llm.Message{Role: "user", Content: message}, llm.Message{Role: "assistant", Content: trial.Reply})
				}
			}
		}
	}
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
