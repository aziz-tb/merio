package airuns

import (
	"context"
	"fmt"
	"strings"

	agentmodels "github.com/aziz-tb/merio/internal/ai/agents/models"
	executionmodels "github.com/aziz-tb/merio/internal/ai/executions/models"
	"github.com/aziz-tb/merio/internal/ai/runner"
	wfairuns "github.com/aziz-tb/merio/internal/workflows/airuns"
	"github.com/zerodha/logf"
)

// Narrow dependency seams (interfaces, not concrete managers) so the invoker is
// unit-testable with fakes — the same pattern the runner uses for AgentLoader /
// UsageRecorder. *aiagents.Manager, *executions.Manager, and *runner.Runner
// satisfy these in production.
type subAgentResolver interface {
	GetBySlug(slug string) (agentmodels.Agent, error)
}

type subRunner interface {
	Run(ctx context.Context, req runner.RunRequest) (runner.RunResponse, error)
}

type subRunPersister interface {
	PersistCompleteRun(ctx context.Context, params wfairuns.AgentRunWorkflowParams, resp runner.RunResponse) (int64, error)
	PersistFailedRun(ctx context.Context, params wfairuns.AgentRunWorkflowParams, resp runner.RunResponse, msg string) (int64, error)
}

// TODO(slice-6): move orchestrator logic to internal/workflows/orchestrator.
// In slice 6 the orchestrator becomes its own Temporal workflow and runs each
// sub-agent as a CHILD workflow (reusing the bulk_reindex bounded-fan-out
// pattern). Until then a sub-agent runs synchronously, in-process, inside the
// orchestrator's RunAgentActivity via this invoker. The brief->report contract
// is designed so that move is a relocation, not a rewrite. This file is the
// only place orchestration logic lives on the airuns side; the runner stays a
// primitive behind the runner.SubAgentInvoker seam.

// subAgentInvoker implements runner.SubAgentInvoker. It runs one sub-agent to
// completion using the same Runner, persists the full sub-agent trace as a
// normal agent_executions row (source="subagent"), and returns only the report
// to the orchestrator (reads summarized in report_text; raw write outputs in
// tool_outputs).
type subAgentInvoker struct {
	runner     subRunner
	agents     subAgentResolver
	executions subRunPersister
	lo         *logf.Logger
}

// subAgentInvoker builds the invoker from the activity's deps, or returns nil
// when a dep needed to run a sub-agent is missing — in which case the runner
// simply doesn't offer call_subagent (an orchestrator logs a warning and runs
// without it).
func (a *Activities) subAgentInvoker() runner.SubAgentInvoker {
	if a == nil || a.Runner == nil || a.Agents == nil || a.Executions == nil {
		return nil
	}
	return &subAgentInvoker{
		runner:     a.Runner,
		agents:     a.Agents,
		executions: a.Executions,
		lo:         a.Lo,
	}
}

// InvokeSubAgent resolves the sub-agent by slug, runs it via the shared Runner,
// persists its trace, and returns the report. Sub-agent slugs are unique
// (ai_agents.slug UNIQUE constraint) and stable (locked after create in the
// edit UI), so a slug reference resolves deterministically; an unknown slug
// surfaces as a recoverable tool error to the orchestrator, never a silent
// break.
func (s *subAgentInvoker) InvokeSubAgent(
	ctx context.Context,
	parent runner.SubAgentParentContext,
	call runner.SubAgentCall,
) (runner.SubAgentReport, error) {
	sub, err := s.agents.GetBySlug(call.AgentRef)
	if err != nil {
		return runner.SubAgentReport{}, fmt.Errorf("unknown sub-agent %q", call.AgentRef)
	}
	// Every ai_agents row is a sub-agent now (orchestrators are not rows), so
	// there is nothing to guard against here.

	// TODO(slice-6): consider a real parent_execution_id column. For now the
	// child run's trace id is derived from the parent's plus the orchestrator
	// tool_use id (unique within the run), which links the two runs without a
	// schema change. Revisit a real FK when multi-agent parentage matters.
	childTraceID := parent.TraceID + ":sub:" + call.CallID

	subReq := runner.RunRequest{
		AgentID:             sub.ID,
		InboundMessage:      call.Brief,
		ConversationHistory: parent.History,
		ConversationID:      parent.ConversationID,
		InboxID:             parent.InboxID,
		VerificationLevel:   parent.VerificationLevel,
		InvokerUserID:       parent.InvokerUserID,
		TraceID:             childTraceID,
		// SubAgentInvoker left nil — a sub-agent cannot call sub-agents.
	}

	resp, runErr := s.runner.Run(ctx, subReq)
	s.persistSubRun(ctx, parent, sub.ID, childTraceID, resp, runErr)
	return shapeSubAgentReport(resp, runErr), nil
}

// persistSubRun writes the sub-agent's full trace as an agent_executions row so
// it shows up alongside the orchestrator run. Best effort: a persistence
// failure never fails the orchestrator's run.
func (s *subAgentInvoker) persistSubRun(
	ctx context.Context,
	parent runner.SubAgentParentContext,
	subAgentID int,
	childTraceID string,
	resp runner.RunResponse,
	runErr error,
) {
	params := wfairuns.AgentRunWorkflowParams{
		TraceID:           childTraceID,
		Source:            executionmodels.SourceSubagent,
		AgentID:           subAgentID,
		ConversationID:    parent.ConversationID,
		InboxID:           parent.InboxID,
		VerificationLevel: parent.VerificationLevel,
		InvokerUserID:     parent.InvokerUserID,
	}
	var perr error
	if runErr != nil {
		_, perr = s.executions.PersistFailedRun(ctx, params, resp, runErr.Error())
	} else {
		_, perr = s.executions.PersistCompleteRun(ctx, params, resp)
	}
	if perr != nil && s.lo != nil {
		s.lo.Warn("call_subagent: failed to persist sub-agent execution",
			"trace_id", childTraceID, "error", perr)
	}
}

// shapeSubAgentReport turns a sub-agent RunResponse into the report the
// orchestrator sees. report_text is ALWAYS non-empty so the orchestrator can
// always reason about the outcome (an incomplete sub-agent yields an explicit
// "could not complete" line, never ""). tool_outputs is a typed empty slice in
// slice 1 (no write tools yet); slice 4 fills it by filtering write-category
// tool calls — the shape is unchanged across that slice.
func shapeSubAgentReport(resp runner.RunResponse, runErr error) runner.SubAgentReport {
	report := runner.SubAgentReport{
		ToolOutputs: []runner.SubAgentToolOutput{},
	}
	switch {
	case runErr != nil:
		report.ReportText = "sub-agent could not complete: " + runErr.Error()
	case resp.Draft != nil && strings.TrimSpace(*resp.Draft) != "":
		report.ReportText = strings.TrimSpace(*resp.Draft)
	default:
		reason := strings.TrimSpace(resp.PartialReason)
		if reason == "" {
			reason = string(resp.Status)
		}
		if reason == "" {
			reason = "no report produced"
		}
		report.ReportText = "sub-agent could not complete: " + reason
	}
	// TODO(slice-4): populate ToolOutputs from resp.ToolCalls filtered to
	// write-category tools (reads stay summarized in report_text).
	return report
}
