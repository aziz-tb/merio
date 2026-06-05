package main

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"

	aiopmodels "github.com/aziz-tb/merio/internal/ai/operations/models"
	amodels "github.com/aziz-tb/merio/internal/auth/models"
	authzModels "github.com/aziz-tb/merio/internal/authz/models"
	"github.com/aziz-tb/merio/internal/automation/models"
	contractpkg "github.com/aziz-tb/merio/internal/contract"
	contractmodels "github.com/aziz-tb/merio/internal/contract/models"
	cmodels "github.com/aziz-tb/merio/internal/conversation/models"
	customerpkg "github.com/aziz-tb/merio/internal/customer"
	"github.com/aziz-tb/merio/internal/customerdata"
	"github.com/aziz-tb/merio/internal/dbutil"
	"github.com/aziz-tb/merio/internal/detection"
	"github.com/aziz-tb/merio/internal/envelope"
	"github.com/aziz-tb/merio/internal/stringutil"
	itemporal "github.com/aziz-tb/merio/internal/temporal"
	umodels "github.com/aziz-tb/merio/internal/user/models"
	vmodels "github.com/aziz-tb/merio/internal/view/models"
	wmodels "github.com/aziz-tb/merio/internal/webhook/models"
	"github.com/valyala/fasthttp"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/fastglue"
)

// looksLikeUUID is a cheap structural check used to distinguish conversation
// UUIDs from operator-facing reference numbers like "#1234".
func looksLikeUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			isHex := (r >= '0' && r <= '9') ||
				(r >= 'a' && r <= 'f') ||
				(r >= 'A' && r <= 'F')
			if !isHex {
				return false
			}
		}
	}
	return true
}

type assigneeChangeReq struct {
	AssigneeID int `json:"assignee_id"`
}

type teamAssigneeChangeReq struct {
	AssigneeID int `json:"assignee_id"`
}

type priorityUpdateReq struct {
	Priority string `json:"priority"`
}

type statusUpdateReq struct {
	Status       string `json:"status"`
	SnoozedUntil string `json:"snoozed_until,omitempty"`
}

type tagsUpdateReq struct {
	Tags []string `json:"tags"`
}

type createConversationRequest struct {
	InboxID          int            `json:"inbox_id"`
	AssignedAgentID  int            `json:"agent_id"`
	AssignedTeamID   int            `json:"team_id"`
	Email            string         `json:"contact_email"`
	FirstName        string         `json:"first_name"`
	LastName         string         `json:"last_name"`
	ExternalUserID   string         `json:"external_user_id"`
	Subject          string         `json:"subject"`
	Content          string         `json:"content"`
	Attachments      []int          `json:"attachments"`
	Initiator        string         `json:"initiator"` // "contact" | "agent"
	CustomAttributes map[string]any `json:"custom_attributes"`
}

type conversationStatsResponse struct {
	Lists map[string]int `json:"lists"`
	Teams map[string]int `json:"teams"`
	Views map[string]int `json:"views"`
}

func hasPermission(user umodels.User, perm string) bool {
	return slices.Contains(user.Permissions, perm)
}

func conversationListTotal(conversations []cmodels.ConversationListItem) int {
	if len(conversations) == 0 {
		return 0
	}
	return conversations[0].Total
}

func withOpenConversationStatus(filtersJSON string) (string, error) {
	var filters []dbutil.Filter
	if filtersJSON = strings.TrimSpace(filtersJSON); filtersJSON != "" && filtersJSON != "null" {
		if err := json.Unmarshal([]byte(filtersJSON), &filters); err != nil {
			return "", err
		}
	}

	openFilters := make([]dbutil.Filter, 0, len(filters)+1)
	for _, filter := range filters {
		if filter.Model == "conversation_statuses" {
			continue
		}
		if filter.Model == "conversations" && filter.Field == "status_id" {
			continue
		}
		openFilters = append(openFilters, filter)
	}
	openFilters = append(openFilters, dbutil.Filter{
		Model:    "conversation_statuses",
		Field:    "name",
		Operator: "equals",
		Value:    cmodels.StatusOpen,
	})

	out, err := json.Marshal(openFilters)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func conversationAccessListsForUser(user umodels.User) []string {
	lists := []string{}
	hasTeamAll := hasPermission(user, authzModels.PermConversationsReadTeamAll)
	for _, perm := range user.Permissions {
		if perm == authzModels.PermConversationsReadAll {
			// No further lists required as user has access to all conversations.
			return []string{cmodels.AllConversations}
		}
		if perm == authzModels.PermConversationsReadUnassigned {
			lists = append(lists, cmodels.UnassignedConversations)
		}
		if perm == authzModels.PermConversationsReadAssigned {
			lists = append(lists, cmodels.AssignedConversations)
		}
		// Skip TeamUnassignedConversations if user has TeamAllConversations (superset).
		if perm == authzModels.PermConversationsReadTeamInbox && !hasTeamAll {
			lists = append(lists, cmodels.TeamUnassignedConversations)
		}
		if perm == authzModels.PermConversationsReadTeamAll {
			lists = append(lists, cmodels.TeamAllConversations)
		}
	}
	return lists
}

func canAccessView(view vmodels.View, user umodels.User) bool {
	switch view.Visibility {
	case vmodels.VisibilityUser:
		return view.UserID != nil && *view.UserID == user.ID
	case vmodels.VisibilityAll:
		return true
	case vmodels.VisibilityTeam:
		return view.TeamID != nil && slices.Contains(user.Teams.IDs(), *view.TeamID)
	default:
		return false
	}
}

// handleGetAllConversations retrieves all conversations.
func handleGetAllConversations(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		user    = r.RequestCtx.UserValue("user").(amodels.User)
		order   = string(r.RequestCtx.QueryArgs().Peek("order"))
		orderBy = string(r.RequestCtx.QueryArgs().Peek("order_by"))
		filters = string(r.RequestCtx.QueryArgs().Peek("filters"))
		total   = 0
	)
	page, pageSize := getPagination(r)

	conversations, err := app.conversation.GetAllConversationsList(user.ID, order, orderBy, filters, page, pageSize)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	if len(conversations) > 0 {
		total = conversations[0].Total
	}

	return r.SendEnvelope(envelope.PageResults{
		Results:    conversations,
		Total:      total,
		PerPage:    pageSize,
		TotalPages: (total + pageSize - 1) / pageSize,
		Page:       page,
	})
}

