package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"

	amodels "github.com/abhinavxd/libredesk/internal/auth/models"
	authzModels "github.com/abhinavxd/libredesk/internal/authz/models"
	cmodels "github.com/abhinavxd/libredesk/internal/conversation/models"
	"github.com/abhinavxd/libredesk/internal/envelope"
	"github.com/abhinavxd/libredesk/internal/inbox"
	mmodels "github.com/abhinavxd/libredesk/internal/media/models"
	umodels "github.com/abhinavxd/libredesk/internal/user/models"
	wlmodels "github.com/abhinavxd/libredesk/internal/worklog/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// fwdSubjectPrefix matches a leading Fwd:/FW:/FWD: (case-insensitive, with optional trailing whitespace).
// Used to strip an existing forward prefix before prepending a fresh "Fwd: " so the chain doesn't stack.
var fwdSubjectPrefix = regexp.MustCompile(`(?i)^\s*(fwd?|fw):\s*`)

type forwardReq struct {
	To           []string `json:"to"`
	CC           []string `json:"cc"`
	BCC          []string `json:"bcc"`
	MessageUUIDs []string `json:"message_uuids"`
	Body         string   `json:"body"`
	SkipQuote    bool     `json:"skip_quote"`
	Comment      string   `json:"comment"`
}

type messageReq struct {
	Attachments []int                  `json:"attachments"`
	Message     string                 `json:"message"`
	Private     bool                   `json:"private"`
	To          []string               `json:"to"`
	CC          []string               `json:"cc"`
	BCC         []string               `json:"bcc"`
	SenderType  string                 `json:"sender_type"`
	Mentions    []cmodels.MentionInput `json:"mentions"`
	EchoID      string                 `json:"echo_id"`
}

// handleGetMessages returns messages for a conversation.
func handleGetMessages(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		uuid    = r.RequestCtx.UserValue("uuid").(string)
		auser   = r.RequestCtx.UserValue("user").(amodels.User)
		total   = 0
		private *bool
	)
	page, pageSize := getPagination(r)

	// Parse optional private filter (null = no filter)
	if r.RequestCtx.QueryArgs().Has("private") {
		p := r.RequestCtx.QueryArgs().GetBool("private")
		private = &p
	}

	// Parse repeated type params: ?type=incoming&type=outgoing
	var msgTypes []string
	for _, v := range r.RequestCtx.QueryArgs().PeekMulti("type") {
		msgTypes = append(msgTypes, string(v))
	}

	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Check permission
	_, err = enforceConversationAccess(app, uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	messages, pageSize, err := app.conversation.GetConversationMessages(uuid, page, pageSize, private, msgTypes)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	rootURL, _ := app.setting.GetAppRootURL()
	for i := range messages {
		total = messages[i].Total
		// Populate attachment URLs
		for j := range messages[i].Attachments {
			att := messages[i].Attachments[j]
			messages[i].Attachments[j].URL = app.media.GetURL(att.UUID, att.ContentType, att.Name)
		}
		resolveContentCIDs(&messages[i], rootURL)
	}

	// Process CSAT status for all messages (will only affect CSAT messages)
	app.conversation.ProcessCSATStatus(messages)

	// Strip CSAT UUID from agent sessions to prevent self-rating.
	if r.RequestCtx.UserValue("auth_method") != "api_key" {
		for i := range messages {
			if messages[i].HasCSAT() {
				messages[i].StripCSATUUID()
			}
		}
	}

	return r.SendEnvelope(envelope.PageResults{
		Total:      total,
		Results:    messages,
		Page:       page,
		PerPage:    pageSize,
		TotalPages: (total + pageSize - 1) / pageSize,
	})
}

