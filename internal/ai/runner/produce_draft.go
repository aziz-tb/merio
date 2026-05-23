package runner

import (
	"fmt"
	"strings"
	"time"

	"github.com/abhinavxd/libredesk/internal/ai/llm"
)

// produceDraftToolName is the platform-mandatory terminal tool every
// agent must call to commit its customer-facing draft. Runner-owned,
// not a built-in capability — operators cannot disable or filter it.
const produceDraftToolName = "produce_draft"

// produceDraftInstructions is appended to the operator-authored agent
// prompt at runtime only. It is never stored back into ai_agents.prompt.
// Kept as a single constant so later localization can swap it cleanly.
const produceDraftInstructions = `Interne Plattformregel für den finalen Entwurf:
- Rufe produce_draft genau einmal als letzten Schritt dieses Laufs auf.
- Übergib im Feld "draft" ausschließlich kundensichtbaren Antworttext.
- Schreibe keinen Denkprozess, keine Audit-Notizen und keine internen Kommentare in "draft".
- Beende den Lauf erst nach dem erfolgreichen Aufruf von produce_draft.`

var produceDraftToolDef = llm.ToolDef{
	Name: produceDraftToolName,
	Description: `Submit your final customer-facing draft response.
This is the terminal action of your run — calling produce_draft ends
the run and commits the draft for operator review.

The draft must contain ONLY the customer-facing text. Do not include
any reasoning, audit notes, thinking-aloud, or internal commentary.
Your internal reasoning is captured separately by the platform.

Call produce_draft exactly once, as your final action. The run cannot
complete cleanly without it.`,
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"draft": map[string]any{
				"type":        "string",
				"description": "The customer-facing draft text. Plain text only.",
			},
		},
		"required": []string{"draft"},
	},
}

func maybeHandleProduceDraftTurn(turnToolUses []llm.ContentBlock) (handled bool, draft *string, logs []ToolCallLog, partialReason string) {
	var produceUses []llm.ContentBlock
	for _, tu := range turnToolUses {
		if tu.ToolName == produceDraftToolName {
			produceUses = append(produceUses, tu)
		}
	}
	if len(produceUses) == 0 {
		return false, nil, nil, ""
	}

	if len(produceUses) != 1 || len(turnToolUses) != 1 {
		log := newProduceDraftLog(produceUses[0], "validation_error", "produce_draft must be the only tool call in its turn and appear exactly once", nil)
		return true, nil, []ToolCallLog{log}, "produce_draft must be the only tool call in its turn and appear exactly once"
	}

	raw, ok := produceUses[0].ToolInput["draft"]
	if !ok {
		log := newProduceDraftLog(produceUses[0], "validation_error", "produce_draft missing required draft argument", nil)
		return true, nil, []ToolCallLog{log}, "produce_draft called with missing draft argument"
	}
	draftText, ok := raw.(string)
	if !ok || strings.TrimSpace(draftText) == "" {
		log := newProduceDraftLog(produceUses[0], "validation_error", "produce_draft draft argument must be a non-empty string", nil)
		return true, nil, []ToolCallLog{log}, "produce_draft called with missing or empty draft argument"
	}

	trimmed := draftText
	log := newProduceDraftLog(produceUses[0], "success", "", map[string]any{"committed": true})
	return true, &trimmed, []ToolCallLog{log}, ""
}

func newProduceDraftLog(tu llm.ContentBlock, status, errMsg string, data any) ToolCallLog {
	startedAt := time.Now().UTC()
	finishedAt := time.Now().UTC()
	return ToolCallLog{
		ToolKind:   "runner",
		ToolID:     produceDraftToolName,
		ToolName:   "Produce draft",
		Input:      tu.ToolInput,
		Status:     status,
		Data:       data,
		Error:      errMsg,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		DurationMs: finishedAt.Sub(startedAt).Milliseconds(),
	}
}

func incompleteWithoutProduceDraft(reason string) string {
	if strings.TrimSpace(reason) == "" {
		return "agent ended without calling produce_draft; draft not committed"
	}
	return fmt.Sprintf("%s; produce_draft was not called, so draft was not committed", reason)
}

func buildRuntimeSystemPrompt(operatorPrompt string) string {
	operatorPrompt = strings.TrimSpace(operatorPrompt)
	if operatorPrompt == "" {
		return produceDraftInstructions
	}
	return operatorPrompt + "\n\n" + produceDraftInstructions
}
