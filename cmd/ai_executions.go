package main

import (
	"errors"
	"html"
	"strconv"
	"strings"

	aiexecutions "github.com/aziz-tb/merio/internal/ai/executions"
	executionsmodels "github.com/aziz-tb/merio/internal/ai/executions/models"
	amodels "github.com/aziz-tb/merio/internal/auth/models"
	cmodels "github.com/aziz-tb/merio/internal/conversation/models"
	"github.com/aziz-tb/merio/internal/envelope"
	mmodels "github.com/aziz-tb/merio/internal/media/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// sendExecutionResp is the body returned by handleSendExecution. Each
// step (send/close/record) can succeed or fail independently.
type sendExecutionResp struct {
	Success         bool                                    `json:"success"`
	MessageSent     bool                                    `json:"message_sent"`
	TicketClosed    bool                                    `json:"ticket_closed"`
	OutcomeRecorded bool                                    `json:"outcome_recorded"`
	Outcome         *executionsmodels.AgentExecutionOutcome `json:"outcome,omitempty"`
	Error           string                                  `json:"error,omitempty"`
}

func handleGetAgentExecution(r *fastglue.Request) error {
	app := r.Context.(*App)
	auser := r.RequestCtx.UserValue("user").(amodels.User)
	idStr, _ := r.RequestCtx.UserValue("id").(string)
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest,
			app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}

	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	exec, err := app.aiExecutions.GetByID(id)
	if err != nil {
		if errors.Is(err, aiexecutions.ErrNotFound) {
			return r.SendErrorEnvelope(fasthttp.StatusNotFound, "agent execution not found", nil, envelope.InputError)
		}
		return sendErrorEnvelope(r, err)
	}

	// Keyed by execution id, not conversation uuid; ai_agents:run is not
	// conversation-scoped, so resolve the conversation and enforce
	// row-level access on it.
	conv, err := app.conversation.GetConversation(int(exec.ConversationID), "", "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if allowed, err := app.authz.EnforceConversationAccess(user, conv); err != nil || !allowed {
		return sendErrorEnvelope(r, envelope.NewError(envelope.PermissionError, app.i18n.T("status.deniedPermission"), nil))
	}

	return r.SendEnvelope(exec)
}

// handleListLatestAgentExecutions returns the latest composer-source run
// per enabled agent for the conversation, with its outcome (if any) and
// an is_stale flag (agent's current updated_at vs. the run-time
// snapshot). Discarded outcomes are filtered out in SQL. Gated on
// ai_agents:run.
func handleListLatestAgentExecutions(r *fastglue.Request) error {
	app := r.Context.(*App)
	auser := r.RequestCtx.UserValue("user").(amodels.User)
	uuid, _ := r.RequestCtx.UserValue("uuid").(string)
	if uuid == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	// Enforce row-level access — the ai_agents:run perm alone is not
	// conversation-scoped.
	conv, err := enforceConversationAccess(app, uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	rows, err := app.aiExecutions.LatestForConversation(int64(conv.ID))
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(map[string]any{
		"executions": rows,
	})
}

// handleListAgentRuns returns paginated past runs for one agent. Filters:
// status enum, free-text ILIKE on draft. Cursor pagination via
// (before_run_at, before_id) taken from the last row of the prior page.
// Gated on ai_agents:manage. Excludes discarded outcomes.
func handleListAgentRuns(r *fastglue.Request) error {
	app := r.Context.(*App)

	idStr, _ := r.RequestCtx.UserValue("id").(string)
	id, err := strconv.Atoi(idStr)
	if err != nil || id == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest,
			app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}

	q := func(k string) string {
		return strings.TrimSpace(string(r.RequestCtx.QueryArgs().Peek(k)))
	}

	params := aiexecutions.ListByAgentParams{
		AgentID: id,
		Status:  q("status"),
		Search:  q("q"),
	}
	// Cursor: both values must be present, else treated as first page.
	beforeRunAtRaw := q("before_run_at")
	beforeIDRaw := q("before_id")
	if beforeRunAtRaw != "" && beforeIDRaw != "" {
		if bid, err := strconv.ParseInt(beforeIDRaw, 10, 64); err == nil {
			ts := beforeRunAtRaw
			params.BeforeRunAt = &ts
			params.BeforeID = &bid
		}
	}
	if v := q("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			params.Limit = n
		}
	}

	rows, err := app.aiExecutions.ListByAgent(params)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(map[string]any{
		"executions": rows,
	})
}

