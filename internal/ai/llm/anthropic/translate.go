package anthropic

import (
	"encoding/json"
	"fmt"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/aziz-tb/merio/internal/ai/llm/catalog"
	"github.com/aziz-tb/merio/internal/ai/llm/preset"
)

// reqToSDKParams turns our provider-agnostic CompletionRequest +
// resolved preset into the SDK's MessageNewParams. Cache markers are
// applied as a separate pass (cache.go) because they need to mutate
// individual content blocks after the params are built.
//
// llm.RoleSystem messages are hoisted into MessageNewParams.System rather
// than stay in Messages — the SDK rejects "system" as a message role.
// Adjacent system messages are concatenated; we preserve order.
func reqToSDKParams(req llm.CompletionRequest, p preset.Preset) (sdk.MessageNewParams, error) {
	if len(req.Messages) == 0 {
		return sdk.MessageNewParams{}, llm.ErrEmptyMessages
	}

	out := sdk.MessageNewParams{
		MaxTokens:   int64(p.MaxTokens),
		Model:       sdk.Model(p.Model),
		Temperature: param.NewOpt(p.Temperature),
	}
	if p.TopP != nil {
		out.TopP = param.NewOpt(*p.TopP)
	}
	if p.TopK != nil {
		out.TopK = param.NewOpt(int64(*p.TopK))
	}
	if len(p.StopSequences) > 0 {
		out.StopSequences = append([]string{}, p.StopSequences...)
	}

	// Walk messages once, separating system from user/assistant turns.
	systemBlocks := make([]sdk.TextBlockParam, 0)
	turnMessages := make([]sdk.MessageParam, 0, len(req.Messages))

	for i, m := range req.Messages {
		switch m.Role {
		case llm.RoleSystem:
			for _, b := range m.Content {
				if b.Type != llm.ContentBlockTypeText {
					return sdk.MessageNewParams{}, fmt.Errorf("anthropic: messages[%d] system role only supports text content blocks", i)
				}
				systemBlocks = append(systemBlocks, sdk.TextBlockParam{Text: b.Text})
			}
		case llm.RoleUser, llm.RoleAssistant:
			blocks, err := blocksToSDK(m.Content)
			if err != nil {
				return sdk.MessageNewParams{}, fmt.Errorf("anthropic: messages[%d]: %w", i, err)
			}
			role := sdk.MessageParamRoleUser
			if m.Role == llm.RoleAssistant {
				role = sdk.MessageParamRoleAssistant
			}
			turnMessages = append(turnMessages, sdk.MessageParam{Role: role, Content: blocks})
		default:
			return sdk.MessageNewParams{}, fmt.Errorf("anthropic: messages[%d] unknown role %q", i, m.Role)
		}
	}

	if len(systemBlocks) > 0 {
		out.System = systemBlocks
	}
	out.Messages = turnMessages

	// Translate tools.
	if len(req.Tools) > 0 {
		out.Tools = make([]sdk.ToolUnionParam, 0, len(req.Tools))
		for i, t := range req.Tools {
			toolParam, err := toolDefToSDK(t)
			if err != nil {
				return sdk.MessageNewParams{}, fmt.Errorf("anthropic: tools[%d] %q: %w", i, t.Name, err)
			}
			out.Tools = append(out.Tools, sdk.ToolUnionParam{OfTool: &toolParam})
		}
	}

	if req.ToolChoice != nil {
		switch req.ToolChoice.Type {
		case "", "auto":
			// Anthropic's default behavior is auto; leave the field unset so
			// existing callers keep byte-for-byte compatible requests.
		case "any":
			out.ToolChoice = sdk.ToolChoiceUnionParam{OfAny: &sdk.ToolChoiceAnyParam{}}
		case "tool":
			if req.ToolChoice.Name == "" {
				return sdk.MessageNewParams{}, fmt.Errorf("anthropic: tool_choice.type=tool requires name")
			}
			out.ToolChoice = sdk.ToolChoiceParamOfTool(req.ToolChoice.Name)
		default:
			return sdk.MessageNewParams{}, fmt.Errorf("anthropic: unknown tool_choice.type %q", req.ToolChoice.Type)
		}
	}

	return out, nil
}

