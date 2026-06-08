package aitools

import (
	"context"

	toolmodels "github.com/aziz-tb/merio/internal/ai/tools/models"
	"github.com/aziz-tb/merio/internal/function"
	fnmodels "github.com/aziz-tb/merio/internal/function/models"
	"github.com/jmoiron/sqlx"
)

// Service owns cross-table writes that must commit atomically.
type Service struct {
	db        *sqlx.DB
	tools     *Manager
	functions *function.Manager
}

// NewService returns a thin transactional layer over ai_tools + functions.
func NewService(db *sqlx.DB, tools *Manager, functions *function.Manager) *Service {
	return &Service{db: db, tools: tools, functions: functions}
}

// CreateToolWithFunction inserts the functions row and linked ai_tools row
// in one transaction.
func (s *Service) CreateToolWithFunction(ctx context.Context, tool toolmodels.Tool, fn fnmodels.Function) (toolmodels.Tool, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return toolmodels.Tool{}, err
	}
	defer tx.Rollback()

	fnID, err := s.functions.CreateTx(ctx, tx, &fn)
	if err != nil {
		return toolmodels.Tool{}, err
	}
	tool.FunctionID = fnID
	out, err := s.tools.CreateTx(ctx, tx, tool)
	if err != nil {
		return toolmodels.Tool{}, err
	}
	if err := tx.Commit(); err != nil {
		return toolmodels.Tool{}, err
	}
	return out, nil
}

// UpdateToolWithFunction updates the linked functions row and ai_tools row
// in one transaction.
func (s *Service) UpdateToolWithFunction(ctx context.Context, toolID int, functionName string, tool toolmodels.Tool, fn fnmodels.Function) (toolmodels.Tool, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return toolmodels.Tool{}, err
	}
	defer tx.Rollback()

	if err := s.functions.UpdateTx(ctx, tx, functionName, &fn); err != nil {
		return toolmodels.Tool{}, err
	}
	out, err := s.tools.UpdateTx(ctx, tx, toolID, tool)
	if err != nil {
		return toolmodels.Tool{}, err
	}
	if err := tx.Commit(); err != nil {
		return toolmodels.Tool{}, err
	}
	return out, nil
}

// removeAgentToolRef strips a deleted custom tool from every agent's tools
// JSONB. Agent tool refs ({"id":N,"kind":"custom"}) have no FK, so deleting
// an ai_tools row would otherwise leave dangling refs that make those agents
// fail validation on their next update. $1 is the tool id (used as both int,
// to match the stored JSON number, and text, to compare the extracted value).
const removeAgentToolRef = `
UPDATE ai_agents
SET tools = COALESCE((
    SELECT jsonb_agg(e)
    FROM jsonb_array_elements(tools) e
    WHERE NOT (e->>'kind' = 'custom' AND e->>'id' = $1::text)
), '[]'::jsonb)
WHERE tools @> jsonb_build_array(jsonb_build_object('id', $1::int, 'kind', 'custom'))`

// DeleteToolWithFunction deletes the ai_tools row and linked functions row,
// and removes the tool from any agent that referenced it — all in one
// transaction. ai_tools must be deleted first because the FK from
// ai_tools.function_id to functions.id is ON DELETE RESTRICT. Returns the
// number of agents whose tool list was cleaned up.
func (s *Service) DeleteToolWithFunction(ctx context.Context, toolID int, functionName string) (int, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if err := s.tools.DeleteTx(ctx, tx, toolID); err != nil {
		return 0, err
	}
	if err := s.functions.DeleteTx(ctx, tx, functionName); err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, removeAgentToolRef, toolID)
	if err != nil {
		return 0, err
	}
	agentsCleaned, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(agentsCleaned), nil
}
