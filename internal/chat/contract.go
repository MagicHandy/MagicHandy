// Package chat orchestrates local LLM turns into app-level semantic actions.
package chat

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	// MotionActionNone leaves motion unchanged.
	MotionActionNone = "none"
	// MotionActionStart starts motion through the motion engine.
	MotionActionStart = "start"
	// MotionActionTarget retargets already running motion through the motion engine.
	MotionActionTarget = "target"
	// MotionActionUpdate retargets active dynamic geometry through the motion engine.
	MotionActionUpdate = "update"
	// MotionActionStop stops motion through the motion engine.
	MotionActionStop = "stop"
)

// Mood is model-reported reply-register state. It is inert metadata and never
// enters MotionCommand, MotionContext, or transport dispatch.
type Mood string

// Moods match the reviewed STGPT-RV register and remain stable JSON protocol
// values even when surrounding prompt prose is localized.
const (
	MoodCurious      Mood = "Curious"
	MoodTeasing      Mood = "Teasing"
	MoodPlayful      Mood = "Playful"
	MoodLoving       Mood = "Loving"
	MoodExcited      Mood = "Excited"
	MoodPassionate   Mood = "Passionate"
	MoodSeductive    Mood = "Seductive"
	MoodAnticipatory Mood = "Anticipatory"
	MoodBreathless   Mood = "Breathless"
	MoodDominant     Mood = "Dominant"
	MoodSubmissive   Mood = "Submissive"
	MoodVulnerable   Mood = "Vulnerable"
	MoodConfident    Mood = "Confident"
	MoodIntimate     Mood = "Intimate"
	MoodNeedy        Mood = "Needy"
	MoodOverwhelmed  Mood = "Overwhelmed"
	MoodAfterglow    Mood = "Afterglow"
)

var moodValues = []Mood{
	MoodCurious, MoodTeasing, MoodPlayful, MoodLoving, MoodExcited,
	MoodPassionate, MoodSeductive, MoodAnticipatory, MoodBreathless,
	MoodDominant, MoodSubmissive, MoodVulnerable, MoodConfident,
	MoodIntimate, MoodNeedy, MoodOverwhelmed, MoodAfterglow,
}

// Moods returns the accepted mood values in prompt order.
func Moods() []Mood {
	return append([]Mood(nil), moodValues...)
}

// AssistantResponse is the only model output shape accepted by MagicHandy.
type AssistantResponse struct {
	Reply   string         `json:"reply"`
	NewMood *Mood          `json:"new_mood,omitempty"`
	Motion  *MotionCommand `json:"motion,omitempty"`
}

// MotionCommand is semantic motion intent, not a transport command.
type MotionCommand struct {
	Action    string `json:"action"`
	PatternID string `json:"pattern_id,omitempty"`
	// Intensity is accepted only to decode responses produced by older prompts.
	// normalizePacing removes it before validation and dispatch.
	Intensity    *int `json:"intensity,omitempty"`
	SpeedPercent *int `json:"speed_percent,omitempty"`
	// Area optionally focuses motion on a named zone. Named zones localize to
	// bounded relative windows in deterministic code (the STGPT-RV area-focus
	// lesson: zones, never raw model-authored depth numbers).
	Area string `json:"area,omitempty"`
	// Dynamic geometry is available only in Dynamic generation mode. Pointer
	// fields preserve omitted running values without conflating zero with absent.
	CenterPercent    *int     `json:"center_percent,omitempty"`
	SpanPercent      *int     `json:"span_percent,omitempty"`
	SpanMinPercent   *int     `json:"span_min_percent,omitempty"`
	SpanProfile      string   `json:"span_profile,omitempty"`
	Anchors          []string `json:"anchors,omitempty"`
	VariationPercent *int     `json:"variation_percent,omitempty"`
	SegmentSeconds   *int     `json:"segment_seconds,omitempty"`
}

const (
	// DynamicSpanProfileSteady holds one stroke length.
	DynamicSpanProfileSteady = "steady"
	// DynamicSpanProfileBreathe produces a slow coherent swell.
	DynamicSpanProfileBreathe = "breathe"
	// DynamicSpanProfileWander produces smooth correlated range changes.
	DynamicSpanProfileWander = "wander"
	// DynamicSpanProfileContrast groups tight, medium, and broad strokes.
	DynamicSpanProfileContrast = "contrast"
)