// handleGetAssignedConversations retrieves conversations assigned to the current user.
func handleGetAssignedConversations(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		user    = r.RequestCtx.UserValue("user").(amodels.User)
		order   = string(r.RequestCtx.QueryArgs().Peek("order"))
		orderBy = string(r.RequestCtx.QueryArgs().Peek("order_by"))
		filters = string(r.RequestCtx.QueryArgs().Peek("filters"))
		total   = 0
	)
	page, pageSize := getPagination(r)
	conversations, err := app.conversation.GetAssignedConversationsList(user.ID, user.ID, order, orderBy, filters, page, pageSize)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if len(conversations) > 0 {
		total = conversations[0].Total
	}

	return r.SendEnvelope(envelope.PageResults{
		Results:    conversations,
		Total:      total,
		PerPage:    pageSize,
		TotalPages: (total + pageSize - 1) / pageSize,
		Page:       page,
	})
}

// handleGetUnassignedConversations retrieves unassigned conversations.
func handleGetUnassignedConversations(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		user    = r.RequestCtx.UserValue("user").(amodels.User)
		order   = string(r.RequestCtx.QueryArgs().Peek("order"))
		orderBy = string(r.RequestCtx.QueryArgs().Peek("order_by"))
		filters = string(r.RequestCtx.QueryArgs().Peek("filters"))
		total   = 0
	)
	page, pageSize := getPagination(r)

	conversations, err := app.conversation.GetUnassignedConversationsList(user.ID, order, orderBy, filters, page, pageSize)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if len(conversations) > 0 {
		total = conversations[0].Total
	}

	return r.SendEnvelope(envelope.PageResults{
		Results:    conversations,
		Total:      total,
		PerPage:    pageSize,
		TotalPages: (total + pageSize - 1) / pageSize,
		Page:       page,
	})
}

// handleGetMentionedConversations retrieves conversations where the current user is mentioned.
func handleGetMentionedConversations(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		user    = r.RequestCtx.UserValue("user").(amodels.User)
		order   = string(r.RequestCtx.QueryArgs().Peek("order"))
		orderBy = string(r.RequestCtx.QueryArgs().Peek("order_by"))
		filters = string(r.RequestCtx.QueryArgs().Peek("filters"))
		total   = 0
	)
	page, pageSize := getPagination(r)

	conversations, err := app.conversation.GetMentionedConversationsList(user.ID, order, orderBy, filters, page, pageSize)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if len(conversations) > 0 {
		total = conversations[0].Total
	}

	return r.SendEnvelope(envelope.PageResults{
		Results:    conversations,
		Total:      total,
		PerPage:    pageSize,
		TotalPages: (total + pageSize - 1) / pageSize,
		Page:       page,
	})
}

// handleGetViewConversations retrieves conversations for a view.
func handleGetViewConversations(r *fastglue.Request) error {
	var (
		app       = r.Context.(*App)
		auser     = r.RequestCtx.UserValue("user").(amodels.User)
		viewID, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
		order     = string(r.RequestCtx.QueryArgs().Peek("order"))
		orderBy   = string(r.RequestCtx.QueryArgs().Peek("order_by"))
		total     = 0
	)
	page, pageSize := getPagination(r)
	if viewID < 1 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	// Check if user has access to the view.
	view, err := app.view.Get(viewID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	if !canAccessView(view, user) {
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, app.i18n.T("conversation.viewPermissionDenied"), nil, envelope.PermissionError)
	}

	// Prepare lists user has access to based on user permissions, internally this prepares the SQL query.
	lists := conversationAccessListsForUser(user)
	if len(lists) == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, app.i18n.T("status.deniedPermission"), nil, envelope.PermissionError)
	}

	conversations, err := app.conversation.GetViewConversationsList(user.ID, user.ID, user.Teams.IDs(), lists, order, orderBy, string(view.Filters), page, pageSize)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if len(conversations) > 0 {
		total = conversations[0].Total
	}

	return r.SendEnvelope(envelope.PageResults{
		Results:    conversations,
		Total:      total,
		PerPage:    pageSize,
		TotalPages: (total + pageSize - 1) / pageSize,
		Page:       page,
	})
}

