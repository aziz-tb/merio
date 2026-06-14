package airuns

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/aziz-tb/merio/internal/ai/runner"
	approvalsmodels "github.com/aziz-tb/merio/internal/approvals/models"
	convmodels "github.com/aziz-tb/merio/internal/conversation/models"
	wfairuns "github.com/aziz-tb/merio/internal/workflows/airuns"
	"github.com/volatiletech/null/v9"
)

// This file is the APPROVAL RESUME orchestration (Slice 4b). A resolved
// approval (approve/edit/reject) drives a resuming run that reuses the
// committed runner resume mechanism (Slice 4a). Three shapes:
//   - orchestrator-own write  → one composer resume (execute/reject, continue)
//   - single-agent write      → one agent resume
//   - sub-agent write         → the in-place 2-step: resume the sub-agent
//     (execute the write, finish its report), then resume the composer feeding
//     that report at its call_subagent point. One activity, sequential, no
//     durable cross-run state — the report is the natural call_subagent result.
//
// runResume is split out so it takes already-loaded inputs + a Runner: the DB
// I/O (load approval, traces, tool slug; re-derive the seed) lives in the
// activity entry (resumeApproval), and the orchestration is unit-testable with
// a fake-backed Runner — including the contract-sensitive sub-agent 2-step.

// resumeSeed is the conversation context a resuming run is rebuilt from. The
// persisted trace is the substance (the actual reasoning up to the gate); this
// seed is the minor framing input, re-derived from the conversation at resume
// time (so a customer message arriving during the approval gap is naturally
// picked up — the situation changed).
type resumeSeed struct {
	conversationID    int
	inboxID           int
	verificationLevel int
	invokerUserID     int
	inbound           string
	context           map[string]any
	history           []runner.ConversationMessage
	approvalSlugs     []string
	deadline          time.Duration
	eventPublisher    runner.EventPublisher
}

// resumeInputs is everything runResume needs, loaded/decided by the caller.
type resumeInputs struct {
	// Origin shape.
	subAgentGate bool // parent_trace_id set → the 2-step
	agentID      *int // the gated run's agent (sub-agent or single-agent); nil = orchestrator-own

	// The gated write.
	toolSlug        string
	toolUseID       string
	inputs          map[string]any // approved/edited inputs (nil on reject)
	rejected        bool
	rejectionReason string

	// held marks an approved write that must NOT auto-execute because the
	// situation changed during the approval window (a customer replied — 4c).
	// The resume feeds a neutral reconsider notice instead of dispatching, so
	// the agent re-decides with the new message in view rather than acting blind.
	held       bool
	heldReason string

	// Persisted traces (substance).
	gatedTrace    []runner.TurnTrace // the loop that gated (sub-agent trace for a sub-agent gate; else the gated run)
	composerTrace []runner.TurnTrace // the composer/parent trace (sub-agent gate only)

	// Fresh trace ids for the resuming run(s).
	resumeTraceID string

	seed resumeSeed
}

func baseResumeReq(seed resumeSeed, traceID string) runner.RunRequest {
	return runner.RunRequest{
		InboundMessage:        seed.inbound,
		Context:               seed.context,
		ConversationHistory:   seed.history,
		ConversationID:        seed.conversationID,
		InboxID:               seed.inboxID,
		VerificationLevel:     seed.verificationLevel,
		InvokerUserID:         seed.invokerUserID,
		TraceID:               traceID,
		ApprovalRequiredSlugs: seed.approvalSlugs,
		Deadline:              seed.deadline,
		EventPublisher:        seed.eventPublisher,
	}
}

