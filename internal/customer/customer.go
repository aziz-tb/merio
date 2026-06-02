// Package customer manages the customers cache table — ERP-resolved
// customer records persisted by the identification workflow and read back
// by the sidebar, verification engine, and agent-run hydration. The table
// is a cache, not a system of record: last_synced_at drives the TTL and
// callers refetch from the ERP when a row goes stale.
package customer

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"time"

	customererp "github.com/abhinavxd/libredesk/internal/customer/erp"
	"github.com/abhinavxd/libredesk/internal/customer/models"
	"github.com/abhinavxd/libredesk/internal/dbutil"
	"github.com/abhinavxd/libredesk/internal/envelope"
	execmodels "github.com/abhinavxd/libredesk/internal/exec/runner/models"
	"github.com/abhinavxd/libredesk/internal/function"
	"github.com/abhinavxd/libredesk/internal/setting"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/types"
	"github.com/knadh/go-i18n"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs embed.FS

	// ErrCustomerNotFound is returned when no customer matches the lookup.
	ErrCustomerNotFound = errors.New("customer not found")
)

// Manager handles CRUD against the customers cache table.
type Manager struct {
	q        queries
	lo       *logf.Logger
	i18n     *i18n.I18n
	settings settingsStore
	function functionRunner
}

// Opts contains options for initializing the Manager.
type Opts struct {
	DB       *sqlx.DB
	Lo       *logf.Logger
	I18n     *i18n.I18n
	Settings settingsStore
	Function functionRunner
}

type settingsStore interface {
	Get(key string) (types.JSONText, error)
}

type functionRunner interface {
	Run(ctx context.Context, req function.RunRequest) (*execmodels.Result, error)
}

// queries contains prepared SQL queries.
type queries struct {
	GetByID            *sqlx.Stmt `query:"get-customer-by-id"`
	GetByCustomerID    *sqlx.Stmt `query:"get-customer-by-customer-id"`
	Insert             *sqlx.Stmt `query:"insert-customer"`
	Upsert             *sqlx.Stmt `query:"upsert-customer"`
	UpdateLastSyncedAt *sqlx.Stmt `query:"update-customer-last-synced-at"`
	InvalidateCache    *sqlx.Stmt `query:"invalidate-customer-cache"`
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
	}, nil
}

