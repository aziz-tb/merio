package runner

import (
	"fmt"
	"strings"
	"time"

	"github.com/aziz-tb/merio/internal/ai/llm"
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
	tu, present, errMsg := loneTerminalToolUse(turnToolUses, func(name string) bool { return name == produceDraftToolName })
	if !present {
		return false, nil, nil, ""
	}
	if errMsg != "" {
		full := produceDraftToolName + " " + errMsg
		return true, nil, []ToolCallLog{newProduceDraftLog(tu, "validation_error", full, nil)}, full
	}

	raw, ok := tu.ToolInput["draft"]
	if !ok {
		log := newProduceDraftLog(tu, "validation_error", "produce_draft missing required draft argument", nil)
		return true, nil, []ToolCallLog{log}, "produce_draft called with missing draft argument"
	}
	draftText, ok := raw.(string)
	if !ok || strings.TrimSpace(draftText) == "" {
		log := newProduceDraftLog(tu, "validation_error", "produce_draft draft argument must be a non-empty string", nil)
		return true, nil, []ToolCallLog{log}, "produce_draft called with missing or empty draft argument"
	}

	trimmed := draftText
	log := newProduceDraftLog(tu, "success", "", map[string]any{"committed": true})
	return true, &trimmed, []ToolCallLog{log}, ""
}

// loneTerminalToolUse finds the single terminal tool_use in a turn. A terminal
// tool (produce_draft for ordinary agents; answer/escalate for composers)
// must be the ONLY tool call in its turn and appear exactly once — terminating
// a run is not something to mix with other work. Returns present=false when no
// terminal tool was called, so the caller proceeds to normal tool dispatch.
// When a terminal tool is present but the turn is malformed (mixed with other
// tools, or duplicated), errMsg is non-empty (sans tool-name prefix) so the
// caller surfaces a recoverable validation error naming its own tool.
func loneTerminalToolUse(turnToolUses []llm.ContentBlock, isTerminal func(name string) bool) (tu llm.ContentBlock, present bool, errMsg string) {
	var terminal []llm.ContentBlock
	for _, t := range turnToolUses {
		if isTerminal(t.ToolName) {
			terminal = append(terminal, t)
		}
	}
	if len(terminal) == 0 {
		return llm.ContentBlock{}, false, ""
	}
	if len(terminal) != 1 || len(turnToolUses) != 1 {
		return terminal[0], true, "must be the only tool call in its turn and appear exactly once"
	}
	return terminal[0], true, ""
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

// joinPromptSections joins two prompt sections with a blank line, dropping
// either when empty. Used to assemble a composer's runtime prompt
// (operator/stub prompt + terminal-decision/call_subagent instructions)
// without the produce_draft rule.
func joinPromptSections(a, b string) string {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + "\n\n" + b
	}
}
