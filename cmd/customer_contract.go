package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/volatiletech/null/v9"

	aiopmodels "github.com/aziz-tb/merio/internal/ai/operations/models"
	amodels "github.com/aziz-tb/merio/internal/auth/models"
	"github.com/aziz-tb/merio/internal/contract"
	contractmodels "github.com/aziz-tb/merio/internal/contract/models"
	cmodels "github.com/aziz-tb/merio/internal/conversation/models"
	"github.com/aziz-tb/merio/internal/customer"
	customermodels "github.com/aziz-tb/merio/internal/customer/models"
	"github.com/aziz-tb/merio/internal/customerdata"
	"github.com/aziz-tb/merio/internal/envelope"
	execmodels "github.com/aziz-tb/merio/internal/exec/runner/models"
	"github.com/aziz-tb/merio/internal/function"
	umodels "github.com/aziz-tb/merio/internal/user/models"
	verificationmodels "github.com/aziz-tb/merio/internal/verification/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

type assignConversationCustomerReq struct {
	ContractID        string `json:"contract_id"`
	VerificationLevel *int   `json:"verification_level,omitempty"`
}

func handleGetCustomer(r *fastglue.Request) error {
	app := r.Context.(*App)
	user, err := currentAgent(r, app)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if err := requireAnyPermission(r, app, user, "conversations:read", "customers:search"); err != nil {
		return err
	}

	customerID := strings.TrimSpace(r.RequestCtx.UserValue("customer_id").(string))
	if customerID == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "invalid customer id", nil, envelope.InputError)
	}
	out, err := loadCustomerWithContracts(r.RequestCtx, app, customerID)
	if err != nil {
		return sendCustomerContractError(r, app, err)
	}
	return r.SendEnvelope(out)
}

func handleGetContract(r *fastglue.Request) error {
	app := r.Context.(*App)
	user, err := currentAgent(r, app)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if err := requireAnyPermission(r, app, user, "conversations:read", "customers:search"); err != nil {
		return err
	}

	contractID := strings.TrimSpace(r.RequestCtx.UserValue("contract_id").(string))
	if contractID == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "invalid contract id", nil, envelope.InputError)
	}
	contract, err := app.contract.GetFreshByContractID(r.RequestCtx, contractID, contract.FreshOptions{})
	if err != nil {
		return sendCustomerContractError(r, app, err)
	}
	return r.SendEnvelope(contractToAPIResponse(*contract))
}

func handleSearchCustomers(r *fastglue.Request) error {
	app := r.Context.(*App)
	query := strings.TrimSpace(string(r.RequestCtx.QueryArgs().Peek("q")))
	if query == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "q is required", nil, envelope.InputError)
	}
	auser := r.RequestCtx.UserValue("user").(amodels.User)
	traceID := uuid.NewString()
	res, err := app.function.Run(r.RequestCtx, function.RunRequest{
		Name:          "search_customer_by_free_text",
		Input:         map[string]any{"q": query, "offset": 0, "limit": 10},
		InvokerUserID: auser.ID,
		TraceID:       traceID,
	})
	if err != nil {
		if errors.Is(err, function.ErrFunctionCodeEmpty) || errors.Is(err, function.ErrFunctionNotFound) {
			return r.SendErrorEnvelope(fasthttp.StatusServiceUnavailable, "customer search is not configured", nil, envelope.GeneralError)
		}
		app.lo.Error("customer free-text search failed", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}
	if res == nil || res.Kind == execmodels.KindNoop || resultDataEmpty(res.Data) {
		return r.SendEnvelope([]map[string]any{})
	}
	if res.Kind != execmodels.KindSuccess {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, nonEmpty(res.Message, "customer search failed"), nil, envelope.GeneralError)
	}
	decoded, err := res.DecodedData()
	if err != nil {
		if app.lo != nil {
			app.lo.Warn("customer free-text search returned undecodable result", "error", err)
		}
		return r.SendEnvelope([]map[string]any{})
	}
	out, ok := hydrateCustomerSearchResults(r.RequestCtx, app.lo, app.customer, decoded, customer.FreshOptions{
		InvokerUserID: auser.ID,
		TraceID:       traceID,
	})
	if !ok {
		if app.lo != nil {
			app.lo.Warn("customer free-text search returned malformed result", "data", res.Data)
		}
		return r.SendEnvelope([]map[string]any{})
	}
	return r.SendEnvelope(out)
}