// handleGetConversationStats returns Open conversation counts for sidebar buckets.
func handleGetConversationStats(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		stats = conversationStatsResponse{
			Lists: map[string]int{
				cmodels.AssignedConversations:   0,
				cmodels.MentionedConversations:  0,
				cmodels.UnassignedConversations: 0,
				cmodels.AllConversations:        0,
			},
			Teams: map[string]int{},
			Views: map[string]int{},
		}
	)

	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	openFilters, err := withOpenConversationStatus("")
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}

	if hasPermission(user, authzModels.PermConversationsReadAssigned) {
		conversations, err := app.conversation.GetAssignedConversationsList(user.ID, user.ID, "", "", openFilters, 1, 1)
		if err != nil {
			return sendErrorEnvelope(r, err)
		}
		stats.Lists[cmodels.AssignedConversations] = conversationListTotal(conversations)
	}

	if hasPermission(user, authzModels.PermConversationsRead) {
		conversations, err := app.conversation.GetMentionedConversationsList(user.ID, "", "", openFilters, 1, 1)
		if err != nil {
			return sendErrorEnvelope(r, err)
		}
		stats.Lists[cmodels.MentionedConversations] = conversationListTotal(conversations)
	}

	if hasPermission(user, authzModels.PermConversationsReadUnassigned) {
		conversations, err := app.conversation.GetUnassignedConversationsList(user.ID, "", "", openFilters, 1, 1)
		if err != nil {
			return sendErrorEnvelope(r, err)
		}
		stats.Lists[cmodels.UnassignedConversations] = conversationListTotal(conversations)
	}

	if hasPermission(user, authzModels.PermConversationsReadAll) {
		conversations, err := app.conversation.GetAllConversationsList(user.ID, "", "", openFilters, 1, 1)
		if err != nil {
			return sendErrorEnvelope(r, err)
		}
		stats.Lists[cmodels.AllConversations] = conversationListTotal(conversations)
	}

	for _, teamID := range user.Teams.IDs() {
		key := strconv.Itoa(teamID)
		stats.Teams[key] = 0
		if !hasPermission(user, authzModels.PermConversationsReadTeamInbox) {
			continue
		}

		conversations, err := app.conversation.GetTeamUnassignedConversationsList(user.ID, teamID, "", "", openFilters, 1, 1)
		if err != nil {
			return sendErrorEnvelope(r, err)
		}
		stats.Teams[key] = conversationListTotal(conversations)
	}

	canReadViews := hasPermission(user, authzModels.PermConversationsRead)
	viewLists := conversationAccessListsForUser(user)
	views := []vmodels.View{}

	if hasPermission(user, authzModels.PermViewManage) {
		userViews, err := app.view.GetUsersViews(user.ID)
		if err != nil {
			return sendErrorEnvelope(r, err)
		}
		views = append(views, userViews...)
	}

	sharedViews, err := app.view.GetSharedViewsForUser(user.Teams.IDs())
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	views = append(views, sharedViews...)

	for _, view := range views {
		key := strconv.Itoa(view.ID)
		stats.Views[key] = 0
		if !canReadViews || len(viewLists) == 0 || !canAccessView(view, user) {
			continue
		}

		filters, err := withOpenConversationStatus(string(view.Filters))
		if err != nil {
			return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
		}
		conversations, err := app.conversation.GetViewConversationsList(user.ID, user.ID, user.Teams.IDs(), viewLists, "", "", filters, 1, 1)
		if err != nil {
			return sendErrorEnvelope(r, err)
		}
		stats.Views[key] = conversationListTotal(conversations)
	}

	return r.SendEnvelope(stats)
}

