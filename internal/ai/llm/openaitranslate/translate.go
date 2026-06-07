package openaitranslate

import (
	"encoding/json"
	"fmt"
	"strings"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/shared"

	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/aziz-tb/merio/internal/ai/llm/catalog"
	"github.com/aziz-tb/merio/internal/ai/llm/preset"
)

// RequestToParams translates the platform LLM request into OpenAI Chat
// Completions params. It is shared by OpenAI direct and Azure Foundry's OpenAI
// surface so request behavior cannot drift between providers.
func RequestToParams(req llm.CompletionRequest, pre preset.Preset, model catalog.Model) (openai.ChatCompletionNewParams, error) {
	if len(req.Messages) == 0 {
		return openai.ChatCompletionNewParams{}, llm.ErrEmptyMessages
	}

	out := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(pre.Model),
		Messages: make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages)),
	}
	if model.IsReasoningModel {
		out.MaxCompletionTokens = openai.Int(int64(pre.MaxTokens))
		if shouldSendReasoningEffort(req, pre) {
			out.ReasoningEffort = shared.ReasoningEffortMedium
		}
	} else {
		out.MaxTokens = openai.Int(int64(pre.MaxTokens))
		out.Temperature = openai.Float(pre.Temperature)
		if pre.TopP != nil {
			out.TopP = openai.Float(*pre.TopP)
		}
	}
	if len(pre.StopSequences) == 1 {
		out.Stop.OfString = param.NewOpt(pre.StopSequences[0])
	} else if len(pre.StopSequences) > 1 {
		out.Stop.OfStringArray = append([]string{}, pre.StopSequences...)
	}

	for i, m := range req.Messages {
		translated, err := messageToParams(m, model.IsReasoningModel)
		if err != nil {
			return openai.ChatCompletionNewParams{}, fmt.Errorf("openai: messages[%d]: %w", i, err)
		}
		out.Messages = append(out.Messages, translated...)
	}

	if len(req.Tools) > 0 {
		out.Tools = make([]openai.ChatCompletionToolParam, 0, len(req.Tools))
		for _, t := range req.Tools {
			out.Tools = append(out.Tools, openai.ChatCompletionToolParam{
				Function: shared.FunctionDefinitionParam{
					Name:        t.Name,
					Description: param.NewOpt(t.Description),
					Parameters:  shared.FunctionParameters(t.InputSchema),
				},
			})
		}
	}
	if req.ToolChoice != nil {
		switch req.ToolChoice.Type {
		case "", "auto":
			out.ToolChoice.OfAuto = param.NewOpt(string(openai.ChatCompletionToolChoiceOptionAutoAuto))
		case "any":
			out.ToolChoice.OfAuto = param.NewOpt(string(openai.ChatCompletionToolChoiceOptionAutoRequired))
		case "tool":
			if req.ToolChoice.Name == "" {
				return openai.ChatCompletionNewParams{}, fmt.Errorf("openai: tool_choice.type=tool requires name")
			}
			out.ToolChoice = openai.ChatCompletionToolChoiceOptionParamOfChatCompletionNamedToolChoice(
				openai.ChatCompletionNamedToolChoiceFunctionParam{Name: req.ToolChoice.Name},
			)
		default:
			return openai.ChatCompletionNewParams{}, fmt.Errorf("openai: unknown tool_choice.type %q", req.ToolChoice.Type)
		}
	}
	return out, nil
}

func shouldSendReasoningEffort(req llm.CompletionRequest, pre preset.Preset) bool {
	if !req.ThinkingEnabled {
		return false
	}
	// Chat Completions currently rejects gpt-5.4 family requests that combine
	// function tools with reasoning_effort. Agents always carry produce_draft,
	// so omit the effort knob there until this path moves to Responses.
	return !(len(req.Tools) > 0 && strings.HasPrefix(pre.Model, "gpt-5.4"))
}