// runResume executes the resume for a resolved approval. It reuses the runner's
// resume modes (ExecuteWrite / Rejected / InjectedResult). r is the subRunner
// seam (*runner.Runner in production, a fake in tests). invoker is attached to
// composer resumes so any further delegation after the injected report still
// works; it may be nil in tests where the composer answers immediately.
func runResume(ctx context.Context, r subRunner, invoker runner.SubAgentInvoker, in resumeInputs) (runner.RunResponse, error) {
	writeResume := &runner.ResumeContext{
		ToolName:        in.toolSlug,
		ToolUseID:       in.toolUseID,
		Inputs:          in.inputs,
		Rejected:        in.rejected,
		RejectionReason: in.rejectionReason,
		Held:            in.held,
		HeldReason:      in.heldReason,
		// Execute only a clean approval: a reject or a staleness hold feeds a
		// notice instead of dispatching. The sub-agent step copies this struct,
		// so a hold propagates to the 2-step unchanged.
		ExecuteWrite: !in.rejected && !in.held,
	}

	if in.subAgentGate {
		// Step 1: resume the sub-agent loop — execute (or reject) the approved
		// write, then it finishes its report.
		subResume := *writeResume
		subResume.PriorTrace = in.gatedTrace
		subReq := baseResumeReq(in.seed, in.resumeTraceID+":sub")
		subReq.AgentID = deref(in.agentID)
		subReq.Resume = &subResume
		subResp, err := r.Run(ctx, subReq)
		if err != nil {
			return subResp, err
		}

		// Step 2: resume the composer, feeding the sub-agent's report as the
		// call_subagent result it was waiting on.
		report := shapeSubAgentReport(subResp, nil)
		injected := marshalReport(report)
		compReq := baseResumeReq(in.seed, in.resumeTraceID)
		compReq.IsComposer = true
		compReq.SubAgentInvoker = invoker
		compReq.Resume = &runner.ResumeContext{
			PriorTrace:     in.composerTrace,
			ToolName:       runner.CallSubagentToolName,
			InjectedResult: &injected,
		}
		return r.Run(ctx, compReq)
	}

	// Single-loop resume: orchestrator-own or single-agent.
	writeResume.PriorTrace = in.gatedTrace
	req := baseResumeReq(in.seed, in.resumeTraceID)
	req.Resume = writeResume
	if in.agentID != nil {
		req.AgentID = *in.agentID
	} else {
		req.IsComposer = true
		req.SubAgentInvoker = invoker
	}
	return r.Run(ctx, req)
}

