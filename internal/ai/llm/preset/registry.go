package preset

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/aziz-tb/merio/internal/ai/llm/catalog"
)

// Registry is the in-memory set of presets, purpose→preset mapping,
// and platform-level provider API keys, loaded from
// config/ai_presets.json. After Load it's read-only (concurrent
// Resolve / Has / ProviderKey calls are safe without locks).
//
// Why provider keys live here: the Anthropic API key is a platform
// concern, not a tenant-configurable thing — same store as the rest of
// the AI runtime knobs that engineering owns. Operators set it in
// config/ai_presets.json once at deployment; per-tenant key management
// is deferred until there's a real second-provider story.
type Registry struct {
	presets         map[string]Preset // slug → preset
	purposeMappings map[llm.Purpose]string
	providerKeys    map[string]string // provider name → API key
	order           []string          // slugs in file order, for deterministic listing
}

// presetsFile mirrors config/ai_presets.json's top-level shape.
type presetsFile struct {
	Comment         string            `json:"_comment,omitempty"`
	Presets         []Preset          `json:"presets"`
	PurposeMappings map[string]string `json:"purpose_mappings"`
	// ProviderKeys is an optional map of provider name → API key.
	// Anthropic's key lives here in v1. Empty / missing → the runner
	// returns 503 at run time with a "configure the key" message.
	ProviderKeys map[string]string `json:"provider_keys,omitempty"`
}

// Load reads config/ai_presets.json at path and validates it against the
// supplied catalog. Validation is strict — any failure is returned and
// the caller is expected to fail startup loudly:
//
//   - presets is non-empty
//   - every preset has a non-empty unique slug
//   - every preset's provider is "anthropic" (v1)
//   - every preset's model exists in the catalog
//   - temperature in [0, 2]; max_tokens > 0 and ≤ catalog max_output_tokens
//   - caching_extended_ttl only true when the model supports it (catalog flag)
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
		if p.Provider != "anthropic" {
			return nil, fmt.Errorf("preset: %s presets[%q] provider %q unsupported (v1: anthropic only)", source, p.Slug, p.Provider)
		}
		if p.Model == "" {
			return nil, fmt.Errorf("preset: %s presets[%q] has empty model", source, p.Slug)
		}
		if cat != nil {
			m, ok := cat.Get(p.Model)
			if !ok {
				return nil, fmt.Errorf("preset: %s presets[%q] model %q not in catalog (known: %v)", source, p.Slug, p.Model, cat.IDs())
			}
			if p.MaxTokens > 0 && p.MaxTokens > m.MaxOutputTokens {
				return nil, fmt.Errorf("preset: %s presets[%q] max_tokens=%d exceeds model %q max_output_tokens=%d", source, p.Slug, p.MaxTokens, p.Model, m.MaxOutputTokens)
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

	// Provider keys — pass through, normalize to lowercase keys so
	// "Anthropic" and "anthropic" both work in the config file.
	keys := make(map[string]string, len(f.ProviderKeys))
	for k, v := range f.ProviderKeys {
		keys[normalizeProviderName(k)] = v
	}

	return &Registry{
		presets:         presets,
		purposeMappings: mappings,
		providerKeys:    keys,
		order:           order,
	}, nil
}

// normalizeProviderName lowercases the provider name so lookups are
// case-insensitive (mirrors the SDK convention).
func normalizeProviderName(name string) string {
	out := make([]byte, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}

// ProviderKey returns the API key for the named provider, or "" when
// unconfigured. Used by initAnthropicProvider at boot time; the runner
// returns 503 when this is empty.
func (r *Registry) ProviderKey(provider string) string {
	if r == nil {
		return ""
	}
	return r.providerKeys[normalizeProviderName(provider)]
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

// PurposeMapping returns the configured slug for purpose, ignoring any
// agent override. Useful for diagnostics and admin pages.
func (r *Registry) PurposeMapping(purpose llm.Purpose) (string, bool) {
	if r == nil {
		return "", false
	}
	slug, ok := r.purposeMappings[purpose]
	return slug, ok
}

func knownPurposeStrings() []string {
	out := make([]string, 0, len(llm.KnownPurposes))
	for _, p := range llm.KnownPurposes {
		out = append(out, string(p))
	}
	sort.Strings(out)
	return out
}
