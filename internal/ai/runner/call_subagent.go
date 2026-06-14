package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	agentmodels "github.com/aziz-tb/merio/internal/ai/agents/models"
	"github.com/aziz-tb/merio/internal/ai/llm"
)

// This file is the COMPOSER SEAM. Everything specific to the
// composer -> sub-agent boundary lives here so the generic runner loop
// (runner.go) and tool dispatch (dispatch.go) stay primitives that know
// nothing about orchestration. The runner only knows two things:
//   - inject this tool for composer-role agents that carry an invoker, and
//   - route a call_subagent tool_use to the injected SubAgentInvoker.
//
// HOW a sub-agent actually runs (a full Runner.Run, persistence of its trace,
// and shaping of its report) lives entirely behind the SubAgentInvoker seam —
// today in internal/activities/airuns/subagent.go.
//
// TODO(slice-6): move composer logic to internal/workflows/composer.
// When the composer becomes its own workflow running each sub-agent as a
// child workflow, this seam moves with it. The contract below (brief in ->
// report + tool_outputs out) is designed to survive that move unchanged.

// callSubagentToolName is the platform-owned, composer-only tool the
// composer's LLM calls to delegate a scoped task to a specialized
// sub-agent. Runner-owned and injected at runtime like produce_draft; never
// stored in ai_agents.tools and never offered to sub-agents.
const callSubagentToolName = "call_subagent"

// CallSubagentToolName is the exported tool name, used by the approval resume
// path to inject a resumed sub-agent's report back into the composer at its
// call_subagent point.
const CallSubagentToolName = callSubagentToolName

// callSubagentInstructions is appended to a composer's runtime system
// prompt (after the produce_draft rule). Kept as one constant for clean
// localization later, mirroring produceDraftInstructions.
const callSubagentInstructions = `Platform rule for delegating to sub-agents:
- Use call_subagent to hand a scoped task to a specialized sub-agent, named by its slug.
- Pass a clear "brief": what you need that sub-agent to do for this customer.
- It returns a short report of what it did and found, plus the raw outputs of any actions it took. Treat the report like a colleague's hand-off; you compose the final reply yourself.
- A sub-agent never replies to the customer and never decides the ticket — only you do.`

// answerVsDelegateInstructions is the platform default for the general tier
// (Slice 5, §2): the Composer answers general questions directly and delegates
// domain work to a sub-agent. Appended to the composer prompt when delegation is
// available (an invoker is attached); operator answer-vs-delegate guidance rides
// on top as ordinary guidance items. The capability boundary backs this up — the
// Composer lacks domain tools/KB, so it cannot do domain work even if it tried.
const answerVsDelegateInstructions = `Answering vs delegating — decide per customer request:
- ANSWER DIRECTLY, in one pass, for general questions you can cover from your own knowledge, general tools, and general knowledge base: opening hours, company info, how-to, general policy, greetings/status. Do not delegate these.
- DELEGATE to a specialist sub-agent (call_subagent) for anything needing domain expertise or a domain action: billing/invoices, contracts/cancellation, meter readings, account-specific data or changes. You do not have domain tools or domain knowledge by design — delegate instead of guessing.
- A mixed message can be both: answer the general part yourself AND delegate the domain part, then compose one reply.
- When a request is domain-specific and you are unsure, delegate.`

var callSubagentToolDef = llm.ToolDef{
	Name: callSubagentToolName,
	Description: `Delegate a scoped task to a specialized sub-agent and receive its report.
Calling this runs a full sub-agent loop under the hood; it does NOT end your run.
You receive back { report_text, tool_outputs }: report_text is the sub-agent's
own summary of what it did and found; tool_outputs holds the raw output of any
write action it ran. Use the report to plan your next step or compose the reply.`,
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent": map[string]any{
				"type":        "string",
				"description": "The sub-agent's slug (e.g. \"billing\", \"cancellation\").",
			},
			"brief": map[string]any{
				"type":        "string",
				"description": "The scoped task for the sub-agent, in natural language.",
			},
		},
		"required": []string{"agent", "brief"},
	},
}

// SubAgentParentContext is the slice of the composer's RunRequest the
// invoker needs to build the sub-agent run. The runner builds it once and
// threads it through dispatch; the invoker reads it.
type SubAgentParentContext struct {
	ConversationID    int
	InboxID           int
	VerificationLevel int
	InvokerUserID     int
	TraceID           string
	History           []ConversationMessage

	// ApprovalRequiredSlugs threads the composer's approval gate set into each
	// sub-agent run so the gate is origin-agnostic.
	ApprovalRequiredSlugs []string

	// GuidanceProse threads the composer's assembled guidance into each
	// sub-agent run so guidance reaches domain work too (Slice 5).
	GuidanceProse string
}