// handleGetTeamUnassignedConversations returns conversations assigned to a team but not to any user.
func handleGetTeamUnassignedConversations(r *fastglue.Request) error {
	var (
		app       = r.Context.(*App)
		auser     = r.RequestCtx.UserValue("user").(amodels.User)
		teamIDStr = r.RequestCtx.UserValue("id").(string)
		order     = string(r.RequestCtx.QueryArgs().Peek("order"))
		orderBy   = string(r.RequestCtx.QueryArgs().Peek("order_by"))
		filters   = string(r.RequestCtx.QueryArgs().Peek("filters"))
		total     = 0
	)
	page, pageSize := getPagination(r)
	teamID, _ := strconv.Atoi(teamIDStr)
	if teamID < 1 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	// Check if user belongs to the team.
	exists, err := app.team.UserBelongsToTeam(teamID, auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	if !exists {
		return sendErrorEnvelope(r, envelope.NewError(envelope.PermissionError, app.i18n.T("conversation.notMemberOfTeam"), nil))
	}

	conversations, err := app.conversation.GetTeamUnassignedConversationsList(auser.ID, teamID, order, orderBy, filters, page, pageSize)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if len(conversations) > 0 {
		total = conversations[0].Total
	}

	return r.SendEnvelope(envelope.PageResults{
		Results:    conversations,
		Total:      total,
		PerPage:    pageSize,
		TotalPages: (total + pageSize - 1) / pageSize,
		Page:       page,
	})
}

// handleGetConversation retrieves a single conversation by it's UUID.
func handleGetConversation(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)

	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	conv, err := enforceConversationAccess(app, uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	prev, _ := app.conversation.GetContactPreviousConversations(conv.ContactID, 10)
	conv.PreviousConversations = filterCurrentPreviousConv(prev, conv.UUID)
	return r.SendEnvelope(conv)
}

func handleGetConversationVariables(r *fastglue.Request) error {
	var (
		app        = r.Context.(*App)
		identifier = strings.TrimSpace(r.RequestCtx.UserValue("uuid").(string))
		auser      = r.RequestCtx.UserValue("user").(amodels.User)
	)
	if identifier == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "conversation identifier required", nil, envelope.InputError)
	}

	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	conv, err := enforceConversationIdentifierAccess(app, identifier, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	ticketJSON, err := detection.BuildTicketJSON(*conv)
	if err != nil {
		app.lo.Error("error building detection ticket variables", "identifier", identifier, "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}

	var customerJSON map[string]any
	if conv.CustomerID.Valid && app.customer != nil {
		customer, err := app.customer.GetFreshByCustomerID(r.RequestCtx, conv.CustomerID.String, customerpkg.FreshOptions{})
		if err != nil {
			app.lo.Warn("error building detection customer variables", "identifier", identifier, "customer_id", conv.CustomerID.String, "error", err)
		} else if customer != nil {
			var contracts []contractmodels.Contract
			if app.contract != nil {
				contracts, err = loadContractsForCustomer(r.RequestCtx, app.lo, app.contract, customer)
				if err != nil {
					app.lo.Warn("error hydrating contracts for detection customer variables", "identifier", identifier, "customer_id", conv.CustomerID.String, "error", err)
				}
			}
			if m, err := customerdata.CustomerToMap(*customer, contracts); err == nil {
				customerJSON = m
			} else {
				app.lo.Warn("error serializing detection customer variables", "identifier", identifier, "customer_id", conv.CustomerID.String, "error", err)
			}
		}
	}

	var contractJSON map[string]any
	if conv.ContractID.Valid && app.contract != nil {
		contract, err := app.contract.GetFreshByContractID(r.RequestCtx, conv.ContractID.String, contractpkg.FreshOptions{})
		if err != nil {
			app.lo.Warn("error building detection contract variables", "identifier", identifier, "contract_id", conv.ContractID.String, "error", err)
		} else if contract != nil {
			if m, err := customerdata.ContractToMap(*contract); err == nil {
				contractJSON = m
			} else {
				app.lo.Warn("error serializing detection contract variables", "identifier", identifier, "contract_id", conv.ContractID.String, "error", err)
			}
		}
	}

	return r.SendEnvelope(map[string]any{
		"ticket":   ticketJSON,
		"customer": customerJSON,
		"contract": contractJSON,
		"tags":     extractTagSlugsFromConv(*conv),
	})
}

func handleEvaluateDetection(r *fastglue.Request) error {
	app := r.Context.(*App)

	var body struct {
		Detection        json.RawMessage `json:"detection"`
		ConversationUUID string          `json:"conversation_uuid"`
	}
	if err := json.Unmarshal(r.RequestCtx.PostBody(), &body); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}
	if body.ConversationUUID == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "conversation_uuid required", nil, envelope.InputError)
	}
	if app.detectionMgr == nil {
		return r.SendErrorEnvelope(fasthttp.StatusServiceUnavailable, "detection manager unavailable", nil, envelope.GeneralError)
	}

	var det detection.Detection
	if err := json.Unmarshal(body.Detection, &det); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "invalid detection JSON", nil, envelope.InputError)
	}

	auser := r.RequestCtx.UserValue("user").(amodels.User)
	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	conv, err := enforceConversationAccess(app, body.ConversationUUID, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	result, err := app.detectionMgr.Evaluate(app.ctx, det, buildDetectionEvaluationContext(app.ctx, app, *conv, extractTagSlugsFromConv(*conv), ""))
	if err != nil {
		app.lo.Error("error evaluating detection", "conversation_uuid", body.ConversationUUID, "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, err.Error(), nil, envelope.GeneralError)
	}
	return r.SendEnvelope(result)
}

func buildDetectionEvaluationContext(ctx context.Context, app *App, conv cmodels.Conversation, tags []string, traceID string) detection.EvaluationContext {
	ec := detection.EvaluationContext{
		Conversation: conv,
		Tags:         tags,
		TraceID:      traceID,
	}
	if conv.CustomerID.Valid && app.customer != nil {
		customer, err := app.customer.GetFreshByCustomerID(ctx, conv.CustomerID.String, customerpkg.FreshOptions{})
		if err != nil {
			app.lo.Warn("failed to hydrate customer for detection evaluation",
				"conversation_id", conv.ID,
				"customer_id", conv.CustomerID.String,
				"error", err)
		} else {
			ec.Customer = customer
		}
	}
	if conv.ContractID.Valid && app.contract != nil {
		contract, err := app.contract.GetFreshByContractID(ctx, conv.ContractID.String, contractpkg.FreshOptions{})
		if err != nil {
			app.lo.Warn("failed to hydrate contract for detection evaluation",
				"conversation_id", conv.ID,
				"contract_id", conv.ContractID.String,
				"error", err)
		} else {
			ec.Contract = contract
		}
	}
	return ec
}

// handleClassifyConversationTags starts an AI tag-classification workflow.
func handleClassifyConversationTags(r *fastglue.Request) error {
	app := r.Context.(*App)
	uuid := r.RequestCtx.UserValue("uuid").(string)
	if uuid == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "uuid required", nil, envelope.InputError)
	}
	if app.temporal == nil {
		return r.SendErrorEnvelope(fasthttp.StatusServiceUnavailable, "Tag classification is not available; Temporal is not configured.", nil, envelope.GeneralError)
	}

	auser := r.RequestCtx.UserValue("user").(amodels.User)
	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if _, err := enforceConversationAccess(app, uuid, user); err != nil {
		return sendErrorEnvelope(r, err)
	}

	workflowID, err := app.conversation.ClassifyConversation(uuid, "manual", int64(user.ID))
	if err != nil {
		if itemporal.IsAlreadyStarted(err) {
			return r.SendErrorEnvelope(fasthttp.StatusConflict, "Classification already in progress. Please wait.", nil, envelope.GeneralError)
		}
		app.lo.Error("error starting tag classification workflow", "conversation_uuid", uuid, "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to start classification.", nil, envelope.GeneralError)
	}

	return r.SendEnvelope(map[string]any{
		"workflow_id": workflowID,
		"message":     "Classification started.",
	})
}

