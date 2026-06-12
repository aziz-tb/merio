package azure

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zerodha/logf"

	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/aziz-tb/merio/internal/ai/llm/catalog"
	"github.com/aziz-tb/merio/internal/ai/llm/preset"
)

func TestCompleteOpenAIPathUsesFoundryOpenAIHeaders(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "/openai/v1/chat/completions", r.URL.Path)
		require.Equal(t, "test-key", r.Header.Get("api-key"))
		require.Empty(t, r.Header.Get("authorization"))

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "o4-mini", body["model"])
		require.Equal(t, float64(2000), body["max_completion_tokens"])
		require.NotContains(t, body, "temperature")

		return jsonResponse(r, `{
			"id": "chatcmpl_test",
			"object": "chat.completion",
			"created": 1,
			"model": "o4-mini",
			"choices": [{
				"index": 0,
				"finish_reason": "stop",
				"message": {"role": "assistant", "content": "hello from foundry"}
			}],
			"usage": {
				"prompt_tokens": 10,
				"completion_tokens": 5,
				"total_tokens": 15,
				"prompt_tokens_details": {"cached_tokens": 2},
				"completion_tokens_details": {"reasoning_tokens": 3}
			}
		}`), nil
	})}

	provider := newTestProvider(t, "https://foundry.test", client, preset.Preset{
		Slug:        "azure-o4",
		Name:        "Azure o4",
		Provider:    "azure",
		Model:       "o4-mini",
		Temperature: 0.2,
		MaxTokens:   2000,
		Enabled:     true,
	})

	resp, err := provider.Complete(context.Background(), llm.CompletionRequest{
		Purpose:    llm.PurposeReason,
		PresetSlug: "azure-o4",
		Messages: []llm.Message{{
			Role:    llm.RoleUser,
			Content: []llm.ContentBlock{{Type: llm.ContentBlockTypeText, Text: "hello"}},
		}},
		ThinkingEnabled: true,
	})
	require.NoError(t, err)
	require.Equal(t, "azure", resp.Provider)
	require.Equal(t, llm.StopReasonEndTurn, resp.StopReason)
	require.Equal(t, "hello from foundry", llm.ExtractText(resp.Content))
	require.Equal(t, 2, resp.Usage.CachedTokens)
}

func TestCompleteClaudePathUsesDirectAnthropicHTTP(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "/anthropic/v1/messages", r.URL.Path)
		require.Equal(t, "test-key", r.Header.Get(foundryAnthropicAuth))
		require.Equal(t, foundryAnthropicVersion, r.Header.Get("anthropic-version"))

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "claude-haiku-4-5", body["model"])
		require.Equal(t, map[string]any{"type": "any"}, body["tool_choice"])

		system := body["system"].([]any)
		cacheControl := system[0].(map[string]any)["cache_control"].(map[string]any)
		require.Equal(t, "ephemeral", cacheControl["type"])

		return jsonResponse(r, `{
			"id": "msg_test",
			"model": "claude-haiku-4-5",
			"stop_reason": "tool_use",
			"content": [{
				"type": "tool_use",
				"id": "toolu_1",
				"name": "lookup",
				"input": {"query": "abc"}
			}],
			"usage": {
				"input_tokens": 10,
				"output_tokens": 5,
				"cache_creation_input_tokens": 2,
				"cache_read_input_tokens": 3
			}
		}`), nil
	})}

	provider := newTestProvider(t, "https://foundry.test", client, preset.Preset{
		Slug:           "azure-haiku",
		Name:           "Azure Haiku",
		Provider:       "azure",
		Model:          "claude-haiku-4-5",
		Temperature:    0.2,
		MaxTokens:      200,
		CachingEnabled: true,
		Enabled:        true,
	})

	resp, err := provider.Complete(context.Background(), llm.CompletionRequest{
		Purpose:    llm.PurposeReason,
		PresetSlug: "azure-haiku",
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: []llm.ContentBlock{{Type: llm.ContentBlockTypeText, Text: "system"}}},
			{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentBlockTypeText, Text: "use a tool"}}},
		},
		Tools: []llm.ToolDef{{
			Name:        "lookup",
			Description: "Look up a value.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"query": map[string]any{"type": "string"}},
				"required":   []any{"query"},
			},
		}},
		ToolChoice:   &llm.ToolChoice{Type: "any"},
		CachedBlocks: []llm.CachedBlockMarker{{AfterMessageIndex: 0}},
	})
	require.NoError(t, err)
	require.Equal(t, "azure", resp.Provider)
	require.Equal(t, llm.StopReasonToolUse, resp.StopReason)
	require.Equal(t, 12, resp.Usage.InputTokens)
	require.Equal(t, 3, resp.Usage.CachedTokens)
	require.Len(t, resp.Content, 1)
	require.Equal(t, llm.ContentBlockTypeToolUse, resp.Content[0].Type)
	require.Equal(t, "toolu_1", resp.Content[0].ToolUseID)
	require.Equal(t, "lookup", resp.Content[0].ToolName)
	require.Equal(t, "abc", resp.Content[0].ToolInput["query"])
}

func newTestProvider(t *testing.T, endpoint string, client *http.Client, pre preset.Preset) *Provider {
	t.Helper()
	reg := writePresetRegistry(t, pre)
	cat := writeModelCatalog(t, pre)
	lo := logf.New(logf.Opts{Writer: io.Discard})
	p, err := New(Opts{
		APIKey:     "test-key",
		Endpoint:   endpoint,
		Catalog:    cat,
		Registry:   reg,
		Lo:         &lo,
		HTTPClient: client,
	})
	require.NoError(t, err)
	return p
}

func writeModelCatalog(t *testing.T, pre preset.Preset) *catalog.Catalog {
	t.Helper()
	model := catalog.Model{
		ID:              pre.Model,
		Provider:        pre.Provider,
		DisplayName:     pre.Name,
		ContextWindow:   200000,
		MaxOutputTokens: pre.MaxTokens,
		Tier:            catalog.TierHaiku,
		Pricing: catalog.Pricing{
			InputPerMillionTokensUSD:       1,
			OutputPerMillionTokensUSD:      1,
			CachedInputPerMillionTokensUSD: 0.1,
			Currency:                       "USD",
			EffectiveFrom:                  "2026-01-01",
		},
	}
	if strings.HasPrefix(pre.Model, "claude-") {
		model.SupportsCaching = true
		model.SupportsExtendedCaching = true
		model.SupportsThinking = true
	}
	if strings.HasPrefix(pre.Model, "o") || strings.HasPrefix(pre.Model, "gpt-") {
		model.SupportsCaching = true
		model.SupportsThinking = true
		model.IsReasoningModel = strings.HasPrefix(pre.Model, "o") || strings.HasPrefix(pre.Model, "gpt-5")
	}
	raw, err := json.Marshal(map[string]any{"models": []catalog.Model{model}})
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "azure_models.json")
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	cat, err := catalog.LoadMulti(map[string]string{"azure": path})
	require.NoError(t, err)
	return cat
}

func writePresetRegistry(t *testing.T, pre preset.Preset) *preset.Registry {
	t.Helper()
	mappings := map[string]string{}
	for _, purpose := range llm.KnownPurposes {
		if llm.IsNonLLMPurpose(purpose) {
			continue
		}
		mappings[string(purpose)] = pre.Slug
	}
	raw, err := json.Marshal(map[string]any{
		"presets":          []preset.Preset{pre},
		"purpose_mappings": mappings,
	})
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "presets.json")
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	reg, err := preset.Load(path, nil)
	require.NoError(t, err)
	return reg
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(req *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}
