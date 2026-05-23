package tagclassification

import (
	"context"
	"fmt"
	"strings"

	"github.com/abhinavxd/libredesk/internal/ai/llm"
	cmodels "github.com/abhinavxd/libredesk/internal/conversation/models"
	tagmodels "github.com/abhinavxd/libredesk/internal/tag/models"
)

// maxContextChars caps the conversation messages portion of the prompt.
// Newest messages are preserved when the cap is exceeded.
const maxContextChars = 12000

type Result struct {
	Reasoning         string
	Scores            map[string]float64
	Model             string
	PresetUsed        string
	StopReason        string
	InputTokens       int
	OutputTokens      int
	CachedInputTokens int
	CacheMarkersUsed  int
	RetryCount        int
	ProviderRequestID string
	ProviderMetadata  []byte
}

func (a *Activities) runBatchedAI(ctx context.Context, conv cmodels.Conversation, tags []tagmodels.Tag, traceID string) (Result, error) {
	messages, err := a.ConversationMgr.LoadMessagesForLLM(ctx, conv.UUID)
	if err != nil {
		return Result{}, fmt.Errorf("load conversation messages: %w", err)
	}
	messagesText := formatMessagesForPrompt(messages)

	req := llm.CompletionRequest{
		Purpose:    llm.PurposeClassification,
		PresetSlug: "fast-haiku",
		TraceID:    traceID,
		Messages: []llm.Message{{
			Role: llm.RoleUser,
			Content: []llm.ContentBlock{{
				Type: llm.ContentBlockTypeText,
				Text: fmt.Sprintf(
					"You are a customer service ticket classifier.\n\nAnalyze the following ticket and determine which candidate tags apply. For each tag, return a confidence score between 0.0 and 1.0.\n\n%s\n\nCandidate tags:\n%s\n\nUse the classify_tags tool to return your verdict.",
					buildTicketSummary(conv, messagesText),
					buildCandidatesBlock(tags),
				),
			}},
		}},
		Tools: []llm.ToolDef{classifyTagsTool()},
		ToolChoice: &llm.ToolChoice{
			Type: "tool",
			Name: "classify_tags",
		},
	}

	resp, err := a.LLMProvider.Complete(ctx, req)
	if err != nil {
		return Result{}, fmt.Errorf("llm call: %w", err)
	}
	return parseClassifyTagsResponse(resp)
}

func formatMessagesForPrompt(messages []cmodels.Message) string {
	if len(messages) == 0 {
		return "(no public conversation messages)"
	}

	type line struct {
		text   string
		length int
	}
	lines := make([]line, len(messages))
	total := 0
	for i, m := range messages {
		text := fmt.Sprintf("[%s] %s/%s: %s\n",
			m.CreatedAt.UTC().Format("2006-01-02 15:04"),
			m.Type,
			m.SenderType,
			m.Content,
		)
		lines[i] = line{text: text, length: len(text)}
		total += len(text)
	}

	if total <= maxContextChars {
		var b strings.Builder
		b.Grow(total)
		for _, l := range lines {
			b.WriteString(l.text)
		}
		return b.String()
	}

	const marker = "[...earlier messages omitted for length...]\n"
	budget := maxContextChars - len(marker)
	if budget < 0 {
		budget = 0
	}

	used := 0
	keepFrom := len(lines)
	for i := len(lines) - 1; i >= 0; i-- {
		if used+lines[i].length > budget {
			break
		}
		used += lines[i].length
		keepFrom = i
	}
	if keepFrom == len(lines) && len(lines) > 0 && budget > 0 {
		newest := lines[len(lines)-1].text
		if len(newest) > budget {
			newest = newest[len(newest)-budget:]
		}
		return marker + newest
	}

	var b strings.Builder
	b.WriteString(marker)
	for i := keepFrom; i < len(lines); i++ {
		b.WriteString(lines[i].text)
	}
	return b.String()
}

func classifyTagsTool() llm.ToolDef {
	return llm.ToolDef{
		Name:        "classify_tags",
		Description: "Classify the ticket against candidate tags. Return reasoning and a confidence map.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"reasoning": map[string]any{
					"type":        "string",
					"description": "Brief overall analysis of the ticket.",
				},
				"scores": map[string]any{
					"type":        "object",
					"description": "Map of tag name to confidence score.",
					"additionalProperties": map[string]any{
						"type":    "number",
						"minimum": 0,
						"maximum": 1,
					},
				},
			},
			"required": []string{"reasoning", "scores"},
		},
	}
}

func buildTicketSummary(conv cmodels.Conversation, messagesText string) string {
	var b strings.Builder
	b.WriteString("Ticket:\n")
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
	b.WriteString("\nMessages:\n")
	b.WriteString(messagesText)
	return b.String()
}

func buildCandidatesBlock(tags []tagmodels.Tag) string {
	var b strings.Builder
	for _, t := range tags {
		b.WriteString("- ")
		b.WriteString(t.Name)
		b.WriteString(": ")
		if t.Detection.AI != nil && strings.TrimSpace(t.Detection.AI.Prompt) != "" {
			b.WriteString(t.Detection.AI.Prompt)
		} else {
			b.WriteString(t.Description)
		}
		b.WriteString("\n")
	}
	return b.String()
}