// handleValidateConversationAgents starts an agent-validation workflow.
func handleValidateConversationAgents(r *fastglue.Request) error {
	app := r.Context.(*App)
	uuid := r.RequestCtx.UserValue("uuid").(string)
	if uuid == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "uuid required", nil, envelope.InputError)
	}
	if app.temporal == nil {
		return r.SendErrorEnvelope(fasthttp.StatusServiceUnavailable, "Agent validation is not available; Temporal is not configured.", nil, envelope.GeneralError)
	}

	auser := r.RequestCtx.UserValue("user").(amodels.User)
	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if _, err := enforceConversationAccess(app, uuid, user); err != nil {
		return sendErrorEnvelope(r, err)
	}

	workflowID, err := app.conversation.ValidateConversationAgents(r.RequestCtx, uuid, "manual", int64(user.ID))
	if err != nil {
		if itemporal.IsAlreadyStarted(err) {
			return r.SendErrorEnvelope(fasthttp.StatusConflict, "Validation already in progress. Please wait.", nil, envelope.GeneralError)
		}
		app.lo.Error("error starting agent validation workflow", "conversation_uuid", uuid, "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to start validation.", nil, envelope.GeneralError)
	}

	return r.SendEnvelope(map[string]any{
		"workflow_id": workflowID,
		"message":     "Validation started.",
	})
}

// handleGetLatestAgentValidation returns the latest agent-validation operation
// for a conversation, or 404 when validation has never run.
func handleGetLatestAgentValidation(r *fastglue.Request) error {
	app := r.Context.(*App)
	uuid := r.RequestCtx.UserValue("uuid").(string)
	if uuid == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "uuid required", nil, envelope.InputError)
	}

	auser := r.RequestCtx.UserValue("user").(amodels.User)
	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if _, err := enforceConversationAccess(app, uuid, user); err != nil {
		return sendErrorEnvelope(r, err)
	}

	conv, err := app.conversation.GetConversation(0, uuid, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	ops, err := app.aiOperations.ListByTypeForConversation(r.RequestCtx, int64(conv.ID), aiopmodels.TypeAgentValidation, 1)
	if err != nil {
		app.lo.Error("fetch latest agent validation", "conversation_uuid", uuid, "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "failed to fetch validation", nil, envelope.GeneralError)
	}
	if len(ops) == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "no validation found", nil, envelope.GeneralError)
	}

	return r.SendEnvelope(ops[0])
}

func handleListConversationAIOperations(r *fastglue.Request) error {
	app := r.Context.(*App)
	uuid := r.RequestCtx.UserValue("uuid").(string)
	if uuid == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "uuid required", nil, envelope.InputError)
	}

	auser := r.RequestCtx.UserValue("user").(amodels.User)
	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	conv, err := enforceConversationAccess(app, uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	limit := 50
	if raw := strings.TrimSpace(string(r.RequestCtx.QueryArgs().Peek("limit"))); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "invalid limit", nil, envelope.InputError)
		}
		limit = parsed
	}

	opType := strings.TrimSpace(string(r.RequestCtx.QueryArgs().Peek("operation_type")))
	var ops []aiopmodels.Operation
	if opType != "" {
		ops, err = app.aiOperations.ListByTypeForConversation(r.RequestCtx, int64(conv.ID), opType, limit)
	} else {
		ops, err = app.aiOperations.ListForConversation(r.RequestCtx, int64(conv.ID), limit)
	}
	if err != nil {
		app.lo.Error("fetch conversation ai operations", "conversation_uuid", uuid, "operation_type", opType, "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "failed to fetch ai operations", nil, envelope.GeneralError)
	}
	return r.SendEnvelope(ops)
}

