package llm

import (
	"errors"
	"fmt"
)

// CloudError is a safe, typed outcome. Remote error text can contain request
// data, so it is deliberately never included in logs or diagnostics.
type CloudError struct {
	Kind string
}

// HoldMotion prevents a declined, stale, or unavailable cloud plan from
// triggering deterministic library fallback or another billing path.
func (e *CloudError) HoldMotion() bool { return true }

func (e *CloudError) Error() string {
	switch e.Kind {
	case "refusal":
		return "The selected cloud provider declined this request."
	case "quota":
		return "Cloud usage is unavailable or its limit was reached. Review your usage settings."
	case "signed_out":
		return "Connect the selected cloud account again."
	case "permission":
		return "The selected provider has not authorized this request. Check the account or API key."
	case "model":
		return "Select a model available to the connected account."
	case "incomplete":
		return "The cloud response did not complete. Motion was held."
	case "stale":
		return "The motion context changed while planning. The cloud proposal was discarded."
	case "timeout":
		return "Cloud planning timed out. Motion was held."
	case "unsupported":
		return "Decisions selects complete library patterns. Choose Library motion to use it."
	case "capability":
		return "This model or provider does not support the selected structured output policy."
	default:
		return "The cloud provider is unavailable. Motion was held."
	}
}

// IsCloudOutcome keeps a cloud failure out of local repair/fallback paths.
func IsCloudOutcome(err error) bool {
	var outcome *CloudError
	return errors.As(err, &outcome)
}

func cloudResponseError(status int, code string) error {
	kind := "unavailable"
	switch {
	case code == "subscription_sharing_usage_limit_exceeded" || code == "subscription_sharing_usage_unavailable" || status == 429 || status == 402:
		kind = "quota"
	case status == 401:
		kind = "signed_out"
	case status == 403:
		kind = "permission"
	case code == "model_not_found":
		kind = "model"
	}
	return &CloudError{Kind: kind}
}

// ValidateCloudModel bounds model identifiers before constructing a request.
func ValidateCloudModel(model string) error {
	if len(model) == 0 || len(model) > 160 {
		return fmt.Errorf("a bounded cloud model identifier is required")
	}
	return nil
}