// DynamicSpanProfiles lists the compact range-envelope vocabulary exposed to
// local models. It is duplicated at the chat boundary to keep chat independent
// of the motion package.
func DynamicSpanProfiles() []string {
	return []string{
		DynamicSpanProfileSteady,
		DynamicSpanProfileBreathe,
		DynamicSpanProfileWander,
		DynamicSpanProfileContrast,
	}
}

// Named area-focus zones the model may request. "full" explicitly clears an
// active focus.
const (
	AreaZoneTip   = "tip"
	AreaZoneShaft = "shaft"
	AreaZoneBase  = "base"
	AreaZoneFull  = "full"
)

// AreaZones lists the accepted area values in prompt order.
func AreaZones() []string {
	return []string{AreaZoneTip, AreaZoneShaft, AreaZoneBase, AreaZoneFull}
}

// Named dynamic anchors use MagicHandy's semantic direction: base is 0 and
// tip is 100. The inset endpoints avoid repeatedly striking hard limits.
var dynamicAnchorPositions = map[string]int{
	"base": 8, "lower": 28, "middle": 50, "upper": 72, "tip": 92,
}

// DynamicAnchorNames lists the compact anchor vocabulary in travel order.
func DynamicAnchorNames() []string {
	return []string{"base", "lower", "middle", "upper", "tip"}
}

// DynamicAnchorPosition resolves one model-facing anchor to semantic position.
func DynamicAnchorPosition(name string) (int, bool) {
	position, ok := dynamicAnchorPositions[strings.ToLower(strings.TrimSpace(name))]
	return position, ok
}

// PatternChoice is one enabled library entry exposed to the model as data.
type PatternChoice struct {
	ID          string
	Name        string
	Description string
	Tags        []string
	Weight      float64
}

type unknownPatternError struct {
	patternID string
}

func (e unknownPatternError) Error() string {
	return fmt.Sprintf("unknown motion pattern %q", e.patternID)
}

// ParseAssistantResponse validates one strict JSON response from the model.
func ParseAssistantResponse(raw string) (AssistantResponse, error) {
	return parseAssistantResponse(raw, defaultPatternChoices(), false, nil)
}

// ParseAssistantResponseWithPatterns accepts only the supplied enabled IDs.
func ParseAssistantResponseWithPatterns(raw string, patterns []PatternChoice) (AssistantResponse, error) {
	return parseAssistantResponse(raw, patterns, true, nil)
}

func parseAssistantResponseForCapabilities(raw string, patterns []PatternChoice, capabilities Capabilities, context *MotionContext) (AssistantResponse, error) {
	response, err := decodeAssistantResponse(raw)
	if err != nil {
		return AssistantResponse{}, err
	}
	// Fields the active prompt did not advertise are inert model noise. Strip
	// them before validation so utility chat does not repair an unused mood.
	enforceCapabilities(&response, capabilities)
	if err := validateAssistantMood(&response); err != nil {
		return AssistantResponse{}, err
	}
	patternsEnabled := capabilities.Motion && capabilities.Patterns
	dynamicEnabled := capabilities.Motion && capabilities.MotionMode == MotionModeDynamic
	if dynamicEnabled {
		normalizeDynamicGeometry(&response)
	}
	var currentSpeed *int
	if patternsEnabled && context != nil && context.Running && context.SpeedPercent >= 1 && context.SpeedPercent <= 100 {
		speed := context.SpeedPercent
		currentSpeed = &speed
	}
	preserveCurrentPatternSpeed(&response, currentSpeed)
	if err := validateAssistantResponse(&response, patterns, patternsEnabled, dynamicEnabled); err != nil {
		return AssistantResponse{}, err
	}
	if dynamicEnabled {
		if err := validateDynamicSpanEnvelopeState(response.Motion, context); err != nil {
			return AssistantResponse{}, err
		}
	}
	return response, nil
}

// normalizeDynamicGeometry resolves one unambiguous local-model redundancy.
// Named anchors are an ordered route and therefore carry more information than
// a center/span window; the motion runtime already gives them precedence. Drop
// copied window fields here so a valid anchor decision does not enter a repair
// loop merely because the model repeated values from the adjacent start example.
func normalizeDynamicGeometry(response *AssistantResponse) {
	if response.Motion == nil || len(response.Motion.Anchors) == 0 {
		return
	}
	response.Motion.CenterPercent = nil
	response.Motion.SpanPercent = nil
}

func parseAssistantResponse(raw string, patterns []PatternChoice, curation bool, currentSpeed *int) (AssistantResponse, error) {
	response, err := decodeAssistantResponse(raw)
	if err != nil {
		return AssistantResponse{}, err
	}
	preserveCurrentPatternSpeed(&response, currentSpeed)
	if err := validateAssistantResponse(&response, patterns, curation, false); err != nil {
		return AssistantResponse{}, err
	}
	return response, nil
}