// blocksToSDK translates a non-system message's Content blocks to the
// SDK's content-block union shape.
func blocksToSDK(blocks []llm.ContentBlock) ([]sdk.ContentBlockParamUnion, error) {
	out := make([]sdk.ContentBlockParamUnion, 0, len(blocks))
	for i, b := range blocks {
		switch b.Type {
		case llm.ContentBlockTypeText:
			out = append(out, sdk.NewTextBlock(b.Text))
		case llm.ContentBlockTypeThinking:
			if b.Signature == "" || b.Text == "" {
				return nil, fmt.Errorf("content[%d] thinking block missing signature or text", i)
			}
			out = append(out, sdk.NewThinkingBlock(b.Signature, b.Text))
		case llm.ContentBlockTypeRedactedThinking:
			if b.Data == "" {
				return nil, fmt.Errorf("content[%d] redacted_thinking block missing data", i)
			}
			out = append(out, sdk.NewRedactedThinkingBlock(b.Data))
		case llm.ContentBlockTypeToolUse:
			if b.ToolUseID == "" || b.ToolName == "" {
				return nil, fmt.Errorf("content[%d] tool_use missing id or name", i)
			}
			out = append(out, sdk.NewToolUseBlock(b.ToolUseID, b.ToolInput, b.ToolName))
		case llm.ContentBlockTypeToolResult:
			if b.ResultToolUseID == "" {
				return nil, fmt.Errorf("content[%d] tool_result missing result_tool_use_id", i)
			}
			out = append(out, sdk.NewToolResultBlock(b.ResultToolUseID, b.ResultContent, b.ResultIsError))
		default:
			return nil, fmt.Errorf("content[%d] unknown block type %q", i, b.Type)
		}
	}
	return out, nil
}

// thinkingConfigForModel returns the Anthropic thinking configuration
// for a catalog model. v1 uses adaptive thinking whenever the catalog
// marks the model as capable.
func thinkingConfigForModel(model catalog.Model) (sdk.ThinkingConfigParamUnion, bool) {
	if !model.SupportsThinking {
		return sdk.ThinkingConfigParamUnion{}, false
	}
	return sdk.ThinkingConfigParamUnion{
		OfAdaptive: &sdk.ThinkingConfigAdaptiveParam{
			Type: "adaptive",
		},
	}, true
}

// toolDefToSDK turns our provider-agnostic ToolDef into the SDK's
// ToolParam. Properties + required map directly; unknown top-level keys
// flow through ExtraFields so future Schema features (additionalProperties,
// etc.) don't need code changes here.
func toolDefToSDK(t llm.ToolDef) (sdk.ToolParam, error) {
	if t.Name == "" {
		return sdk.ToolParam{}, fmt.Errorf("tool name is empty")
	}

	schema := sdk.ToolInputSchemaParam{}
	extra := map[string]any{}
	for k, v := range t.InputSchema {
		switch k {
		case "type":
			// SDK auto-marshals as "object"; non-object types aren't supported
			// for tool input. Surface if the caller passes one.
			if s, ok := v.(string); ok && s != "" && s != "object" {
				return sdk.ToolParam{}, fmt.Errorf("tool %q input_schema.type=%q unsupported (must be \"object\")", t.Name, s)
			}
		case "properties":
			schema.Properties = v
		case "required":
			schema.Required = stringSlice(v)
		default:
			extra[k] = v
		}
	}
	if len(extra) > 0 {
		schema.ExtraFields = extra
	}

	return sdk.ToolParam{
		Name:        t.Name,
		Description: param.NewOpt(t.Description),
		InputSchema: schema,
	}, nil
}

