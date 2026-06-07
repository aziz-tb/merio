//go:build live_smoke

package azure

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zerodha/logf"

	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/aziz-tb/merio/internal/ai/llm/preset"
)

func TestLiveSmokeAzureProviderOpenAIClaudeAndToolUse(t *testing.T) {
	key := os.Getenv("AZURE_API_KEY")
	endpoint := os.Getenv("AZURE_FOUNDRY_ENDPOINT")
	if key == "" || endpoint == "" {
		t.Skip("AZURE_API_KEY and AZURE_FOUNDRY_ENDPOINT are required")
	}

	t.Run("openai_o4_reasoning", func(t *testing.T) {
		pre := preset.Preset{
			Slug:        "live-azure-o4",
			Name:        "Live Azure o4",
			Provider:    "azure",
			Model:       envOr("AZURE_OPENAI_DEPLOYMENT", "o4-mini"),
			Temperature: 0.2,
			MaxTokens:   2000,
			Enabled:     true,
		}
		provider := newLiveProvider(t, key, endpoint, pre)
		resp, err := provider.Complete(liveCtx(t), llm.CompletionRequest{
			Purpose:    llm.PurposeReason,
			PresetSlug: pre.Slug,
			Messages: []llm.Message{{
				Role:    llm.RoleUser,
				Content: []llm.ContentBlock{{Type: llm.ContentBlockTypeText, Text: "Say hello in 5 words"}},
			}},
			ThinkingEnabled: true,
		})
		require.NoError(t, err)
		require.NotEmpty(t, llm.ExtractText(resp.Content))

		metadata := providerMetadata(t, resp)
		require.Greater(t, metadataNumber(metadata, "reasoning_tokens"), float64(0))
	})

	t.Run("claude_cache_usage", func(t *testing.T) {
		pre := preset.Preset{
			Slug:           "live-azure-haiku",
			Name:           "Live Azure Haiku",
			Provider:       "azure",
			Model:          envOr("AZURE_ANTHROPIC_DEPLOYMENT", "claude-haiku-4-5"),
			Temperature:    0.2,
			MaxTokens:      200,
			CachingEnabled: true,
			Enabled:        true,
		}
		provider := newLiveProvider(t, key, endpoint, pre)
		resp, err := provider.Complete(liveCtx(t), llm.CompletionRequest{
			Purpose:    llm.PurposeReason,
			PresetSlug: pre.Slug,
			Messages: []llm.Message{
				{Role: llm.RoleSystem, Content: []llm.ContentBlock{{Type: llm.ContentBlockTypeText, Text: "Answer briefly."}}},
				{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentBlockTypeText, Text: "Say hello in 5 words"}}},
			},
			CachedBlocks: []llm.CachedBlockMarker{{AfterMessageIndex: 0}},
		})
		require.NoError(t, err)
		require.NotEmpty(t, llm.ExtractText(resp.Content))

		metadata := providerMetadata(t, resp)
		require.Contains(t, metadata, "cache_creation_input_tokens")
		require.Contains(t, metadata, "cache_read_input_tokens")
	})

	t.Run("claude_tool_use", func(t *testing.T) {
		pre := preset.Preset{
			Slug:        "live-azure-haiku-tool",
			Name:        "Live Azure Haiku Tool",
			Provider:    "azure",
			Model:       envOr("AZURE_ANTHROPIC_DEPLOYMENT", "claude-haiku-4-5"),
			Temperature: 0.2,
			MaxTokens:   300,
			Enabled:     true,
		}
		provider := newLiveProvider(t, key, endpoint, pre)
		resp, err := provider.Complete(liveCtx(t), llm.CompletionRequest{
			Purpose:    llm.PurposeReason,
			PresetSlug: pre.Slug,
			Messages: []llm.Message{{
				Role:    llm.RoleUser,
				Content: []llm.ContentBlock{{Type: llm.ContentBlockTypeText, Text: "Call lookup_weather for Paris."}},
			}},
			Tools: []llm.ToolDef{{
				Name:        "lookup_weather",
				Description: "Look up weather for a city.",
				InputSchema: map[string]any{
					"type":       "object",
					"properties": map[string]any{"city": map[string]any{"type": "string"}},
					"required":   []any{"city"},
				},
			}},
			ToolChoice: &llm.ToolChoice{Type: "tool", Name: "lookup_weather"},
		})
		require.NoError(t, err)
		require.True(t, hasToolUse(resp.Content), "expected tool_use block, got %#v", resp.Content)
	})
}

func newLiveProvider(t *testing.T, key string, endpoint string, pre preset.Preset) *Provider {
	t.Helper()
	reg := writePresetRegistry(t, pre)
	cat := writeModelCatalog(t, pre)
	lo := logf.New(logf.Opts{Writer: io.Discard})
	p, err := New(Opts{
		APIKey:     key,
		Endpoint:   endpoint,
		Catalog:    cat,
		Registry:   reg,
		Lo:         &lo,
		HTTPClient: &http.Client{Timeout: 90 * time.Second},
	})
	require.NoError(t, err)
	return p
}

func liveCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func providerMetadata(t *testing.T, resp llm.CompletionResponse) map[string]any {
	t.Helper()
	out := map[string]any{}
	require.NoError(t, json.Unmarshal(resp.Observability.ProviderMetadata, &out))
	return out
}

func metadataNumber(metadata map[string]any, key string) float64 {
	switch v := metadata[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	default:
		return 0
	}
}

func hasToolUse(content []llm.ContentBlock) bool {
	for _, block := range content {
		if block.Type == llm.ContentBlockTypeToolUse {
			return true
		}
	}
	return false
}

func envOr(name string, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
