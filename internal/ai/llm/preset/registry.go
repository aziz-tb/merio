package preset

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/aziz-tb/merio/internal/ai/llm/catalog"
)

// Registry is the in-memory set of presets and purpose→preset mappings loaded
// from config/ai_presets.json. After Load it's read-only (concurrent Resolve /
// Has calls are safe without locks). Provider API keys are resolved by
// secret.Resolver, not by the preset registry.
type Registry struct {
	presets         map[string]Preset // slug → preset
	purposeMappings map[llm.Purpose]string
	order           []string // slugs in file order, for deterministic listing
}

// presetsFile mirrors config/ai_presets.json's top-level shape.
type presetsFile struct {
	Comment         string            `json:"_comment,omitempty"`
	Presets         []Preset          `json:"presets"`
	PurposeMappings map[string]string `json:"purpose_mappings"`
}

var allowedProviders = map[string]struct{}{
	"anthropic": {},
	"openai":    {},
	"azure":     {},
}

// Load reads config/ai_presets.json at path and validates it against the
// supplied catalog. Validation is strict — any failure is returned and
// the caller is expected to fail startup loudly:
//
//   - presets is non-empty
//   - every preset has a non-empty unique slug
//   - every preset's provider is one of anthropic, openai, azure
//   - every preset's model exists in the catalog
//   - temperature in [0, 2]; max_tokens > 0 and ≤ catalog max_output_tokens
//   - caching_enabled only true when the model supports prompt caching
//   - caching_extended_ttl only true when the model supports it (catalog flag)
//   - every visibility entry is a known llm.Purpose
//   - purpose_mappings covers exactly llm.KnownPurposes
//   - every mapped slug exists in presets
//   - every mapped slug references an enabled preset
//
// If catalog is nil, model-existence checks are skipped (useful in tests).
func Load(path string, cat *catalog.Catalog) (*Registry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("preset: reading %s: %w", path, err)
	}
	var f presetsFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("preset: parsing %s: %w", path, err)
	}
	return build(path, f, cat)
}

func build(source string, f presetsFile, cat *catalog.Catalog) (*Registry, error) {
	if len(f.Presets) == 0 {
		return nil, fmt.Errorf("preset: %s has empty presets[]", source)
	}

	presets := make(map[string]Preset, len(f.Presets))
	order := make([]string, 0, len(f.Presets))
	for i, p := range f.Presets {
		if p.Slug == "" {
			return nil, fmt.Errorf("preset: %s presets[%d] has empty slug", source, i)
		}
		if _, dup := presets[p.Slug]; dup {
			return nil, fmt.Errorf("preset: %s contains duplicate slug %q", source, p.Slug)
		}
		if p.Name == "" {
			return nil, fmt.Errorf("preset: %s presets[%q] has empty name", source, p.Slug)
		}
		if _, ok := allowedProviders[p.Provider]; !ok {
			return nil, fmt.Errorf("preset: %s presets[%q] provider %q unsupported (allowed: anthropic, openai, azure)", source, p.Slug, p.Provider)
		}
		if p.Model == "" {
			return nil, fmt.Errorf("preset: %s presets[%q] has empty model", source, p.Slug)
		}
		if cat != nil {
			m, ok := cat.Get(p.Provider, p.Model)
			if !ok {
				return nil, fmt.Errorf("preset: %s presets[%q] provider/model %q/%q not in catalog (known: %v)", source, p.Slug, p.Provider, p.Model, cat.IDs())
			}
			if p.MaxTokens > 0 && p.MaxTokens > m.MaxOutputTokens {
				return nil, fmt.Errorf("preset: %s presets[%q] max_tokens=%d exceeds model %q max_output_tokens=%d", source, p.Slug, p.MaxTokens, p.Model, m.MaxOutputTokens)
			}
			if p.CachingEnabled && !m.SupportsCaching {
				return nil, fmt.Errorf("preset: %s presets[%q] caching_enabled=true but model %q does not support prompt caching", source, p.Slug, p.Model)
			}
			if p.CachingExtendedTTL && !m.SupportsExtendedCaching {
				return nil, fmt.Errorf("preset: %s presets[%q] caching_extended_ttl=true but model %q does not support extended caching", source, p.Slug, p.Model)
			}
		}
		if p.Temperature < 0 || p.Temperature > 2 {
			return nil, fmt.Errorf("preset: %s presets[%q] temperature %g out of range [0, 2]", source, p.Slug, p.Temperature)
		}
		if p.MaxTokens <= 0 {
			return nil, fmt.Errorf("preset: %s presets[%q] max_tokens must be > 0 (got %d)", source, p.Slug, p.MaxTokens)
		}
		for _, v := range p.Visibility {
			if !llm.IsKnownPurpose(v) {
				return nil, fmt.Errorf("preset: %s presets[%q] visibility %q is not a known purpose (known: %v)", source, p.Slug, v, knownPurposeStrings())
			}
		}
		presets[p.Slug] = p
		order = append(order, p.Slug)
	}

	// Validate purpose_mappings: keys cover exactly llm.KnownPurposes,
	// values point to enabled presets.
	if len(f.PurposeMappings) == 0 {
		return nil, fmt.Errorf("preset: %s purpose_mappings is empty", source)
	}
	for keyRaw, slug := range f.PurposeMappings {
		if !llm.IsKnownPurpose(keyRaw) {
			return nil, fmt.Errorf("preset: %s purpose_mappings key %q is not a known purpose (known: %v)", source, keyRaw, knownPurposeStrings())
		}
		p, ok := presets[slug]
		if !ok {
			return nil, fmt.Errorf("preset: %s purpose_mappings[%q] = %q not found in presets[]", source, keyRaw, slug)
		}
		if !p.Enabled {
			return nil, fmt.Errorf("preset: %s purpose_mappings[%q] = %q references a disabled preset", source, keyRaw, slug)
		}
	}
	for _, p := range llm.KnownPurposes {
		// Non-LLM purposes (embedding, future reranking) are typed for
		// ai_usage.purpose safety but don't resolve to a chat preset.
		// They are exempt from the "every known purpose has a mapping"
		// check.
		if llm.IsNonLLMPurpose(p) {
			continue
		}
		if _, ok := f.PurposeMappings[string(p)]; !ok {
			return nil, fmt.Errorf("preset: %s purpose_mappings missing entry for purpose %q", source, p)
		}
	}

	mappings := make(map[llm.Purpose]string, len(f.PurposeMappings))
	for keyRaw, slug := range f.PurposeMappings {
		mappings[llm.Purpose(keyRaw)] = slug
	}

	return &Registry{
		presets:         presets,
		purposeMappings: mappings,
		order:           order,
	}, nil
}

