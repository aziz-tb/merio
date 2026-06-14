package runner

import (
	"strings"
	"time"

	"github.com/aziz-tb/merio/internal/ai/llm"
)

// This file is the COMPOSER TERMINAL DECISION. A composer run does
// not end with produce_draft; it ends with a real decision — answer or
// escalate (AI_LAYER_DESIGN.md §4). These are platform-owned terminal tools,
// modeled on produce_draft (runtime ToolDef injection + a runtime
// system-prompt instruction + in-loop handling that completes the run). They
// are injected only for composer agents and are never stored in
// ai_agents.tools nor offered to sub-agents.
//
// The fail-safe (synthesizeEscalation) lives here too: a composer run is
// structurally incapable of ending without a decision — every path that would
// otherwise end un-terminated defaults to ESCALATE (fail safe to a human). The
// guarantee is in the platform, so the composer prompt never has to beg
// for a decision.

const (
	answerToolName   = "answer"
	escalateToolName = "escalate"
)

// isTerminalDecisionTool reports whether a tool name is one of the
// composer's terminal decision tools.
func isTerminalDecisionTool(name string) bool {
	return name == answerToolName || name == escalateToolName
}

// terminalDecisionInstructions is appended to a composer's runtime system
// prompt in place of produce_draft's instructions. Kept as one constant for
// clean localization later, mirroring produceDraftInstructions /
// callSubagentInstructions. English to stay consistent with the adjacent
// callSubagentInstructions block.
const terminalDecisionInstructions = `Platform rule for ending the run — finish with exactly one terminal decision:
- answer: submit the final customer-facing reply in "message". This completes the run; the message is the reply sent to the customer.
- escalate: hand the ticket to a human when it is not yours to resolve. Put the reason for the human in "internal_reason" (internal only, never shown to the customer); optionally set "customer_message" as a short holding note to the customer.
Call exactly one of answer or escalate as your final action, with no other tool call in that turn. Plain-text replies do not count — a reply only reaches the customer when submitted via answer.`

var answerToolDef = llm.ToolDef{
	Name: answerToolName,
	Description: `Submit your final customer-facing reply and complete the run.
This is a terminal action: calling answer ends your run with the ticket
answered. "message" must contain ONLY the customer-facing text — no reasoning,
audit notes, or internal commentary. Call exactly once, as your final action,
with no other tool call in the same turn.`,
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message": map[string]any{
				"type":        "string",
				"description": "The customer-facing reply. Plain text only.",
			},
		},
		"required": []string{"message"},
	},
}

var escalateToolDef = llm.ToolDef{
	Name: escalateToolName,
	Description: `Hand the ticket to a human and complete the run as "not for AI".
This is a terminal action. Put the reason for the human in "internal_reason"
(internal only, never shown to the customer). Optionally set "customer_message"
as a brief holding message to the customer. Call exactly once, as your final
action, with no other tool call in the same turn.`,
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"internal_reason": map[string]any{
				"type":        "string",
				"description": "Why this ticket needs a human. Internal only, never shown to the customer.",
			},
			"customer_message": map[string]any{
				"type":        "string",
				"description": "Optional short holding message shown to the customer while a human takes over.",
			},
		},
		"required": []string{"internal_reason"},
	},
}

// terminalDecision is the parsed result of a composer's terminal tool
// turn. customerText carries the answer reply, or the optional escalate
// holding message ("" when none).
type terminalDecision struct {
	kind           string
	customerText   string
	internalReason string
}

// maybeHandleTerminalDecisionTurn detects and validates a composer
// terminal decision (answer/escalate) in a tool turn. It mirrors
// maybeHandleProduceDraftTurn: handled=false means no terminal tool was called
// (the caller proceeds to normal dispatch — e.g. call_subagent); handled=true
// with decision!=nil is a committed decision; handled=true with decision==nil
// is a recoverable validation error that ends the run (the composer
// fail-safe then escalates it).
func maybeHandleTerminalDecisionTurn(turnToolUses []llm.ContentBlock) (handled bool, decision *terminalDecision, logs []ToolCallLog, partialReason string) {
	tu, present, errMsg := loneTerminalToolUse(turnToolUses, isTerminalDecisionTool)
	if !present {
		return false, nil, nil, ""
	}
	if errMsg != "" {
		full := tu.ToolName + " " + errMsg
		return true, nil, []ToolCallLog{newTerminalDecisionLog(tu, "validation_error", full, nil)}, full
	}

	switch tu.ToolName {
	case answerToolName:
		message, ok := nonEmptyStringArg(tu.ToolInput, "message")
		if !ok {
			full := `answer requires a non-empty "message" argument`
			return true, nil, []ToolCallLog{newTerminalDecisionLog(tu, "validation_error", full, nil)}, full
		}
		d := &terminalDecision{kind: DecisionKindAnswer, customerText: message}
		log := newTerminalDecisionLog(tu, "success", "", map[string]any{"committed": true, "decision": DecisionKindAnswer})
		return true, d, []ToolCallLog{log}, ""

	case escalateToolName:
		reason, ok := nonEmptyStringArg(tu.ToolInput, "internal_reason")
		if !ok {
			full := `escalate requires a non-empty "internal_reason" argument`
			return true, nil, []ToolCallLog{newTerminalDecisionLog(tu, "validation_error", full, nil)}, full
		}
		holding, _ := optionalStringArg(tu.ToolInput, "customer_message")
		d := &terminalDecision{kind: DecisionKindEscalate, customerText: holding, internalReason: reason}
		log := newTerminalDecisionLog(tu, "success", "", map[string]any{"committed": true, "decision": DecisionKindEscalate})
		return true, d, []ToolCallLog{log}, ""
	}

	// Unreachable: loneTerminalToolUse only matches isTerminalDecisionTool names.
	return false, nil, nil, ""
}

