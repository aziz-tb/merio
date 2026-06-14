package runner

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	agentmodels "github.com/aziz-tb/merio/internal/ai/agents/models"
	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/aziz-tb/merio/internal/ai/tools/builtin"
	"github.com/aziz-tb/merio/internal/ai/tools/resolver"
)

// This file is the APPROVAL SEAM for the run-ends-at-write mechanism (Slice 4).
// Two halves:
//   - the gate: firstGatedToolUse detects a tool whose slug is in the
//     approval-required set; the runner loop ends the run StatusAwaitingApproval
//     BEFORE dispatching it. Slug-driven and origin-agnostic — the same check
//     covers a composer write and a sub-agent write because both run through the
//     one Runner.Run loop.
//   - the resume: seedResumeMessages reconstructs the gated run's prior context
//     as a read-only transcript and authors a single assistant(tool_use) +
//     tool_result pair for the approved write (executed exactly once), a
//     rejection, or an injected sub-agent report — then the loop continues
//     composing. It never re-reasons its way back to the write.

// firstGatedToolUse returns the first tool_use in the turn whose slug is in the
// approval-required set AND resolves to a custom tool (so we have its ai_tools
// id for the record). Returns nil when nothing in the turn is gated. A slug
// that is gated by config but not a resolved custom tool for this agent is left
// to normal dispatch (surfaces as unavailable) rather than gated as a phantom.
func firstGatedToolUse(turnToolUses []llm.ContentBlock, resolvedByName map[string]resolver.ResolvedTool, approvalSlugs []string) *ProposedWrite {
	if len(approvalSlugs) == 0 || len(turnToolUses) == 0 {
		return nil
	}
	gated := make(map[string]struct{}, len(approvalSlugs))
	for _, s := range approvalSlugs {
		gated[s] = struct{}{}
	}
	for _, tu := range turnToolUses {
		if _, ok := gated[tu.ToolName]; !ok {
			continue
		}
		tool, found := resolvedByName[tu.ToolName]
		if !found || tool.Kind != agentmodels.ToolRefKindCustom || tool.Custom == nil {
			continue
		}
		return &ProposedWrite{
			ToolID:    tool.Custom.ID,
			ToolSlug:  tu.ToolName,
			ToolUseID: tu.ToolUseID,
			Inputs:    tu.ToolInput,
		}
	}
	return nil
}

// bubbledApproval returns the proposed write from a call_subagent result whose
// sub-agent paused for write approval, or nil. Lets the composer loop detect a
// sub-agent gate and end its own run awaiting approval.
func bubbledApproval(results []ToolCallLog) *ProposedWrite {
	for _, res := range results {
		if res.ToolID != callSubagentToolName {
			continue
		}
		if report, ok := res.Data.(SubAgentReport); ok && report.AwaitingApproval != nil {
			return report.AwaitingApproval
		}
	}
	return nil
}

// agentIDPtr returns a pointer to id, or nil when id is 0 (the orchestrator,
// which has no ai_agents row).
func agentIDPtr(id int) *int {
	if id <= 0 {
		return nil
	}
	return &id
}

// newAwaitingApprovalLog records the gated proposal as a runner-owned tool log
// so the trace shows where the run paused for approval.
func newAwaitingApprovalLog(pw ProposedWrite) ToolCallLog {
	now := time.Now().UTC()
	return ToolCallLog{
		ToolKind:   "runner",
		ToolID:     pw.ToolSlug,
		ToolName:   "Awaiting approval",
		Input:      pw.Inputs,
		Status:     "awaiting_approval",
		StartedAt:  now,
		FinishedAt: now,
	}
}

