package catalog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadMultiMergesProviderCatalogs(t *testing.T) {
	anthropicPath := writeCatalog(t, "anthropic", `{
  "models": [{
    "id": "same-model",
    "provider": "anthropic",
    "display_name": "Anthropic Model",
    "context_window": 1000,
    "max_output_tokens": 100,
    "supports_caching": true,
    "supports_extended_caching": true,
    "supports_thinking": true,
    "is_reasoning_model": false,
    "tier": "sonnet",
    "pricing": {
      "input_per_million_tokens_usd": 1,
      "output_per_million_tokens_usd": 2,
      "cached_input_per_million_tokens_usd": 0.1,
      "currency": "USD",
      "effective_from": "2026-01-01"
    }
  }]
}`)
	openaiPath := writeCatalog(t, "openai", `{
  "models": [{
    "id": "same-model",
    "provider": "openai",
    "display_name": "OpenAI Model",
    "context_window": 2000,
    "max_output_tokens": 200,
    "supports_caching": true,
    "supports_extended_caching": false,
    "supports_thinking": true,
    "is_reasoning_model": true,
    "tier": "mini",
    "pricing": {
      "input_per_million_tokens_usd": 3,
      "output_per_million_tokens_usd": 4,
      "cached_input_per_million_tokens_usd": 0.3,
      "currency": "USD",
      "effective_from": "2026-01-01"
    }
  }]
}`)

	cat, err := LoadMulti(map[string]string{
		"anthropic": anthropicPath,
		"openai":    openaiPath,
	})
	require.NoError(t, err)

	anthropicModel, ok := cat.Get("anthropic", "same-model")
	require.True(t, ok)
	require.Equal(t, "Anthropic Model", anthropicModel.DisplayName)
	require.False(t, anthropicModel.IsReasoningModel)

	openaiModel, ok := cat.Get("openai", "same-model")
	require.True(t, ok)
	require.Equal(t, "OpenAI Model", openaiModel.DisplayName)
	require.True(t, openaiModel.IsReasoningModel)
	require.Equal(t, []string{"anthropic:same-model", "openai:same-model"}, cat.IDs())
}

func TestLoadMultiRejectsDuplicateProviderModel(t *testing.T) {
	path := writeCatalog(t, "openai", `{
  "models": [
    {
      "id": "dup",
      "provider": "openai",
      "display_name": "One",
      "context_window": 1,
      "max_output_tokens": 1,
      "pricing": {"input_per_million_tokens_usd": 1, "output_per_million_tokens_usd": 1, "cached_input_per_million_tokens_usd": 0.1, "currency": "USD"}
    },
    {
      "id": "dup",
      "provider": "openai",
      "display_name": "Two",
      "context_window": 1,
      "max_output_tokens": 1,
      "pricing": {"input_per_million_tokens_usd": 1, "output_per_million_tokens_usd": 1, "cached_input_per_million_tokens_usd": 0.1, "currency": "USD"}
    }
  ]
}`)

	_, err := LoadMulti(map[string]string{"openai": path})
	require.ErrorContains(t, err, `duplicate model "dup"`)
}

func TestLoadMultiRejectsMissingPricing(t *testing.T) {
	path := writeCatalog(t, "azure", `{
  "models": [{
    "id": "no-price",
    "provider": "azure",
    "display_name": "No Price",
    "context_window": 1,
    "max_output_tokens": 1,
    "pricing": {"currency": "USD"}
  }]
}`)

	_, err := LoadMulti(map[string]string{"azure": path})
	require.ErrorContains(t, err, "missing pricing")
}

func TestLoadMultiRejectsProviderMismatch(t *testing.T) {
	path := writeCatalog(t, "azure", `{
  "models": [{
    "id": "wrong-provider",
    "provider": "openai",
    "display_name": "Wrong Provider",
    "context_window": 1,
    "max_output_tokens": 1,
    "pricing": {"input_per_million_tokens_usd": 1, "output_per_million_tokens_usd": 1, "cached_input_per_million_tokens_usd": 0.1, "currency": "USD"}
  }]
}`)

	_, err := LoadMulti(map[string]string{"azure": path})
	require.ErrorContains(t, err, `does not match file provider "azure"`)
}

func writeCatalog(t *testing.T, provider, raw string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), provider+"_models.json")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o600))
	return path
}
