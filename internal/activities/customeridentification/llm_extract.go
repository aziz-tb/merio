package customeridentification

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/aziz-tb/merio/internal/ai/usage"
	"github.com/aziz-tb/merio/internal/ai/usage/redact"
	cimodels "github.com/aziz-tb/merio/internal/workflows/customeridentification"
)

func (a *Activities) LLMExtractActivity(ctx context.Context, in cimodels.LLMExtractInput) (cimodels.LLMExtractOutput, error) {
	out := cimodels.LLMExtractOutput{Ran: false, ExtractedAttrs: map[string]any{}}
	if !in.Settings.LLMExtractionEnabled {
		return out, nil
	}
	prompt, allowed := buildExtractionPrompt(in)
	if len(allowed) == 0 {
		return out, nil
	}
	out.Ran = true

	req := llm.CompletionRequest{
		Purpose:    llm.PurposeExtractParameters,
		PresetSlug: "fast-haiku",
		TraceID:    in.TraceID,
		Messages: []llm.Message{{
			Role: llm.RoleUser,
			Content: []llm.ContentBlock{{
				Type: llm.ContentBlockTypeText,
				Text: prompt,
			}},
		}},
		Tools: []llm.ToolDef{extractAttributesTool(allowed)},
		ToolChoice: &llm.ToolChoice{
			Type: "tool",
			Name: "extract_customer_identification",
		},
	}

	start := time.Now()
	resp, err := a.LLMProvider.Complete(ctx, req)
	latencyMs := int(time.Since(start).Milliseconds())
	if err != nil {
		return out, err
	}
	out.Usage = cimodels.LLMUsage{
		Provider:          a.LLMProvider.Name(),
		Model:             resp.Model,
		PresetUsed:        resp.PresetUsed,
		StopReason:        string(resp.StopReason),
		InputTokens:       resp.Usage.InputTokens,
		OutputTokens:      resp.Usage.OutputTokens,
		CachedInputTokens: resp.Usage.CachedTokens,
		CacheMarkersUsed:  resp.CacheMetrics.Markers,
		LatencyMs:         latencyMs,
		RetryCount:        resp.Observability.RetryCount,
		ProviderRequestID: resp.Observability.ProviderRequestID,
		ProviderMetadata:  resp.Observability.ProviderMetadata,
	}
	out.ExtractedAttrs = parseExtractionResponse(resp, allowed)
	a.recordExtractionUsage(ctx, in, req, resp, out.ExtractedAttrs, latencyMs)
	return out, nil
}