func extractTagSlugsFromConv(conv cmodels.Conversation) []string {
	return conv.TagSlugs()
}

// handleGetContactPageVisits returns the recent page visits for the contact of a conversation.
func handleGetContactPageVisits(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)

	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	conv, err := enforceConversationAccess(app, uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	pages := getPageVisitsFromRedis(app, conv.ContactID)
	return r.SendEnvelope(pages)
}

// handleUpdateConversationAssigneeLastSeen updates the current user's last seen timestamp for a conversation.
func handleUpdateConversationAssigneeLastSeen(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)
	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	_, err = enforceConversationAccess(app, uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	if err = app.conversation.UpdateUserLastSeen(uuid, auser.ID); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}

// handleMarkConversationAsUnread marks a conversation as unread for the current user.
func handleMarkConversationAsUnread(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)
	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	_, err = enforceConversationAccess(app, uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	if err = app.conversation.MarkAsUnread(uuid, auser.ID); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}

// handleGetConversationParticipants retrieves participants of a conversation.
func handleGetConversationParticipants(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)
	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	_, err = enforceConversationAccess(app, uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	p, err := app.conversation.GetConversationParticipants(uuid)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(p)
}

// handleUpdateUserAssignee updates the user assigned to a conversation.
func handleUpdateUserAssignee(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		req   = assigneeChangeReq{}
	)

	if err := r.Decode(&req, "json"); err != nil {
		app.lo.Error("error decoding assignee change request", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}

	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	conversation, err := enforceConversationAccess(app, uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Already assigned?
	if conversation.AssignedUserID.Int == req.AssigneeID {
		return r.SendEnvelope(true)
	}

	if err := app.conversation.UpdateConversationUserAssignee(uuid, req.AssigneeID, user); err != nil {
		return sendErrorEnvelope(r, err)
	}

	return r.SendEnvelope(true)
}

// handleUpdateTeamAssignee updates the team assigned to a conversation.
func handleUpdateTeamAssignee(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		req   = teamAssigneeChangeReq{}
	)

	if err := r.Decode(&req, "json"); err != nil {
		app.lo.Error("error decoding team assignee change request", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}

	assigneeID := req.AssigneeID

	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	_, err = app.team.Get(assigneeID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	conversation, err := enforceConversationAccess(app, uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Already assigned?
	if conversation.AssignedTeamID.Int == assigneeID {
		return r.SendEnvelope(true)
	}
	if err := app.conversation.UpdateConversationTeamAssignee(uuid, assigneeID, user); err != nil {
		return sendErrorEnvelope(r, err)
	}

	return r.SendEnvelope(true)
}

// handleUpdateConversationPriority updates the priority of a conversation.
func handleUpdateConversationPriority(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		req   = priorityUpdateReq{}
	)

	if err := r.Decode(&req, "json"); err != nil {
		app.lo.Error("error decoding priority update request", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}

	priority := strings.TrimSpace(req.Priority)
	if priority == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.Ts("globals.messages.empty", "name", "`priority`"), nil, envelope.InputError)
	}
	priorities, err := app.priority.GetAll()
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	validPriority := false
	for _, p := range priorities {
		if p.Name == priority {
			validPriority = true
			break
		}
	}
	if !validPriority {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("validation.invalidValue"), nil, envelope.InputError)
	}

	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	_, err = enforceConversationAccess(app, uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if err := app.conversation.UpdateConversationPriority(uuid, 0 /**priority_id**/, priority, user); err != nil {
		return sendErrorEnvelope(r, err)
	}

	return r.SendEnvelope(true)
}

// handleUpdateConversationStatus updates the status of a conversation.
func handleUpdateConversationStatus(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		req   = statusUpdateReq{}
	)

	if err := r.Decode(&req, "json"); err != nil {
		app.lo.Error("error decoding status update request", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}

	status := req.Status
	snoozedUntil := req.SnoozedUntil

	// Validate inputs
	if status == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.Ts("globals.messages.empty", "name", "`status`"), nil, envelope.InputError)
	}
	if snoozedUntil == "" && status == cmodels.StatusSnoozed {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.Ts("globals.messages.empty", "name", "`snoozed_until`"), nil, envelope.InputError)
	}
	if status == cmodels.StatusSnoozed {
		_, err := time.ParseDuration(snoozedUntil)
		if err != nil {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
		}
	}

	// Enforce conversation access.
	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	conversation, err := enforceConversationAccess(app, uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Update conversation status.
	if err := app.conversation.UpdateConversationStatus(uuid, 0 /**status_id**/, status, snoozedUntil, user); err != nil {
		return sendErrorEnvelope(r, err)
	}
	if conversation.ContractID.Valid {
		invalidateLinkedConversationCaches(app, conversation, "conversation_status_change")
	}

	// If status is `Resolved`, send CSAT survey if enabled on inbox.
	if status == cmodels.StatusResolved {
		// Check if CSAT is enabled on the inbox and send CSAT survey message.
		inbox, err := app.inbox.GetDBRecord(conversation.InboxID)
		if err != nil {
			return sendErrorEnvelope(r, err)
		}
		if inbox.CSATEnabled {
			if err := app.conversation.SendCSATReply(user.ID, *conversation); err != nil {
				return sendErrorEnvelope(r, err)
			}
		}
	}
	return r.SendEnvelope(true)
}

