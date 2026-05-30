// Package contract manages the contracts cache table — ERP-resolved
// contract records belonging to a customer, persisted by the identification
// workflow and read back by the sidebar (multi-contract PillSwitcher), the
// verification engine, and agent-run hydration. Like customers, the table
// is a cache: last_synced_at drives the TTL and callers refetch from the
// ERP when a row goes stale.
package contract

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/abhinavxd/libredesk/internal/contract/models"
	customererp "github.com/abhinavxd/libredesk/internal/customer/erp"
	custmodels "github.com/abhinavxd/libredesk/internal/customer/models"
	"github.com/abhinavxd/libredesk/internal/customerdata"
	"github.com/abhinavxd/libredesk/internal/dbutil"
	"github.com/abhinavxd/libredesk/internal/envelope"
	execmodels "github.com/abhinavxd/libredesk/internal/exec/runner/models"
	"github.com/abhinavxd/libredesk/internal/function"
	"github.com/abhinavxd/libredesk/internal/setting"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/types"
	"github.com/knadh/go-i18n"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs embed.FS

	// ErrContractNotFound is returned when no contract matches the lookup.
	ErrContractNotFound = errors.New("contract not found")
)

var zone1ContractKeys = map[string]bool{
	"id": true, "contractId": true, "contract_id": true,
	"customerId": true, "customer_id": true,
	"business": true,
	"lastname": true, "lastName": true, "surname": true,
	"company": true, "companyName": true,
	"firstname": true, "firstName": true,
	"email": true, "mail": true,
	"phone": true, "phoneNumber": true,
	"zip": true, "postalCode": true,
	"deliveryAddress": true, "delivery_address": true, "address": true,
	"billingAddress": true, "billing_address": true,
	"agentPreview": true, "agent_preview": true,
	"erpUrls": true, "erp_urls": true, "erpURLs": true,
	"customer": true,
}

// Manager handles CRUD against the contracts cache table.
type Manager struct {
	q        queries
	lo       *logf.Logger
	i18n     *i18n.I18n
	settings settingsStore
	function functionRunner
	customer customerStore
}

// Opts contains options for initializing the Manager.
type Opts struct {
	DB       *sqlx.DB
	Lo       *logf.Logger
	I18n     *i18n.I18n
	Settings settingsStore
	Function functionRunner
	Customer customerStore
}

type settingsStore interface {
	Get(key string) (types.JSONText, error)
}

type functionRunner interface {
	Run(ctx context.Context, req function.RunRequest) (*execmodels.Result, error)
}

type customerStore interface {
	GetByCustomerID(customerID string) (custmodels.Customer, error)
	Upsert(c custmodels.Customer) (custmodels.Customer, error)
}

// queries contains prepared SQL queries.
type queries struct {
	GetByID            *sqlx.Stmt `query:"get-contract-by-id"`
	GetByContractID    *sqlx.Stmt `query:"get-contract-by-contract-id"`
	ListByCustomerID   *sqlx.Stmt `query:"list-contracts-by-customer-id"`
	Insert             *sqlx.Stmt `query:"insert-contract"`
	Upsert             *sqlx.Stmt `query:"upsert-contract"`
	UpdateLastSyncedAt *sqlx.Stmt `query:"update-contract-last-synced-at"`
	InvalidateCache    *sqlx.Stmt `query:"invalidate-contract-cache"`
}

// New creates and returns a new instance of the Manager.
func New(opts Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, err
	}
	return &Manager{
		q:        q,
		lo:       opts.Lo,
		i18n:     opts.I18n,
		settings: opts.Settings,
		function: opts.Function,
		customer: opts.Customer,
	}, nil
}