// handleGetMessage fetches a single from DB using the uuid.
func handleGetMessage(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		cuuid = r.RequestCtx.UserValue("cuuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)
	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Check permission
	_, err = enforceConversationAccess(app, cuuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	message, err := app.conversation.GetMessage(uuid)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Ensure the message belongs to the conversation in the route. Return 404
	// (not 403) so we don't leak the existence of messages in other conversations.
	if message.ConversationUUID != cuuid {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.T("globals.messages.notFound"), nil, envelope.NotFoundError)
	}

	// Process CSAT status for the message (will only affect CSAT messages)
	messages := []cmodels.Message{message}
	app.conversation.ProcessCSATStatus(messages)
	message = messages[0]

	// Strip CSAT UUID from agent sessions to prevent self-rating.
	if r.RequestCtx.UserValue("auth_method") != "api_key" && message.HasCSAT() {
		message.StripCSATUUID()
	}

	rootURL, _ := app.setting.GetAppRootURL()
	for j := range message.Attachments {
		att := message.Attachments[j]
		message.Attachments[j].URL = app.media.GetURL(att.UUID, att.ContentType, att.Name)
	}
	resolveContentCIDs(&message, rootURL)

	return r.SendEnvelope(message)
}

// handleRetryMessage changes message status to `pending`, so it's enqueued for sending.
func handleRetryMessage(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		cuuid = r.RequestCtx.UserValue("cuuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)

	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Check permission
	_, err = enforceConversationAccess(app, cuuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Only outgoing agent messages that have failed can be retried.
	msg, err := app.conversation.GetMessage(uuid)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if msg.SenderType != cmodels.SenderTypeAgent || msg.Status != cmodels.MessageStatusFailed || msg.SenderID != user.ID || msg.ConversationUUID != cuuid {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}

	if err = app.conversation.MarkMessageAsPending(uuid); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}

// handleSendMessage sends a message in a conversation.
func handleSendMessage(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		cuuid = r.RequestCtx.UserValue("cuuid").(string)
		req   = messageReq{}
	)

	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Check access to conversation.
	conv, err := enforceConversationAccess(app, cuuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	if err := r.Decode(&req, "json"); err != nil {
		app.lo.Error("error unmarshalling message request", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}

	// Make sure the inbox is enabled.
	inbox, err := app.inbox.GetDBRecord(conv.InboxID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if !inbox.Enabled {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("status.disabledInbox"), nil, envelope.InputError)
	}

	if req.SenderType != umodels.UserTypeAgent && req.SenderType != umodels.UserTypeContact {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	// Contacts cannot send private messages
	if req.SenderType == umodels.UserTypeContact && req.Private {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}

	// Check if user has permission to send messages as contact
	if req.SenderType == umodels.UserTypeContact {
		parts := strings.Split(authzModels.PermMessagesWriteAsContact, ":")
		if len(parts) != 2 {
			app.lo.Error("error parsing permission string", "permission", authzModels.PermMessagesWriteAsContact)
			return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.T("globals.messages.somethingWentWrong"), nil))
		}
		ok, err := app.authz.Enforce(user, parts[0], parts[1])
		if err != nil {
			app.lo.Error("error checking permission", "error", err)
			return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.T("globals.messages.somethingWentWrong"), nil))
		}
		if !ok {
			return r.SendErrorEnvelope(fasthttp.StatusForbidden, app.i18n.T("status.deniedPermission"), nil, envelope.PermissionError)
		}
	}

	// Get media for all attachments, skip any already associated with a model.
	media, err := getUnassociatedMedia(app, req.Attachments)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}

	// Create contact message.
	if req.SenderType == umodels.UserTypeContact {
		message, err := app.conversation.CreateContactMessage(media, int(conv.ContactID), cuuid, req.Message, cmodels.ContentTypeHTML, false)
		if err != nil {
			return sendErrorEnvelope(r, err)
		}
		return r.SendEnvelope(message)
	}

	// Send private note.
	if req.Private {
		message, err := app.conversation.SendPrivateNote(media, user.ID, cuuid, req.Message, req.Mentions)
		if err != nil {
			return sendErrorEnvelope(r, err)
		}
		return r.SendEnvelope(message)
	}

	meta := map[string]any{}
	if req.EchoID != "" {
		meta["echo_id"] = req.EchoID
	}
	message, err := app.conversation.QueueReply(media, conv.InboxID, user.ID, conv.ContactID, cuuid, req.Message, req.To, req.CC, req.BCC, meta)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(message)
}