func decodeAssistantResponse(raw string) (AssistantResponse, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return AssistantResponse{}, errors.New("assistant response is empty")
	}

	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var response AssistantResponse
	if err := decoder.Decode(&response); err != nil {
		return AssistantResponse{}, fmt.Errorf("assistant response must be strict JSON: %w", err)
	}
	var extra struct{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return AssistantResponse{}, errors.New("assistant response must contain exactly one JSON object")
	}
	return response, nil
}

func preserveCurrentPatternSpeed(response *AssistantResponse, currentSpeed *int) {
	if currentSpeed != nil && response.Motion != nil && strings.TrimSpace(response.Motion.PatternID) != "" &&
		response.Motion.Intensity == nil && response.Motion.SpeedPercent == nil {
		speed := *currentSpeed
		response.Motion.SpeedPercent = &speed
	}
}

// normalizePacing converts the retired model-facing intensity alias into the
// single speed_percent control. Keeping the field in the decoder lets saved
// history and responses from an older prompt remain usable, but no downstream
// motion path has to preserve two names for the same semantic value.
//
// When both arrive, the canonical field wins. This is deterministic and avoids
// dropping an otherwise valid motion decision.
func normalizePacing(motion *MotionCommand) {
	if motion == nil {
		return
	}
	if motion.SpeedPercent == nil && motion.Intensity != nil {
		speed := *motion.Intensity
		motion.SpeedPercent = &speed
	}
	motion.Intensity = nil
}

func validateAssistantResponse(response *AssistantResponse, patterns []PatternChoice, curation bool, dynamic bool) error {
	response.Reply = strings.TrimSpace(response.Reply)
	if response.Reply == "" {
		return errors.New("assistant response reply is required")
	}
	if err := validateAssistantMood(response); err != nil {
		return err
	}
	if response.Motion == nil {
		return nil
	}

	response.Motion.Action = strings.ToLower(strings.TrimSpace(response.Motion.Action))
	response.Motion.PatternID = strings.TrimSpace(response.Motion.PatternID)
	response.Motion.Area = strings.ToLower(strings.TrimSpace(response.Motion.Area))
	response.Motion.SpanProfile = strings.ToLower(strings.TrimSpace(response.Motion.SpanProfile))
	for index := range response.Motion.Anchors {
		response.Motion.Anchors[index] = strings.ToLower(strings.TrimSpace(response.Motion.Anchors[index]))
	}
	normalizePacing(response.Motion)
	switch response.Motion.Action {
	case MotionActionNone, MotionActionStart, MotionActionTarget, MotionActionUpdate, MotionActionStop:
	default:
		return fmt.Errorf("unknown motion action %q", response.Motion.Action)
	}
	if response.Motion.PatternID != "" {
		resolved, ok := resolvePatternID(response.Motion.PatternID, patterns)
		if !ok {
			return unknownPatternError{patternID: response.Motion.PatternID}
		}
		response.Motion.PatternID = resolved
	}
	if response.Motion.Area != "" && !oneOfZone(response.Motion.Area) {
		return fmt.Errorf("unknown motion area %q", response.Motion.Area)
	}
	if err := validateMotionRanges(*response.Motion); err != nil {
		return err
	}
	return validateMotionCombination(*response.Motion, curation, dynamic)
}

func validateAssistantMood(response *AssistantResponse) error {
	if response.NewMood == nil {
		return nil
	}
	if _, ok := validMood(*response.NewMood); !ok {
		return fmt.Errorf("unknown assistant mood %q", *response.NewMood)
	}
	return nil
}

func validMood(value Mood) (Mood, bool) {
	for _, allowed := range moodValues {
		if value == allowed {
			return allowed, true
		}
	}
	return "", false
}

func validateMotionRanges(command MotionCommand) error {
	if command.SpeedPercent != nil && (*command.SpeedPercent < 1 || *command.SpeedPercent > 100) {
		return errors.New("motion speed_percent must be between 1 and 100")
	}
	if err := validateDynamicMotionRanges(command); err != nil {
		return err
	}
	return validateDynamicAnchors(command.Anchors)
}