// Get retrieves a contract by internal id.
func (m *Manager) Get(id int64) (models.Contract, error) {
	var c models.Contract
	if err := m.q.GetByID.Get(&c, id); err != nil {
		if err == sql.ErrNoRows {
			return c, ErrContractNotFound
		}
		m.lo.Error("error fetching contract by id", "id", id, "error", err)
		return c, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return c, nil
}

// GetFresh returns a cached contract when it is still within the configured
// ERP TTL, otherwise refreshes it through the search_contract_by_id function
// and upserts the returned ERP shape. Empty ERP responses keep the stale row
// rather than blanking a known contract.
func (m *Manager) GetFresh(ctx context.Context, id int64) (*models.Contract, error) {
	c, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	return m.refreshIfStale(ctx, c, FreshOptions{})
}

type FreshOptions struct {
	InvokerUserID  int
	ConversationID int
	TraceID        string
}

func (m *Manager) GetFreshWithOptions(ctx context.Context, id int64, opts FreshOptions) (*models.Contract, error) {
	c, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	return m.refreshIfStale(ctx, c, opts)
}

// GetFreshByContractID loads by ERP contract id. If the row is missing, it
// performs the by-id ERP lookup and inserts the returned cache record.
func (m *Manager) GetFreshByContractID(ctx context.Context, contractID string, opts FreshOptions) (*models.Contract, error) {
	c, err := m.GetByContractID(contractID)
	if err == nil {
		return m.refreshIfStale(ctx, c, opts)
	}
	if !errors.Is(err, ErrContractNotFound) {
		return nil, err
	}
	refreshed, ok, err := m.refreshFromERP(ctx, contractID, "", opts)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrContractNotFound
	}
	return refreshed, nil
}

// EnsureHydrated makes sure every provided ERP contract id is present in the
// local cache and fresh enough for the configured TTL. Individual hydration
// failures are logged and skipped so callers can still render partial results.
func (m *Manager) EnsureHydrated(ctx context.Context, contractIDs []string, opts FreshOptions) error {
	for _, id := range contractIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, err := m.GetFreshByContractID(ctx, id, opts); err != nil {
			if m.lo != nil {
				m.lo.Warn("EnsureHydrated: contract fetch failed; skipping", "contract_id", id, "error", err)
			}
			continue
		}
	}
	return nil
}

// UpsertFromExecutorData maps one ERP executor result to a contract row and
// upserts it. It is shared by GetFresh and the operator free-text search
// endpoint so both surfaces interpret executor payloads identically.
func (m *Manager) UpsertFromExecutorData(ctx context.Context, raw json.RawMessage, fallbackContractID, fallbackCustomerID string, opts FreshOptions) (*models.Contract, bool, error) {
	c, ok, err := m.contractFromExecutorData(ctx, raw, fallbackContractID, fallbackCustomerID, opts)
	if err != nil || !ok {
		return nil, ok, err
	}
	out, err := m.Upsert(c)
	if err != nil {
		return nil, false, err
	}
	return &out, true, nil
}

func (m *Manager) refreshIfStale(ctx context.Context, cached models.Contract, opts FreshOptions) (*models.Contract, error) {
	ttl := setting.DurationMinutes(m.settings, "customer_identification.erp_cache_ttl_minutes", time.Hour)
	// A zero TTL disables cache hits and intentionally falls through to ERP.
	if ttl > 0 && !cached.LastSyncedAt.IsZero() && cached.LastSyncedAt.After(time.Now().Add(-ttl)) {
		return &cached, nil
	}
	refreshed, ok, err := m.refreshFromERP(ctx, cached.ContractID, cached.CustomerID, opts)
	if err != nil {
		return nil, err
	}
	if !ok {
		if m.lo != nil {
			m.lo.Warn("contract refresh returned empty result; keeping cached row", "contract_id", cached.ContractID)
		}
		return &cached, nil
	}
	return refreshed, nil
}

func (m *Manager) refreshFromERP(ctx context.Context, contractID, fallbackCustomerID string, opts FreshOptions) (*models.Contract, bool, error) {
	if m.function == nil {
		return nil, false, errors.New("contract: function runner required for stale cache refresh")
	}
	res, err := m.function.Run(ctx, function.RunRequest{
		Name:           "search_contract_by_id",
		Input:          map[string]any{"contractId": contractID},
		InvokerUserID:  opts.InvokerUserID,
		ConversationID: opts.ConversationID,
		TraceID:        opts.TraceID,
	})
	if err != nil {
		return nil, false, err
	}
	if res == nil || res.Kind != execmodels.KindSuccess || len(res.Data) == 0 || string(res.Data) == "null" {
		return nil, false, nil
	}
	return m.UpsertFromExecutorData(ctx, res.Data, contractID, fallbackCustomerID, opts)
}

