// Package tag handles the management of tags.
package tag

import (
	"database/sql"
	"embed"
	"errors"

	"github.com/abhinavxd/libredesk/internal/dbutil"
	"github.com/abhinavxd/libredesk/internal/envelope"
	"github.com/abhinavxd/libredesk/internal/tag/models"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/go-i18n"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs embed.FS
)

type Manager struct {
	q    queries
	lo   *logf.Logger
	i18n *i18n.I18n
}

// Opts contains options for initializing the Manager.
type Opts struct {
	DB   *sqlx.DB
	Lo   *logf.Logger
	I18n *i18n.I18n
}

// queries contains prepared SQL queries.
type queries struct {
	GetAllTags                  *sqlx.Stmt `query:"get-all"`
	GetPublicTags               *sqlx.Stmt `query:"get-public"`
	GetEnabledForClassification *sqlx.Stmt `query:"get-enabled-for-classification"`
	GetTag                      *sqlx.Stmt `query:"get"`
	GetTagByID                  *sqlx.Stmt `query:"get-by-id"`
	GetTagsByParentID           *sqlx.Stmt `query:"get-by-parent-id"`
	InsertTag                   *sqlx.Stmt `query:"insert"`
	DeleteTag                   *sqlx.Stmt `query:"delete"`
	UpdateTag                   *sqlx.Stmt `query:"update"`
	BulkImport                  *sqlx.Stmt `query:"bulk-import"`
}

// New creates and returns a new instance of the Manager.
func New(opts Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, err
	}
	return &Manager{q: q, lo: opts.Lo, i18n: opts.I18n}, nil
}

// GetAll retrieves all tags, optionally including private tags.
func (t *Manager) GetAll(includePrivate bool) ([]models.Tag, error) {
	var tags = make([]models.Tag, 0)
	stmt := t.q.GetPublicTags
	if includePrivate {
		stmt = t.q.GetAllTags
	}
	if err := stmt.Select(&tags); err != nil {
		t.lo.Error("error fetching tags", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, t.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return tags, nil
}

// GetEnabledForClassification returns tags with non-manual detection enabled.
func (t *Manager) GetEnabledForClassification() ([]models.Tag, error) {
	var tags = make([]models.Tag, 0)
	if err := t.q.GetEnabledForClassification.Select(&tags); err != nil {
		t.lo.Error("error fetching classification tags", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, t.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return tags, nil
}

// Get retrieves a tag by ID.
func (t *Manager) Get(id int) (models.Tag, error) {
	var tag models.Tag
	if err := t.q.GetTag.Get(&tag, id); err != nil {
		t.lo.Error("error fetching tag", "tag_id", id, "error", err)
		return tag, envelope.NewError(envelope.GeneralError, t.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return tag, nil
}

// Create creates a new tag.
func (t *Manager) Create(tag models.Tag, userID int) (models.Tag, error) {
	tag.SetDefaults()
	if err := t.validateParent(0, tag.ParentID); err != nil {
		return models.Tag{}, err
	}

	var created models.Tag
	if err := t.q.InsertTag.Get(&created,
		tag.Name,
		nullOrInt(tag.ParentID),
		tag.Type,
		tag.Color,
		tag.Visibility,
		tag.Description,
		tag.Detection,
		nullOrInt(tag.SLAPolicyID),
		nullOrInt(tag.PriorityID),
		nullOrPositiveInt(userID),
	); err != nil {
		if dbutil.IsUniqueViolationError(err) {
			return created, envelope.NewError(envelope.ConflictError, t.i18n.T("errors.alreadyExistsTag"), nil)
		}
		t.lo.Error("error inserting tag", "error", err)
		return created, envelope.NewError(envelope.GeneralError, t.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return created, nil
}

// Delete deletes a tag by ID.
func (t *Manager) Delete(id int) error {
	if _, err := t.q.DeleteTag.Exec(id); err != nil {
		t.lo.Error("error deleting tag", "error", err)
		return envelope.NewError(envelope.GeneralError, t.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// Update updates a tag by ID.
func (t *Manager) Update(id int, tag models.Tag, userID int) (models.Tag, error) {
	tag.SetDefaults()
	if err := t.validateParent(id, tag.ParentID); err != nil {
		return models.Tag{}, err
	}
	if tag.ParentID.Valid {
		children, err := t.getChildren(id)
		if err != nil {
			t.lo.Error("error fetching child tags for validation", "tag_id", id, "error", err)
			return models.Tag{}, envelope.NewError(envelope.GeneralError, t.i18n.T("globals.messages.somethingWentWrong"), nil)
		}
		if len(children) > 0 {
			return models.Tag{}, envelope.NewError(envelope.InputError, t.i18n.T("admin.tags.errors.hasChildren"), nil)
		}
	}

	var updated models.Tag
	if err := t.q.UpdateTag.Get(&updated,
		id,
		tag.Name,
		nullOrInt(tag.ParentID),
		tag.Type,
		tag.Color,
		tag.Visibility,
		tag.Description,
		tag.Detection,
		nullOrInt(tag.SLAPolicyID),
		nullOrInt(tag.PriorityID),
		nullOrPositiveInt(userID),
	); err != nil {
		if dbutil.IsUniqueViolationError(err) {
			return updated, envelope.NewError(envelope.ConflictError, t.i18n.T("errors.alreadyExistsTag"), nil)
		}
		t.lo.Error("error updating tag", "error", err)
		return updated, envelope.NewError(envelope.GeneralError, t.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return updated, nil
}

// BulkImport inserts tag names with default metadata.
func (t *Manager) BulkImport(names []string) error {
	for _, name := range names {
		if _, err := t.q.BulkImport.Exec(name); err != nil {
			t.lo.Error("error importing tag", "name", name, "error", err)
			return envelope.NewError(envelope.GeneralError, t.i18n.T("globals.messages.somethingWentWrong"), nil)
		}
	}
	return nil
}

func nullOrInt(v null.Int) any {
	if !v.Valid {
		return nil
	}
	return v.Int
}

func nullOrPositiveInt(v int) any {
	if v <= 0 {
		return nil
	}
	return v
}

// validateParent rejects parent assignments that would create cycles or
// violate the one-level nesting rule.
func (t *Manager) validateParent(tagID int, parentID null.Int) error {
	if !parentID.Valid {
		return nil
	}
	if tagID != 0 && parentID.Int == tagID {
		return envelope.NewError(envelope.InputError, t.i18n.T("admin.tags.errors.selfParent"), nil)
	}

	var parent models.Tag
	if err := t.q.GetTagByID.Get(&parent, parentID.Int); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return envelope.NewError(envelope.InputError, t.i18n.T("admin.tags.errors.parentNotFound"), nil)
		}
		t.lo.Error("error fetching parent tag for validation", "tag_id", parentID.Int, "error", err)
		return envelope.NewError(envelope.GeneralError, t.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	if parent.ParentID.Valid {
		return envelope.NewError(envelope.InputError, t.i18n.T("admin.tags.errors.parentHasParent"), nil)
	}
	return nil
}

func (t *Manager) getChildren(parentID int) ([]models.Tag, error) {
	var children []models.Tag
	if err := t.q.GetTagsByParentID.Select(&children, parentID); err != nil {
		return nil, err
	}
	return children, nil
}