// SubAgentCall is one call_subagent invocation parsed from a tool_use block.
// CallID is the composer's tool_use id — unique within the run — so the
// invoker can derive a stable, unique child trace id.
type SubAgentCall struct {
	AgentRef string
	Brief    string
	CallID   string
}

// SubAgentToolOutput is the raw output of one write-category tool a sub-agent
// ran. The composer reports these to the customer as fact, so they travel
// unparaphrased (reads stay summarized in ReportText).
//
// Slice 1 ships no write tools, so the invoker returns an empty (non-nil)
// slice. Slice 4 populates it by filtering the sub-agent's tool calls to
// write-category outputs — the shape here is exactly the one Slice 4 fills,
// so the contract does not change.
type SubAgentToolOutput struct {
	ToolID   string `json:"tool_id"`
	ToolName string `json:"tool_name,omitempty"`
	Status   string `json:"status"`
	Data     any    `json:"data,omitempty"`
	Error    string `json:"error,omitempty"`
}

// SubAgentReport is what a sub-agent run returns to the composer: a short
// natural-language summary (reads already interpreted) plus raw write outputs.
// Only this travels up; the full sub-agent trace is persisted separately for
// humans.
type SubAgentReport struct {
	ReportText  string               `json:"report_text"`
	ToolOutputs []SubAgentToolOutput `json:"tool_outputs"`

	// AwaitingApproval is set when the sub-agent paused at an approval-gated
	// write instead of completing. It bubbles up so the composer run ends
	// StatusAwaitingApproval too (it can't compose without the sub-agent's real
	// result). The approval record is persisted by the invoker against the
	// sub-agent's run; the composer run carries no separate record. Slice 4.
	AwaitingApproval *ProposedWrite `json:"awaiting_approval,omitempty"`
}

// SubAgentInvoker runs one sub-agent and returns its report. It is the seam
// between the generic runner and orchestration: the runner calls it when an
// composer emits call_subagent; the implementation
// (internal/activities/airuns/subagent.go) runs a full Runner.Run, persists
// the sub-agent trace, and shapes the report. A nil invoker on a RunRequest
// means the run cannot call sub-agents — true for every generic agent and for
// every recursively-invoked sub-agent run, which is what makes recursion
// impossible.
type SubAgentInvoker interface {
	InvokeSubAgent(ctx context.Context, parent SubAgentParentContext, call SubAgentCall) (SubAgentReport, error)
}

// composerAgentSlug labels the synthetic platform composer in traces
// and events. The composer is NOT an ai_agents row — it is a platform
// component executed via the Runner — so this is a display label, not a
// lookup key.
const composerAgentSlug = "composer"

// composerStubPrompt is the platform-owned composer system prompt. The
// composer is configured by the platform, not authored as an agent, so its
// prompt lives in code rather than ai_agents.prompt.
//
// TODO(slice-3): composer behavioral guidance comes from Composer config —
// this stub is replaced/augmented by per-inbox guidance items resolved at run
// start. For now it is a minimal placeholder so the loop has a system prompt.
const composerStubPrompt = `You are the support composer. You own the conversation: decide which specialized sub-agents to consult and in what order, then compose the single customer-facing reply yourself. Delegate scoped tasks to sub-agents via call_subagent and use their reports to write the final draft.`

// platformComposerAgent is the synthetic agent config a composer run executes
// with. The composer is not loaded from ai_agents (it is a platform component,
// not a user-authored agent), so the runner uses this instead of Agents.Get when
// RunRequest.IsComposer is set. Its general-tier capability — general tool refs +
// general KB scope (Slice 5, §2) — is supplied by the caller from Composer
// config and flows through the SAME resolve path sub-agents use; on top, the
// runner injects the platform-owned answer/escalate (+ call_subagent) tools.
// Empty tools/scope = no general capability (answer from the base prompt only).
func platformComposerAgent(tools, kbScope json.RawMessage) agentmodels.Agent {
	return agentmodels.Agent{
		Slug:          composerAgentSlug,
		Prompt:        composerStubPrompt,
		Enabled:       true,
		MaxIterations: defaultMaxIterations,
		Tools:         tools,
		KBScope:       kbScope,
	}
}