// Contract executor mapping intentionally stays local to this package:
// it resolves/creates the linked customer and maps contract-only fields
// like zip, agent_preview, and erp_urls.
func (m *Manager) contractFromExecutorData(ctx context.Context, raw json.RawMessage, fallbackContractID, fallbackCustomerID string, opts FreshOptions) (models.Contract, bool, error) {
	raw, data, err := customerdata.UnwrapObject(raw, "contract")
	if err != nil {
		return models.Contract{}, false, fmt.Errorf("parse contract executor output: %w", err)
	}
	if len(data) == 0 {
		return models.Contract{}, false, nil
	}
	contractID := customerdata.FirstString(data, "contract_id", "contractId", "id")
	if contractID == "" {
		contractID = fallbackContractID
	}
	if contractID == "" {
		return models.Contract{}, false, fmt.Errorf("contract executor output missing contract id")
	}

	customerID, err := m.resolveCustomerID(ctx, data, fallbackCustomerID, opts)
	if err != nil {
		return models.Contract{}, false, err
	}
	if customerID == "" {
		return models.Contract{}, false, fmt.Errorf("contract executor output missing customer id")
	}

	business, found, err := customerdata.FirstBool(data, "business")
	if err != nil {
		return models.Contract{}, false, fmt.Errorf("business is required: %w", err)
	}
	if !found {
		return models.Contract{}, false, fmt.Errorf("business is required")
	}
	company := customerdata.FirstString(data, "company", "companyName")
	firstname := customerdata.FirstString(data, "firstname", "firstName")
	lastname := customerdata.FirstString(data, "lastname", "lastName", "surname")
	if business {
		if strings.TrimSpace(company) == "" {
			return models.Contract{}, false, fmt.Errorf("business contract requires company")
		}
	} else if strings.TrimSpace(lastname) == "" {
		return models.Contract{}, false, fmt.Errorf("private contract requires lastname")
	}

	agentPreview := customerdata.FirstRaw(data, "agent_preview", "agentPreview")
	if len(agentPreview) == 0 {
		agentPreview = json.RawMessage(`[]`)
	}
	erpURLs := customerdata.FirstRaw(data, "erp_urls", "erpUrls", "erpURLs")
	if len(erpURLs) == 0 {
		erpURLs = json.RawMessage(`[]`)
	}
	extra := customerdata.BuildRemainder(raw, zone1ContractKeys)

	return models.Contract{
		ContractID:      contractID,
		CustomerID:      customerID,
		Business:        business,
		Firstname:       nullString(firstname),
		Lastname:        nullString(lastname),
		Company:         nullString(company),
		Email:           nullString(customerdata.FirstString(data, "email", "mail")),
		Phone:           nullString(customerdata.FirstString(data, "phone", "phoneNumber")),
		Zip:             nullString(customerdata.FirstString(data, "zip", "postalCode")),
		DeliveryAddress: nullString(customerdata.FirstString(data, "delivery_address", "deliveryAddress", "address")),
		BillingAddress:  nullString(customerdata.FirstString(data, "billing_address", "billingAddress")),
		AgentPreview:    agentPreview,
		ErpURLs:         erpURLs,
		ExtraData:       extra,
	}, true, nil
}

func (m *Manager) resolveCustomerID(ctx context.Context, data map[string]json.RawMessage, fallbackCustomerID string, opts FreshOptions) (string, error) {
	if nested := customerdata.FirstRaw(data, "customer"); len(nested) > 0 && m.customer != nil {
		c, ok, err := customererp.FromExecutorData(nested, customerdata.FirstString(data, "customerId", "customer_id"))
		if err != nil {
			return "", err
		}
		if ok {
			upserted, err := m.customer.Upsert(c)
			if err != nil {
				return "", err
			}
			return upserted.CustomerID, nil
		}
	}
	customerID := customerdata.FirstString(data, "customer_id", "customerId")
	if customerID != "" {
		if m.customer == nil {
			return customerID, nil
		}
		c, err := m.customer.GetByCustomerID(customerID)
		if err == nil {
			return c.CustomerID, nil
		}
		if m.function == nil {
			return "", err
		}
		res, runErr := m.function.Run(ctx, function.RunRequest{
			Name:           "search_customer_by_id",
			Input:          map[string]any{"customerId": customerID},
			InvokerUserID:  opts.InvokerUserID,
			ConversationID: opts.ConversationID,
			TraceID:        opts.TraceID,
		})
		if runErr != nil {
			return "", runErr
		}
		if res != nil && res.Kind == execmodels.KindSuccess && len(res.Data) > 0 && string(res.Data) != "null" {
			c, ok, err := customererp.FromExecutorData(res.Data, customerID)
			if err != nil {
				return "", err
			}
			if ok {
				upserted, err := m.customer.Upsert(c)
				if err != nil {
					return "", err
				}
				return upserted.CustomerID, nil
			}
		}
	}
	return fallbackCustomerID, nil
}