func validateDynamicMotionRanges(command MotionCommand) error {
	if err := validateDynamicGeometryRanges(command); err != nil {
		return err
	}
	if command.VariationPercent != nil && (*command.VariationPercent < 0 || *command.VariationPercent > 100) {
		return errors.New("motion variation_percent must be between 0 and 100")
	}
	if command.SegmentSeconds != nil && (*command.SegmentSeconds < 4 || *command.SegmentSeconds > 120) {
		return errors.New("motion segment_seconds must be between 4 and 120")
	}
	return nil
}

func validateDynamicGeometryRanges(command MotionCommand) error {
	if command.CenterPercent != nil && (*command.CenterPercent < 0 || *command.CenterPercent > 100) {
		return errors.New("motion center_percent must be between 0 and 100")
	}
	if command.SpanPercent != nil && (*command.SpanPercent < 20 || *command.SpanPercent > 100) {
		return errors.New("motion span_percent must be between 20 and 100")
	}
	if command.SpanMinPercent != nil && (*command.SpanMinPercent < 20 || *command.SpanMinPercent > 100) {
		return errors.New("motion span_min_percent must be between 20 and 100")
	}
	if command.SpanProfile != "" && !validDynamicSpanProfile(command.SpanProfile) {
		return fmt.Errorf("unknown motion span_profile %q", command.SpanProfile)
	}
	if command.SpanMinPercent != nil {
		if outer, ok := commandDynamicOuterSpan(command); ok && *command.SpanMinPercent > outer {
			return errors.New("motion span_min_percent cannot exceed the outer span")
		}
	}
	return nil
}

func validateDynamicAnchors(anchors []string) error {
	if len(anchors) > 0 {
		if len(anchors) < 2 || len(anchors) > 6 {
			return errors.New("motion anchors must contain between 2 and 6 names")
		}
		minimum, maximum := 100, 0
		previous := -1
		for _, name := range anchors {
			position, ok := DynamicAnchorPosition(name)
			if !ok {
				return fmt.Errorf("unknown motion anchor %q", name)
			}
			if position == previous {
				return errors.New("motion anchors cannot repeat consecutively")
			}
			previous = position
			minimum = min(minimum, position)
			maximum = max(maximum, position)
		}
		if maximum-minimum < 20 {
			return errors.New("motion anchors must span at least 20 percent of travel")
		}
	}
	return nil
}

func validateMotionCombination(command MotionCommand, curation bool, dynamic bool) error {
	if command.PatternID != "" && curation && command.SpeedPercent == nil {
		return errors.New("pattern_id requires speed_percent")
	}
	switch command.Action {
	case MotionActionNone:
		if hasMotionTargetFields(command) {
			return errors.New("motion action none cannot include target fields")
		}
	case MotionActionStop:
		if hasMotionTargetFields(command) {
			return errors.New("motion action stop cannot include target fields")
		}
	}
	if dynamic {
		return validateDynamicMotionCombination(command)
	}
	if hasDynamicMotionFields(command) || command.Action == MotionActionUpdate {
		return errors.New("dynamic motion fields are not enabled")
	}
	return nil
}

func validateDynamicMotionCombination(command MotionCommand) error {
	if command.PatternID != "" || command.Area != "" || command.Action == MotionActionTarget {
		return errors.New("dynamic motion accepts update geometry, not pattern_id, area, or target")
	}
	if len(command.Anchors) > 0 && (command.CenterPercent != nil || command.SpanPercent != nil) {
		return errors.New("dynamic motion accepts either anchors or center/span, not both")
	}
	if command.Action == MotionActionStart {
		if command.SpeedPercent == nil {
			return errors.New("dynamic motion start requires speed_percent")
		}
		if len(command.Anchors) == 0 && (command.CenterPercent == nil || command.SpanPercent == nil) {
			return errors.New("dynamic motion start requires anchors or both center_percent and span_percent")
		}
		if variableDynamicSpanProfile(command.SpanProfile) && command.SpanMinPercent == nil {
			return errors.New("a variable span_profile requires span_min_percent on dynamic motion start")
		}
	}
	if command.Action == MotionActionUpdate && command.SpeedPercent == nil && !hasDynamicMotionFields(command) {
		return errors.New("dynamic motion update requires at least one changed field")
	}
	return nil
}