func handleGetConversationCustomer(r *fastglue.Request) error {
	app := r.Context.(*App)
	conv, err := scopedConversation(r, app)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if !conv.CustomerID.Valid {
		return r.SendEnvelope(nil)
	}
	out, err := loadCustomerWithContracts(r.RequestCtx, app, conv.CustomerID.String)
	if err != nil {
		return sendCustomerContractError(r, app, err)
	}
	return r.SendEnvelope(out)
}

func handleGetConversationContract(r *fastglue.Request) error {
	app := r.Context.(*App)
	conv, err := scopedConversation(r, app)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if !conv.ContractID.Valid {
		return r.SendEnvelope(nil)
	}
	contract, err := app.contract.GetFreshByContractID(r.RequestCtx, conv.ContractID.String, contract.FreshOptions{})
	if err != nil {
		return sendCustomerContractError(r, app, err)
	}
	return r.SendEnvelope(contractToAPIResponse(*contract))
}

func handleAssignConversationCustomer(r *fastglue.Request) error {
	app := r.Context.(*App)
	conv, err := scopedConversation(r, app)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	var req assignConversationCustomerReq
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}
	req.ContractID = strings.TrimSpace(req.ContractID)
	if req.ContractID == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "contract_id is required", nil, envelope.InputError)
	}
	verificationLevel := int(verificationmodels.VerificationLevelManual)
	if req.VerificationLevel != nil {
		switch *req.VerificationLevel {
		case int(verificationmodels.VerificationLevelPartial), int(verificationmodels.VerificationLevelManual):
			verificationLevel = *req.VerificationLevel
		default:
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "verification_level must be 1 or 3", nil, envelope.InputError)
		}
	}
	auser := r.RequestCtx.UserValue("user").(amodels.User)
	contractRow, err := loadAssignableContract(r.RequestCtx, app, req, auser.ID, conv.ID)
	if err != nil {
		if errors.Is(err, contract.ErrContractNotFound) {
			return r.SendErrorEnvelope(fasthttp.StatusUnprocessableEntity, "contract is not loaded", nil, envelope.DataError)
		}
		return sendCustomerContractError(r, app, err)
	}
	if err := persistManualCustomerLink(r.RequestCtx, app, conv, contractRow, auser.ID, verificationLevel); err != nil {
		return sendCustomerContractError(r, app, err)
	}
	out, err := loadCustomerWithContracts(r.RequestCtx, app, contractRow.CustomerID)
	if err != nil {
		return sendCustomerContractError(r, app, err)
	}
	return r.SendEnvelope(out)
}

func handleRemoveConversationCustomer(r *fastglue.Request) error {
	app := r.Context.(*App)
	conv, err := scopedConversation(r, app)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if !conv.CustomerID.Valid {
		return r.SendEnvelope(map[string]any{})
	}
	auser := r.RequestCtx.UserValue("user").(amodels.User)
	if err := persistManualCustomerRemove(r.RequestCtx, app, conv, auser.ID); err != nil {
		return sendCustomerContractError(r, app, err)
	}
	return r.SendEnvelope(map[string]any{})
}

func loadAssignableContract(ctx context.Context, app *App, req assignConversationCustomerReq, operatorID int, conversationID int) (contractmodels.Contract, error) {
	c, err := app.contract.GetFreshByContractID(ctx, req.ContractID, contract.FreshOptions{
		InvokerUserID:  operatorID,
		ConversationID: conversationID,
		TraceID:        uuid.NewString(),
	})
	if err != nil {
		return contractmodels.Contract{}, err
	}
	if c == nil {
		return contractmodels.Contract{}, contract.ErrContractNotFound
	}
	return *c, nil
}

func currentAgent(r *fastglue.Request, app *App) (umodels.User, error) {
	auser := r.RequestCtx.UserValue("user").(amodels.User)
	return app.user.GetAgent(auser.ID, "")
}

func scopedConversation(r *fastglue.Request, app *App) (*cmodels.Conversation, error) {
	user, err := currentAgent(r, app)
	if err != nil {
		return nil, err
	}
	uuid := r.RequestCtx.UserValue("uuid").(string)
	return enforceConversationAccess(app, uuid, user)
}

