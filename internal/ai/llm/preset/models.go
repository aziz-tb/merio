// Package preset loads and resolves AI presets from config/ai_presets.json.
// Presets are platform-tuning decisions (which model + temperature +
// max_tokens + caching config to use for a given Purpose), authored in a
// JSON file in version control rather than the database. Reload requires
// app restart in v1.
package preset

// Preset is one entry in config/ai_presets.json's "presets" array. Field
// tags mirror the JSON key names exactly so the file is the human-edit
// surface.
//
// Pointer fields (TopP, TopK) are deliberate: a NULL in the file means
// "use the provider default," distinct from "set this knob to 0." Empty
// StopSequences means "no stop sequences" — same behavior either way, so
// a slice (nil-vs-empty) is fine.
type Preset struct {
	Slug               string   `json:"slug"`
	Name               string   `json:"name"`
	Description        string   `json:"description,omitempty"`
	Provider           string   `json:"provider"` // provider id: anthropic, openai, azure
	Model              string   `json:"model"`    // must exist in catalog
	Temperature        float64  `json:"temperature"`
	MaxTokens          int      `json:"max_tokens"`
	TopP               *float64 `json:"top_p,omitempty"`
	TopK               *int     `json:"top_k,omitempty"`
	StopSequences      []string `json:"stop_sequences,omitempty"`
	CachingEnabled     bool     `json:"caching_enabled"`
	CachingExtendedTTL bool     `json:"caching_extended_ttl"`
	Enabled            bool     `json:"enabled"`
	// Visibility lists the UI preset-select use cases this preset shows up
	// in (each value is a known llm.Purpose). It is decoupled from
	// purpose_mappings: it controls which presets a user may *pick* in a
	// given select, not which preset is the platform *default* for a
	// purpose. Empty/omitted means the preset appears in no select.
	Visibility []string `json:"visibility,omitempty"`
}