// handleUpdateConversationtags updates conversation tags.
func handleUpdateConversationtags(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		req   = tagsUpdateReq{}
	)

	if err := r.Decode(&req, "json"); err != nil {
		app.lo.Error("error decoding tags update request", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}

	tagNames := req.Tags

	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	_, err = enforceConversationAccess(app, uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	if err := app.conversation.SetConversationTags(uuid, models.ActionSetTags, tagNames, user); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}

// handleUpdateConversationCustomAttributes updates custom attributes of a conversation.
func handleUpdateConversationCustomAttributes(r *fastglue.Request) error {
	var (
		app        = r.Context.(*App)
		attributes = map[string]any{}
		auser      = r.RequestCtx.UserValue("user").(amodels.User)
		uuid       = r.RequestCtx.UserValue("uuid").(string)
	)
	if err := r.Decode(&attributes, ""); err != nil {
		app.lo.Error("error unmarshalling custom attributes JSON", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}

	// Enforce conversation access.
	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	_, err = enforceConversationAccess(app, uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Update custom attributes.
	if err := app.conversation.UpdateConversationCustomAttributes(uuid, attributes); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}

// handleUpdateContactCustomAttributes updates custom attributes of a contact.
func handleUpdateContactCustomAttributes(r *fastglue.Request) error {
	var (
		app        = r.Context.(*App)
		attributes = map[string]any{}
		auser      = r.RequestCtx.UserValue("user").(amodels.User)
		uuid       = r.RequestCtx.UserValue("uuid").(string)
	)
	if err := r.Decode(&attributes, ""); err != nil {
		app.lo.Error("error unmarshalling custom attributes JSON", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}

	// Enforce conversation access.
	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	conversation, err := enforceConversationAccess(app, uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if err := app.user.SaveCustomAttributes(conversation.ContactID, attributes, false); err != nil {
		return sendErrorEnvelope(r, err)
	}
	// Broadcast update.
	app.conversation.BroadcastContactUpdate(conversation.ContactID, map[string]any{"custom_attributes": attributes})
	return r.SendEnvelope(true)
}

// enforceConversationAccess fetches the conversation and checks if the user has access to it.
func enforceConversationAccess(app *App, uuid string, user umodels.User) (*cmodels.Conversation, error) {
	conversation, err := app.conversation.GetConversation(0, uuid, "")
	if err != nil {
		return nil, err
	}
	allowed, err := app.authz.EnforceConversationAccess(user, conversation)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, envelope.NewError(envelope.PermissionError, "Permission denied", nil)
	}
	return &conversation, nil
}

func enforceConversationIdentifierAccess(app *App, identifier string, user umodels.User) (*cmodels.Conversation, error) {
	conversation, err := getConversationByIdentifier(app, identifier)
	if err != nil {
		return nil, err
	}
	allowed, err := app.authz.EnforceConversationAccess(user, conversation)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, envelope.NewError(envelope.PermissionError, "Permission denied", nil)
	}
	return &conversation, nil
}

func getConversationByIdentifier(app *App, identifier string) (cmodels.Conversation, error) {
	identifier = strings.TrimSpace(identifier)
	if looksLikeUUID(identifier) {
		return app.conversation.GetConversation(0, identifier, "")
	}
	refNum := strings.TrimPrefix(identifier, "#")
	return app.conversation.GetConversation(0, "", refNum)
}

// handleRemoveUserAssignee removes the user assigned to a conversation.
func handleRemoveUserAssignee(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)
	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	_, err = enforceConversationAccess(app, uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if err = app.conversation.RemoveConversationAssignee(uuid, "user", user); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}

// handleRemoveTeamAssignee removes the team assigned to a conversation.
func handleRemoveTeamAssignee(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)
	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	_, err = enforceConversationAccess(app, uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if err = app.conversation.RemoveConversationAssignee(uuid, "team", user); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}

// filterCurrentPreviousConv removes the current conversation from the list of previous conversations.
func filterCurrentPreviousConv(convs []cmodels.PreviousConversation, uuid string) []cmodels.PreviousConversation {
	for i, c := range convs {
		if c.UUID == uuid {
			return append(convs[:i], convs[i+1:]...)
		}
	}
	return []cmodels.PreviousConversation{}
}

// handleCreateConversation creates a new conversation and sends a message to it.
func handleCreateConversation(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		req   = createConversationRequest{}
	)

	if err := r.Decode(&req, "json"); err != nil {
		app.lo.Error("error decoding create conversation request", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}

	// Validate the request
	if err := validateCreateConversationRequest(req, app); err != nil {
		return sendErrorEnvelope(r, err)
	}

	to := []string{req.Email}
	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Find or create contact.
	contact := umodels.User{
		Email:            null.StringFrom(req.Email),
		FirstName:        req.FirstName,
		LastName:         req.LastName,
		ExternalUserID:   null.NewString(req.ExternalUserID, req.ExternalUserID != ""),
		CustomAttributes: json.RawMessage(`{}`),
	}
	if err := app.user.CreateContact(&contact); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.T("globals.messages.somethingWentWrong"), nil))
	}

	// Create conversation first.
	conversationID, conversationUUID, err := app.conversation.CreateConversation(
		contact.ID,
		req.InboxID,
		"",         /** last_message **/
		time.Now(), /** last_message_at **/
		req.Subject,
		true, /** append reference number to subject? **/
		nil,
		req.CustomAttributes,
		0, 0,
	)
	if err != nil {
		app.lo.Error("error creating conversation", "error", err)
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.T("globals.messages.somethingWentWrong"), nil))
	}

	// Get media for the attachment ids, skip any already associated with a model.
	media, err := getUnassociatedMedia(app, req.Attachments)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}

	// Send initial message based on the initiator of conversation.
	switch req.Initiator {
	case umodels.UserTypeAgent:
		// Queue reply.
		if _, err := app.conversation.QueueReply(media, req.InboxID, auser.ID /**sender_id**/, contact.ID, conversationUUID, req.Content, to, nil /**cc**/, nil /**bcc**/, map[string]any{} /**meta**/, nil /**sendInboxID**/); err != nil {
			// Delete the conversation if msg queue fails.
			if err := app.conversation.DeleteConversation(conversationUUID); err != nil {
				app.lo.Error("error deleting conversation", "error", err)
			}
			return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.T("globals.messages.errorSendingMessage"), nil))
		}
		// Trigger webhook for agent-initiated conversation, for contact intitiated the incoming message hooks handle it.
		if c, err := app.conversation.GetConversation(0, conversationUUID, ""); err == nil {
			app.webhook.TriggerEvent(wmodels.EventConversationCreated, c)
		}
	case umodels.UserTypeContact:
		// Create contact message.
		if _, err := app.conversation.CreateContactMessage(media, contact.ID, conversationUUID, req.Content, cmodels.ContentTypeHTML, true); err != nil {
			// Delete the conversation if message creation fails.
			if err := app.conversation.DeleteConversation(conversationUUID); err != nil {
				app.lo.Error("error deleting conversation", "error", err)
			}
			return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.T("globals.messages.errorSendingMessage"), nil))
		}
	default:
		// Guard anyway.
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	// Assign the conversation to team/agent if provided, always assign team first as it clears assigned agent.
	if req.AssignedTeamID > 0 {
		app.conversation.UpdateConversationTeamAssignee(conversationUUID, req.AssignedTeamID, user)
	}
	if req.AssignedAgentID > 0 {
		app.conversation.UpdateConversationUserAssignee(conversationUUID, req.AssignedAgentID, user)
	}

	conversation, _ := app.conversation.GetConversation(conversationID, "", "")
	return r.SendEnvelope(conversation)
}

