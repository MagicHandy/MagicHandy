package funscript

import "encoding/json"

// Action is one funscript keyframe (timestamp ms + position 0–100).
type Action struct {
	At  int     `json:"at"`
	Pos float64 `json:"pos"`
}

// SourceFormat identifies how actions were loaded.
type SourceFormat string

const (
	SourceFormatCSV       SourceFormat = "csv"
	SourceFormatJSON      SourceFormat = "json"
	SourceFormatFunscript SourceFormat = "funscript"
)

// LoadedFunscript is the normalized load result from any supported source.
type LoadedFunscript struct {
	Actions      []Action
	SourceFormat SourceFormat
	Metadata     map[string]any
	ExtraFields  map[string]any
	SourcePath   string
}

// BlockFeatures holds numeric motion metrics for one block.
type BlockFeatures struct {
	StartMS               int     `json:"start_ms"`
	EndMS                 int     `json:"end_ms"`
	DurationMS            int     `json:"duration_ms"`
	MinPos                int     `json:"min_pos"`
	MaxPos                int     `json:"max_pos"`
	AvgPos                float64 `json:"avg_pos"`
	Amplitude             int     `json:"amplitude"`
	ActionCount           int     `json:"action_count"`
	AvgSpeed              float64 `json:"avg_speed"`
	MaxSpeed              float64 `json:"max_speed"`
	StrokeCount           int     `json:"stroke_count"`
	HoldRatio             float64 `json:"hold_ratio"`
	MedianStrokeAmplitude float64 `json:"median_stroke_amplitude"`
	P25StrokeAmplitude    float64 `json:"p25_stroke_amplitude"`
	P75StrokeAmplitude    float64 `json:"p75_stroke_amplitude"`
	MedianStrokeSpeedPPS  float64 `json:"median_stroke_speed_pps"`
	P40StrokeSpeedPPS     float64 `json:"p40_stroke_speed_pps"`
	P90StrokeSpeedPPS     float64 `json:"p90_stroke_speed_pps"`
	StrokeLegCount        float64 `json:"stroke_leg_count"`
}

// Classification holds motion labels for one block.
type Classification struct {
	Zone         string   `json:"zone"`
	StrokeLength string   `json:"stroke_length"`
	Speed        string   `json:"speed"`
	Rhythm       string   `json:"rhythm"`
	Intensity    float64  `json:"intensity"`
	Tags         []string `json:"tags"`
}

// BlockRecord is one library-ready motion block from ingest.
type BlockRecord struct {
	ID              string         `json:"id"`
	Features        *BlockFeatures `json:"features,omitempty"`
	StartMS         int            `json:"start_ms"`
	EndMS           int            `json:"end_ms"`
	SourceEndMS     int            `json:"source_end_ms,omitempty"`
	MotionEndMS     int            `json:"motion_end_ms,omitempty"`
	SourceTimeRange string         `json:"source_time_range,omitempty"`
	MotionTimeRange string         `json:"motion_time_range,omitempty"`
	SourceRangeSlug string         `json:"source_range_slug,omitempty"`
	DurationMS      int            `json:"duration_ms"`
	MinPos          int            `json:"min_pos"`
	MaxPos          int            `json:"max_pos"`
	AvgPos          float64        `json:"avg_pos"`
	Amplitude       int            `json:"amplitude"`
	ActionCount     int            `json:"action_count"`
	Zone            string         `json:"zone"`
	StrokeLength    string         `json:"stroke_length"`
	Speed           string         `json:"speed"`
	Rhythm          string         `json:"rhythm"`
	Intensity       float64        `json:"intensity"`
	Tags            []string       `json:"tags"`
	Actions         []StoredAction `json:"actions"`
	SemanticSummary string         `json:"semantic_summary,omitempty"`
	ContentHash     string         `json:"content_hash,omitempty"`
	IsFullScript    bool           `json:"is_full_script,omitempty"`
	QAWarnings      []string       `json:"qa_warnings,omitempty"`
}

// StoredAction is the persisted keyframe shape (integer pos when whole).
type StoredAction struct {
	At  int     `json:"at"`
	Pos float64 `json:"pos"`
}

// MarshalJSON emits integer pos when the value is whole.
func (a StoredAction) MarshalJSON() ([]byte, error) {
	pos := a.Pos
	if isWholePos(pos) {
		type alias struct {
			At  int `json:"at"`
			Pos int `json:"pos"`
		}
		return json.Marshal(alias{At: a.At, Pos: int(pos)})
	}
	type alias struct {
		At  int     `json:"at"`
		Pos float64 `json:"pos"`
	}
	return json.Marshal(alias{At: a.At, Pos: roundPos(pos)})
}

// IngestSource describes the imported file.
type IngestSource struct {
	Filename     string       `json:"filename"`
	Path         string       `json:"path"`
	Hash         string       `json:"hash"`
	ImportedAt   string       `json:"imported_at"`
	SourceFormat SourceFormat `json:"source_format"`
}

// IngestSummary aggregates ingest statistics.
type IngestSummary struct {
	ActionCount        int              `json:"action_count"`
	DurationMS         int              `json:"duration_ms"`
	BlockCount         int              `json:"block_count"`
	FullScriptBlockID  string           `json:"full_script_block_id,omitempty"`
	QARejectedSegments []map[string]any `json:"qa_rejected_segments"`
}

// IngestResult is the full pipeline output.
type IngestResult struct {
	Source            IngestSource   `json:"source"`
	Metadata          map[string]any `json:"metadata"`
	ExtraFields       map[string]any `json:"extra_fields"`
	NormalizedActions []StoredAction `json:"normalized_actions"`
	ImportedActions   []StoredAction `json:"imported_actions"`
	Summary           IngestSummary  `json:"summary"`
	Blocks            []BlockRecord  `json:"blocks"`
}

func isWholePos(pos float64) bool {
	return abs(pos-mathRound(pos)) < 1e-6
}

func mathRound(v float64) float64 {
	if v < 0 {
		return float64(int(v - 0.5))
	}
	return float64(int(v + 0.5))
}

func roundPos(pos float64) float64 {
	return float64(int(pos*10000+0.5)) / 10000
}
