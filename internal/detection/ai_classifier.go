package detection

import (
	"context"
	"strings"
	"time"

	"github.com/abhinavxd/libredesk/internal/ai/llm"
	"github.com/abhinavxd/libredesk/internal/ai/usage"
	"github.com/google/uuid"
)

func (m *Manager) evaluateAI(ctx context.Context, ai *AIClassifier, ec EvaluationContext) (EvaluationResult, error) {
	if m.llmProvider == nil {
		return EvaluationResult{Matched: false, Reasoning: "no LLM provider"}, nil
	}
	if ai == nil || strings.TrimSpace(ai.Prompt) == "" {
		return EvaluationResult{Matched: false, Reasoning: "empty AI prompt"}, nil
	}

	toolDef := llm.ToolDef{
		Name:        "evaluate_detection",
		Description: "Provide a verdict on whether the detection prompt applies to the ticket.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"matches":    map[string]any{"type": "boolean"},
				"confidence": map[string]any{"type": "number", "minimum": 0, "maximum": 1},
				"reasoning":  map[string]any{"type": "string"},
			},
			"required": []string{"matches", "confidence", "reasoning"},
		},
	}

	startedAt := time.Now()
	resp, err := m.llmProvider.Complete(ctx, llm.CompletionRequest{
		Purpose:    llm.PurposeClassification,
		PresetSlug: m.presetSlug,
		Messages: []llm.Message{{
			Role: llm.RoleUser,
			Content: []llm.ContentBlock{{
				Type: llm.ContentBlockTypeText,
				Text: "Evaluate whether the detection prompt applies to this ticket.\n\nTicket:\n" +
					buildTicketSummary(ec) + "\nDetection prompt:\n" + ai.Prompt +
					"\n\nUse the evaluate_detection tool to provide your verdict.",
			}},
		}},
		Tools: []llm.ToolDef{toolDef},
		ToolChoice: &llm.ToolChoice{
			Type: "tool",
			Name: "evaluate_detection",
		},
	})
	if err != nil {
		// A failed Complete carries no model/tokens, so there's nothing
		// meaningful to write to ai_usage; log and surface the error.
		// (Mirrors tag classification's success-only usage write.)
		if m.lo != nil {
			m.lo.Warn("detection: AI evaluation LLM call failed", "error", err)
		}
		return EvaluationResult{}, err
	}

	// Batch 9 audit-gap fix: record the LLM call to ai_usage so synchronous
	// AI detection is visible to cost tracking. Best-effort — a write
	// failure never fails the evaluation.
	m.recordUsage(ctx, ec, resp, int(time.Since(startedAt).Milliseconds()))

	toolUses := llm.ExtractToolUses(resp.Content)
	if len(toolUses) == 0 {
		return EvaluationResult{Matched: false, Reasoning: "LLM did not call tool"}, nil
	}

	input := toolUses[0].ToolInput
	matches, _ := input["matches"].(bool)
	confidence, _ := input["confidence"].(float64)
	reasoning, _ := input["reasoning"].(string)

	return EvaluationResult{
		Matched:   matches,
		Score:     confidence,
		Reasoning: reasoning,
	}, nil
}

func buildTicketSummary(ec EvaluationContext) string {
	conv := ec.Conversation
	var b strings.Builder

	if conv.Subject.Valid {
		b.WriteString("Subject: ")
		b.WriteString(conv.Subject.String)
		b.WriteString("\n")
	}
	if conv.Contact.Email.Valid {
		b.WriteString("Contact email: ")
		b.WriteString(conv.Contact.Email.String)
		b.WriteString("\n")
	}
	if conv.InboxChannel != "" {
		b.WriteString("Channel: ")
		b.WriteString(conv.InboxChannel)
		b.WriteString("\n")
	}
	if conv.LastMessage.Valid {
		b.WriteString("Last message:\n")
		msg := conv.LastMessage.String
		if len(msg) > 2000 {
			msg = msg[:2000] + "..."
		}
		b.WriteString(msg)
		b.WriteString("\n")
	}
	if len(ec.Tags) > 0 {
		b.WriteString("Current tags: ")
		b.WriteString(strings.Join(ec.Tags, ", "))
		b.WriteString("\n")
	}
	return b.String()
}

// recordUsage writes one ai_usage row for a successful AI-detection LLM
// call. Best-effort: a nil usage manager, zero system inbox, or write
// error is logged and swallowed — auditing must never break detection.
func (m *Manager) recordUsage(ctx context.Context, ec EvaluationContext, resp llm.CompletionResponse, latencyMs int) {
	if m.usageMgr == nil || m.systemInboxID == 0 {
		return
	}
	traceID := ec.TraceID
	if traceID == "" {
		traceID = "detection:" + uuid.NewString()
	}

	presetSlug := resp.PresetUsed
	if presetSlug == "" {
		presetSlug = m.presetSlug
	}

	row := usage.Row{
		CreatedAt:           time.Now().UTC(),
		TraceID:             traceID,
		CallSequenceInTrace: 1,
		InboxID:             m.systemInboxID,
		Provider:            "anthropic",
		Model:               resp.Model,
		PresetSlug:          presetSlug,
		Purpose:             string(llm.PurposeClassification),
		StopReason:          string(resp.StopReason),
		InputTokens:         resp.Usage.InputTokens,
		OutputTokens:        resp.Usage.OutputTokens,
		CachedInputTokens:   resp.Usage.CachedTokens,
		CacheMarkersUsed:    resp.CacheMetrics.Markers,
		LatencyMs:           latencyMs,
		RetryCount:          resp.Observability.RetryCount,
		Outcome:             usage.OutcomeSuccess,
	}
	if resp.Observability.ProviderRequestID != "" {
		id := resp.Observability.ProviderRequestID
		row.ProviderRequestID = &id
	}
	if len(resp.Observability.ProviderMetadata) > 0 {
		row.ProviderMetadata = []byte(resp.Observability.ProviderMetadata)
	}
	if ec.Conversation.ID > 0 {
		cid := int64(ec.Conversation.ID)
		row.ConversationID = &cid
	}
	if err := m.usageMgr.Record(ctx, row); err != nil && m.lo != nil {
		m.lo.Warn("detection: ai_usage write failed", "trace_id", traceID, "error", err)
	}
}
