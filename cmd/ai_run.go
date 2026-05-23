package main

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/google/uuid"
	enumspb "go.temporal.io/api/enums/v1"

	executionmodels "github.com/abhinavxd/libredesk/internal/ai/executions/models"
	"github.com/abhinavxd/libredesk/internal/ai/runner"
	amodels "github.com/abhinavxd/libredesk/internal/auth/models"
	"github.com/abhinavxd/libredesk/internal/envelope"
	itemporal "github.com/abhinavxd/libredesk/internal/temporal"
	wfairuns "github.com/abhinavxd/libredesk/internal/workflows/airuns"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

type runAIAgentReq struct {
	InboundMessage      string                       `json:"inbound_message"`
	Context             map[string]any               `json:"context"`
	ConversationID      int                          `json:"conversation_id"`
	ConversationHistory []runner.ConversationMessage `json:"conversation_history"`
	LegitimationLevel   int                          `json:"legitimation_level"`
	MaxIterations       int                          `json:"max_iterations"`
	TotalTokenBudget    int                          `json:"total_token_budget"`
	DeadlineMs          int                          `json:"deadline_ms"`
	TraceID             string                       `json:"trace_id"`
}

type runAIAgentResp struct {
	TraceID string `json:"trace_id"`
	Status  string `json:"status"`
}

func handleRunAIAgent(r *fastglue.Request) error {
	app := r.Context.(*App)
	if app.temporal == nil {
		return r.SendErrorEnvelope(fasthttp.StatusServiceUnavailable,
			"AI run workflow service is not configured",
			nil,
			envelope.GeneralError,
		)
	}

	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if err != nil || id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest,
			app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}

	var body runAIAgentReq
	if err := r.Decode(&body, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest,
			app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}
	if body.InboundMessage == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest,
			"inbound_message is required", nil, envelope.InputError)
	}

	user, _ := r.RequestCtx.UserValue("user").(amodels.User)
	inboxID := app.systemInboxID
	if body.ConversationID > 0 {
		conv, convErr := app.conversation.GetConversation(body.ConversationID, "", "")
		if convErr == nil && conv.InboxID > 0 {
			inboxID = conv.InboxID
		}
	}

	if body.ConversationID > 0 {
		existing, err := findRunningAgentRun(r.RequestCtx, app.temporal, body.ConversationID, &id)
		if err != nil {
			app.lo.Error("failed to check in-flight agent runs",
				"agent_id", id,
				"conversation_id", body.ConversationID,
				"error", err)
			return r.SendErrorEnvelope(fasthttp.StatusInternalServerError,
				app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
		}
		if existing != nil {
			return r.SendErrorEnvelope(fasthttp.StatusConflict,
				"A run is already in progress for this conversation.", nil, envelope.InputError)
		}
	}

	traceID := uuid.NewString()
	workflowID := agentRunWorkflowID(traceID)
	enqueuedAt := time.Now().UTC()
	params := wfairuns.AgentRunWorkflowParams{
		TraceID:           traceID,
		Source:            executionmodels.SourceComposer,
		AgentID:           id,
		ConversationID:    body.ConversationID,
		Context:           body.Context,
		InboxID:           inboxID,
		LegitimationLevel: body.LegitimationLevel,
		MaxIterations:     body.MaxIterations,
		TotalTokenBudget:  body.TotalTokenBudget,
		DeadlineMs:        body.DeadlineMs,
		InvokerUserID:     user.ID,
		InboundMessage:    body.InboundMessage,
		EnqueuedAt:        enqueuedAt,
	}
	if len(body.ConversationHistory) > 0 {
		params.ConversationHistory = make([]wfairuns.ConversationMessage, 0, len(body.ConversationHistory))
		for _, msg := range body.ConversationHistory {
			params.ConversationHistory = append(params.ConversationHistory, wfairuns.ConversationMessage{
				Sender: msg.Sender,
				Text:   msg.Text,
			})
		}
	}

	publishRunEnqueued(r.RequestCtx, app, traceID, params)

	startCtx, cancel := context.WithTimeout(r.RequestCtx, 10*time.Second)
	defer cancel()
	_, err = app.temporal.StartWorkflow(startCtx, itemporal.StartOptions{
		ID:                       workflowID,
		TaskQueue:                itemporal.QueueAIRuns,
		ReusePolicy:              enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		WorkflowExecutionTimeout: 15 * time.Minute,
		SearchAttributes: map[string]any{
			itemporal.SATraceID:      traceID,
			itemporal.SAWorkflowKind: wfairuns.WorkflowKind,
			itemporal.SAEntityType:   "conversation",
			itemporal.SAEntityID:     strconv.Itoa(body.ConversationID),
			itemporal.SAAgentID:      strconv.Itoa(id),
			itemporal.SAInboxID:      strconv.Itoa(inboxID),
		},
	}, wfairuns.AgentRunWorkflow, params)
	if err != nil {
		app.lo.Error("failed to start agent run workflow",
			"trace_id", traceID,
			"workflow_id", workflowID,
			"error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError,
			app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}

	r.RequestCtx.SetStatusCode(fasthttp.StatusAccepted)
	return r.SendEnvelope(runAIAgentResp{
		TraceID: traceID,
		Status:  "accepted",
	})
}

func publishRunEnqueued(ctx context.Context, app *App, traceID string, params wfairuns.AgentRunWorkflowParams) {
	if app == nil || app.aiRunEvents == nil {
		return
	}
	payload := runner.RunEnqueuedPayload{
		Source:         params.Source,
		AgentID:        params.AgentID,
		ConversationID: params.ConversationID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		app.lo.Warn("failed to marshal run.enqueued payload",
			"trace_id", traceID,
			"error", err)
		return
	}
	if err := app.aiRunEvents.Publish(ctx, traceID, runner.RunEvent{
		Type:      runner.EventTypeRunEnqueued,
		EmittedAt: time.Now().UTC(),
		Payload:   body,
	}); err != nil {
		app.lo.Warn("failed to publish run.enqueued",
			"trace_id", traceID,
			"error", err)
	}
}
