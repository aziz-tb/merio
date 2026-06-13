// Package catalog loads model catalog JSON files into an in-memory Catalog
// used by the preset registry to validate that every preset's provider/model
// pair references a known model. The catalog is immutable after load; a typo
// in a model id crashes startup rather than silently falling back.
package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// Tier classifies models into rough cost/quality bands for ops UI hints.
// Not load-bearing in routing — the preset itself names the model.
type Tier string

// The model cost/quality tiers.
const (
	TierHaiku  Tier = "haiku"
	TierSonnet Tier = "sonnet"
	TierOpus   Tier = "opus"
)

// Model is one entry in a model catalog JSON file.
type Model struct {
	ID                      string `json:"id"`
	Provider                string `json:"provider"`
	DisplayName             string `json:"display_name"`
	ContextWindow           int    `json:"context_window"`
	MaxOutputTokens         int    `json:"max_output_tokens"`
	SupportsCaching         bool   `json:"supports_caching"`
	SupportsExtendedCaching bool   `json:"supports_extended_caching"`
	// SupportsThinking means the model emits provider-visible thinking blocks
	// that must be preserved and round-tripped. Claude-specific today; distinct
	// from OpenAI reasoning-token controls.
	SupportsThinking bool `json:"supports_thinking"`
	// IsReasoningModel means the translator must use reasoning-model request
	// semantics: max_completion_tokens instead of max_tokens, no temperature
	// unless the provider allows it (OpenAI o-series, GPT reasoning modes). Does
	// not imply visible thinking blocks.
	IsReasoningModel bool    `json:"is_reasoning_model"`
	Tier             Tier    `json:"tier"`
	Pricing          Pricing `json:"pricing"`
}

// Pricing carries the model's per-million-token rates. Long-context premium
// tiers (e.g. Anthropic prompts >200k tokens) are NOT modeled; the runtime
// applies the base rate uniformly.
type Pricing struct {
	InputPerMillionTokensUSD       float64 `json:"input_per_million_tokens_usd"`
	OutputPerMillionTokensUSD      float64 `json:"output_per_million_tokens_usd"`
	CachedInputPerMillionTokensUSD float64 `json:"cached_input_per_million_tokens_usd"`
	Currency                       string  `json:"currency"`
	EffectiveFrom                  string  `json:"effective_from"`
}

// Catalog is the loaded set of models, keyed by provider:id for O(1) lookup.
// Immutable after Load; source-file edits need a restart to take effect.
type Catalog struct {
	models map[string]Model
	all    []Model
}

// catalogFile is the JSON file's top-level shape; the _comment key is ignored.
type catalogFile struct {
	Comment string  `json:"_comment,omitempty"`
	Models  []Model `json:"models"`
}

// Load reads one catalog file at path. It stays as a compatibility wrapper for
// tests and single-catalog deployments; production uses LoadMulti.
func Load(path string) (*Catalog, error) {
	if path == "" {
		return &Catalog{models: map[string]Model{}, all: []Model{}}, nil
	}
	return LoadMulti(map[string]string{"anthropic": path})
}

// LoadMulti reads and merges provider-specific catalog files. The paths map is
// keyed by provider name ("anthropic", "openai", "azure"); every model in a
// file must carry the same provider value. Duplicate provider/model pairs are
// rejected.
func LoadMulti(paths map[string]string) (*Catalog, error) {
	if len(paths) == 0 {
		return &Catalog{models: map[string]Model{}, all: []Model{}}, nil
	}

	providers := make([]string, 0, len(paths))
	for provider := range paths {
		providers = append(providers, provider)
	}
	sort.Strings(providers)

	models := make(map[string]Model)
	all := make([]Model, 0)
	for _, provider := range providers {
		path := paths[provider]
		if path == "" {
			continue
		}
		fileModels, err := loadFile(path, provider)
		if err != nil {
			return nil, err
		}
		for _, m := range fileModels {
			k := key(m.Provider, m.ID)
			if _, dup := models[k]; dup {
				return nil, fmt.Errorf("catalog: duplicate model %q for provider %q", m.ID, m.Provider)
			}
			models[k] = m
			all = append(all, m)
		}
	}
	return &Catalog{models: models, all: all}, nil
}

func loadFile(path string, provider string) ([]Model, error) {
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

	for i, m := range f.Models {
		if m.ID == "" {
			return nil, fmt.Errorf("catalog: %s models[%d].id is empty", path, i)
		}
		if m.Provider == "" {
			return nil, fmt.Errorf("catalog: %s model %q provider is empty", path, m.ID)
		}
		if provider != "" && m.Provider != provider {
			return nil, fmt.Errorf("catalog: %s model %q provider=%q does not match file provider %q", path, m.ID, m.Provider, provider)
		}
		// Pricing must be non-zero: ai_usage cost columns depend on it, and a
		// missing entry would silently log $0 costs. Fail loudly at boot.
		if m.Pricing.InputPerMillionTokensUSD <= 0 ||
			m.Pricing.OutputPerMillionTokensUSD <= 0 {
			return nil, fmt.Errorf(
				"catalog: %s model %q is missing pricing (input/output per-million USD required)",
				path, m.ID)
		}
		if m.Pricing.Currency == "" {
			return nil, fmt.Errorf("catalog: %s model %q pricing.currency is empty", path, m.ID)
		}
	}
	return f.Models, nil
}

// Get returns the catalog entry for provider/modelID, or zero + false.
func (c *Catalog) Get(provider, modelID string) (Model, bool) {
	if c == nil {
		return Model{}, false
	}
	m, ok := c.models[key(provider, modelID)]
	return m, ok
}

// All returns the catalog's models in file order as a fresh copy safe to mutate.
func (c *Catalog) All() []Model {
	if c == nil {
		return nil
	}
	out := make([]Model, len(c.all))
	copy(out, c.all)
	return out
}

// IDs returns provider-qualified model IDs in file order.
func (c *Catalog) IDs() []string {
	if c == nil {
		return nil
	}
	out := make([]string, 0, len(c.all))
	for _, m := range c.all {
		out = append(out, key(m.Provider, m.ID))
	}
	return out
}

func key(provider, modelID string) string {
	return provider + ":" + modelID
}