// Get retrieves a customer by internal row id. External callers should use
// GetByCustomerID; BIGSERIAL ids are implementation details only.
func (m *Manager) Get(id int64) (models.Customer, error) {
	var c models.Customer
	if err := m.q.GetByID.Get(&c, id); err != nil {
		if err == sql.ErrNoRows {
			return c, ErrCustomerNotFound
		}
		m.lo.Error("error fetching customer by id", "id", id, "error", err)
		return c, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return c, nil
}

// GetByCustomerID retrieves a customer by its ERP-side customer id.
func (m *Manager) GetByCustomerID(customerID string) (models.Customer, error) {
	var c models.Customer
	if err := m.q.GetByCustomerID.Get(&c, customerID); err != nil {
		if err == sql.ErrNoRows {
			return c, ErrCustomerNotFound
		}
		m.lo.Error("error fetching customer by customer id", "customer_id", customerID, "error", err)
		return c, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return c, nil
}

// GetFresh returns a cached customer when it is still within the configured
// ERP TTL, otherwise refreshes it through the search_customer_by_id function
// and upserts the returned ERP shape. Empty ERP responses keep the stale row
// rather than blanking a known customer.
func (m *Manager) GetFresh(ctx context.Context, id int64) (*models.Customer, error) {
	c, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	return m.refreshIfStale(ctx, c, FreshOptions{})
}

// FreshOptions lets workflow activities pass audit context through to the
// sandbox runner. Plain callers use GetFresh and receive the same cache
// behavior without a conversation-scoped trace.
type FreshOptions struct {
	InvokerUserID  int
	ConversationID int
	TraceID        string
}

// GetFreshWithOptions is the workflow-aware variant of GetFresh.
func (m *Manager) GetFreshWithOptions(ctx context.Context, id int64, opts FreshOptions) (*models.Customer, error) {
	c, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	return m.refreshIfStale(ctx, c, opts)
}

// GetFreshByCustomerID loads by ERP customer id. If the row is missing, it
// performs the by-id ERP lookup and inserts the returned cache record.
func (m *Manager) GetFreshByCustomerID(ctx context.Context, customerID string, opts FreshOptions) (*models.Customer, error) {
	c, err := m.GetByCustomerID(customerID)
	if err == nil {
		refreshed, err := m.refreshIfStale(ctx, c, opts)
		if err != nil {
			if m.lo != nil {
				m.lo.Warn("customer refresh failed; keeping cached row", "customer_id", c.CustomerID, "error", err)
			}
			return &c, nil
		}
		return refreshed, nil
	}
	if !errors.Is(err, ErrCustomerNotFound) {
		return nil, err
	}
	refreshed, ok, err := m.refreshFromERP(ctx, customerID, opts)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrCustomerNotFound
	}
	return refreshed, nil
}

func (m *Manager) refreshIfStale(ctx context.Context, cached models.Customer, opts FreshOptions) (*models.Customer, error) {
	ttl := setting.DurationMinutes(m.settings, "customer_identification.erp_cache_ttl_minutes", time.Hour)
	// A zero TTL disables cache hits and intentionally falls through to ERP.
	if ttl > 0 && !cached.LastSyncedAt.IsZero() && cached.LastSyncedAt.After(time.Now().Add(-ttl)) {
		return &cached, nil
	}
	refreshed, ok, err := m.refreshFromERP(ctx, cached.CustomerID, opts)
	if err != nil {
		return nil, err
	}
	if !ok {
		if m.lo != nil {
			m.lo.Warn("customer refresh returned empty result; keeping cached row", "customer_id", cached.CustomerID)
		}
		return &cached, nil
	}
	return refreshed, nil
}

func (m *Manager) refreshFromERP(ctx context.Context, customerID string, opts FreshOptions) (*models.Customer, bool, error) {
	if m.function == nil {
		return nil, false, errors.New("customer: function runner required for stale cache refresh")
	}
	res, err := m.function.Run(ctx, function.RunRequest{
		Name:           "search_customer_by_id",
		Input:          map[string]any{"customerId": customerID},
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
	c, ok, err := customererp.FromExecutorData(res.Data, customerID)
	if err != nil || !ok {
		return nil, ok, err
	}
	out, err := m.Upsert(c)
	if err != nil {
		return nil, false, err
	}
	return &out, true, nil
}

func (m *Manager) Upsert(c models.Customer) (models.Customer, error) {
	defaults(&c)
	var out models.Customer
	if err := m.q.Upsert.Get(&out,
		c.CustomerID, c.Business, c.Firstname, c.Lastname, c.Company, c.Email, c.Phone,
		c.Zip, c.DeliveryAddress, c.BillingAddress, c.ContractIDs, c.ExtraData,
	); err != nil {
		m.lo.Error("error upserting customer", "customer_id", c.CustomerID, "error", err)
		return out, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// UpdateLastSyncedAt bumps the TTL clock after a fresh ERP fetch.
func (m *Manager) UpdateLastSyncedAt(id int64) error {
	if _, err := m.q.UpdateLastSyncedAt.Exec(id); err != nil {
		m.lo.Error("error updating customer last_synced_at", "id", id, "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// InvalidateCache forces the next read to refetch by pushing last_synced_at
// into the far past. Used by the cache-invalidation hooks (conversation
// status change, AI agent write-tool execution).
func (m *Manager) InvalidateCache(customerID string) error {
	if _, err := m.q.InvalidateCache.Exec(customerID); err != nil {
		m.lo.Error("error invalidating customer cache", "customer_id", customerID, "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// defaults fills in JSON columns that must not be NULL when the caller
// left them empty.
func defaults(c *models.Customer) {
	if len(c.ContractIDs) == 0 {
		c.ContractIDs = json.RawMessage(`[]`)
	}
	if len(c.ExtraData) == 0 {
		c.ExtraData = json.RawMessage(`{}`)
	}
}
