package chat

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestLayeredDirectionRoundTripPreservationAndRemoval(t *testing.T) {
	before := DefaultLayeredScore(45)
	limits := config.DefaultSettings().Motion
	raw := `{"edits":{"composition":"manual","directional_pace":{"up_rate_percent":27,"down_rate_percent":100}},"reply":"Faster down, slower up."}`
	_, after, changed, err := parseHostedLayeredReply(raw, before, limits)
	want := *motion.CloneFlowSpec(&before)
	want.DirectionalPace = &motion.DirectionalPace{UpRatePercent: 27, DownRatePercent: 100}
	if err != nil || !reflect.DeepEqual(after, want) || len(changed) == 0 {
		t.Fatalf("direction edit lost or changed other dimensions: %v", err)
	}
	for _, patch := range []string{`{}`, `{"composition":"full_slider"}`, `{"composition":"manual","controls":{"speed_percent":{"op":"adjust","value":5}}}`, `{"composition":"phrase","phrase":{"strokes":[{"bottom_percent":0,"top_percent":100,"count":2,"rate_percent":100}]}}`} {
		_, next, _, err := parseHostedLayeredReply(`{"edits":`+patch+`,"reply":"Test."}`, after, limits)
		if err != nil || !reflect.DeepEqual(next.DirectionalPace, want.DirectionalPace) {
			t.Fatal("unrelated edit lost direction contrast", err)
		}
	}
	_, removed, _, err := parseHostedLayeredReply(strings.Replace(raw, `:27`, `:100`, 1), after, limits)
	if err != nil || !reflect.DeepEqual(removed, before) {
		t.Fatal("neutral rates did not restore the symmetric score", err)
	}
	state := MotionContext{Running: true, MotionMode: MotionModeLayered, Layered: &before}
	schema := InteractiveResponseSchema(nil, Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: true}, &state)
	if _, err := llm.OpenAISchema(schema); err != nil {
		t.Fatal(err)
	}
	domain, err := llm.DomainOutput(`{"proposal":{"action":"update","edits":{"composition":"manual","directional_pace":{"up_rate_percent":27,"down_rate_percent":100}},"reply":"Test."}}`, schema)
	if err != nil {
		t.Fatal(err)
	}
	if _, next, _, err := parseHostedLayeredReply(domain, before, limits); err != nil || !reflect.DeepEqual(next, want) {
		t.Fatal("provider round trip lost rates", err)
	}
}

func TestLayeredDirectionRejectsMalformedTransactionally(t *testing.T) {
	before := DefaultLayeredScore(45)
	before.DirectionalPace = &motion.DirectionalPace{UpRatePercent: 30, DownRatePercent: 100}
	for _, value := range []string{`null`, `{}`, `{"up_rate_percent":30}`, `{"up_rate_percent":0,"down_rate_percent":100}`, `{"up_rate_percent":null,"down_rate_percent":100}`, `{"up_rate_percent":27.5,"down_rate_percent":100}`, `{"up_rate_percent":27,"down_rate_percent":101}`, `{"up_rate_percent":27,"down_rate_percent":100,"up_rate_percent":99}`} {
		_, next, _, err := parseHostedLayeredReply(`{"edits":{"composition":"manual","directional_pace":`+value+`},"reply":"Test."}`, before, config.DefaultSettings().Motion)
		if err == nil || !reflect.DeepEqual(next, before) {
			t.Fatalf("malformed direction changed score: %s %v", value, err)
		}
	}
	data, _ := json.Marshal(layeredScoreContext(before))
	if !strings.Contains(string(data), "directional_pace") {
		t.Fatal("model cannot see active direction rates")
	}
}

func TestDirectionalPaceIsAdvertisedAndAcceptedOnlyByAdvanced(t *testing.T) {
	before := DefaultLayeredScore(45)
	state := MotionContext{Running: true, MotionMode: MotionModeLayered, Layered: &before}
	for _, level := range config.ModelCapabilityLevels() {
		caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, SimpleMotion: level == config.ModelCapabilitySimple, IntermediateMotion: level == config.ModelCapabilityIntermediate, AdvancedMotion: level == config.ModelCapabilityAdvanced}
		schema := string(InteractiveResponseSchema(nil, caps, &state))
		if strings.Contains(schema, "directional_pace") != caps.AdvancedMotion {
			t.Fatalf("wrong directional schema for %s", level)
		}
		raw := `{"action":"update","edits":{"directional_pace":{"up_rate_percent":30,"down_rate_percent":100}},"reply":"Test."}`
		s := Service{Capabilities: &caps}
		response, err := s.parseLayeredResponse(raw, before, state, config.DefaultSettings().Motion, false)
		if caps.AdvancedMotion {
			if err != nil || response.Motion == nil {
				t.Fatal("advanced direction rejected", err)
			}
		} else if err == nil || response.Motion != nil {
			t.Fatal("unsupported tier applied directional edit")
		}
	}
}