func nullString(s string) null.String {
	if strings.TrimSpace(s) == "" {
		return null.String{}
	}
	return null.StringFrom(strings.TrimSpace(s))
}

// GetByContractID retrieves a contract by its ERP-side contract id.
func (m *Manager) GetByContractID(contractID string) (models.Contract, error) {
	var c models.Contract
	if err := m.q.GetByContractID.Get(&c, contractID); err != nil {
		if err == sql.ErrNoRows {
			return c, ErrContractNotFound
		}
		m.lo.Error("error fetching contract by contract id", "contract_id", contractID, "error", err)
		return c, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return c, nil
}

// ListByCustomerID returns every contract belonging to a customer, used by
// the sidebar to populate the multi-contract selector.
func (m *Manager) ListByCustomerID(customerID string) ([]models.Contract, error) {
	out := make([]models.Contract, 0)
	if err := m.q.ListByCustomerID.Select(&out, customerID); err != nil {
		m.lo.Error("error listing contracts by customer id", "customer_id", customerID, "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// Create inserts a new contract row and returns it.
func (m *Manager) Create(c models.Contract) (models.Contract, error) {
	defaults(&c)
	var out models.Contract
	if err := m.q.Insert.Get(&out,
		c.ContractID, c.CustomerID, c.Business, c.Firstname, c.Lastname, c.Company, c.Email, c.Phone,
		c.Zip, c.DeliveryAddress, c.BillingAddress, c.AgentPreview, c.ErpURLs, c.ExtraData,
	); err != nil {
		m.lo.Error("error creating contract", "contract_id", c.ContractID, "error", err)
		return out, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// Upsert inserts a freshly-loaded ERP contract or refreshes the cached row
// keyed by contract_id, bumping last_synced_at. Used by the workflow's
// SearchContractByIdActivity after an ERP fetch.
func (m *Manager) Upsert(c models.Contract) (models.Contract, error) {
	defaults(&c)
	var out models.Contract
	if err := m.q.Upsert.Get(&out,
		c.ContractID, c.CustomerID, c.Business, c.Firstname, c.Lastname, c.Company, c.Email, c.Phone,
		c.Zip, c.DeliveryAddress, c.BillingAddress, c.AgentPreview, c.ErpURLs, c.ExtraData,
	); err != nil {
		m.lo.Error("error upserting contract", "contract_id", c.ContractID, "error", err)
		return out, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// UpdateLastSyncedAt bumps the TTL clock after a fresh ERP fetch.
func (m *Manager) UpdateLastSyncedAt(id int64) error {
	if _, err := m.q.UpdateLastSyncedAt.Exec(id); err != nil {
		m.lo.Error("error updating contract last_synced_at", "id", id, "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// InvalidateCache forces the next read to refetch by pushing last_synced_at
// into the far past. Used by the cache-invalidation hooks (conversation
// status change, AI agent write-tool execution).
func (m *Manager) InvalidateCache(contractID string) error {
	if _, err := m.q.InvalidateCache.Exec(contractID); err != nil {
		m.lo.Error("error invalidating contract cache", "contract_id", contractID, "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// defaults fills in JSON columns that must not be NULL when the caller
// left them empty.
func defaults(c *models.Contract) {
	if len(c.AgentPreview) == 0 {
		c.AgentPreview = json.RawMessage(`[]`)
	}
	if len(c.ErpURLs) == 0 {
		c.ErpURLs = json.RawMessage(`[]`)
	}
	if len(c.ExtraData) == 0 {
		c.ExtraData = json.RawMessage(`{}`)
	}
}
