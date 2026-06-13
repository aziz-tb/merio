// Package preset loads and resolves AI presets (model + tuning knobs per
// Purpose) from config/ai_presets.json. Reload requires app restart in v1.
package preset

// Preset is one entry in config/ai_presets.json's "presets" array.
//
// TopP and TopK are pointers so a NULL in the file means "use the provider
// default," distinct from "set this knob to 0."
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
	Visibility         []string `json:"visibility,omitempty"`
}