func requireAnyPermission(r *fastglue.Request, app *App, user umodels.User, permissions ...string) error {
	for _, perm := range permissions {
		parts := strings.Split(perm, ":")
		if len(parts) != 2 {
			continue
		}
		allowed, err := app.authz.Enforce(user, parts[0], parts[1])
		if err != nil {
			return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
		}
		if allowed {
			return nil
		}
	}
	return r.SendErrorEnvelope(fasthttp.StatusForbidden, app.i18n.T("status.deniedPermission"), nil, envelope.PermissionError)
}

func loadCustomerWithContracts(ctx context.Context, app *App, customerID string) (json.RawMessage, error) {
	cust, err := app.customer.GetFreshByCustomerID(ctx, customerID, customer.FreshOptions{})
	if err != nil {
		return nil, err
	}
	contracts, err := loadContractsForCustomer(ctx, app.lo, app.contract, cust)
	if err != nil {
		return nil, err
	}
	return customerToAPIResponse(*cust, contracts), nil
}

type contractHydrationStore interface {
	EnsureHydrated(context.Context, []string, contract.FreshOptions) error
	ListByCustomerID(string) ([]contractmodels.Contract, error)
}

type warnLogger interface {
	Warn(string, ...any)
}

func loadContractsForCustomer(ctx context.Context, lo warnLogger, contractStore contractHydrationStore, cust *customermodels.Customer) ([]contractmodels.Contract, error) {
	expectedContractIDs := contractIDsFromRaw(cust.ContractIDs)
	if len(expectedContractIDs) > 0 {
		if err := contractStore.EnsureHydrated(ctx, expectedContractIDs, contract.FreshOptions{}); err != nil && lo != nil {
			lo.Warn("contract hydration cascade failed; returning partial data", "customer_id", cust.CustomerID, "error", err)
		}
	}
	return contractStore.ListByCustomerID(cust.CustomerID)
}

type customerIDHydrator interface {
	GetFreshByCustomerID(context.Context, string, customer.FreshOptions) (*customermodels.Customer, error)
}

func hydrateCustomerSearchResults(ctx context.Context, lo interface{ Warn(string, ...any) }, hydrator customerIDHydrator, data any, opts customer.FreshOptions) ([]json.RawMessage, bool) {
	ids, ok := customerSearchIDs(data)
	if !ok {
		return []json.RawMessage{}, false
	}
	out := make([]json.RawMessage, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		row, err := hydrator.GetFreshByCustomerID(ctx, id, opts)
		if err != nil {
			if lo != nil {
				lo.Warn("customer hydration failed during search; skipping", "customer_id", id, "error", err)
			}
			continue
		}
		out = append(out, customerToAPIResponse(*row, nil))
	}
	return out, true
}

func customerSearchIDs(data any) ([]string, bool) {
	items := make([]map[string]any, 0)
	switch arr := data.(type) {
	case []map[string]any:
		items = arr
	case []any:
		for _, item := range arr {
			if m, ok := mapFromAny(item); ok {
				items = append(items, m)
			} else {
				return nil, false
			}
		}
	default:
		return nil, false
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, ciFirstString(item, "id", "customerId", "customer_id"))
	}
	return ids, true
}

func mapFromAny(v any) (map[string]any, bool) {
	if m, ok := v.(map[string]any); ok {
		return m, true
	}
	return nil, false
}

func resultDataEmpty(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed == "" || trimmed == "null" || trimmed == "{}" || trimmed == "[]"
}

func ciFirstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		v, ok := m[key]
		if !ok || v == nil {
			continue
		}
		switch x := v.(type) {
		case string:
			return strings.TrimSpace(x)
		case json.Number:
			return x.String()
		case float64:
			if x == float64(int64(x)) {
				return strconv.FormatInt(int64(x), 10)
			}
			return strconv.FormatFloat(x, 'f', -1, 64)
		case int:
			return strconv.Itoa(x)
		case int64:
			return strconv.FormatInt(x, 10)
		}
	}
	return ""
}

func contractIDsFromRaw(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var items []any
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		id := ciValueString(item)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func ciValueString(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case json.Number:
		return strings.TrimSpace(x.String())
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	default:
		return ""
	}
}

func customerToAPIResponse(c customermodels.Customer, contracts []contractmodels.Contract) json.RawMessage {
	return customerdata.CustomerToAPIResponse(c, contracts)
}

func contractToAPIResponse(c contractmodels.Contract) json.RawMessage {
	return customerdata.ContractToAPIResponse(c)
}

