package main

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	enumspb "go.temporal.io/api/enums/v1"

	executionmodels "github.com/aziz-tb/merio/internal/ai/executions/models"
	approvalspkg "github.com/aziz-tb/merio/internal/approvals"
	approvalsmodels "github.com/aziz-tb/merio/internal/approvals/models"
	amodels "github.com/aziz-tb/merio/internal/auth/models"
	"github.com/aziz-tb/merio/internal/envelope"
	itemporal "github.com/aziz-tb/merio/internal/temporal"
	wfairuns "github.com/aziz-tb/merio/internal/workflows/airuns"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

type resolveApprovalReq struct {
	Decision string         `json:"decision"` // approve | edit | reject
	Inputs   map[string]any `json:"inputs"`   // edit only — the operator-edited tool inputs
	Reason   string         `json:"reason"`   // reject only — fed back to the agent
}

// handleResolveWriteApproval resolves a pending write approval and starts the
// resuming run. Order is load-bearing: the atomic status flip happens FIRST
// (the manager's WHERE status='pending' update; 0 rows → 409), and only on
// success is the resume workflow started — so a double-click can't start two
// resumes and the write can't double-execute.
func handleResolveWriteApproval(r *fastglue.Request) error {
	app := r.Context.(*App)
	if app.temporal == nil {
		return r.SendErrorEnvelope(fasthttp.StatusServiceUnavailable,
			"AI run workflow service is not configured", nil, envelope.GeneralError)
	}
	convUUID, _ := r.RequestCtx.UserValue("uuid").(string)
	idStr, _ := r.RequestCtx.UserValue("id").(string)
	id, err := strconv.ParseInt(idStr, 10, 64)
	if convUUID == "" || err != nil || id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}

	var body resolveApprovalReq
	if err := r.Decode(&body, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}
	user, _ := r.RequestCtx.UserValue("user").(amodels.User)

	conv, err := app.conversation.GetConversation(0, convUUID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	ap, err := app.approvals.GetByID(id)
	if err != nil {
		if errors.Is(err, approvalspkg.ErrNotFound) {
			return r.SendErrorEnvelope(fasthttp.StatusNotFound, "approval not found", nil, envelope.InputError)
		}
		return sendErrorEnvelope(r, err)
	}
	if ap.ConversationID != int64(conv.ID) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "approval does not belong to this conversation", nil, envelope.InputError)
	}

	rp := approvalspkg.ResolveParams{ID: id}
	uid := int64(user.ID)
	rp.ReviewedByUserID = &uid
	switch body.Decision {
	case "approve":
		rp.Status = approvalsmodels.StatusApproved
		rp.ApprovedInputs = ap.ProposedInputs // execute as proposed
	case "edit":
		rp.Status = approvalsmodels.StatusEdited
		inputs, _ := json.Marshal(body.Inputs)
		rp.ApprovedInputs = inputs // execute with the operator's edits
	case "reject":
		rp.Status = approvalsmodels.StatusRejected
		reason := body.Reason
		rp.RejectionReason = &reason
	default:
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "decision must be approve, edit, or reject", nil, envelope.InputError)
	}

	// Atomic flip FIRST — the idempotency lock.
	resolved, err := app.approvals.Resolve(rp)
	if err != nil {
		switch {
		case errors.Is(err, approvalspkg.ErrNotPending):
			return r.SendErrorEnvelope(fasthttp.StatusConflict, "this approval has already been resolved", nil, envelope.InputError)
		case errors.Is(err, approvalspkg.ErrNotFound):
			return r.SendErrorEnvelope(fasthttp.StatusNotFound, "approval not found", nil, envelope.InputError)
		default:
			return sendErrorEnvelope(r, err)
		}
	}

	// Only now (the flip succeeded) start the resuming run.
	traceID := uuid.NewString()
	params := wfairuns.AgentRunWorkflowParams{
		TraceID:          traceID,
		Source:           executionmodels.SourceComposer,
		ConversationID:   conv.ID,
		InboxID:          conv.InboxID,
		InvokerUserID:    user.ID,
		ResumeApprovalID: resolved.ID,
		EnqueuedAt:       time.Now().UTC(),
	}
	startCtx, cancel := context.WithTimeout(r.RequestCtx, 10*time.Second)
	defer cancel()
	_, err = app.temporal.StartWorkflow(startCtx, itemporal.StartOptions{
		ID:                       agentRunWorkflowID(traceID),
		TaskQueue:                itemporal.QueueAIRuns,
		ReusePolicy:              enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		WorkflowExecutionTimeout: 15 * time.Minute,
		SearchAttributes: map[string]any{
			itemporal.SATraceID:      traceID,
			itemporal.SAWorkflowKind: wfairuns.WorkflowKind,
			itemporal.SAEntityType:   "conversation",
			itemporal.SAEntityID:     strconv.Itoa(conv.ID),
			itemporal.SAInboxID:      strconv.Itoa(conv.InboxID),
		},
	}, wfairuns.AgentRunWorkflow, params)
	if err != nil {
		app.lo.Error("failed to start resume run workflow", "trace_id", traceID, "approval_id", id, "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}

	r.RequestCtx.SetStatusCode(fasthttp.StatusAccepted)
	return r.SendEnvelope(map[string]any{
		"trace_id": traceID,
		"status":   "accepted",
		"approval": resolved,
	})
}
