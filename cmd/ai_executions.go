package main

import (
	"errors"
	"html"
	"strconv"
	"strings"

	aiexecutions "github.com/abhinavxd/libredesk/internal/ai/executions"
	executionsmodels "github.com/abhinavxd/libredesk/internal/ai/executions/models"
	amodels "github.com/abhinavxd/libredesk/internal/auth/models"
	cmodels "github.com/abhinavxd/libredesk/internal/conversation/models"
	"github.com/abhinavxd/libredesk/internal/envelope"
	mmodels "github.com/abhinavxd/libredesk/internal/media/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// sendExecutionResp is the body returned by handleSendExecution. The
// shape is partial-failure-aware: every step can succeed or fail
// independently, and the client renders accordingly.
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
	idStr, _ := r.RequestCtx.UserValue("id").(string)
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest,
			app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}

	exec, err := app.aiExecutions.GetByID(id)
	if err != nil {
		if errors.Is(err, aiexecutions.ErrNotFound) {
			return r.SendErrorEnvelope(fasthttp.StatusNotFound, "agent execution not found", nil, envelope.InputError)
		}
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(exec)
}

// handleListLatestAgentExecutions returns the latest composer-source
// run per enabled agent for the given conversation, joined with the
// outcome (if any) and stamped with an is_stale flag derived from the
// agent's current updated_at vs. the snapshot at run time. The
// Composer tab fetches this on mount; rows with an outcome ='discarded'
// are filtered out at the SQL level.
//
// Gated on ai_agents:run — same permission the run endpoint uses, per
// the locked decision.
func handleListLatestAgentExecutions(r *fastglue.Request) error {
	app := r.Context.(*App)
	uuid, _ := r.RequestCtx.UserValue("uuid").(string)
	if uuid == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	conv, err := app.conversation.GetConversation(0, uuid, "")
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

// handleListAgentRuns returns paginated past runs for one agent —
// powers the Edit Agent layout's History tab. Filters: status enum,
// free-text ILIKE on draft. Cursor pagination via (before_run_at,
// before_id): pass the values from the last row of the previous page
// to fetch the next page.
//
// Gated on ai_agents:manage (same as other agent-edit endpoints).
// Excludes runs with a 'discarded' outcome, matching the
// conversation-scoped Composer rule.
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
	// Cursor: both must be present to take effect; one without the
	// other is treated as "first page" so a malformed query falls
	// through cleanly.
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

// handleSendAgentExecution is the "send the agent's draft as a reply +
// close the ticket + record the outcome" action. Returns 200 with a
// partial-failure-aware body on every non-error path; only step 1
// (the reply queue) failing returns 5xx — the customer didn't get a
// message, so the row musn't be committed and the operator needs to
// retry.
//
// Locked behavior:
//  1. Verify the execution belongs to the conversation in the URL (else 400).
//  2. Verify the agent's current updated_at matches the snapshot taken
//     at run time (else 409 — caller regenerates first).
//  3. Verify no outcome row exists for this execution (else 409 — return
//     the existing outcome so the UI can render it).
//  4. Convert the draft (plain text, possibly multi-paragraph) to HTML
//     using a paragraph-aware split: blank lines become <p> boundaries;
//     single newlines become <br>.
//  5. Queue the reply via app.conversation.QueueReply (internal API).
//  6. Close the ticket via app.conversation.UpdateConversationStatus
//     with status="Closed".
//  7. Insert the outcome row (sent, message_id, committed_by_user_id).
//
// On step 6 failure after 5 succeeded: still record the outcome (the
// customer-visible action happened) and return success=false with a
// descriptive error string. The UI surfaces "reply sent, close failed"
// and the operator closes manually.
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

	// 1. Load the conversation by UUID and confirm scope.
	conv, err := app.conversation.GetConversation(0, uuid, "")
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

	// 4. Idempotency: existing outcome → 409 with the existing row. The
	//    fastglue Request type doesn't expose SendEnvelopeWithStatus,
	//    so we hand-roll the error envelope shape SendErrorEnvelope
	//    uses; the response body carries the existing outcome.
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

	// 5. Convert the agent's draft (plain text) to HTML. Paragraphs
	//    split on blank lines; intra-paragraph newlines become <br>.
	body := plainTextToHTML(exec.Draft.String)

	// 6. Queue the reply via internal API. For email channels QueueReply
	//    requires non-empty `to`; the conversation's contact email is
	//    the natural default.
	toList := []string{}
	if conv.Contact.Email.Valid && strings.TrimSpace(conv.Contact.Email.String) != "" {
		toList = []string{conv.Contact.Email.String}
	}
	message, err := app.conversation.QueueReply(
		[]mmodels.Media{}, conv.InboxID, user.ID, conv.ContactID,
		uuid, body, toList, nil, nil, map[string]any{},
	)
	if err != nil {
		app.lo.Error("send: QueueReply failed", "execution_id", execID, "error", err)
		return sendErrorEnvelope(r, err)
	}

	// 7. Close the ticket. statusID=0 + status="Closed" matches the
	//    existing PUT /conversations/:uuid/status handler convention
	//    (the manager resolves the status row by name).
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
		// Outcome failed to record but reply landed: log loudly, the
		// row is recoverable via backfill. Still return success because
		// the customer-visible action happened.
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

// plainTextToHTML wraps an agent's draft (plain text, possibly with
// blank-line-separated paragraphs) into HTML the rich-text composer
// renders cleanly. Strategy: split on blank lines into paragraphs;
// within a paragraph, single newlines become <br>. HTML-escape each
// segment so any `<` / `>` / `&` in the draft renders literally rather
// than as markup.
//
// This is the v1 conversion. If agents start emitting Markdown
// intentionally, swap this for a Markdown renderer.
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
