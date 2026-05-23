// Package catalog loads config/anthropic_models.json into an in-memory
// Catalog used by the preset registry to validate that every preset's
// model field references a known model.
//
// The catalog is data, not config in the koanf sense: edits require a
// version-control PR + restart, and a typo in a model id should crash
// startup rather than silently fall back. v1 ships with one provider
// (Anthropic); when Bedrock or Azure land, this package gains a second
// catalog file or a multi-provider catalog shape.
package catalog

import (
	"encoding/json"
	"fmt"
	"os"
)

// Tier classifies models into rough cost/quality bands for ops UI hints.
// Not load-bearing in routing — the preset itself names the model.
type Tier string

const (
	TierHaiku  Tier = "haiku"
	TierSonnet Tier = "sonnet"
	TierOpus   Tier = "opus"
)

// Model is one entry in config/anthropic_models.json. Field tags mirror
// the JSON key names exactly so the file remains the human-edit surface.
type Model struct {
	ID                      string  `json:"id"`
	DisplayName             string  `json:"display_name"`
	ContextWindow           int     `json:"context_window"`
	MaxOutputTokens         int     `json:"max_output_tokens"`
	SupportsCaching         bool    `json:"supports_caching"`
	SupportsExtendedCaching bool    `json:"supports_extended_caching"`
	SupportsThinking        bool    `json:"supports_thinking"`
	Tier                    Tier    `json:"tier"`
	Pricing                 Pricing `json:"pricing"`
}

// Pricing carries the model's per-million-token rates. Currency is USD
// in v1; the field exists so a future multi-currency rollout has the
// shape ready. EffectiveFrom is the date the rate took effect — kept
// for traceability when Anthropic ships price changes. Long-context
// premium tiers (Anthropic charges more for prompts >200k tokens) are
// NOT modeled in v1 — the runtime applies the base rate uniformly and
// a future "pricing tier per call" extension can fold them in.
type Pricing struct {
	InputPerMillionTokensUSD       float64 `json:"input_per_million_tokens_usd"`
	OutputPerMillionTokensUSD      float64 `json:"output_per_million_tokens_usd"`
	CachedInputPerMillionTokensUSD float64 `json:"cached_input_per_million_tokens_usd"`
	Currency                       string  `json:"currency"`
	EffectiveFrom                  string  `json:"effective_from"`
}

// Catalog is the loaded set of models, keyed by ID for O(1) lookup. It's
// immutable after Load — any edits to the source file require an app
// restart to take effect (v1 limitation).
type Catalog struct {
	models map[string]Model
	all    []Model
}

// catalogFile mirrors the JSON file's top-level shape. The leading
// underscore key is informational and ignored.
type catalogFile struct {
	Comment string  `json:"_comment,omitempty"`
	Models  []Model `json:"models"`
}

// Load reads the catalog file at path. Returns a populated Catalog or a
// descriptive error on parse / shape failure. Callers that pass an empty
// path get an empty Catalog (Has returns false for everything) — useful
// in tests that don't exercise model validation.
func Load(path string) (*Catalog, error) {
	if path == "" {
		return &Catalog{models: map[string]Model{}, all: []Model{}}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("catalog: reading %s: %w", path, err)
	}
	var f catalogFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("catalog: parsing %s: %w", path, err)
	}
	if len(f.Models) == 0 {
		return nil, fmt.Errorf("catalog: %s contains no models", path)
	}

	models := make(map[string]Model, len(f.Models))
	for i, m := range f.Models {
		if m.ID == "" {
			return nil, fmt.Errorf("catalog: %s models[%d].id is empty", path, i)
		}
		if _, dup := models[m.ID]; dup {
			return nil, fmt.Errorf("catalog: %s contains duplicate model id %q", path, m.ID)
		}
		// Pricing must be present + non-zero for every model. ai_usage
		// cost columns depend on it; a missing entry would silently log
		// $0 costs. Fail loudly at boot instead.
		if m.Pricing.InputPerMillionTokensUSD <= 0 ||
			m.Pricing.OutputPerMillionTokensUSD <= 0 {
			return nil, fmt.Errorf(
				"catalog: %s model %q is missing pricing (input/output per-million USD required)",
				path, m.ID)
		}
		if m.Pricing.Currency == "" {
			return nil, fmt.Errorf("catalog: %s model %q pricing.currency is empty", path, m.ID)
		}
		models[m.ID] = m
	}
	return &Catalog{models: models, all: f.Models}, nil
}

// Has reports whether the catalog contains a model with the given id.
func (c *Catalog) Has(modelID string) bool {
	if c == nil {
		return false
	}
	_, ok := c.models[modelID]
	return ok
}

// Get returns the catalog entry for modelID, or zero + false.
func (c *Catalog) Get(modelID string) (Model, bool) {
	if c == nil {
		return Model{}, false
	}
	m, ok := c.models[modelID]
	return m, ok
}

// All returns the catalog's models in their original file order. The
// returned slice is a fresh copy; callers can mutate it without affecting
// the catalog.
func (c *Catalog) All() []Model {
	if c == nil {
		return nil
	}
	out := make([]Model, len(c.all))
	copy(out, c.all)
	return out
}

// IDs returns the model IDs in the catalog (file order).
func (c *Catalog) IDs() []string {
	if c == nil {
		return nil
	}
	out := make([]string, 0, len(c.all))
	for _, m := range c.all {
		out = append(out, m.ID)
	}
	return out
}