// handleForwardMessage forwards one or more existing messages to new recipients within the same
// conversation. v1 is email-only (the inbox channel must be ChannelEmail). Each source message's
// attachments are CLONED (new media rows + duplicated blob), not reattached, so the originals keep
// their attachments and each cloned media's ContentID is preserved verbatim — the CID rewrite at
// send time resolves cid:... refs in the body to the cloned media's URLs.
//
// Two modes:
//   - New (preferred): caller sends `message_uuids[]` + `body` (full HTML built client-side) +
//     `skip_quote: true`. Server clones attachments from each source, submits Body verbatim.
//   - Legacy: caller sends `comment` only with the URL-path source UUID. Server builds the quoted
//     body via buildForwardedBody and appends the original content. Kept for back-compat; remove
//     once all callers migrate.
func handleForwardMessage(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		cuuid = r.RequestCtx.UserValue("cuuid").(string)
		muuid = r.RequestCtx.UserValue("uuid").(string)
		req   = forwardReq{}
	)

	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	conv, err := enforceConversationAccess(app, cuuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	if err := r.Decode(&req, "json"); err != nil {
		app.lo.Error("error unmarshalling forward request", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}

	if len(req.To) == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.Ts("globals.messages.empty", "name", "`to`"), nil, envelope.InputError)
	}

	inboxRecord, err := app.inbox.GetDBRecord(conv.InboxID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if !inboxRecord.Enabled {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("status.disabledInbox"), nil, envelope.InputError)
	}
	// v1 constraint: email-only.
	if inboxRecord.Channel != inbox.ChannelEmail {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Forwarding is only supported on email conversations.", nil, envelope.InputError)
	}

	// Resolve source UUIDs: prefer body field, fall back to URL path for legacy single-source callers.
	sourceUUIDs := req.MessageUUIDs
	if len(sourceUUIDs) == 0 && muuid != "" {
		sourceUUIDs = []string{muuid}
	}
	if len(sourceUUIDs) == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.Ts("globals.messages.empty", "name", "`message_uuids`"), nil, envelope.InputError)
	}

	// Load all source messages up-front so we can fail early on missing/inaccessible UUIDs and
	// gather the primary (oldest) for subject derivation.
	sources := make([]cmodels.Message, 0, len(sourceUUIDs))
	for _, uuid := range sourceUUIDs {
		m, err := app.conversation.GetMessage(uuid)
		if err != nil {
			return sendErrorEnvelope(r, err)
		}
		sources = append(sources, m)
	}
	primary := sources[0]

	// B1 worklog: forward action. Started after access checks succeed and sources are loaded —
	// the soonest the action is committed-to-attempt.
	var fwdUserID *int64
	if user.ID > 0 {
		v := int64(user.ID)
		fwdUserID = &v
	}
	fwdCtxJSON, _ := json.Marshal(map[string]any{
		"primary_message_uuid": primary.UUID,
		"source_count":         len(sources),
		"to_count":             len(req.To),
		"cc_count":             len(req.CC),
		"bcc_count":            len(req.BCC),
		"skip_quote":           req.SkipQuote,
	})
	wlID, _ := app.worklog.StartWorklog(context.Background(), int64(conv.ID), fwdUserID, wlmodels.ActionTypeForward, fwdCtxJSON)
	defer app.worklog.EndWorklog(context.Background(), wlID, nil)

	// Subject: derive from the primary source, stripping any existing Fwd: prefix so the chain doesn't stack.
	var primaryMeta map[string]any
	if len(primary.Meta) > 0 {
		_ = json.Unmarshal(primary.Meta, &primaryMeta)
	}
	primarySubject, _ := primaryMeta["subject"].(string)
	cleanedSubject := "Fwd: " + fwdSubjectPrefix.ReplaceAllString(primarySubject, "")

	// Build body. New path: client supplied full HTML in Body; submit verbatim.
	// Legacy path: server-build a single quoted block from the primary source's content.
	var body string
	if req.SkipQuote && strings.TrimSpace(req.Body) != "" {
		body = req.Body
	} else {
		primaryFrom, _ := primaryMeta["from"].([]any)
		primaryTo, _ := primaryMeta["to"].([]any)
		body = buildForwardedBody(req.Comment, primaryFrom, primaryTo, primarySubject, primary.CreatedAt.Format("Mon, 2 Jan 2006 15:04:05 -0700"), primary.Content)
	}

	// Clone every source's attachments into a single attachment list for the new forward message.
	// Attachments expose UUID (not the numeric media.id), so look up each via Get(0, uuid) first.
	// Pass modelID=0 — InsertMessage downstream calls mediaStore.Attach for each media in the
	// message, pointing the cloned rows at the new message's ID.
	clonedMedia := make([]mmodels.Media, 0)
	for _, src := range sources {
		for _, att := range src.Attachments {
			srcMedia, err := app.media.Get(0, att.UUID)
			if err != nil {
				app.lo.Error("error fetching source media for forward clone", "src_uuid", att.UUID, "error", err)
				return sendErrorEnvelope(r, err)
			}
			cm, err := app.media.Clone(srcMedia.ID, mmodels.ModelMessages, 0)
			if err != nil {
				app.lo.Error("error cloning media for forward", "src_id", srcMedia.ID, "error", err)
				return sendErrorEnvelope(r, err)
			}
			clonedMedia = append(clonedMedia, cm)
		}
	}

	// Collect source UUIDs for meta. Multi-source: include both primary (legacy readers) and full list.
	forwardedUUIDs := make([]string, 0, len(sources))
	for _, s := range sources {
		forwardedUUIDs = append(forwardedUUIDs, s.UUID)
	}
	meta := map[string]any{
		"subject":             cleanedSubject,
		"forwarded_from":      primary.UUID,
		"forwarded_from_list": forwardedUUIDs,
	}

	newMsg, err := app.conversation.QueueReply(clonedMedia, conv.InboxID, user.ID, conv.ContactID, cuuid, body, req.To, req.CC, req.BCC, meta)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Audit-log the forward. newValue = recipient list joined with ", ".
	if err := app.conversation.InsertConversationActivity(cmodels.ActivityMessageForward, cuuid, strings.Join(req.To, ", "), user); err != nil {
		// Non-fatal: log but don't fail the request.
		app.lo.Error("error inserting forward activity log", "error", err)
	}

	return r.SendEnvelope(newMsg)
}

