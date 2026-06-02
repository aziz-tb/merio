package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	enumspb "go.temporal.io/api/enums/v1"
	workflowservice "go.temporal.io/api/workflowservice/v1"

	amodels "github.com/abhinavxd/libredesk/internal/auth/models"
	cmodels "github.com/abhinavxd/libredesk/internal/conversation/models"
	"github.com/abhinavxd/libredesk/internal/envelope"
	itemporal "github.com/abhinavxd/libredesk/internal/temporal"
	wfcustomeridentification "github.com/abhinavxd/libredesk/internal/workflows/customeridentification"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

type identifyConversationCustomerResp struct {
	TraceID    string `json:"trace_id"`
	WorkflowID string `json:"workflow_id"`
}

type inFlightCustomerIdentification struct {
	TraceID    string
	WorkflowID string
	StartedAt  time.Time
}

func handleIdentifyConversationCustomer(r *fastglue.Request) error {
	app := r.Context.(*App)
	if app.temporal == nil {
		return r.SendErrorEnvelope(fasthttp.StatusServiceUnavailable,
			"Customer identification is not available; Temporal is not configured.",
			nil,
			envelope.GeneralError,
		)
	}

	conversationUUID := r.RequestCtx.UserValue("uuid").(string)
	if conversationUUID == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "uuid required", nil, envelope.InputError)
	}

	auser := r.RequestCtx.UserValue("user").(amodels.User)
	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	conv, err := enforceConversationAccess(app, conversationUUID, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	existing, err := findRunningCustomerIdentification(r.RequestCtx, app.temporal, conv.ID)
	if err != nil {
		app.lo.Error("failed to check in-flight customer identification",
			"conversation_id", conv.ID,
			"error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError,
			app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}
	if existing != nil {
		return r.SendErrorEnvelope(fasthttp.StatusConflict,
			"Customer identification is already in progress for this conversation.",
			map[string]any{
				"trace_id":    existing.TraceID,
				"workflow_id": existing.WorkflowID,
			},
			envelope.InputError,
		)
	}

	messageID, err := latestInboundMessageID(app, conversationUUID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	traceID := uuid.NewString()
	workflowID := customerIdentificationWorkflowID(conv.ID, traceID)
	triggeredBy := int64(user.ID)
	startCtx, cancel := context.WithTimeout(r.RequestCtx, 10*time.Second)
	defer cancel()
	_, err = app.temporal.StartWorkflow(startCtx, itemporal.StartOptions{
		ID:                       workflowID,
		TaskQueue:                itemporal.QueuePipeline,
		ReusePolicy:              enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		ErrorWhenAlreadyStarted:  true,
		WorkflowExecutionTimeout: 15 * time.Minute,
		SearchAttributes: map[string]any{
			itemporal.SATraceID:      traceID,
			itemporal.SAWorkflowKind: wfcustomeridentification.WorkflowKind,
			itemporal.SAEntityType:   "conversation",
			itemporal.SAEntityID:     strconv.Itoa(conv.ID),
			itemporal.SAInboxID:      strconv.Itoa(conv.InboxID),
		},
	}, wfcustomeridentification.CustomerIdentificationWorkflow, wfcustomeridentification.WorkflowInput{
		ConversationID:    int64(conv.ID),
		MessageID:         int64(messageID),
		Channel:           conv.InboxChannel,
		TriggeredByUserID: &triggeredBy,
		TraceID:           traceID,
	})
	if err != nil {
		if itemporal.IsAlreadyStarted(err) {
			return r.SendErrorEnvelope(fasthttp.StatusConflict,
				"Customer identification is already in progress for this conversation.",
				nil,
				envelope.InputError,
			)
		}
		app.lo.Error("failed to start customer identification workflow",
			"trace_id", traceID,
			"workflow_id", workflowID,
			"conversation_id", conv.ID,
			"error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError,
			app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}

	r.RequestCtx.SetStatusCode(fasthttp.StatusAccepted)
	return r.SendEnvelope(identifyConversationCustomerResp{
		TraceID:    traceID,
		WorkflowID: workflowID,
	})
}

func latestInboundMessageID(app *App, conversationUUID string) (int, error) {
	private := false
	messages, _, err := app.conversation.GetConversationMessages(conversationUUID, 1, 1, &private, []string{cmodels.MessageIncoming})
	if err != nil {
		return 0, err
	}
	if len(messages) == 0 {
		return 0, envelope.NewError(envelope.InputError, "no inbound message found for conversation", nil)
	}
	return messages[0].ID, nil
}

func customerIdentificationWorkflowID(conversationID int, traceID string) string {
	return fmt.Sprintf("customer-id:%d:%s", conversationID, traceID)
}

func findRunningCustomerIdentification(
	ctx context.Context,
	tc *itemporal.Client,
	conversationID int,
) (*inFlightCustomerIdentification, error) {
	if tc == nil || conversationID <= 0 {
		return nil, nil
	}

	query := fmt.Sprintf(
		`ExecutionStatus="Running" AND %s="%s" AND %s="%s" AND %s="%d"`,
		itemporal.SAWorkflowKind, wfcustomeridentification.WorkflowKind,
		itemporal.SAEntityType, "conversation",
		itemporal.SAEntityID, conversationID,
	)
	resp, err := tc.SDK().ListWorkflow(ctx, &workflowservice.ListWorkflowExecutionsRequest{
		Namespace: tc.Config().Namespace,
		PageSize:  1,
		Query:     query,
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Executions) == 0 {
		return nil, nil
	}

	info := resp.Executions[0]
	run := &inFlightCustomerIdentification{
		WorkflowID: info.Execution.GetWorkflowId(),
		StartedAt:  info.GetStartTime().AsTime(),
	}
	prefix := fmt.Sprintf("customer-id:%d:", conversationID)
	if strings.HasPrefix(run.WorkflowID, prefix) {
		run.TraceID = strings.TrimPrefix(run.WorkflowID, prefix)
	}
	if run.TraceID == "" {
		run.TraceID = searchAttributeString(info.SearchAttributes.GetIndexedFields()[itemporal.SATraceID])
	}
	return run, nil
}
