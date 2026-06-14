package airuns

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	activitydeps "github.com/aziz-tb/merio/internal/activities/dependencies"
	aiagents "github.com/aziz-tb/merio/internal/ai/agents"
	executions "github.com/aziz-tb/merio/internal/ai/executions"
	executionmodels "github.com/aziz-tb/merio/internal/ai/executions/models"
	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/aziz-tb/merio/internal/ai/runner"
	runnerevents "github.com/aziz-tb/merio/internal/ai/runner/events"
	aitools "github.com/aziz-tb/merio/internal/ai/tools"
	"github.com/aziz-tb/merio/internal/approvals"
	"github.com/aziz-tb/merio/internal/composer"
	"github.com/aziz-tb/merio/internal/conversation"
	wfairuns "github.com/aziz-tb/merio/internal/workflows/airuns"
	"github.com/zerodha/logf"
	"go.temporal.io/sdk/activity"
)

type Activities struct {
	Runner       *runner.Runner
	Agents       *aiagents.Manager
	Tools        *aitools.Manager
	Executions   *executions.Manager
	Composer     *composer.Manager
	Approvals    *approvals.Manager
	Conversation *conversation.Manager
	Events       runner.EventPublisher
	Lo           *logf.Logger
}

func NewActivities(deps activitydeps.Dependencies) (*Activities, error) {
	return &Activities{
		Runner:       deps.AgentRunner,
		Agents:       deps.Agents,
		Tools:        deps.Tools,
		Executions:   deps.Executions,
		Composer:     deps.Composer,
		Approvals:    deps.Approvals,
		Conversation: deps.Conversation,
		Events:       deps.RunEvents,
		Lo:           deps.Logger("ai-run-activities"),
	}, nil
}

func (a *Activities) RunAgentActivity(ctx context.Context, params wfairuns.AgentRunWorkflowParams) (*wfairuns.AgentRunResult, error) {
	if a == nil || a.Runner == nil {
		return nil, errors.New("ai run activity: runner is required")
	}
	if a.Executions == nil {
		return nil, errors.New("ai run activity: executions manager is required")
	}
	if a.Lo == nil {
		return nil, errors.New("ai run activity: logger is required")
	}

	hbCtx, stopHeartbeat := context.WithCancel(ctx)
	defer stopHeartbeat()
	go a.heartbeatLoop(hbCtx)

	pub := a.publisherForSource(params.Source)

	var resp runner.RunResponse
	var runErr error
	if params.ResumeApprovalID > 0 {
		// Resume a resolved write approval (Slice 4b): reconstruct context from
		// the gated run's trace and run the resume (single-loop or sub-agent
		// 2-step). The result flows through the same persist/gate/event path.
		resp, runErr = a.resumeApproval(ctx, params, pub)
	} else {
		req := buildRunRequest(params)
		req.EventPublisher = pub
		// The runner uses the invoker only on a composer run (gated on
		// RunRequest.IsComposer); sub-agent runs it spawns leave it nil, so
		// recursion can't occur.
		req.SubAgentInvoker = a.subAgentInvoker()
		req.ApprovalRequiredSlugs = a.approvalSlugs(params.InboxID)
		resp, runErr = a.Runner.Run(ctx, req)
	}
	if runErr != nil {
		// Composer fail-safe (Layer 2): a terminal transient failure with no
		// Temporal retry pending becomes an escalation, so the ticket reaches a
		// human instead of being dropped. Bugs/config errors, unrecognized
		// errors, and runs that still have a retry pending fall through to the
		// loud-failure path below — preserving Temporal's retry net and never
		// masking a real bug as "escalated". See composerErrorDisposition.
		willRetry := activity.GetInfo(ctx).Attempt < wfairuns.RunAgentMaxAttempts
		if composerErrorDisposition(runErr, resp.IsComposer, willRetry) == dispositionEscalate {
			resp = escalateAfterTerminalError(resp, runErr)
			runErr = nil
			// Fall through to the complete-run path: it records the escalated
			// outcome (PersistCompleteRun) and emits run.completed.
		} else {
			executionID, persistErr := a.Executions.PersistFailedRun(ctx, params, resp, runErr.Error())
			a.emitTerminalFailure(ctx, params, resp, executionID, runErr)
			if persistErr != nil {
				return nil, persistErr
			}
			return nil, runErr
		}
	}

	// Approval gate: a run that stopped at an approval-required write persists a
	// pending record (idempotent — the partial-unique index dedupes activity
	// retries). The run itself is still persisted below so a resume can read its
	// trace.
	if resp.Status == runner.StatusAwaitingApproval && resp.ProposedWrite != nil {
		a.persistGateApproval(params, resp.ProposedWrite)
	}

	executionID, persistErr := a.Executions.PersistCompleteRun(ctx, params, resp)
	if persistErr != nil {
		a.emitTerminalFailure(ctx, params, resp, 0, persistErr)
		return nil, persistErr
	}

	eventType := runner.EventTypeRunCompleted
	if resp.Status == runner.StatusAwaitingApproval {
		eventType = runner.EventTypeAwaitingApproval
	}
	payload := runner.RunCompletedPayload{
		ExecutionID: executionID,
		Source:      params.Source,
		Status:      resp.Status,
	}
	if params.Source == executionmodels.SourceComposer {
		payload.Response = &resp
	}
	if pub != nil {
		if err := pub.Publish(ctx, params.TraceID, runner.RunEvent{
			Type:      eventType,
			EmittedAt: time.Now().UTC(),
			Payload:   mustMarshalPayload(payload),
		}); err != nil {
			a.Lo.Warn("ai-run activity: failed to publish run.completed",
				"trace_id", params.TraceID,
				"error", err,
			)
		}
	}

	return &wfairuns.AgentRunResult{
		ExecutionID:  executionID,
		TraceID:      params.TraceID,
		Status:       string(resp.Status),
		DraftPresent: resp.Draft != nil,
	}, nil
}

