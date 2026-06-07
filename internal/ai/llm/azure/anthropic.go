package azure

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zerodha/logf"

	"github.com/aziz-tb/merio/internal/ai/llm"
	anthropicshared "github.com/aziz-tb/merio/internal/ai/llm/anthropic/shared"
	"github.com/aziz-tb/merio/internal/ai/llm/catalog"
	"github.com/aziz-tb/merio/internal/ai/llm/preset"
)

const (
	modelFamilyClaude = "claude"
	modelFamilyOpenAI = "openai"
)

type anthropicRequest struct {
	Model         string                  `json:"model"`
	MaxTokens     int                     `json:"max_tokens"`
	Messages      []anthropicMessage      `json:"messages"`
	System        []anthropicContentBlock `json:"system,omitempty"`
	Temperature   *float64                `json:"temperature,omitempty"`
	TopP          *float64                `json:"top_p,omitempty"`
	TopK          *int                    `json:"top_k,omitempty"`
	StopSequences []string                `json:"stop_sequences,omitempty"`
	Tools         []anthropicTool         `json:"tools,omitempty"`
	ToolChoice    *anthropicToolChoice    `json:"tool_choice,omitempty"`
	Thinking      *anthropicThinking      `json:"thinking,omitempty"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicContentBlock struct {
	Type         string                 `json:"type"`
	Text         string                 `json:"text,omitempty"`
	Thinking     string                 `json:"thinking,omitempty"`
	Signature    string                 `json:"signature,omitempty"`
	Data         string                 `json:"data,omitempty"`
	ID           string                 `json:"id,omitempty"`
	Name         string                 `json:"name,omitempty"`
	Input        map[string]any         `json:"input,omitempty"`
	ToolUseID    string                 `json:"tool_use_id,omitempty"`
	Content      any                    `json:"content,omitempty"`
	IsError      *bool                  `json:"is_error,omitempty"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

type anthropicCacheControl struct {
	Type string `json:"type"`
	TTL  string `json:"ttl,omitempty"`
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

type anthropicToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

type anthropicThinking struct {
	Type string `json:"type"`
}

type anthropicResponse struct {
	ID         string                          `json:"id"`
	Model      string                          `json:"model"`
	StopReason string                          `json:"stop_reason"`
	Content    []anthropicshared.ResponseBlock `json:"content"`
	Usage      struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

func modelFamily(model string, meta catalog.Model) (string, error) {
	lower := strings.ToLower(model)
	if strings.HasPrefix(lower, "claude-") {
		return modelFamilyClaude, nil
	}
	if strings.HasPrefix(lower, "gpt-") ||
		strings.HasPrefix(lower, "o1") ||
		strings.HasPrefix(lower, "o3") ||
		strings.HasPrefix(lower, "o4") {
		return modelFamilyOpenAI, nil
	}
	if meta.SupportsThinking && !meta.IsReasoningModel {
		return modelFamilyClaude, nil
	}
	if meta.IsReasoningModel {
		return modelFamilyOpenAI, nil
	}
	return "", fmt.Errorf("azure: unsupported Foundry model deployment %q", model)
}

func reqToAnthropicParams(req llm.CompletionRequest, pre preset.Preset, model catalog.Model, lo *logf.Logger) (anthropicRequest, error) {
	if len(req.Messages) == 0 {
		return anthropicRequest{}, llm.ErrEmptyMessages
	}

	out := anthropicRequest{
		Model:     pre.Model,
		MaxTokens: pre.MaxTokens,
		Messages:  make([]anthropicMessage, 0, len(req.Messages)),
	}

	thinkingActive := false
	if req.ThinkingEnabled {
		if model.SupportsThinking {
			out.Thinking = &anthropicThinking{Type: anthropicshared.ThinkingTypeAdaptive}
			thinkingActive = true
		} else if lo != nil {
			lo.Warn("azure.anthropic.extended_thinking.unsupported_model", "model", pre.Model)
		}
	}
	if !thinkingActive {
		out.Temperature = &pre.Temperature
		out.TopP = pre.TopP
		out.TopK = pre.TopK
	} else {
		suppressThinkingSampling(lo, pre)
	}
	if len(pre.StopSequences) > 0 {
		out.StopSequences = append([]string{}, pre.StopSequences...)
	}

	for i, msg := range req.Messages {
		switch msg.Role {
		case llm.RoleSystem:
			for _, b := range msg.Content {
				if b.Type != llm.ContentBlockTypeText {
					return anthropicRequest{}, fmt.Errorf("azure anthropic: messages[%d] system role only supports text content blocks", i)
				}
				out.System = append(out.System, anthropicContentBlock{Type: "text", Text: b.Text})
			}
		case llm.RoleUser, llm.RoleAssistant:
			blocks, err := blocksToAnthropic(msg.Content)
			if err != nil {
				return anthropicRequest{}, fmt.Errorf("azure anthropic: messages[%d]: %w", i, err)
			}
			role := "user"
			if msg.Role == llm.RoleAssistant {
				role = "assistant"
			}
			out.Messages = append(out.Messages, anthropicMessage{Role: role, Content: blocks})
		default:
			return anthropicRequest{}, fmt.Errorf("azure anthropic: messages[%d] unknown role %q", i, msg.Role)
		}
	}

	if len(req.Tools) > 0 {
		out.Tools = make([]anthropicTool, 0, len(req.Tools))
		for i, t := range req.Tools {
			if t.Name == "" {
				return anthropicRequest{}, fmt.Errorf("azure anthropic: tools[%d] name is empty", i)
			}
			out.Tools = append(out.Tools, anthropicTool{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: t.InputSchema,
			})
		}
	}

	if req.ToolChoice != nil {
		switch req.ToolChoice.Type {
		case "", "auto":
		case "any":
			out.ToolChoice = &anthropicToolChoice{Type: "any"}
		case "tool":
			if req.ToolChoice.Name == "" {
				return anthropicRequest{}, fmt.Errorf("azure anthropic: tool_choice.type=tool requires name")
			}
			out.ToolChoice = &anthropicToolChoice{Type: "tool", Name: req.ToolChoice.Name}
		default:
			return anthropicRequest{}, fmt.Errorf("azure anthropic: unknown tool_choice.type %q", req.ToolChoice.Type)
		}
	}

	return out, nil
}

func blocksToAnthropic(blocks []llm.ContentBlock) ([]anthropicContentBlock, error) {
	out := make([]anthropicContentBlock, 0, len(blocks))
	for i, b := range blocks {
		switch b.Type {
		case llm.ContentBlockTypeText:
			out = append(out, anthropicContentBlock{Type: "text", Text: b.Text})
		case llm.ContentBlockTypeThinking:
			if b.Signature == "" || b.Text == "" {
				return nil, fmt.Errorf("content[%d] thinking block missing signature or text", i)
			}
			out = append(out, anthropicContentBlock{Type: "thinking", Thinking: b.Text, Signature: b.Signature})
		case llm.ContentBlockTypeRedactedThinking:
			if b.Data == "" {
				return nil, fmt.Errorf("content[%d] redacted_thinking block missing data", i)
			}
			out = append(out, anthropicContentBlock{Type: "redacted_thinking", Data: b.Data})
		case llm.ContentBlockTypeToolUse:
			if b.ToolUseID == "" || b.ToolName == "" {
				return nil, fmt.Errorf("content[%d] tool_use missing id or name", i)
			}
			out = append(out, anthropicContentBlock{
				Type:  "tool_use",
				ID:    b.ToolUseID,
				Name:  b.ToolName,
				Input: b.ToolInput,
			})
		case llm.ContentBlockTypeToolResult:
			if b.ResultToolUseID == "" {
				return nil, fmt.Errorf("content[%d] tool_result missing result_tool_use_id", i)
			}
			isErr := b.ResultIsError
			out = append(out, anthropicContentBlock{
				Type:      "tool_result",
				ToolUseID: b.ResultToolUseID,
				Content:   []anthropicContentBlock{{Type: "text", Text: b.ResultContent}},
				IsError:   &isErr,
			})
		default:
			return nil, fmt.Errorf("content[%d] unknown block type %q", i, b.Type)
		}
	}
	return out, nil
}

func suppressThinkingSampling(lo *logf.Logger, pre preset.Preset) {
	if lo == nil {
		return
	}
	if pre.Temperature != 1.0 {
		lo.Info("azure.anthropic.thinking.suppress_temperature",
			"model", pre.Model,
			"configured", pre.Temperature,
			"reason", "thinking requires temperature=1 or unset",
		)
	}
	if pre.TopK != nil {
		lo.Info("azure.anthropic.thinking.suppress_top_k",
			"model", pre.Model,
			"configured", *pre.TopK,
			"reason", "thinking requires top_k unset",
		)
	}
	if pre.TopP != nil && !anthropicshared.TopPAllowedForThinking(*pre.TopP) {
		lo.Info("azure.anthropic.thinking.suppress_top_p",
			"model", pre.Model,
			"configured", *pre.TopP,
			"allowed_range", "[0.95, 1.0]",
			"reason", "thinking requires top_p in [0.95, 1.0] or unset",
		)
	}
}

func applyAnthropicCacheMarkers(params *anthropicRequest, req llm.CompletionRequest, pre preset.Preset) int {
	targets := anthropicshared.CacheMarkerTargets(req, pre.CachingEnabled)
	if len(targets) == 0 {
		return 0
	}

	cc := &anthropicCacheControl{Type: "ephemeral"}
	if pre.CachingExtendedTTL {
		cc.TTL = "1h"
	}

	placed := 0
	for _, t := range targets {
		if t.Slice == anthropicshared.CacheSliceSystem {
			if t.Index >= 0 && t.Index < len(params.System) {
				params.System[t.Index].CacheControl = cc
				placed++
			}
			continue
		}
		if t.Index < 0 || t.Index >= len(params.Messages) {
			continue
		}
		msg := &params.Messages[t.Index]
		if len(msg.Content) == 0 {
			continue
		}
		msg.Content[len(msg.Content)-1].CacheControl = cc
		placed++
	}
	return placed
}

func anthropicToCompletionResponse(resp *anthropicResponse, pre preset.Preset, markers int, attempts int) llm.CompletionResponse {
	inputTokens := resp.Usage.InputTokens + resp.Usage.CacheCreationInputTokens
	metadata, _ := json.Marshal(map[string]any{
		"cache_creation_input_tokens": resp.Usage.CacheCreationInputTokens,
		"cache_read_input_tokens":     resp.Usage.CacheReadInputTokens,
		"input_tokens_raw":            resp.Usage.InputTokens,
		"model_reported":              resp.Model,
		"sdk_stop_reason":             resp.StopReason,
	})

	return llm.CompletionResponse{
		StopReason: llm.StopReason(resp.StopReason),
		Content:    anthropicContentToBlocks(resp.Content),
		Usage: llm.Usage{
			InputTokens:  inputTokens,
			OutputTokens: resp.Usage.OutputTokens,
			CachedTokens: resp.Usage.CacheReadInputTokens,
		},
		CacheMetrics: llm.CacheMetrics{
			HitTokens: resp.Usage.CacheReadInputTokens,
			Markers:   markers,
		},
		Provider:   "azure",
		PresetUsed: pre.Slug,
		Model:      pre.Model,
		Observability: llm.ResponseObservability{
			Provider:          "azure",
			ProviderRequestID: resp.ID,
			RetryCount:        attempts,
			ProviderMetadata:  metadata,
		},
	}
}

func anthropicContentToBlocks(content []anthropicshared.ResponseBlock) []llm.ContentBlock {
	return anthropicshared.ResponseBlocksToContentBlocks(content, "azure anthropic")
}
