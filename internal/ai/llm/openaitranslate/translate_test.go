package openaitranslate

import (
	"encoding/json"
	"testing"

	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/aziz-tb/merio/internal/ai/llm/catalog"
	"github.com/aziz-tb/merio/internal/ai/llm/preset"
)

func TestRequestToParamsOmitsReasoningEffortForGPT54ToolCalls(t *testing.T) {
	params := requestParams(t, "gpt-5.4", true, []llm.ToolDef{toolDef()})
	payload := marshalPayload(t, params)

	if _, ok := payload["reasoning_effort"]; ok {
		t.Fatalf("expected reasoning_effort to be omitted for gpt-5.4 tool calls, got %v", payload["reasoning_effort"])
	}
	if _, ok := payload["max_completion_tokens"]; !ok {
		t.Fatalf("expected max_completion_tokens for reasoning model request")
	}
	tools, ok := payload["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected one tool in payload, got %#v", payload["tools"])
	}
	tool, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatalf("expected object tool payload, got %#v", tools[0])
	}
	if got := tool["type"]; got != "function" {
		t.Fatalf("expected tool type=function, got %#v", got)
	}
}

func TestRequestToParamsKeepsReasoningEffortForGPT51ToolCalls(t *testing.T) {
	params := requestParams(t, "gpt-5.1", true, []llm.ToolDef{toolDef()})
	payload := marshalPayload(t, params)

	if got := payload["reasoning_effort"]; got != "medium" {
		t.Fatalf("expected reasoning_effort=medium, got %#v", got)
	}
}

func TestRequestToParamsKeepsReasoningEffortForGPT54WithoutTools(t *testing.T) {
	params := requestParams(t, "gpt-5.4", true, nil)
	payload := marshalPayload(t, params)

	if got := payload["reasoning_effort"]; got != "medium" {
		t.Fatalf("expected reasoning_effort=medium without tools, got %#v", got)
	}
}

func requestParams(t *testing.T, model string, thinking bool, tools []llm.ToolDef) any {
	t.Helper()
	params, err := RequestToParams(llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: []llm.ContentBlock{{Type: llm.ContentBlockTypeText, Text: "You are concise."}}},
			{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentBlockTypeText, Text: "Call produce_draft."}}},
		},
		Tools:           tools,
		ThinkingEnabled: thinking,
	}, preset.Preset{
		Slug:      "test",
		Provider:  "openai",
		Model:     model,
		MaxTokens: 1000,
	}, catalog.Model{
		IsReasoningModel: true,
	})
	if err != nil {
		t.Fatalf("RequestToParams: %v", err)
	}
	return params
}

func toolDef() llm.ToolDef {
	return llm.ToolDef{
		Name:        "produce_draft",
		Description: "Commit the customer-facing draft.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"draft": map[string]any{"type": "string"},
			},
			"required": []string{"draft"},
		},
	}
}

func marshalPayload(t *testing.T, params any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal params: %v\nraw=%s", err, raw)
	}
	return payload
}