func messageToParams(m llm.Message, reasoningModel bool) ([]openai.ChatCompletionMessageParamUnion, error) {
	textParts := make([]string, 0)
	toolCalls := make([]openai.ChatCompletionMessageToolCallParam, 0)
	toolResults := make([]openai.ChatCompletionMessageParamUnion, 0)

	for i, b := range m.Content {
		switch b.Type {
		case llm.ContentBlockTypeText:
			textParts = append(textParts, b.Text)
		case llm.ContentBlockTypeThinking, llm.ContentBlockTypeRedactedThinking:
			// OpenAI Chat Completions does not round-trip Anthropic thinking
			// blocks. They stay provider-specific to Claude paths.
		case llm.ContentBlockTypeToolUse:
			raw, err := json.Marshal(b.ToolInput)
			if err != nil {
				return nil, fmt.Errorf("content[%d] tool input marshal: %w", i, err)
			}
			toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallParam{
				ID: b.ToolUseID,
				Function: openai.ChatCompletionMessageToolCallFunctionParam{
					Name:      b.ToolName,
					Arguments: string(raw),
				},
			})
		case llm.ContentBlockTypeToolResult:
			toolResults = append(toolResults, openai.ToolMessage(b.ResultContent, b.ResultToolUseID))
		default:
			return nil, fmt.Errorf("content[%d] unknown block type %q", i, b.Type)
		}
	}

	text := strings.Join(textParts, "\n")
	switch m.Role {
	case llm.RoleSystem:
		if reasoningModel {
			return []openai.ChatCompletionMessageParamUnion{openai.DeveloperMessage(text)}, nil
		}
		return []openai.ChatCompletionMessageParamUnion{openai.SystemMessage(text)}, nil
	case llm.RoleUser:
		out := make([]openai.ChatCompletionMessageParamUnion, 0, 1+len(toolResults))
		if strings.TrimSpace(text) != "" {
			out = append(out, openai.UserMessage(text))
		}
		out = append(out, toolResults...)
		return out, nil
	case llm.RoleAssistant:
		msg := openai.AssistantMessage(text)
		if msg.OfAssistant != nil && len(toolCalls) > 0 {
			msg.OfAssistant.ToolCalls = toolCalls
		}
		return []openai.ChatCompletionMessageParamUnion{msg}, nil
	default:
		return nil, fmt.Errorf("unknown role %q", m.Role)
	}
}

// ResponseToCompletionResponse translates an OpenAI Chat Completion into the
// platform response shape.
func ResponseToCompletionResponse(resp *openai.ChatCompletion, pre preset.Preset, provider string, attempts int) llm.CompletionResponse {
	out := llm.CompletionResponse{
		Provider:   provider,
		PresetUsed: pre.Slug,
		Model:      pre.Model,
		Observability: llm.ResponseObservability{
			Provider:   provider,
			RetryCount: attempts,
		},
	}
	if resp == nil {
		return out
	}
	out.Observability.ProviderRequestID = resp.ID
	out.Usage = llm.Usage{
		InputTokens:  int(resp.Usage.PromptTokens),
		OutputTokens: int(resp.Usage.CompletionTokens),
		CachedTokens: int(resp.Usage.PromptTokensDetails.CachedTokens),
	}
	out.CacheMetrics.HitTokens = int(resp.Usage.PromptTokensDetails.CachedTokens)

	if len(resp.Choices) == 0 {
		return out
	}
	choice := resp.Choices[0]
	out.StopReason = mapStopReason(choice.FinishReason)
	out.Content = messageToBlocks(choice.Message)

	metadata, _ := json.Marshal(map[string]any{
		"sdk_finish_reason": choice.FinishReason,
		"reasoning_tokens":  int(resp.Usage.CompletionTokensDetails.ReasoningTokens),
		"cached_tokens":     int(resp.Usage.PromptTokensDetails.CachedTokens),
		"model_reported":    resp.Model,
	})
	out.Observability.ProviderMetadata = metadata
	return out
}

func messageToBlocks(msg openai.ChatCompletionMessage) []llm.ContentBlock {
	out := make([]llm.ContentBlock, 0, 1+len(msg.ToolCalls))
	if strings.TrimSpace(msg.Content) != "" {
		out = append(out, llm.ContentBlock{Type: llm.ContentBlockTypeText, Text: msg.Content})
	}
	for _, tc := range msg.ToolCalls {
		input := map[string]any{}
		if tc.Function.Arguments != "" {
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
		}
		out = append(out, llm.ContentBlock{
			Type:      llm.ContentBlockTypeToolUse,
			ToolUseID: tc.ID,
			ToolName:  tc.Function.Name,
			ToolInput: input,
		})
	}
	return out
}

func mapStopReason(reason string) llm.StopReason {
	switch reason {
	case "stop":
		return llm.StopReasonEndTurn
	case "length":
		return llm.StopReasonMaxTokens
	case "tool_calls", "function_call":
		return llm.StopReasonToolUse
	case "content_filter":
		return llm.StopReasonRefusal
	default:
		return llm.StopReason(reason)
	}
}