func persistManualCustomerLink(ctx context.Context, app *App, conv *cmodels.Conversation, contractRow contractmodels.Contract, operatorID int, verificationLevel int) error {
	tx, err := app.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		UPDATE conversations
		SET customer_id = $2,
		    contract_id = $3,
		    verification_level = $4,
		    verification_message = NULL,
		    updated_at = NOW()
		WHERE id = $1;
	`, conv.ID, contractRow.CustomerID, contractRow.ContractID, verificationLevel); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE users
		SET linked_customer_id = $2,
		    linked_customer_id_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1;
	`, conv.ContactID, contractRow.CustomerID); err != nil {
		return err
	}

	payload, err := manualLinkPayload(conv, contractRow, verificationLevel)
	if err != nil {
		return err
	}
	if err := insertManualAIOperation(ctx, tx, int64(conv.ID), operatorID, payload); err != nil {
		return err
	}
	return tx.Commit()
}

func persistManualCustomerRemove(ctx context.Context, app *App, conv *cmodels.Conversation, operatorID int) error {
	tx, err := app.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		UPDATE conversations
		SET customer_id = NULL,
		    contract_id = NULL,
		    verification_level = $2,
		    verification_message = NULL,
		    updated_at = NOW()
		WHERE id = $1;
	`, conv.ID, int(verificationmodels.VerificationLevelUnverified)); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE users
		SET linked_customer_id = NULL,
		    linked_customer_id_at = NULL,
		    updated_at = NOW()
		WHERE id = $1;
	`, conv.ContactID); err != nil {
		return err
	}

	payload, err := manualRemovePayload(conv)
	if err != nil {
		return err
	}
	if err := insertManualAIOperation(ctx, tx, int64(conv.ID), operatorID, payload); err != nil {
		return err
	}
	return tx.Commit()
}

func insertManualAIOperation(ctx context.Context, tx *sqlx.Tx, conversationID int64, operatorID int, payload json.RawMessage) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO ai_operations (
		    operation_type, conversation_id, triggered_by, triggered_by_user_id,
		    trace_id, status, error_message, payload
		)
		VALUES ($1, $2, $3, $4, $5, $6, NULL, $7);
	`,
		aiopmodels.TypeCustomerIdentification,
		conversationID,
		aiopmodels.TriggeredByManual,
		operatorID,
		uuid.NewString(),
		aiopmodels.StatusSuccess,
		[]byte(payload),
	)
	return err
}

func manualLinkPayload(conv *cmodels.Conversation, contractRow contractmodels.Contract, verificationLevel int) (json.RawMessage, error) {
	return json.Marshal(map[string]any{
		"method":  "manual",
		"channel": conv.InboxChannel,
		"action":  "manual_assign",
		"swap": map[string]any{
			"previous_contract_id":        nullStringValue(conv.ContractID),
			"previous_customer_id":        nullStringValue(conv.CustomerID),
			"previous_verification_level": conv.VerificationLevel,
			"new_contract_id":             contractRow.ContractID,
			"new_customer_id":             contractRow.CustomerID,
			"reason":                      nil,
		},
		"verification": map[string]any{
			"level":   verificationLevel,
			"source":  "manual",
			"message": nil,
		},
	})
}

func manualRemovePayload(conv *cmodels.Conversation) (json.RawMessage, error) {
	return json.Marshal(map[string]any{
		"method":  "manual",
		"channel": conv.InboxChannel,
		"action":  "manual_remove",
		"remove": map[string]any{
			"previous_contract_id":        nullStringValue(conv.ContractID),
			"previous_customer_id":        nullStringValue(conv.CustomerID),
			"previous_verification_level": conv.VerificationLevel,
		},
		"verification": map[string]any{
			"level":   int(verificationmodels.VerificationLevelUnverified),
			"source":  "manual",
			"message": nil,
		},
	})
}

func nullStringValue(n null.String) any {
	if !n.Valid {
		return nil
	}
	return n.String
}

func sendCustomerContractError(r *fastglue.Request, app *App, err error) error {
	switch {
	case errors.Is(err, customer.ErrCustomerNotFound):
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "customer not found", nil, envelope.NotFoundError)
	case errors.Is(err, contract.ErrContractNotFound):
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "contract not found", nil, envelope.NotFoundError)
	case errors.Is(err, sql.ErrNoRows):
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "not found", nil, envelope.NotFoundError)
	default:
		app.lo.Error("customer/contract handler failed", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}
}

func nonEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
