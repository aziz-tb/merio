// Package partitions keeps the ai_usage table's monthly partition
// runway extended. Postgres rejects an INSERT whose created_at falls
// outside any existing partition — so if the application reaches the
// 1st of a month and no partition exists for it, every ai_usage write
// fails. The Manager runs at boot and on a daily timer to guarantee
// the next N months are always pre-created.
//
// This is the fallback for pg_partman, which the postgres:17-alpine
// image doesn't bundle. The locked design choice (see F4 prompt) is to
// install pg_partman where the deployment environment allows it; this
// package is the documented downgrade when it doesn't.
//
// Tradeoffs versus pg_partman:
//   - No declarative retention (drop-old-partitions). When we want
//     retention, add a Drop(before time.Time) method here. Not in v1.
//   - Concurrent partition creation across replicas: we lean on
//     CREATE TABLE IF NOT EXISTS PARTITION OF being a Postgres
//     idempotent operation — two replicas racing produce one winner;
//     the loser's IF NOT EXISTS short-circuits cleanly.
//   - No per-child custom index strategies. PG14+'s partition
//     inheritance covers our use case (every parent index propagates).
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
	// Table is the unqualified table name. Partitions are named
	// "<table>_YYYY_MM" so the table itself must not collide.
	Table string
	// Premake is the number of partitions to keep alive ahead of "now",
	// counting the current month. Premake=4 means current + next 3.
	// The migration creates 4 at install time; the Manager just keeps
	// that runway extended on a wall-clock schedule.
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
// (Premake - 1) months if any are absent. Idempotent — running it a
// second time within the same month is a no-op. Safe to call from
// multiple replicas simultaneously: PG resolves CREATE TABLE IF NOT
// EXISTS atomically and the losers short-circuit without raising.
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

// RunDaily blocks the calling goroutine and ticks once a day, calling
// EnsureRunway each tick. Ticks at the same wall-clock minute it was
// started; jitter would be nice but a missed tick is acceptable
// because the v1 premake window (4 months) is two orders of magnitude
// larger than a missed-tick gap.
//
// Returns when ctx is cancelled. Errors from EnsureRunway are logged
// but never propagated — partition creation failing is operationally
// alarming but not fatal until we actually cross into a month with no
// partition.
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