// seedResumeMessages builds the message history for a resuming run: the prior
// context reconstructed as a read-only transcript + the original seed, then a
// single authored assistant(tool_use)+tool_result pair for the gated call. The
// approved write is dispatched here exactly once; a rejection or an injected
// sub-agent report is fed without dispatch. Returns the messages plus a
// synthetic trace turn / tool log so the write's output joins tool_outputs.
func (r *Runner) seedResumeMessages(
	ctx context.Context, req RunRequest, agent agentmodels.Agent, composerInstructions string,
	resolvedByName map[string]resolver.ResolvedTool, rt *builtin.ToolRuntimeContext, pub EventPublisher,
) (messages []llm.Message, extraTrace []TurnTrace, extraLogs []ToolCallLog) {
	rc := req.Resume
	messages = buildResumeSeedMessages(agent.Prompt, req, composerInstructions)

	// An internally-consistent id pairing the authored tool_use with its
	// tool_result. It need not match the original (the reconstructed messages
	// are a fresh request), so the original tool_use_id stays informational.
	syntheticID := "resume_" + rc.ToolName

	var (
		resultContent string
		isErr         bool
		log           ToolCallLog
	)
	switch {
	case rc.Rejected:
		resultContent = rejectionResultText(rc.RejectionReason)
		isErr = true
		log = newResumeToolLog(rc, "rejected", resultContent)
	case rc.Held:
		resultContent = heldResultText(rc.HeldReason)
		isErr = true
		log = newResumeToolLog(rc, "held", resultContent)
	case rc.InjectedResult != nil:
		resultContent = *rc.InjectedResult
		log = newResumeToolLog(rc, "success", resultContent)
	default: // ExecuteWrite — dispatch the approved write exactly once.
		synthTU := llm.ContentBlock{
			Type: llm.ContentBlockTypeToolUse, ToolUseID: syntheticID,
			ToolName: rc.ToolName, ToolInput: rc.Inputs,
		}
		log = r.dispatchOneTool(ctx, synthTU, resolvedByName, rt,
			req.InvokerUserID, req.ConversationID, req.VerificationLevel,
			req.TraceID, 0, pub, nil, SubAgentParentContext{})
		resultContent = encodeToolResult(log)
		isErr = log.Status != "success"
	}

	messages = append(messages,
		llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{
			Type: llm.ContentBlockTypeToolUse, ToolUseID: syntheticID, ToolName: rc.ToolName, ToolInput: rc.Inputs,
		}}},
		llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{
			Type: llm.ContentBlockTypeToolResult, ResultToolUseID: syntheticID, ResultContent: resultContent, ResultIsError: isErr,
		}}},
	)
	extraTrace = []TurnTrace{{TurnIndex: 0, ToolCalls: []ToolCallLog{log}}}
	extraLogs = []ToolCallLog{log}
	return messages, extraTrace, extraLogs
}

// buildResumeSeedMessages assembles the system+inbound seed for a resuming run,
// with the prior trace rendered as a read-only transcript system block so the
// model has its prior context without replaying tool_use/thinking turns.
func buildResumeSeedMessages(systemPrompt string, req RunRequest, composerInstructions string) []llm.Message {
	out := make([]llm.Message, 0, 5)

	// Same composer/sub-agent split as a fresh run: the Composer gets its full
	// assembled ComposerSystemPrompt (skeleton + guidance + roster + decision),
	// falling back to the skeleton+decision floor; sub-agents get the runtime
	// prompt + the guidance block.
	var runtimePrompt string
	if req.IsComposer {
		if strings.TrimSpace(req.ComposerSystemPrompt) != "" {
			runtimePrompt = req.ComposerSystemPrompt
		} else {
			runtimePrompt = MinimalComposerSystemPrompt(composerInstructions)
		}
	} else {
		runtimePrompt = buildRuntimeSystemPrompt(systemPrompt)
	}
	if strings.TrimSpace(runtimePrompt) != "" {
		out = append(out, llm.Message{Role: llm.RoleSystem, Content: []llm.ContentBlock{{Type: llm.ContentBlockTypeText, Text: runtimePrompt}}})
	}
	if !req.IsComposer {
		if msg, ok := guidanceSystemBlock(req.GuidanceProse); ok {
			out = append(out, msg)
		}
	}
	if len(req.Context) > 0 {
		if body, err := json.MarshalIndent(req.Context, "", "  "); err == nil {
			out = append(out, llm.Message{Role: llm.RoleSystem, Content: []llm.ContentBlock{{Type: llm.ContentBlockTypeText, Text: "Context:\n" + string(body)}}})
		}
	}
	if formatted := formatConversationHistory(req.ConversationHistory); formatted != "" {
		out = append(out, llm.Message{Role: llm.RoleSystem, Content: []llm.ContentBlock{{Type: llm.ContentBlockTypeText, Text: formatted}}})
	}
	if req.Resume != nil {
		if transcript := formatTraceTranscript(req.Resume.PriorTrace); transcript != "" {
			out = append(out, llm.Message{Role: llm.RoleSystem, Content: []llm.ContentBlock{{Type: llm.ContentBlockTypeText, Text: transcript}}})
		}
	}
	out = append(out, llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentBlockTypeText, Text: req.InboundMessage}}})
	return out
}