// subAgentParentContext projects the composer's RunRequest into the
// context the invoker needs to build the sub-agent run.
func subAgentParentContext(req RunRequest) SubAgentParentContext {
	return SubAgentParentContext{
		ConversationID:        req.ConversationID,
		InboxID:               req.InboxID,
		VerificationLevel:     req.VerificationLevel,
		InvokerUserID:         req.InvokerUserID,
		TraceID:               req.TraceID,
		History:               req.ConversationHistory,
		ApprovalRequiredSlugs: req.ApprovalRequiredSlugs,
		GuidanceProse:         req.GuidanceProse,
	}
}

// handleCallSubagent routes a call_subagent tool_use to the invoker and shapes
// the result as a ToolCallLog the runner feeds back as the tool result. It
// never aborts the run: a missing invoker, bad input, or a failed invocation
// all surface as an error tool result the composer can re-reason about
// (re-plan, ask, or escalate) — never a silent break. Emits the same
// dispatched/completed events as a normal tool so the live trace is uniform.
func (r *Runner) handleCallSubagent(
	ctx context.Context,
	tu llm.ContentBlock,
	invoker SubAgentInvoker,
	parent SubAgentParentContext,
	traceID string,
	turnIndex int,
	pub EventPublisher,
) (logEntry ToolCallLog) {
	startedAt := time.Now().UTC()
	logEntry = ToolCallLog{
		ToolKind:  "runner",
		ToolID:    callSubagentToolName,
		ToolName:  "Call sub-agent",
		Input:     tu.ToolInput,
		StartedAt: startedAt,
	}
	r.emit(ctx, traceID, EventTypeToolDispatched, ToolDispatchedPayload{
		TurnIndex: turnIndex,
		ToolID:    callSubagentToolName,
		ToolName:  logEntry.ToolName,
		ToolKind:  logEntry.ToolKind,
		Input:     tu.ToolInput,
	}, pub)
	defer func() {
		finishedAt := time.Now().UTC()
		logEntry.FinishedAt = finishedAt
		logEntry.DurationMs = finishedAt.Sub(startedAt).Milliseconds()
		r.emit(ctx, traceID, EventTypeToolCompleted, ToolCompletedPayload{
			TurnIndex:  turnIndex,
			ToolID:     callSubagentToolName,
			ToolName:   logEntry.ToolName,
			ToolKind:   logEntry.ToolKind,
			Status:     logEntry.Status,
			DurationMs: logEntry.DurationMs,
			Data:       logEntry.Data,
			Error:      logEntry.Error,
		}, pub)
	}()

	if invoker == nil {
		logEntry.Status = "unavailable"
		logEntry.Error = "call_subagent is not available for this run"
		return logEntry
	}
	agentRef, brief, perr := parseCallSubagentInput(tu.ToolInput)
	if perr != nil {
		logEntry.Status = "validation_error"
		logEntry.Error = perr.Error()
		return logEntry
	}

	report, err := invoker.InvokeSubAgent(ctx, parent, SubAgentCall{
		AgentRef: agentRef,
		Brief:    brief,
		CallID:   tu.ToolUseID,
	})
	if err != nil {
		logEntry.Status = "execution_error"
		logEntry.Error = err.Error()
		return logEntry
	}
	logEntry.Status = "success"
	logEntry.Data = report
	return logEntry
}

// parseCallSubagentInput validates the model-supplied arguments. Both fields
// are required and must be non-empty strings; anything else is a recoverable
// validation error the composer can correct on its next turn.
func parseCallSubagentInput(input map[string]any) (agentRef, brief string, err error) {
	rawAgent, ok := input["agent"]
	if !ok {
		return "", "", fmt.Errorf("call_subagent: missing required \"agent\" argument")
	}
	agentRef, ok = rawAgent.(string)
	if !ok || strings.TrimSpace(agentRef) == "" {
		return "", "", fmt.Errorf("call_subagent: \"agent\" must be a non-empty string (the sub-agent slug)")
	}
	rawBrief, ok := input["brief"]
	if !ok {
		return "", "", fmt.Errorf("call_subagent: missing required \"brief\" argument")
	}
	brief, ok = rawBrief.(string)
	if !ok || strings.TrimSpace(brief) == "" {
		return "", "", fmt.Errorf("call_subagent: \"brief\" must be a non-empty string")
	}
	return strings.TrimSpace(agentRef), brief, nil
}
