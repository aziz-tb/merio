package tagclassification

import (
	"fmt"

	"github.com/abhinavxd/libredesk/internal/ai/llm"
)

func parseClassifyTagsResponse(resp llm.CompletionResponse) (Result, error) {
	toolUses := llm.ExtractToolUses(resp.Content)
	if len(toolUses) == 0 {
		return Result{}, fmt.Errorf("no tool_use block in response")
	}

	input := toolUses[0].ToolInput
	reasoning, _ := input["reasoning"].(string)

	scoresRaw, ok := input["scores"].(map[string]any)
	if !ok {
		return Result{}, fmt.Errorf("scores field missing or wrong type")
	}
	scores := make(map[string]float64, len(scoresRaw))
	for tagName, v := range scoresRaw {
		switch x := v.(type) {
		case float64:
			scores[tagName] = x
		case int:
			scores[tagName] = float64(x)
		case int64:
			scores[tagName] = float64(x)
		}
	}

	return Result{
		Reasoning:         reasoning,
		Scores:            scores,
		Model:             resp.Model,
		PresetUsed:        resp.PresetUsed,
		StopReason:        string(resp.StopReason),
		InputTokens:       resp.Usage.InputTokens,
		OutputTokens:      resp.Usage.OutputTokens,
		CachedInputTokens: resp.Usage.CachedTokens,
		CacheMarkersUsed:  resp.CacheMetrics.Markers,
		RetryCount:        resp.Observability.RetryCount,
		ProviderRequestID: resp.Observability.ProviderRequestID,
		ProviderMetadata:  resp.Observability.ProviderMetadata,
	}, nil
}