// validateDynamicSpanEnvelopeState checks the effective update, including
// fields preserved from the authoritative running target. Without this check a
// variable profile with no usable floor would normalize to steady motion while
// the assistant's reply still claimed that stroke length was changing.
func validateDynamicSpanEnvelopeState(command *MotionCommand, context *MotionContext) error {
	if command == nil || (command.Action != MotionActionStart && command.Action != MotionActionUpdate) {
		return nil
	}

	profile := effectiveDynamicSpanProfile(command, context)
	if !variableDynamicSpanProfile(profile) {
		return nil
	}

	outer, hasOuter := effectiveDynamicOuterSpan(command, context)
	floor, hasFloor := effectiveDynamicSpanFloor(command, context)
	if !hasFloor {
		return errors.New("a variable span_profile requires a usable span_min_percent")
	}
	if hasOuter && floor >= outer {
		return errors.New("a variable span_profile requires span_min_percent below the outer span")
	}
	return nil
}

func effectiveDynamicSpanProfile(command *MotionCommand, context *MotionContext) string {
	profile := strings.ToLower(strings.TrimSpace(command.SpanProfile))
	if profile == "" && context != nil && context.Running {
		profile = strings.ToLower(strings.TrimSpace(context.SpanProfile))
	}
	if command.SpanMinPercent != nil &&
		(profile == "" || profile == DynamicSpanProfileSteady) {
		// A floor-only command intentionally selects ordinary organic movement
		// at the HTTP boundary, so validate that same effective target.
		return DynamicSpanProfileWander
	}
	return profile
}

func effectiveDynamicOuterSpan(command *MotionCommand, context *MotionContext) (int, bool) {
	if outer, ok := commandDynamicOuterSpan(*command); ok {
		return outer, true
	}
	if context != nil && context.Running && context.SpanPercent >= 20 {
		return context.SpanPercent, true
	}
	return 0, false
}

func effectiveDynamicSpanFloor(command *MotionCommand, context *MotionContext) (int, bool) {
	if command.SpanMinPercent != nil {
		return *command.SpanMinPercent, true
	}
	if context != nil && context.Running && context.SpanMinPercent >= 20 {
		return context.SpanMinPercent, true
	}
	return 0, false
}

func hasMotionTargetFields(command MotionCommand) bool {
	return command.PatternID != "" || command.Intensity != nil || command.SpeedPercent != nil ||
		command.Area != "" || hasDynamicMotionFields(command)
}

func hasDynamicMotionFields(command MotionCommand) bool {
	return command.CenterPercent != nil || command.SpanPercent != nil || len(command.Anchors) > 0 ||
		command.SpanMinPercent != nil || command.SpanProfile != "" ||
		command.VariationPercent != nil || command.SegmentSeconds != nil
}

func validDynamicSpanProfile(profile string) bool {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case DynamicSpanProfileSteady, DynamicSpanProfileBreathe,
		DynamicSpanProfileWander, DynamicSpanProfileContrast:
		return true
	default:
		return false
	}
}

func variableDynamicSpanProfile(profile string) bool {
	profile = strings.ToLower(strings.TrimSpace(profile))
	return profile == DynamicSpanProfileBreathe || profile == DynamicSpanProfileWander ||
		profile == DynamicSpanProfileContrast
}

func commandDynamicOuterSpan(command MotionCommand) (int, bool) {
	if command.SpanPercent != nil {
		return *command.SpanPercent, true
	}
	if len(command.Anchors) < 2 {
		return 0, false
	}
	minimum, maximum := 100, 0
	for _, name := range command.Anchors {
		position, ok := DynamicAnchorPosition(name)
		if !ok {
			return 0, false
		}
		minimum = min(minimum, position)
		maximum = max(maximum, position)
	}
	return maximum - minimum, true
}

func oneOfZone(zone string) bool {
	for _, allowed := range AreaZones() {
		if zone == allowed {
			return true
		}
	}
	return false
}

func resolvePatternID(patternID string, patterns []PatternChoice) (string, bool) {
	patternID = strings.TrimSpace(patternID)
	for _, pattern := range patterns {
		actual := strings.TrimSpace(pattern.ID)
		if actual != "" && (strings.EqualFold(actual, patternID) || strings.EqualFold(modelPatternID(actual), patternID)) {
			return actual, true
		}
	}
	return "", false
}

// modelPatternID is a stable, opaque handle used only at the LLM boundary.
// Persisted IDs often contain import filenames or obsolete pace labels; showing
// those IDs made the model choose by label instead of the reviewed geometry.
func modelPatternID(patternID string) string {
	patternID = strings.ToLower(strings.TrimSpace(patternID))
	if patternID == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(patternID))
	return fmt.Sprintf("p-%x", digest[:6])
}

func defaultPatternChoices() []PatternChoice {
	return []PatternChoice{{ID: "stroke"}, {ID: "pulse"}, {ID: "tease"}}
}
