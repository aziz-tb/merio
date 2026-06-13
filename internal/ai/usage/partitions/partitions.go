// Package partitions keeps a partitioned table's monthly partition runway
// extended. Postgres rejects an INSERT whose created_at falls outside any
// existing partition, so the Manager pre-creates the next N months at boot
// and on a daily timer. It is the fallback for pg_partman, which the
// postgres:17-alpine image doesn't bundle. No retention/drop in v1.
package partitions

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/zerodha/logf"
)

// Manager owns the partition runway for a single partitioned table.
type Manager struct {
	db      *sqlx.DB
	table   string // e.g. "ai_usage"
	premake int    // number of future months to keep pre-created (inclusive of current)
	lo      *logf.Logger
}

// Opts configures Manager.
type Opts struct {
	DB *sqlx.DB
	// Table is the unqualified table name; partitions are named "<table>_YYYY_MM".
	Table string
	// Premake is the number of partitions to keep ahead of "now", counting the
	// current month (Premake=4 means current + next 3).
	Premake int
	Lo      *logf.Logger
}

// New builds a Manager. Premake defaults to 4 when zero is passed
// (current month + 3 ahead).
func New(opt Opts) *Manager {
	if opt.Premake <= 0 {
		opt.Premake = 4
	}
	return &Manager{
		db:      opt.DB,
		table:   opt.Table,
		premake: opt.Premake,
		lo:      opt.Lo,
	}
}

// EnsureRunway creates the current month's partition plus the next
// (Premake - 1) months if absent. Idempotent and safe to call concurrently
// from multiple replicas: CREATE TABLE IF NOT EXISTS resolves atomically.
func (m *Manager) EnsureRunway(ctx context.Context) error {
	now := time.Now().UTC()
	for i := 0; i < m.premake; i++ {
		start := time.Date(now.Year(), now.Month()+time.Month(i), 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 1, 0)
		partition := fmt.Sprintf("%s_%04d_%02d", m.table, start.Year(), start.Month())
		stmt := fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s PARTITION OF %s
			    FOR VALUES FROM ('%s') TO ('%s');
		`, partition, m.table, start.Format("2006-01-02"), end.Format("2006-01-02"))
		if _, err := m.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("partitions: create %s: %w", partition, err)
		}
	}
	return nil
}

// RunDaily blocks and calls EnsureRunway at boot and once a day until ctx is
// cancelled. EnsureRunway errors are logged, not propagated.
func (m *Manager) RunDaily(ctx context.Context) {
	if err := m.EnsureRunway(ctx); err != nil {
		m.lo.Error("partitions.ensure_runway", "table", m.table, "error", err)
	}
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := m.EnsureRunway(ctx); err != nil {
				m.lo.Error("partitions.ensure_runway", "table", m.table, "error", err)
			}
		}
	}
}
