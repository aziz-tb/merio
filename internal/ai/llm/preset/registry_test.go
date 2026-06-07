package preset

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/aziz-tb/merio/internal/ai/llm/catalog"
	"github.com/stretchr/testify/require"
)

func TestBuildRejectsCachingPresetForNonCachingModel(t *testing.T) {
	cat := testCatalog(t, `{
  "models": [{
    "id": "no-cache",
    "provider": "openai",
    "display_name": "No Cache",
    "context_window": 1000,
    "max_output_tokens": 100,
    "supports_caching": false,
    "supports_extended_caching": false,
    "supports_thinking": false,
    "is_reasoning_model": false,
    "tier": "haiku",
    "pricing": {
      "input_per_million_tokens_usd": 1,
      "output_per_million_tokens_usd": 2,
      "cached_input_per_million_tokens_usd": 0.1,
      "currency": "USD",
      "effective_from": "2026-01-01"
    }
  }]
}`)

	_, err := build("test", presetsFile{
		Presets: []Preset{{
			Slug:           "rewrite-default",
			Name:           "Rewrite",
			Provider:       "openai",
			Model:          "no-cache",
			Temperature:    0.2,
			MaxTokens:      100,
			CachingEnabled: true,
			Enabled:        true,
		}},
		PurposeMappings: allPresetMappings("rewrite-default"),
	}, cat)
	require.ErrorContains(t, err, `caching_enabled=true but model "no-cache" does not support prompt caching`)
}

func TestRepositoryAIConfigLoads(t *testing.T) {
	cat, err := catalog.LoadMulti(map[string]string{
		"anthropic": repoConfigPath("anthropic_models.json"),
		"openai":    repoConfigPath("openai_models.json"),
		"azure":     repoConfigPath("azure_models.json"),
	})
	require.NoError(t, err)

	_, err = Load(repoConfigPath("ai_presets.json"), cat)
	require.NoError(t, err)
}

func testCatalog(t *testing.T, body string) *catalog.Catalog {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "openai_models.json")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	cat, err := catalog.LoadMulti(map[string]string{"openai": path})
	require.NoError(t, err)
	return cat
}

func repoConfigPath(name string) string {
	return filepath.Join("..", "..", "..", "..", "config", name)
}

func allPresetMappings(slug string) map[string]string {
	out := make(map[string]string)
	for _, p := range llm.KnownPurposes {
		if llm.IsNonLLMPurpose(p) {
			continue
		}
		out[string(p)] = slug
	}
	return out
}