// formatTraceTranscript renders a prior run's turns into a read-only context
// block for a resuming run — text + each tool call's input and (truncated)
// output. It is context, not a replay: the approved write's result is fed
// separately as the latest tool result.
func formatTraceTranscript(trace []TurnTrace) string {
	if len(trace) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Work already done on this ticket before approval (read-only context)\n")
	b.WriteString("You reasoned and acted as below. The approved action's result is provided as the latest tool result — continue from there and do not repeat actions already taken.\n")
	for _, t := range trace {
		if txt := strings.TrimSpace(t.Text); txt != "" {
			b.WriteString("\n")
			b.WriteString(txt)
			b.WriteString("\n")
		}
		for _, tc := range t.ToolCalls {
			b.WriteString("- called ")
			b.WriteString(tc.ToolID)
			if args := jsonInline(tc.Input); args != "" {
				b.WriteString("(")
				b.WriteString(args)
				b.WriteString(")")
			}
			b.WriteString(" → ")
			if tc.Status == "success" {
				b.WriteString(clipString(dataToString(tc.Data), 500))
			} else {
				b.WriteString(tc.Status)
				if tc.Error != "" {
					b.WriteString(": ")
					b.WriteString(clipString(tc.Error, 300))
				}
			}
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// rejectionResultText is the tool_result fed back when an operator rejects a
// proposed write — the agent re-reasons (same self-correct path as a malformed
// terminal decision), it does not retry the rejected action.
func rejectionResultText(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "no reason given"
	}
	return "This action was rejected by a human reviewer (" + reason + "). Do not retry it; reconsider and either answer without it or escalate."
}

// heldResultText is the tool_result fed back when a write was approved but HELD
// because the situation changed during the approval window (e.g. the customer
// replied). The action was not executed; the agent re-reasons with the change in
// view and re-proposes only if still appropriate (which re-gates for fresh
// review) — it never silently executes the stale write.
func heldResultText(detail string) string {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		detail = "the situation changed while this action awaited approval"
	}
	return "This action was approved but HELD and NOT executed because " + detail + ". Reconsider with that in view: take the action again only if it is still appropriate (it will be re-reviewed), otherwise respond or escalate accordingly."
}

func newResumeToolLog(rc *ResumeContext, status, content string) ToolCallLog {
	now := time.Now().UTC()
	log := ToolCallLog{
		ToolKind:   "runner",
		ToolID:     rc.ToolName,
		ToolName:   "Resumed " + rc.ToolName,
		Input:      rc.Inputs,
		Status:     status,
		StartedAt:  now,
		FinishedAt: now,
	}
	if status == "rejected" || status == "held" {
		log.Error = content
	} else {
		log.Data = content
	}
	return log
}

func jsonInline(v map[string]any) string {
	if len(v) == 0 {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return clipString(string(b), 300)
}

func dataToString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

func clipString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
