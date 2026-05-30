package aitools

import (
	"context"

	toolmodels "github.com/abhinavxd/libredesk/internal/ai/tools/models"
	"github.com/abhinavxd/libredesk/internal/function"
	fnmodels "github.com/abhinavxd/libredesk/internal/function/models"
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

// DeleteToolWithFunction deletes the ai_tools row and linked functions row
// in one transaction. ai_tools must be deleted first because the FK from
// ai_tools.function_id to functions.id is ON DELETE RESTRICT.
func (s *Service) DeleteToolWithFunction(ctx context.Context, toolID int, functionName string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := s.tools.DeleteTx(ctx, tx, toolID); err != nil {
		return err
	}
	if err := s.functions.DeleteTx(ctx, tx, functionName); err != nil {
		return err
	}
	return tx.Commit()
}