func (a *Activities) publisherForSource(source string) runner.EventPublisher {
	if source == executionmodels.SourceComposer && a.Events != nil {
		return a.Events
	}
	return runnerevents.NoopPublisher{}
}

func buildRunRequest(params wfairuns.AgentRunWorkflowParams) runner.RunRequest {
	history := make([]runner.ConversationMessage, 0, len(params.ConversationHistory))
	for _, msg := range params.ConversationHistory {
		history = append(history, runner.ConversationMessage{
			Sender: msg.Sender,
			Text:   msg.Text,
		})
	}
	req := runner.RunRequest{
		AgentID:             params.AgentID,
		InboundMessage:      params.InboundMessage,
		ConversationHistory: history,
		Context:             params.Context,
		ConversationID:      params.ConversationID,
		InboxID:             params.InboxID,
		VerificationLevel:   params.VerificationLevel,
		MaxIterations:       params.MaxIterations,
		TotalTokenBudget:    params.TotalTokenBudget,
		TraceID:             params.TraceID,
		InvokerUserID:       params.InvokerUserID,
		IsComposer:          params.IsComposer,
	}
	if params.DeadlineMs > 0 {
		req.Deadline = time.Duration(params.DeadlineMs) * time.Millisecond
	}
	return req
}

func (a *Activities) heartbeatLoop(ctx context.Context) {
	defer func() {
		if rec := recover(); rec != nil {
			a.Lo.Error("ai-run activity heartbeat panic recovered", "panic", fmt.Sprintf("%v", rec))
		}
	}()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			activity.RecordHeartbeat(ctx)
		}
	}
}

func (a *Activities) emitTerminalFailure(
	ctx context.Context,
	params wfairuns.AgentRunWorkflowParams,
	resp runner.RunResponse,
	executionID int64,
	runErr error,
) {
	pub := a.publisherForSource(params.Source)
	if pub == nil {
		return
	}

	var execID *int64
	if executionID > 0 {
		execID = &executionID
	}
	payload := runner.RunFailedPayload{
		ExecutionID: execID,
		Source:      params.Source,
		Status:      runner.StatusFailed,
		Error:       sanitizeRunFailure(runErr, resp),
	}
	if err := pub.Publish(ctx, params.TraceID, runner.RunEvent{
		Type:      runner.EventTypeRunFailed,
		EmittedAt: time.Now().UTC(),
		Payload:   mustMarshalPayload(payload),
	}); err != nil {
		a.Lo.Warn("ai-run activity: failed to publish run.failed",
			"trace_id", params.TraceID,
			"error", err,
		)
	}
}

// escalateAfterTerminalError converts a terminally-failed composer run into
// a committed escalation so the ticket reaches a human. The internal reason
// records the sanitized failure (the same operator-safe string a run.failed
// event would carry — no raw provider internals leak to reviewers).
func escalateAfterTerminalError(resp runner.RunResponse, runErr error) runner.RunResponse {
	return runner.EscalateOnTerminalError(resp, "AI run failed terminally: "+sanitizeRunFailure(runErr, resp))
}

func sanitizeRunFailure(err error, resp runner.RunResponse) string {
	switch {
	case errors.Is(err, llm.ErrCircuitOpen), errors.Is(err, llm.ErrRateLimited):
		return "AI service temporarily unavailable"
	case errors.Is(err, llm.ErrInvalidRequest):
		return "Request validation failed"
	case errors.Is(err, llm.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return "Run timed out"
	}

	for i := len(resp.ToolCalls) - 1; i >= 0; i-- {
		call := resp.ToolCalls[i]
		if call.Status == "success" {
			continue
		}
		if strings.TrimSpace(call.ToolID) == "" {
			break
		}
		reason := strings.TrimSpace(call.Error)
		if reason == "" {
			reason = call.Status
		}
		return fmt.Sprintf("Tool %s failed: %s", call.ToolID, reason)
	}
	return "AI run failed"
}

func mustMarshalPayload(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
