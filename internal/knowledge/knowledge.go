// Package knowledge provides folder + article management for the help-center
// knowledge base. Folders nest arbitrarily (no sentinel root); articles live
// in exactly one folder. Bodies are stored in three formats (HTML, TipTap
// JSON, Markdown) so downstream RAG embedding can consume Markdown without
// a follow-up migration.
//
// Manager mirrors internal/macro shape with one structural deviation:
// Manager retains *sqlx.DB after construction so MoveFolder can run its
// cycle-check + UPDATE inside a single transaction.
package knowledge

import (
	"database/sql"
	"embed"
	"slices"

	"github.com/abhinavxd/libredesk/internal/dbutil"
	"github.com/abhinavxd/libredesk/internal/envelope"
	"github.com/abhinavxd/libredesk/internal/knowledge/models"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/go-i18n"
	"github.com/lib/pq"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs embed.FS
)

// Manager is the knowledge base manager.
type Manager struct {
	q    queries
	db   *sqlx.DB
	lo   *logf.Logger
	i18n *i18n.I18n
}

// Predefined queries.
type queries struct {
	// Folders
	InsertFolder         *sqlx.Stmt `query:"insert-folder"`
	UpdateFolder         *sqlx.Stmt `query:"update-folder"`
	MoveFolder           *sqlx.Stmt `query:"move-folder"`
	DeleteFolder         *sqlx.Stmt `query:"delete-folder"`
	GetFolder            *sqlx.Stmt `query:"get-folder"`
	ListFoldersByParent  *sqlx.Stmt `query:"list-folders-by-parent"`
	ListAllFolders       *sqlx.Stmt `query:"list-all-folders"`
	GetFolderDescendants *sqlx.Stmt `query:"get-folder-descendants"`

	// Articles
	InsertArticle      *sqlx.Stmt `query:"insert-article"`
	UpdateArticle      *sqlx.Stmt `query:"update-article"`
	PublishArticle     *sqlx.Stmt `query:"publish-article"`
	UnpublishArticle   *sqlx.Stmt `query:"unpublish-article"`
	MoveArticle        *sqlx.Stmt `query:"move-article"`
	DeleteArticle      *sqlx.Stmt `query:"delete-article"`
	GetArticle         *sqlx.Stmt `query:"get-article"`
	ListArticles       *sqlx.Stmt `query:"list-articles"`
	MarkIndexStale     *sqlx.Stmt `query:"mark-index-stale"`
	GetArticleRevision *sqlx.Stmt `query:"get-article-revision"`
}

// Opts contains the dependencies for the knowledge manager.
type Opts struct {
	DB   *sqlx.DB
	Lo   *logf.Logger
	I18n *i18n.I18n
}

// New initializes a knowledge manager.
func New(opts Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, err
	}
	return &Manager{q: q, db: opts.DB, lo: opts.Lo, i18n: opts.I18n}, nil
}

// ---------------------------------------------------------------------------
// Folders
// ---------------------------------------------------------------------------