// terminalDecisionErrorResults builds the tool_result blocks that feed a
// malformed terminal decision back to the composer so it can correct itself
// on the next turn. Every tool_use in the turn gets a result (the provider API
// requires exactly one per tool_use): the terminal tool carries the specific
// validation message; any other tool in the same turn is reported as not
// executed (a terminal decision must stand alone). Nothing is dispatched — the
// composer simply retries its decision.
func terminalDecisionErrorResults(turnToolUses []llm.ContentBlock, terminalErrMsg string) []llm.ContentBlock {
	blocks := make([]llm.ContentBlock, 0, len(turnToolUses))
	for _, tu := range turnToolUses {
		msg := terminalErrMsg
		if !isTerminalDecisionTool(tu.ToolName) {
			msg = "not executed: a terminal decision (answer/escalate) must be the only tool call in its turn"
		}
		blocks = append(blocks, llm.ContentBlock{
			Type:            llm.ContentBlockTypeToolResult,
			ResultToolUseID: tu.ToolUseID,
			ResultContent:   msg,
			ResultIsError:   true,
		})
	}
	return blocks
}

// decisionDraft returns the customer-facing text a decision commits to Draft:
// the answer message, or the escalate holding message when present (nil when
// an escalation carries no customer message).
func decisionDraft(d *terminalDecision) *string {
	if d == nil {
		return nil
	}
	switch d.kind {
	case DecisionKindAnswer:
		msg := d.customerText
		return &msg
	case DecisionKindEscalate:
		if strings.TrimSpace(d.customerText) == "" {
			return nil
		}
		holding := d.customerText
		return &holding
	}
	return nil
}

// synthesizeEscalation rewrites a run response into a committed escalation.
// Used by both fail-safe layers: the runner's deferred normalizer (an
// composer loop that ended without a decision) and the activity boundary
// (a terminal transient failure after retries are exhausted). It sets
// StatusComplete + an escalate Decision so the same persistence path records
// an escalated outcome a human can see, and clears the now-superseded
// incomplete/failed signal.
func synthesizeEscalation(resp RunResponse, internalReason string) RunResponse {
	resp.Status = StatusComplete
	resp.Decision = &TerminalDecision{Kind: DecisionKindEscalate, InternalReason: internalReason}
	resp.PartialReason = ""
	resp.Draft = nil
	return resp
}

// EscalateOnTerminalError converts a terminally-failed composer run into an
// escalation so the ticket reaches a human instead of being silently dropped.
// The activity boundary calls this only after deciding the error is terminal
// (no Temporal retry pending) and of a transient class — never for bugs or
// config errors, which must fail loud. internalReason records the sanitized
// failure for the human.
func EscalateOnTerminalError(resp RunResponse, internalReason string) RunResponse {
	return synthesizeEscalation(resp, internalReason)
}

// composerNoDecisionReason builds the internal escalation reason for an
// composer loop that ended without reaching answer/escalate.
func composerNoDecisionReason(resp RunResponse) string {
	r := strings.TrimSpace(resp.PartialReason)
	if r == "" {
		r = string(resp.Status)
	}
	if r == "" {
		r = "run ended without a terminal decision"
	}
	return "composer ended without a decision: " + r
}

// nonEmptyStringArg reads a required string argument, returning the original
// (untrimmed) value and ok=true only when present, a string, and non-empty
// after trimming.
func nonEmptyStringArg(input map[string]any, key string) (val string, ok bool) {
	raw, exists := input[key]
	if !exists {
		return "", false
	}
	s, isStr := raw.(string)
	if !isStr || strings.TrimSpace(s) == "" {
		return "", false
	}
	return s, true
}

// optionalStringArg reads an optional string argument, returning ""/false when
// absent, the wrong type, or empty after trimming.
func optionalStringArg(input map[string]any, key string) (val string, ok bool) {
	return nonEmptyStringArg(input, key)
}

func newTerminalDecisionLog(tu llm.ContentBlock, status, errMsg string, data any) ToolCallLog {
	startedAt := time.Now().UTC()
	finishedAt := time.Now().UTC()
	return ToolCallLog{
		ToolKind:   "runner",
		ToolID:     tu.ToolName,
		ToolName:   terminalDecisionDisplayName(tu.ToolName),
		Input:      tu.ToolInput,
		Status:     status,
		Data:       data,
		Error:      errMsg,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		DurationMs: finishedAt.Sub(startedAt).Milliseconds(),
	}
}

func terminalDecisionDisplayName(name string) string {
	switch name {
	case answerToolName:
		return "Answer"
	case escalateToolName:
		return "Escalate"
	default:
		return name
	}
}
