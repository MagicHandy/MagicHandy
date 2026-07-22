package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

const (
	// MaxMediaFunscriptBytes bounds the paired document read into memory.
	MaxMediaFunscriptBytes int64 = 16 << 20
	// MaxMediaFunscriptActions bounds parsing, API output, and engine content.
	MaxMediaFunscriptActions = motion.MaximumMediaTimelinePoints
	maxMediaTimelineMillis   = int64(30 * 24 * 60 * 60 * 1000)
	minimumPlaybackRate      = 0.25
	maximumPlaybackRate      = 4.0
)

var (
	// ErrFunscriptInvalid reports malformed or unusable paired content.
	ErrFunscriptInvalid = errors.New("paired funscript is invalid")
	// ErrFunscriptTooLarge reports a document outside media playback bounds.
	ErrFunscriptTooLarge = errors.New("paired funscript exceeds playback limits")
	// ErrFunscriptComplete reports an anchor at or beyond the final action.
	ErrFunscriptComplete = errors.New("paired funscript has ended")
)

// FunscriptAction is one validated timeline action.
type FunscriptAction struct {
	AtMillis int64 `json:"at"`
	Position int   `json:"pos"`
}

// Funscript is the bounded document returned to the browser and used to build
// the engine's clock-locked timeline. Filesystem paths never leave Catalog.
type Funscript struct {
	VideoID        string            `json:"video_id"`
	Name           string            `json:"name"`
	DurationMillis int64             `json:"duration_ms"`
	ActionCount    int               `json:"action_count"`
	Actions        []FunscriptAction `json:"actions"`
}

type rawFunscript struct {
	Actions []rawFunscriptAction `json:"actions"`
}

type rawFunscriptAction struct {
	AtMillis *int64 `json:"at"`
	Position *int   `json:"pos"`
}

// LoadFunscript opens and parses the exact-basename pair for one video.
func (c *Catalog) LoadFunscript(ctx context.Context, id string) (Funscript, error) {
	file, video, err := c.OpenFunscript(ctx, id)
	if err != nil {
		return Funscript{}, err
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return Funscript{}, fmt.Errorf("%w: inspect paired file", ErrFunscriptUnavailable)
	}
	if info.Size() > MaxMediaFunscriptBytes {
		return Funscript{}, ErrFunscriptTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxMediaFunscriptBytes+1))
	if err != nil {
		return Funscript{}, fmt.Errorf("%w: read paired file", ErrFunscriptUnavailable)
	}
	if int64(len(data)) > MaxMediaFunscriptBytes {
		return Funscript{}, ErrFunscriptTooLarge
	}
	actions, err := parseFunscriptActions(data)
	if err != nil {
		return Funscript{}, err
	}
	return Funscript{
		VideoID:        video.ID,
		Name:           video.DisplayName,
		DurationMillis: actions[len(actions)-1].AtMillis,
		ActionCount:    len(actions),
		Actions:        actions,
	}, nil
}

func parseFunscriptActions(data []byte) ([]FunscriptAction, error) {
	var document rawFunscript
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("%w: malformed JSON", ErrFunscriptInvalid)
	}
	if len(document.Actions) < 2 {
		return nil, fmt.Errorf("%w: at least two actions are required", ErrFunscriptInvalid)
	}
	if len(document.Actions) > MaxMediaFunscriptActions {
		return nil, ErrFunscriptTooLarge
	}

	actions := make([]FunscriptAction, 0, len(document.Actions)+1)
	for index, raw := range document.Actions {
		if raw.AtMillis == nil || raw.Position == nil {
			return nil, fmt.Errorf("%w: action %d requires at and pos", ErrFunscriptInvalid, index)
		}
		if *raw.AtMillis < 0 || *raw.AtMillis > maxMediaTimelineMillis {
			return nil, fmt.Errorf("%w: action %d time is outside playback bounds", ErrFunscriptInvalid, index)
		}
		if *raw.Position < 0 || *raw.Position > 100 {
			return nil, fmt.Errorf("%w: action %d position must be between 0 and 100", ErrFunscriptInvalid, index)
		}
		actions = append(actions, FunscriptAction{AtMillis: *raw.AtMillis, Position: *raw.Position})
	}
	sort.SliceStable(actions, func(left, right int) bool { return actions[left].AtMillis < actions[right].AtMillis })
	actions = deduplicateFunscriptActions(actions)
	if len(actions) < 2 || actions[len(actions)-1].AtMillis <= 0 {
		return nil, fmt.Errorf("%w: actions require distinct positive times", ErrFunscriptInvalid)
	}
	if actions[0].AtMillis > 0 {
		if len(actions) == MaxMediaFunscriptActions {
			return nil, ErrFunscriptTooLarge
		}
		actions = append([]FunscriptAction{{Position: actions[0].Position}}, actions...)
	}
	return actions, nil
}