// buildForwardedBody produces the HTML body of a forwarded message. The original content is
// inlined unchanged inside a styled blockquote — keeping cid:... references intact so the
// existing CID rewrite at send time resolves them against the cloned media.
func buildForwardedBody(comment string, origFrom, origTo []any, origSubject, origDate, origContent string) string {
	var sb strings.Builder
	if strings.TrimSpace(comment) != "" {
		sb.WriteString("<p>")
		sb.WriteString(html.EscapeString(comment))
		sb.WriteString("</p>")
	}
	sb.WriteString(`<blockquote style="border-left:2px solid #e5e7eb;padding-left:1em;color:#374151;">`)
	sb.WriteString("<p>")
	sb.WriteString(fmt.Sprintf("<b>From:</b> %s<br/>", html.EscapeString(joinAny(origFrom))))
	sb.WriteString(fmt.Sprintf("<b>Date:</b> %s<br/>", html.EscapeString(origDate)))
	sb.WriteString(fmt.Sprintf("<b>Subject:</b> %s<br/>", html.EscapeString(origSubject)))
	sb.WriteString(fmt.Sprintf("<b>To:</b> %s", html.EscapeString(joinAny(origTo))))
	sb.WriteString("</p>")
	// Original content is rendered verbatim (already HTML); do not escape — that would break formatting + cid: refs.
	sb.WriteString(origContent)
	sb.WriteString(`</blockquote>`)
	return sb.String()
}

// joinAny joins []any string elements with ", ". Skips non-string items defensively.
func joinAny(xs []any) string {
	parts := make([]string, 0, len(xs))
	for _, x := range xs {
		if s, ok := x.(string); ok && s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ", ")
}

// resolveContentCIDs replaces inline image cid: references in email message content
// with actual attachment URLs and resolves relative /uploads/ paths to absolute URLs.
func resolveContentCIDs(msg *cmodels.Message, rootURL string) {
	for _, att := range msg.Attachments {
		if att.ContentID != "" && att.URL != "" {
			msg.Content = strings.ReplaceAll(msg.Content, "cid:"+att.ContentID, att.URL)
		}
	}
	if rootURL != "" {
		msg.Content = strings.ReplaceAll(msg.Content, `src="/uploads/`, `src="`+rootURL+`/uploads/`)
		msg.Content = strings.ReplaceAll(msg.Content, `src='/uploads/`, `src='`+rootURL+`/uploads/`)
	}
}
