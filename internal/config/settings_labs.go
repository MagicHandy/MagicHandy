package config

// LabsSettings is an opt-in workspace available in every release. Omitted
// settings from an older installation remain disabled.
type LabsSettings struct {
	Enabled bool `json:"enabled"`
}