func deduplicateFunscriptActions(actions []FunscriptAction) []FunscriptAction {
	result := actions[:0]
	for _, action := range actions {
		if len(result) > 0 && result[len(result)-1].AtMillis == action.AtMillis {
			result[len(result)-1] = action
			continue
		}
		result = append(result, action)
	}
	return result
}

// TimelineFrom slices and rate-scales the authored timeline at a video-clock
// anchor. The first point is linearly interpolated at the exact media time.
func (f Funscript) TimelineFrom(mediaTimeMillis int64, playbackRate float64) (motion.MediaTimelineDefinition, error) {
	if len(f.Actions) < 2 {
		return motion.MediaTimelineDefinition{}, ErrFunscriptInvalid
	}
	if playbackRate < minimumPlaybackRate || playbackRate > maximumPlaybackRate || math.IsNaN(playbackRate) || math.IsInf(playbackRate, 0) {
		return motion.MediaTimelineDefinition{}, errors.New("video playback rate must be between 0.25 and 4")
	}
	mediaTimeMillis = max(int64(0), mediaTimeMillis)
	if mediaTimeMillis >= f.DurationMillis {
		return motion.MediaTimelineDefinition{}, ErrFunscriptComplete
	}

	points := make([]motion.CurvePoint, 0, len(f.Actions))
	points = append(points, motion.CurvePoint{PositionPercent: f.positionAt(mediaTimeMillis)})
	for _, action := range f.Actions {
		if action.AtMillis <= mediaTimeMillis {
			continue
		}
		at := int64(math.Round(float64(action.AtMillis-mediaTimeMillis) / playbackRate))
		if at <= points[len(points)-1].TimeMillis {
			points[len(points)-1].PositionPercent = float64(action.Position)
			continue
		}
		points = append(points, motion.CurvePoint{TimeMillis: at, PositionPercent: float64(action.Position)})
	}
	if len(points) < 2 || points[len(points)-1].TimeMillis <= 0 {
		return motion.MediaTimelineDefinition{}, ErrFunscriptComplete
	}
	return motion.NormalizeMediaTimelineDefinition(motion.MediaTimelineDefinition{
		ID:             f.VideoID,
		Name:           f.Name,
		DurationMillis: points[len(points)-1].TimeMillis,
		Points:         points,
	})
}

func (f Funscript) positionAt(at int64) float64 {
	index := sort.Search(len(f.Actions), func(index int) bool { return f.Actions[index].AtMillis >= at })
	if index == 0 {
		return float64(f.Actions[0].Position)
	}
	if index >= len(f.Actions) {
		return float64(f.Actions[len(f.Actions)-1].Position)
	}
	right := f.Actions[index]
	if right.AtMillis == at {
		return float64(right.Position)
	}
	left := f.Actions[index-1]
	fraction := float64(at-left.AtMillis) / float64(right.AtMillis-left.AtMillis)
	return float64(left.Position) + float64(right.Position-left.Position)*fraction
}