// validateCreateConversationRequest validates the create conversation request fields.
func validateCreateConversationRequest(req createConversationRequest, app *App) error {
	if req.InboxID <= 0 {
		return envelope.NewError(envelope.InputError, app.i18n.Ts("globals.messages.required", "name", "`inbox_id`"), nil)
	}
	if req.Content == "" {
		return envelope.NewError(envelope.InputError, app.i18n.Ts("globals.messages.required", "name", "`content`"), nil)
	}
	if req.Email == "" {
		return envelope.NewError(envelope.InputError, app.i18n.Ts("globals.messages.required", "name", "`contact_email`"), nil)
	}
	if req.FirstName == "" {
		return envelope.NewError(envelope.InputError, app.i18n.Ts("globals.messages.required", "name", "`first_name`"), nil)
	}
	if !stringutil.ValidEmail(req.Email) {
		return envelope.NewError(envelope.InputError, app.i18n.T("validation.invalidEmail"), nil)
	}
	if req.Initiator != umodels.UserTypeContact && req.Initiator != umodels.UserTypeAgent {
		return envelope.NewError(envelope.InputError, app.i18n.T("globals.messages.somethingWentWrong"), nil)
	}

	// Check if inbox exists and is enabled.
	inbox, err := app.inbox.GetDBRecord(req.InboxID)
	if err != nil {
		return err
	}
	if !inbox.Enabled {
		return envelope.NewError(envelope.InputError, app.i18n.T("globals.messages.disabled"), nil)
	}
	if inbox.Channel != "email" {
		return envelope.NewError(envelope.InputError, app.i18n.T("globals.messages.somethingWentWrong"), nil)
	}

	// Validate custom attribute keys. Skip unknown keys.
	if len(req.CustomAttributes) > 0 {
		attrs, err := app.customAttribute.GetAll("conversation")
		if err != nil {
			return err
		}
		validKeys := make(map[string]struct{}, len(attrs))
		for _, a := range attrs {
			validKeys[a.Key] = struct{}{}
		}
		for key := range req.CustomAttributes {
			if _, ok := validKeys[key]; !ok {
				delete(req.CustomAttributes, key)
			}
		}
	}

	return nil
}