// Resolve returns the preset to use for the given purpose, applying the
// agent's per-purpose overrides on top of the global mapping. agentOverrides
// may be nil for non-agent contexts (rewrite UI, ad-hoc test programs).
//
// Slugs in agentOverrides are validated at agent save time (chunk 1's
// TODO landed here in chunk 2 — see Has + handler-side check). At Resolve
// time, an unknown override slug returns ErrPresetNotFound rather than
// silently falling back; this surfaces config/data drift loudly.
func (r *Registry) Resolve(purpose llm.Purpose, agentOverrides map[string]string) (Preset, error) {
	if r == nil {
		return Preset{}, fmt.Errorf("preset: registry is nil")
	}
	if !llm.IsKnownPurpose(string(purpose)) {
		return Preset{}, fmt.Errorf("preset: unknown purpose %q", purpose)
	}
	if slug, ok := agentOverrides[string(purpose)]; ok && slug != "" {
		p, found := r.presets[slug]
		if !found {
			return Preset{}, fmt.Errorf("preset: agent override %q=%q: %w", purpose, slug, llm.ErrPresetNotFound)
		}
		if !p.Enabled {
			return Preset{}, fmt.Errorf("preset: agent override %q=%q references disabled preset", purpose, slug)
		}
		return p, nil
	}
	slug, ok := r.purposeMappings[purpose]
	if !ok {
		// Unreachable if Load validated correctly, but defensive.
		return Preset{}, fmt.Errorf("preset: no default mapping for purpose %q", purpose)
	}
	p, found := r.presets[slug]
	if !found {
		return Preset{}, fmt.Errorf("preset: default mapping for %q points at missing preset %q", purpose, slug)
	}
	return p, nil
}

// Get returns the preset with the given slug, or zero + false. Used by
// agent save-time validation and by ad-hoc callers (test programs).
func (r *Registry) Get(slug string) (Preset, bool) {
	if r == nil {
		return Preset{}, false
	}
	p, ok := r.presets[slug]
	return p, ok
}

// Has reports whether a preset with the given slug exists in the
// registry (regardless of enabled state). Used at agent save time to
// reject unknown override slugs with a 400.
func (r *Registry) Has(slug string) bool {
	if r == nil {
		return false
	}
	_, ok := r.presets[slug]
	return ok
}

// List returns presets in file order. Read-only; safe to call concurrently.
func (r *Registry) List() []Preset {
	if r == nil {
		return nil
	}
	out := make([]Preset, 0, len(r.order))
	for _, slug := range r.order {
		out = append(out, r.presets[slug])
	}
	return out
}

// PurposesForPreset returns the configured purpose names that currently map to
// slug. A preset can serve multiple purposes, so callers should treat the
// result as metadata rather than a single category.
func (r *Registry) PurposesForPreset(slug string) []llm.Purpose {
	if r == nil {
		return nil
	}
	out := make([]llm.Purpose, 0)
	for _, purpose := range llm.KnownPurposes {
		if r.purposeMappings[purpose] == slug {
			out = append(out, purpose)
		}
	}
	return out
}

// Slugs returns all preset slugs in file order. Helper for error messages
// that want to surface the valid options.
func (r *Registry) Slugs() []string {
	if r == nil {
		return nil
	}
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

func knownPurposeStrings() []string {
	out := make([]string, 0, len(llm.KnownPurposes))
	for _, p := range llm.KnownPurposes {
		out = append(out, string(p))
	}
	sort.Strings(out)
	return out
}