// stringSlice converts a JSON-decoded "required" field to []string.
// Handles []any (most common after json.Unmarshal into map[string]any)
// and []string (callers who already typed it).
func stringSlice(v any) []string {
	switch typed := v.(type) {
	case []string:
		out := make([]string, len(typed))
		copy(out, typed)
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, e := range typed {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// sdkToCompletionResponse turns the SDK's Message response into our
// provider-agnostic CompletionResponse.
//
// Note on cache token bookkeeping: Anthropic returns separate counts for
// "cache write" (CacheCreationInputTokens — tokens billed at +25% to
// build a cache entry) and "cache read" (CacheReadInputTokens — tokens
// billed at -90% on subsequent calls). We surface CacheReadInputTokens
// as Usage.CachedTokens (the dimension that drives downstream cost
// dashboards). Cache-creation tokens are absorbed into InputTokens — the
// +25% creation premium is captured in provider_metadata for later
// reporting without changing the core Usage shape.
//
// attempts is the call count from withRetry (1 = no retries; 2+ =
// transient errors absorbed). Threaded into Observability so the
// ai_usage row records retry_count without re-reading callLog.
func sdkToCompletionResponse(msg *sdk.Message, p preset.Preset, markers int, attempts int) llm.CompletionResponse {
	inputTokens := int(msg.Usage.InputTokens) + int(msg.Usage.CacheCreationInputTokens)

	// Provider-specific cache split, mounted into provider_metadata.
	// JSON-marshal failure is non-fatal — we fall back to nil and lose
	// the detail rather than dropping the whole response.
	metadata, _ := json.Marshal(map[string]any{
		"cache_creation_input_tokens": int(msg.Usage.CacheCreationInputTokens),
		"cache_read_input_tokens":     int(msg.Usage.CacheReadInputTokens),
		"input_tokens_raw":            int(msg.Usage.InputTokens),
		"sdk_stop_reason":             string(msg.StopReason),
	})

	out := llm.CompletionResponse{
		StopReason: llm.StopReason(msg.StopReason),
		Content:    sdkContentToBlocks(msg.Content),
		Usage: llm.Usage{
			InputTokens:  inputTokens,
			OutputTokens: int(msg.Usage.OutputTokens),
			CachedTokens: int(msg.Usage.CacheReadInputTokens),
		},
		CacheMetrics: llm.CacheMetrics{
			HitTokens: int(msg.Usage.CacheReadInputTokens),
			Markers:   markers,
		},
		PresetUsed: p.Slug,
		Model:      p.Model,
		Observability: llm.ResponseObservability{
			ProviderRequestID: msg.ID,
			RetryCount:        attempts,
			ProviderMetadata:  metadata,
		},
	}
	return out
}

// sdkContentToBlocks translates the SDK's response content (a union of
// text / tool_use / thinking / etc.) to our ContentBlock list. Unknown
// block types are surfaced as best-effort placeholder text so callers
// see a stable shape rather than silently dropping content.
func sdkContentToBlocks(content []sdk.ContentBlockUnion) []llm.ContentBlock {
	out := make([]llm.ContentBlock, 0, len(content))
	for _, c := range content {
		switch c.Type {
		case "text":
			t := c.AsText()
			out = append(out, llm.ContentBlock{Type: llm.ContentBlockTypeText, Text: t.Text})
		case "thinking":
			tb := c.AsThinking()
			out = append(out, llm.ContentBlock{
				Type:      llm.ContentBlockTypeThinking,
				Text:      tb.Thinking,
				Signature: tb.Signature,
			})
		case "redacted_thinking":
			tb := c.AsRedactedThinking()
			out = append(out, llm.ContentBlock{
				Type: llm.ContentBlockTypeRedactedThinking,
				Data: tb.Data,
			})
		case "tool_use":
			tu := c.AsToolUse()
			input := map[string]any{}
			if len(tu.Input) > 0 {
				_ = json.Unmarshal(tu.Input, &input)
			}
			out = append(out, llm.ContentBlock{
				Type:      llm.ContentBlockTypeToolUse,
				ToolUseID: tu.ID,
				ToolName:  tu.Name,
				ToolInput: input,
			})
		default:
			// Unknown / not-yet-modeled block type. Surface as text so the
			// agent runner sees something rather than dropping silently.
			out = append(out, llm.ContentBlock{
				Type: llm.ContentBlockTypeText,
				Text: fmt.Sprintf("[anthropic: %s block elided]", c.Type),
			})
		}
	}
	return out
}