func marshalReport(report runner.SubAgentReport) string {
	b, err := json.Marshal(report)
	if err != nil {
		return report.ReportText
	}
	return string(b)
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// resumeApproval is the activity entry: it loads the resolved approval + the
// gated run's persisted trace(s) + the tool slug, re-derives a minimal seed,
// and runs the resume. The DB I/O lives here; the orchestration (runResume) is
// unit-tested with a fake runner.
func (a *Activities) resumeApproval(ctx context.Context, params wfairuns.AgentRunWorkflowParams, pub runner.EventPublisher) (runner.RunResponse, error) {
	failed := runner.RunResponse{Status: runner.StatusFailed}
	if a.Approvals == nil || a.Executions == nil || a.Tools == nil || a.Conversation == nil {
		return failed, errors.New("resume: approvals/executions/tools/conversation manager is required")
	}

	ap, err := a.Approvals.GetByID(params.ResumeApprovalID)
	if err != nil {
		return failed, err
	}
	tool, err := a.Tools.Get(ap.ToolID)
	if err != nil {
		return failed, err
	}
	gatedExec, err := a.Executions.GetByTraceID(ap.GatedRunTraceID)
	if err != nil {
		return failed, err
	}

	// 4c: re-derive the conversation so the resume reasons against current
	// reality, and detect customer messages that landed during the approval
	// window. A gap on an approve/edit HOLDS the write. Fail closed — a load
	// failure aborts (Temporal retries) rather than risk executing a stale write.
	conv, err := a.Conversation.GetConversation(int(ap.ConversationID), "", "")
	if err != nil {
		return failed, err
	}
	msgs, err := a.Conversation.LoadMessagesForLLM(ctx, conv.UUID)
	if err != nil {
		return failed, err
	}
	rejected := ap.Status == approvalsmodels.StatusRejected
	gap := gapCustomerMessages(msgs, ap.CreatedAt)
	held := len(gap) > 0 && !rejected

	in := resumeInputs{
		subAgentGate:    ap.ParentTraceID.Valid,
		agentID:         nullIntToPtr(ap.AgentID),
		toolSlug:        tool.Slug,
		toolUseID:       ap.ToolUseID,
		inputs:          decodeJSONMap(ap.ApprovedInputs),
		rejected:        rejected,
		rejectionReason: ap.RejectionReason.String,
		held:            held,
		heldReason:      formatGapReason(gap),
		gatedTrace:      decodeTrace(gatedExec.Trace),
		resumeTraceID:   params.TraceID,
		seed:            a.buildResumeSeed(params, conv, toRunnerHistory(msgs), pub),
	}
	if in.subAgentGate {
		compExec, cErr := a.Executions.GetByTraceID(ap.ParentTraceID.String)
		if cErr != nil {
			return failed, cErr
		}
		in.composerTrace = decodeTrace(compExec.Trace)
	}

	return runResume(ctx, a.Runner, a.subAgentInvoker(), in)
}

const resumeContinuationPrompt = "Continue and finalize your response, grounded in the work already done, the conversation history, and the latest tool result above."

// buildResumeSeed assembles the framing seed for a resuming run from the CURRENT
// conversation — history + verification level — so the resumed agent sees any
// message that arrived during the approval window. The persisted trace remains
// the substance (prior reasoning); this seed is the up-to-date framing. Customer/
// contract context-map hydration (inline in the fresh-run handler) is not
// reachable from the activity and is left to the trace for now.
func (a *Activities) buildResumeSeed(params wfairuns.AgentRunWorkflowParams, conv convmodels.Conversation, history []runner.ConversationMessage, pub runner.EventPublisher) resumeSeed {
	var deadline time.Duration
	if params.DeadlineMs > 0 {
		deadline = time.Duration(params.DeadlineMs) * time.Millisecond
	}
	return resumeSeed{
		conversationID:    conv.ID,
		inboxID:           params.InboxID,
		verificationLevel: conv.VerificationLevel,
		invokerUserID:     params.InvokerUserID,
		inbound:           resumeContinuationPrompt,
		history:           history,
		approvalSlugs:     a.approvalSlugs(params.InboxID),
		deadline:          deadline,
		eventPublisher:    pub,
	}
}

// gapCustomerMessages returns customer (contact) messages created after `since`
// — those that arrived during the approval window. Pure (no DB) so the
// staleness decision is unit-testable.
func gapCustomerMessages(msgs []convmodels.Message, since time.Time) []convmodels.Message {
	var gap []convmodels.Message
	for _, m := range msgs {
		if m.SenderType == convmodels.SenderTypeContact && m.CreatedAt.After(since) {
			gap = append(gap, m)
		}
	}
	return gap
}

// toRunnerHistory maps conversation messages to the runner's history shape,
// labelling the contact sender as "customer" (what formatConversationHistory
// expects). Empty-text messages are dropped.
func toRunnerHistory(msgs []convmodels.Message) []runner.ConversationMessage {
	out := make([]runner.ConversationMessage, 0, len(msgs))
	for _, m := range msgs {
		text := messageText(m)
		if text == "" {
			continue
		}
		sender := "agent"
		if m.SenderType == convmodels.SenderTypeContact {
			sender = "customer"
		}
		out = append(out, runner.ConversationMessage{Sender: sender, Text: text})
	}
	return out
}

// formatGapReason renders the gap messages into the neutral held-reason detail
// the resume seam feeds back (heldResultText), so the agent reconsiders with the
// customer's words in view.
func formatGapReason(gap []convmodels.Message) string {
	parts := make([]string, 0, len(gap))
	for _, m := range gap {
		if t := messageText(m); t != "" {
			parts = append(parts, "\""+clipText(t, 280)+"\"")
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "the customer replied while it awaited approval: " + strings.Join(parts, "; ")
}

func messageText(m convmodels.Message) string {
	if t := strings.TrimSpace(m.TextContent); t != "" {
		return t
	}
	return strings.TrimSpace(m.Content)
}

func clipText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func nullIntToPtr(n null.Int) *int {
	if !n.Valid {
		return nil
	}
	v := n.Int
	return &v
}

func decodeTrace(raw json.RawMessage) []runner.TurnTrace {
	if len(raw) == 0 {
		return nil
	}
	var t []runner.TurnTrace
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil
	}
	return t
}

func decodeJSONMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}
