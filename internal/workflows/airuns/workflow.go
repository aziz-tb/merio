// Package airuns defines the Temporal workflow that owns one agent run.
//
// v1 uses a single long-running activity that wraps runner.Run and
// persists the final result. This gives durable workflow state and
// replayable start/reconnect semantics, but it is not true mid-run
// resume: if the worker dies mid-activity, the workflow fails and the
// caller re-triggers the run.
package airuns

import (
	"strconv"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	itemporal "github.com/abhinavxd/libredesk/internal/temporal"
)

const WorkflowKind = "ai-run"

type ConversationMessage struct {
	Sender string `json:"sender"`
	Text   string `json:"text"`
}

type AgentRunWorkflowParams struct {
	TraceID             string                `json:"trace_id"`
	Source              string                `json:"source"`
	AgentID             int                   `json:"agent_id"`
	ConversationID      int                   `json:"conversation_id"`
	ConversationHistory []ConversationMessage `json:"conversation_history,omitempty"`
	InboundMessage      string                `json:"inbound_message"`
	Context             map[string]any        `json:"context,omitempty"`
	InboxID             int                   `json:"inbox_id"`
	LegitimationLevel   int                   `json:"legitimation_level"`
	MaxIterations       int                   `json:"max_iterations"`
	TotalTokenBudget    int                   `json:"total_token_budget"`
	DeadlineMs          int                   `json:"deadline_ms"`
	InvokerUserID       int                   `json:"invoker_user_id"`
	EnqueuedAt          time.Time             `json:"enqueued_at"`
}

type AgentRunResult struct {
	ExecutionID  int64  `json:"execution_id"`
	TraceID      string `json:"trace_id"`
	Status       string `json:"status"`
	DraftPresent bool   `json:"draft_present"`
}

func AgentRunWorkflow(ctx workflow.Context, params AgentRunWorkflowParams) (*AgentRunResult, error) {
	if err := workflow.UpsertSearchAttributes(ctx, buildSearchAttributes(params)); err != nil {
		return nil, err
	}

	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout:    5 * time.Minute,
		ScheduleToCloseTimeout: 10 * time.Minute,
		HeartbeatTimeout:       30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 1,
		},
	})

	var out AgentRunResult
	if err := workflow.ExecuteActivity(ctx, "RunAgentActivity", params).Get(ctx, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func buildSearchAttributes(params AgentRunWorkflowParams) map[string]any {
	searchAttrs := map[string]any{
		itemporal.SATraceID:      params.TraceID,
		itemporal.SAWorkflowKind: WorkflowKind,
		itemporal.SAEntityType:   "conversation",
		itemporal.SAEntityID:     strconv.Itoa(params.ConversationID),
		itemporal.SAAgentID:      strconv.Itoa(params.AgentID),
	}
	if params.InboxID > 0 {
		searchAttrs[itemporal.SAInboxID] = strconv.Itoa(params.InboxID)
	}
	return searchAttrs
}