// handleSendAgentExecution sends the agent's draft as a reply, closes the
// ticket, and records the outcome. Only a failed reply-queue returns 5xx
// (nothing reached the customer, so the row must not be committed); every
// other path returns 200 with a partial-failure body. Preconditions
// reject mismatched conversation (400), stale agent or already-committed
// run (409). If the close fails after the reply landed, the outcome is
// still recorded and success=false so the operator closes manually.
func handleSendAgentExecution(r *fastglue.Request) error {
	app := r.Context.(*App)
	uuid, _ := r.RequestCtx.UserValue("uuid").(string)
	execIDStr, _ := r.RequestCtx.UserValue("execution_id").(string)
	if uuid == "" || execIDStr == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	execID, err := strconv.ParseInt(execIDStr, 10, 64)
	if err != nil || execID <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}

	auser := r.RequestCtx.UserValue("user").(amodels.User)
	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// 1. Load the conversation by UUID and enforce row-level access
	//    (the ai_agents:run perm alone is not conversation-scoped).
	conv, err := enforceConversationAccess(app, uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// 2. Load the execution and verify it belongs to this conversation.
	exec, err := app.aiExecutions.GetByID(execID)
	if err != nil {
		if errors.Is(err, aiexecutions.ErrNotFound) {
			return r.SendErrorEnvelope(fasthttp.StatusNotFound, "agent execution not found", nil, envelope.InputError)
		}
		return sendErrorEnvelope(r, err)
	}
	if exec.ConversationID != int64(conv.ID) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest,
			"agent execution does not belong to this conversation", nil, envelope.InputError)
	}

	// 3. Stale check. Re-load the agent, compare updated_at.
	agent, err := app.aiAgents.Get(exec.AgentID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if agent.UpdatedAt.After(exec.AgentUpdatedAt) {
		return r.SendErrorEnvelope(fasthttp.StatusConflict,
			"agent has been modified since this run; regenerate first", nil, envelope.InputError)
	}

	// 4. Idempotency: existing outcome → 409 carrying that row. Envelope
	//    is hand-rolled since fastglue has no SendEnvelopeWithStatus.
	if existing, getErr := app.aiExecutions.GetOutcomeByExecutionID(execID); getErr == nil {
		return r.SendJSON(fasthttp.StatusConflict, map[string]any{
			"status":     "error",
			"error_type": envelope.InputError,
			"message":    "this run has already been committed",
			"data": sendExecutionResp{
				Success:         false,
				MessageSent:     existing.Outcome == executionsmodels.OutcomeSent,
				TicketClosed:    existing.Outcome == executionsmodels.OutcomeSent,
				OutcomeRecorded: true,
				Outcome:         &existing,
				Error:           "this run has already been committed",
			},
		})
	} else if !errors.Is(getErr, aiexecutions.ErrNotFound) {
		return sendErrorEnvelope(r, getErr)
	}

	if !exec.Draft.Valid || strings.TrimSpace(exec.Draft.String) == "" {
		return r.SendErrorEnvelope(fasthttp.StatusConflict,
			"agent execution has no committed draft", nil, envelope.InputError)
	}

	// 5. Convert the agent's draft (plain text) to HTML.
	body := plainTextToHTML(exec.Draft.String)

	// 6. Queue the reply. Email channels need a non-empty `to`; default
	//    to the contact's email.
	toList := []string{}
	if conv.Contact.Email.Valid && strings.TrimSpace(conv.Contact.Email.String) != "" {
		toList = []string{conv.Contact.Email.String}
	}
	message, err := app.conversation.QueueReply(
		[]mmodels.Media{}, conv.InboxID, user.ID, conv.ContactID,
		uuid, body, toList, nil, nil, map[string]any{},
		nil,
	)
	if err != nil {
		app.lo.Error("send: QueueReply failed", "execution_id", execID, "error", err)
		return sendErrorEnvelope(r, err)
	}

	// 7. Close the ticket. statusID=0 + name lets the manager resolve the
	//    status row by name, matching the PUT .../status handler.
	closeErr := app.conversation.UpdateConversationStatus(uuid, 0, cmodels.StatusClosed, "", user)

	// 8. Record the outcome regardless of close failure — the
	//    customer-visible action (the reply) already happened.
	committedBy := int64(user.ID)
	msgID := int64(message.ID)
	reason := "operator clicked send reply"
	outcome, outcomeErr := app.aiExecutions.InsertOutcome(aiexecutions.InsertOutcomeParams{
		AgentExecutionID:  execID,
		Outcome:           executionsmodels.OutcomeSent,
		OutcomeReason:     &reason,
		MessageID:         &msgID,
		CommittedByUserID: &committedBy,
	})
	if outcomeErr != nil && !errors.Is(outcomeErr, aiexecutions.ErrOutcomeExists) {
		// Outcome record failed but reply landed: log loudly (recoverable
		// via backfill) and still return success.
		app.lo.Error("send: failed to record outcome",
			"execution_id", execID, "message_id", message.ID, "close_err", closeErr, "error", outcomeErr)
	}

	resp := sendExecutionResp{
		Success:         closeErr == nil,
		MessageSent:     true,
		TicketClosed:    closeErr == nil,
		OutcomeRecorded: outcomeErr == nil,
	}
	if outcomeErr == nil {
		resp.Outcome = &outcome
	}
	if closeErr != nil {
		app.lo.Error("send: close failed but reply landed",
			"execution_id", execID, "error", closeErr)
		resp.Error = "Reply sent; ticket close failed — please close the ticket manually."
	}
	return r.SendEnvelope(resp)
}

// plainTextToHTML wraps a plain-text draft into composer HTML: blank
// lines split <p> paragraphs, single newlines become <br>, and each
// segment is escaped so <, >, & render literally. Swap for a Markdown
// renderer if agents start emitting Markdown.
func plainTextToHTML(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Normalize line endings.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	paragraphs := strings.Split(s, "\n\n")
	var b strings.Builder
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Within a paragraph, preserve single line breaks.
		escaped := html.EscapeString(p)
		withBreaks := strings.ReplaceAll(escaped, "\n", "<br>")
		b.WriteString("<p>")
		b.WriteString(withBreaks)
		b.WriteString("</p>")
	}
	return b.String()
}