func buildExtractionPrompt(in cimodels.LLMExtractInput) (string, map[string]cimodels.SearchParameter) {
	allowed := map[string]cimodels.SearchParameter{}
	for key, p := range in.Settings.SearchParameters {
		if _, alreadyFound := in.PatternAttrs[key]; alreadyFound {
			continue
		}
		allowed[key] = p
	}
	if channelRules, ok := in.Settings.VerificationRulesAsync.ByChannel(in.Channel); ok {
		for _, group := range channelRules.Groups {
			for _, c := range group.CustomCriteria {
				key := strings.TrimSpace(c.Key)
				if key == "" {
					continue
				}
				if _, alreadyFound := in.PatternAttrs[key]; alreadyFound {
					continue
				}
				if _, exists := allowed[key]; !exists {
					allowed[key] = cimodels.SearchParameter{Type: "string", Description: c.Reason}
				}
			}
		}
	}

	keys := make([]string, 0, len(allowed))
	for key := range allowed {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("You are extracting customer identification attributes from a support message.\n")
	b.WriteString("Extract only attributes that are explicitly mentioned. Omit anything not stated.\n\n")
	b.WriteString("Attributes to extract:\n")
	for _, key := range keys {
		p := allowed[key]
		typ := p.Type
		if typ == "" {
			typ = "string"
		}
		fmt.Fprintf(&b, "- %s (%s): %s\n", key, typ, p.Description)
		if key == "contractId" && in.Settings.ContractNumberRegex != "" {
			fmt.Fprintf(&b, "  Format hint: /%s/\n", in.Settings.ContractNumberRegex)
		}
		if key == "customerId" && in.Settings.CustomerNumberRegex != "" {
			fmt.Fprintf(&b, "  Format hint: /%s/\n", in.Settings.CustomerNumberRegex)
		}
	}
	if len(in.Settings.ExcludedEmailDomains) > 0 {
		b.WriteString("\nDo not extract emails from these internal domains:\n")
		for _, domain := range in.Settings.ExcludedEmailDomains {
			fmt.Fprintf(&b, "- %s\n", domain)
		}
	}
	b.WriteString("\nUse the extract_customer_identification tool. Return only found keys.\n\n")
	fmt.Fprintf(&b, "Message subject: %s\n", in.PreparedMessage.Subject)
	fmt.Fprintf(&b, "Sender email: %s\n", in.PreparedMessage.SenderEmail)
	fmt.Fprintf(&b, "Message body:\n%s\n", in.PreparedMessage.Body)
	return b.String(), allowed
}

func extractAttributesTool(allowed map[string]cimodels.SearchParameter) llm.ToolDef {
	properties := map[string]any{}
	keys := make([]string, 0, len(allowed))
	for key := range allowed {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		p := allowed[key]
		typ := p.Type
		if typ == "" {
			typ = "string"
		}
		properties[key] = map[string]any{
			"type":        typ,
			"description": p.Description,
		}
	}
	return llm.ToolDef{
		Name:        "extract_customer_identification",
		Description: "Extract customer and contract identification attributes from the message.",
		InputSchema: map[string]any{
			"type":                 "object",
			"properties":           properties,
			"additionalProperties": false,
		},
	}
}

func parseExtractionResponse(resp llm.CompletionResponse, allowed map[string]cimodels.SearchParameter) map[string]any {
	out := map[string]any{}
	toolUses := llm.ExtractToolUses(resp.Content)
	if len(toolUses) > 0 {
		for k, v := range toolUses[0].ToolInput {
			if _, ok := allowed[k]; ok && isUsefulScalar(v) {
				out[k] = v
			}
		}
		return out
	}
	text := strings.TrimSpace(llm.ExtractText(resp.Content))
	if text == "" {
		return out
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return out
	}
	for k, v := range parsed {
		if _, ok := allowed[k]; ok && isUsefulScalar(v) {
			out[k] = v
		}
	}
	return out
}

func isUsefulScalar(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(x) != ""
	case float64, int, int64, bool, json.Number:
		return true
	default:
		return false
	}
}

func (a *Activities) recordExtractionUsage(ctx context.Context, in cimodels.LLMExtractInput, req llm.CompletionRequest, resp llm.CompletionResponse, extracted map[string]any, latencyMs int) {
	if resp.Model == "" || resp.PresetUsed == "" {
		return
	}
	conversationID := in.ConversationID
	row := usage.Row{
		CreatedAt:           time.Now().UTC(),
		TraceID:             in.TraceID,
		CallSequenceInTrace: 1,
		InboxID:             a.SystemInboxID,
		ConversationID:      &conversationID,
		Provider:            a.LLMProvider.Name(),
		Model:               resp.Model,
		PresetSlug:          resp.PresetUsed,
		Purpose:             string(llm.PurposeExtractParameters),
		StopReason:          string(resp.StopReason),
		InputTokens:         resp.Usage.InputTokens,
		OutputTokens:        resp.Usage.OutputTokens,
		CachedInputTokens:   resp.Usage.CachedTokens,
		CacheMarkersUsed:    resp.CacheMetrics.Markers,
		LatencyMs:           latencyMs,
		RetryCount:          resp.Observability.RetryCount,
		Outcome:             usage.OutcomeSuccess,
		ProviderMetadata:    resp.Observability.ProviderMetadata,
		RequestPayload:      redactedJSON(req),
		ResponsePayload:     redactedJSON(resp),
		StructuredOutput:    redactedJSON(extracted),
	}
	if resp.Observability.ProviderRequestID != "" {
		row.ProviderRequestID = &resp.Observability.ProviderRequestID
	}
	if err := a.UsageMgr.Record(ctx, row); err != nil && a.Lo != nil {
		a.Lo.Warn("customer identification ai_usage write failed", "trace_id", in.TraceID, "error", err)
	}
}

func redactedJSON(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil || len(raw) == 0 {
		return nil
	}
	return json.RawMessage(redact.Bytes(raw))
}
