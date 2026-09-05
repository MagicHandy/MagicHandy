package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// MotionLabPrompt is a deliberately isolated experimental contract. It cannot
// emit a motion action and is never appended to the production chat prompt.
const MotionLabPrompt = `You propose a motion experiment for a preview panel. Nothing you return starts a device.
Return only JSON with exactly these fields: range_anchor_percent, outbound_time_percent, explanation.
The reference separates fixed phrase data from editable controls. Only fixed fields must stay unchanged. Editable values are starting points, not instructions; the user's requested changes take precedence.
range_anchor_percent is an integer 0..100: 0 holds the base end while the other end varies, 50 contracts about the center, 100 holds the tip end. This is independent of speed and stroke length.
outbound_time_percent is an integer 25..75: the authored share of travel time toward the tip (0 is base, 100 is tip). Lower values mean faster outbound and slower return; 50 is balanced. The experiment preserves this rhythm by accepting lower average pace when safety limits require it.
Choose the two controls independently. Use 50 for neutral or unused controls; the ordinary Creative baseline is 50 for both. Respect requests to keep either axis unchanged. Do not return a method name, raw positions, timestamps, transport commands, action or extra fields.
explanation is a brief plain-language hypothesis for this comparison, not a claim that physical motion improved.`

// MotionLabProposal has no authority to dispatch motion or change settings.
type MotionLabProposal struct {
	Method              string `json:"method"`
	RangeAnchorPercent  *int   `json:"range_anchor_percent"`
	OutboundTimePercent *int   `json:"outbound_time_percent"`
	Explanation         string `json:"explanation"`
}

// ProposeMotionLab makes one real provider request. Failures remain visible;
// there is no repair, invented fallback, chat persistence or engine access.
func ProposeMotionLab(ctx context.Context, provider llm.Provider, model, message, reference string) (MotionLabProposal, error) {
	message, err := ValidateUserMessage(message)
	if err != nil {
		return MotionLabProposal{}, err
	}
	raw, err := provider.StreamChat(ctx, llm.ChatRequest{
		Model: model, Temperature: 0.2, MaxTokens: 384, ReasoningMode: "off",
		Messages: []llm.Message{
			{Role: "system", Content: MotionLabPrompt},
			{Role: "user", Content: "Current preview data: " + reference + "\nComparison request: " + message},
		},
	}, func(string) error { return nil })
	if err != nil {
		return MotionLabProposal{}, err
	}
	return decodeMotionLabProposal(raw)
}

func decodeMotionLabProposal(raw string) (MotionLabProposal, error) {
	var fields struct {
		RangeAnchorPercent  *int   `json:"range_anchor_percent"`
		OutboundTimePercent *int   `json:"outbound_time_percent"`
		Explanation         string `json:"explanation"`
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fields); err != nil {
		return MotionLabProposal{}, fmt.Errorf("invalid lab proposal: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return MotionLabProposal{}, errors.New("lab proposal must contain exactly one JSON object")
	}
	proposal := MotionLabProposal{RangeAnchorPercent: fields.RangeAnchorPercent,
		OutboundTimePercent: fields.OutboundTimePercent, Explanation: fields.Explanation}
	if proposal.RangeAnchorPercent == nil || proposal.OutboundTimePercent == nil ||
		*proposal.RangeAnchorPercent < 0 || *proposal.RangeAnchorPercent > 100 ||
		*proposal.OutboundTimePercent < 25 || *proposal.OutboundTimePercent > 75 ||
		strings.TrimSpace(proposal.Explanation) == "" || len(proposal.Explanation) > 1000 {
		return MotionLabProposal{}, errors.New("lab proposal is missing valid controls or an explanation")
	}
	switch {
	case *proposal.RangeAnchorPercent == 50 && *proposal.OutboundTimePercent == 50:
		proposal.Method = "creative"
	case *proposal.OutboundTimePercent == 50:
		proposal.Method = "anchored"
	case *proposal.RangeAnchorPercent == 50:
		proposal.Method = "directional"
	default:
		proposal.Method = "combined"
	}
	return proposal, nil
}