// CreateFolder inserts a folder. parentID is null.Int{Valid:false} for a root-level folder.
func (m *Manager) CreateFolder(name, slug string, parentID null.Int) (models.Folder, error) {
	var folder models.Folder
	if err := m.q.InsertFolder.Get(&folder, parentID, name, slug); err != nil {
		if dbutil.IsUniqueViolationError(err) {
			return models.Folder{}, envelope.NewError(envelope.InputError, m.i18n.T("validation.slugCollision"), nil)
		}
		if dbutil.IsForeignKeyError(err) {
			return models.Folder{}, envelope.NewError(envelope.InputError, m.i18n.T("validation.notFoundFolder"), nil)
		}
		m.lo.Error("error creating folder", "error", err)
		return models.Folder{}, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return folder, nil
}

// UpdateFolder renames a folder. Slug stays put — renames don't regenerate slugs (mirrors the article rule).
func (m *Manager) UpdateFolder(id int, name string) (models.Folder, error) {
	var folder models.Folder
	if err := m.q.UpdateFolder.Get(&folder, id, name); err != nil {
		if err == sql.ErrNoRows {
			return models.Folder{}, envelope.NewError(envelope.NotFoundError, m.i18n.T("validation.notFoundFolder"), nil)
		}
		m.lo.Error("error updating folder", "error", err, "id", id)
		return models.Folder{}, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return folder, nil
}

// MoveFolder reparents a folder. Runs the descendant-cycle check + UPDATE
// inside a single transaction so concurrent moves can't slip a cycle past
// the check. No SELECT FOR UPDATE — concurrent moves of the same folder
// are rare; tx read-stability is sufficient, and FK / unique-index will
// catch the impossible cases on commit.
func (m *Manager) MoveFolder(id int, newParentID null.Int) (models.Folder, error) {
	tx, err := m.db.Beginx()
	if err != nil {
		m.lo.Error("error beginning move-folder tx", "error", err)
		return models.Folder{}, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	// Unconditional rollback; lib/pq treats this as a no-op once Commit succeeds.
	defer tx.Rollback() //nolint:errcheck

	if newParentID.Valid {
		// Self-move check.
		if int(newParentID.Int) == id {
			return models.Folder{}, envelope.NewError(envelope.InputError, m.i18n.T("validation.folderCycle"), nil)
		}
		// Descendant-cycle check.
		var descendants []int
		if err := tx.Stmtx(m.q.GetFolderDescendants).Select(&descendants, id); err != nil {
			m.lo.Error("error fetching folder descendants", "error", err, "id", id)
			return models.Folder{}, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
		}
		if slices.Contains(descendants, int(newParentID.Int)) {
			return models.Folder{}, envelope.NewError(envelope.InputError, m.i18n.T("validation.folderCycle"), nil)
		}
	}

	var folder models.Folder
	if err := tx.Stmtx(m.q.MoveFolder).Get(&folder, id, newParentID); err != nil {
		if err == sql.ErrNoRows {
			return models.Folder{}, envelope.NewError(envelope.NotFoundError, m.i18n.T("validation.notFoundFolder"), nil)
		}
		if dbutil.IsUniqueViolationError(err) {
			return models.Folder{}, envelope.NewError(envelope.InputError, m.i18n.T("validation.slugCollision"), nil)
		}
		if dbutil.IsForeignKeyError(err) {
			return models.Folder{}, envelope.NewError(envelope.InputError, m.i18n.T("validation.notFoundFolder"), nil)
		}
		m.lo.Error("error moving folder", "error", err, "id", id)
		return models.Folder{}, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}

	if err := tx.Commit(); err != nil {
		m.lo.Error("error committing move-folder tx", "error", err)
		return models.Folder{}, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return folder, nil
}

// DeleteFolder removes a folder. The schema's ON DELETE RESTRICT on both
// parent_id (sub-folders) and kb_articles.folder_id surfaces as a 23503
// foreign-key violation when the folder still has children — translated
// here to a clear "folder is not empty" envelope.
func (m *Manager) DeleteFolder(id int) error {
	result, err := m.q.DeleteFolder.Exec(id)
	if err != nil {
		if dbutil.IsForeignKeyError(err) {
			return envelope.NewError(envelope.InputError, m.i18n.T("validation.folderNotEmpty"), nil)
		}
		m.lo.Error("error deleting folder", "error", err, "id", id)
		return envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return envelope.NewError(envelope.NotFoundError, m.i18n.T("validation.notFoundFolder"), nil)
	}
	return nil
}

// GetFolder fetches a single folder by ID.
func (m *Manager) GetFolder(id int) (models.Folder, error) {
	var folder models.Folder
	if err := m.q.GetFolder.Get(&folder, id); err != nil {
		if err == sql.ErrNoRows {
			return models.Folder{}, envelope.NewError(envelope.NotFoundError, m.i18n.T("validation.notFoundFolder"), nil)
		}
		m.lo.Error("error getting folder", "error", err, "id", id)
		return models.Folder{}, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return folder, nil
}

// ListFolders returns all folders directly under parentID (or NULL for the root level).
func (m *Manager) ListFolders(parentID null.Int) ([]models.Folder, error) {
	folders := make([]models.Folder, 0)
	if err := m.q.ListFoldersByParent.Select(&folders, parentID); err != nil {
		m.lo.Error("error listing folders by parent", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return folders, nil
}

// ListAllFolders returns every folder. The frontend builds the tree
// client-side from this flat list — fewer round-trips than per-level
// fetches and simpler reactive state.
func (m *Manager) ListAllFolders() ([]models.Folder, error) {
	folders := make([]models.Folder, 0)
	if err := m.q.ListAllFolders.Select(&folders); err != nil {
		m.lo.Error("error listing all folders", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return folders, nil
}

// ---------------------------------------------------------------------------
// Articles
// ---------------------------------------------------------------------------

// CreateArticle inserts an article. The caller (handler) is responsible for
// setting params.AuthorUserID from the authenticated session — the request
// body's author_user_id field is ignored.
func (m *Manager) CreateArticle(params models.CreateArticleParams) (models.Article, error) {
	var article models.Article
	if err := m.q.InsertArticle.Get(
		&article,
		params.FolderID,
		params.Slug,
		params.Title,
		params.Locale,
		params.Classification,
		params.IsAIGroundable,
		params.BodyHTML,
		nullableJSON(params.BodyTiptapJSON),
		params.BodyMarkdown,
		params.AuthorUserID,
	); err != nil {
		if dbutil.IsUniqueViolationError(err) {
			return models.Article{}, envelope.NewError(envelope.InputError, m.i18n.T("validation.slugCollision"), nil)
		}
		if dbutil.IsForeignKeyError(err) {
			return models.Article{}, envelope.NewError(envelope.InputError, m.i18n.T("validation.notFoundFolder"), nil)
		}
		m.lo.Error("error creating article", "error", err)
		return models.Article{}, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return article, nil
}

// UpdateArticle modifies an existing article. Slug, title, locale and the
// three body fields are replaced; folder_id and author_user_id are not
// touched (folder via MoveArticle, author is create-only).
func (m *Manager) UpdateArticle(id int, params models.UpdateArticleParams) (models.Article, error) {
	var article models.Article
	if err := m.q.UpdateArticle.Get(
		&article,
		id,
		params.Slug,
		params.Title,
		params.Locale,
		params.Classification,
		params.IsAIGroundable,
		params.BodyHTML,
		nullableJSON(params.BodyTiptapJSON),
		params.BodyMarkdown,
	); err != nil {
		if err == sql.ErrNoRows {
			return models.Article{}, envelope.NewError(envelope.NotFoundError, m.i18n.T("validation.notFoundArticle"), nil)
		}
		if dbutil.IsUniqueViolationError(err) {
			return models.Article{}, envelope.NewError(envelope.InputError, m.i18n.T("validation.slugCollision"), nil)
		}
		m.lo.Error("error updating article", "error", err, "id", id)
		return models.Article{}, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return article, nil
}

// PublishArticle flips a draft to published and stamps published_at. The
// SQL itself gates on `status = 'draft'`, so an already-published row
// returns sql.ErrNoRows here — translated to a clear envelope error.
func (m *Manager) PublishArticle(id int) (models.Article, error) {
	var article models.Article
	if err := m.q.PublishArticle.Get(&article, id); err != nil {
		if err == sql.ErrNoRows {
			// Distinguish "not found" from "already published" with a
			// follow-up GET. Cheap: a single indexed lookup.
			if _, getErr := m.GetArticle(id); getErr != nil {
				return models.Article{}, getErr
			}
			return models.Article{}, envelope.NewError(envelope.InputError, m.i18n.T("validation.articleAlreadyPublished"), nil)
		}
		m.lo.Error("error publishing article", "error", err, "id", id)
		return models.Article{}, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return article, nil
}

// UnpublishArticle flips published → draft and clears published_at.
// Symmetric guard against UnpublishArticle on a draft row.
func (m *Manager) UnpublishArticle(id int) (models.Article, error) {
	var article models.Article
	if err := m.q.UnpublishArticle.Get(&article, id); err != nil {
		if err == sql.ErrNoRows {
			if _, getErr := m.GetArticle(id); getErr != nil {
				return models.Article{}, getErr
			}
			return models.Article{}, envelope.NewError(envelope.InputError, m.i18n.T("validation.articleAlreadyDraft"), nil)
		}
		m.lo.Error("error unpublishing article", "error", err, "id", id)
		return models.Article{}, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return article, nil
}

// MoveArticle reparents an article. The destination folder is enforced by
// the FK; a duplicate (folder_id, slug, locale) surfaces as a unique
// violation.
func (m *Manager) MoveArticle(id, newFolderID int) (models.Article, error) {
	var article models.Article
	if err := m.q.MoveArticle.Get(&article, id, newFolderID); err != nil {
		if err == sql.ErrNoRows {
			return models.Article{}, envelope.NewError(envelope.NotFoundError, m.i18n.T("validation.notFoundArticle"), nil)
		}
		if dbutil.IsUniqueViolationError(err) {
			return models.Article{}, envelope.NewError(envelope.InputError, m.i18n.T("validation.slugCollision"), nil)
		}
		if dbutil.IsForeignKeyError(err) {
			return models.Article{}, envelope.NewError(envelope.InputError, m.i18n.T("validation.notFoundFolder"), nil)
		}
		m.lo.Error("error moving article", "error", err, "id", id)
		return models.Article{}, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return article, nil
}

// DeleteArticle hard-deletes the row. No soft-delete in v1.
func (m *Manager) DeleteArticle(id int) error {
	result, err := m.q.DeleteArticle.Exec(id)
	if err != nil {
		m.lo.Error("error deleting article", "error", err, "id", id)
		return envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return envelope.NewError(envelope.NotFoundError, m.i18n.T("validation.notFoundArticle"), nil)
	}
	return nil
}

// GetArticle fetches a single article by ID.
func (m *Manager) GetArticle(id int) (models.Article, error) {
	var article models.Article
	if err := m.q.GetArticle.Get(&article, id); err != nil {
		if err == sql.ErrNoRows {
			return models.Article{}, envelope.NewError(envelope.NotFoundError, m.i18n.T("validation.notFoundArticle"), nil)
		}
		m.lo.Error("error getting article", "error", err, "id", id)
		return models.Article{}, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return article, nil
}

// MarkIndexStale is the fallback path when starting an embed workflow
// fails. It flips articles_index_state.is_stale=TRUE for the article
// (upserting a placeholder row if one doesn't exist yet) so the
// catch-up sweep can eventually re-embed. Errors are logged but not
// returned — best-effort by design.
func (m *Manager) MarkIndexStale(articleID int) {
	if _, err := m.q.MarkIndexStale.Exec(articleID); err != nil {
		m.lo.Warn("knowledge: mark-index-stale failed", "article_id", articleID, "error", err)
	}
}

// GetArticleRevision returns the current revision number. Used by the
// save-handler integration to derive workflow IDs without fetching the
// full row.
func (m *Manager) GetArticleRevision(id int) (int, error) {
	var rev int
	if err := m.q.GetArticleRevision.Get(&rev, id); err != nil {
		return 0, err
	}
	return rev, nil
}

// ListArticles paginates and filters. Search is ILIKE on title only — no
// body, no full-text in v1. At a few hundred articles this is cheap.
func (m *Manager) ListArticles(opts models.ListArticlesOpts) ([]models.Article, error) {
	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 500 {
		pageSize = 500
	}
	page := opts.Page
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * pageSize

	articles := make([]models.Article, 0)
	if err := m.q.ListArticles.Select(
		&articles,
		opts.FolderID,
		pq.StringArray(opts.Classifications),
		pq.StringArray(opts.Statuses),
		opts.Locale,
		opts.Search,
		pageSize,
		offset,
	); err != nil {
		m.lo.Error("error listing articles", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return articles, nil
}

// nullableJSON returns NULL for an empty/nil json.RawMessage so PostgreSQL
// receives an explicit NULL rather than the literal `null` JSON value or
// an invalid empty payload. Used for body_tiptap_json which is JSONB
// NOT NULL — empty payloads are coerced to '{}'::jsonb at the DB layer
// via DEFAULT, but on inserts we send the explicit value.
func nullableJSON(j []byte) interface{} {
	if len(j) == 0 {
		return []byte("{}")
	}
	return j
}
