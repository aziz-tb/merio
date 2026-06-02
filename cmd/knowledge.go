package main

import (
	"regexp"
	"strconv"
	"strings"

	amodels "github.com/aziz-tb/merio/internal/auth/models"
	"github.com/aziz-tb/merio/internal/envelope"
	"github.com/aziz-tb/merio/internal/knowledge/models"
	"github.com/valyala/fasthttp"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/fastglue"
)

// slugRE enforces the on-the-wire slug shape. The frontend hand-rolls the
// derivation; the server is the gate.
var slugRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ---------------------------------------------------------------------------
// Folder handlers
// ---------------------------------------------------------------------------

type folderRequest struct {
	Name     string   `json:"name"`
	Slug     string   `json:"slug"`
	ParentID null.Int `json:"parent_id"`
}

type folderMoveRequest struct {
	ParentID null.Int `json:"parent_id"`
}

func handleListKnowledgeFolders(r *fastglue.Request) error {
	var app = r.Context.(*App)
	folders, err := app.knowledge.ListAllFolders()
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(folders)
}

func handleGetKnowledgeFolder(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		id, err = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	if err != nil || id == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}
	folder, err := app.knowledge.GetFolder(id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(folder)
}

func handleCreateKnowledgeFolder(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req = folderRequest{}
	)
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), err.Error(), envelope.InputError)
	}
	if err := validateFolder(app, req); err != nil {
		return sendErrorEnvelope(r, err)
	}
	folder, err := app.knowledge.CreateFolder(req.Name, req.Slug, req.ParentID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(folder)
}

func handleUpdateKnowledgeFolder(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req = folderRequest{}
	)
	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if err != nil || id == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), err.Error(), envelope.InputError)
	}
	if strings.TrimSpace(req.Name) == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.Ts("globals.messages.empty", "name", "`name`"), nil, envelope.InputError)
	}
	folder, err := app.knowledge.UpdateFolder(id, req.Name)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(folder)
}

func handleMoveKnowledgeFolder(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req = folderMoveRequest{}
	)
	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if err != nil || id == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), err.Error(), envelope.InputError)
	}
	folder, err := app.knowledge.MoveFolder(id, req.ParentID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(folder)
}

func handleDeleteKnowledgeFolder(r *fastglue.Request) error {
	var app = r.Context.(*App)
	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if err != nil || id == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}
	if err := app.knowledge.DeleteFolder(id); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}

// ---------------------------------------------------------------------------
// Article handlers
// ---------------------------------------------------------------------------

type articleMoveRequest struct {
	FolderID int `json:"folder_id"`
}

func handleListKnowledgeArticles(r *fastglue.Request) error {
	var app = r.Context.(*App)
	args := r.RequestCtx.QueryArgs()

	opts := models.ListArticlesOpts{
		Page:     parseInt(string(args.Peek("page")), 1),
		PageSize: parseInt(string(args.Peek("page_size")), 50),
	}
	if v := string(args.Peek("folder_id")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			opts.FolderID = null.IntFrom(n)
		}
	}
	if v := string(args.Peek("locale")); v != "" {
		opts.Locale = null.StringFrom(v)
	}
	if v := string(args.Peek("search")); v != "" {
		opts.Search = null.StringFrom(v)
	}
	for _, v := range args.PeekMulti("classification") {
		if s := string(v); s != "" {
			opts.Classifications = append(opts.Classifications, s)
		}
	}
	for _, v := range args.PeekMulti("status") {
		if s := string(v); s != "" {
			opts.Statuses = append(opts.Statuses, s)
		}
	}

	articles, err := app.knowledge.ListArticles(opts)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(articles)
}

func handleGetKnowledgeArticle(r *fastglue.Request) error {
	var app = r.Context.(*App)
	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if err != nil || id == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}
	article, err := app.knowledge.GetArticle(id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(article)
}

func handleCreateKnowledgeArticle(r *fastglue.Request) error {
	var (
		app    = r.Context.(*App)
		auser  = r.RequestCtx.UserValue("user").(amodels.User)
		params = models.CreateArticleParams{}
	)
	if err := r.Decode(&params, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), err.Error(), envelope.InputError)
	}
	if err := validateArticleCreate(app, params); err != nil {
		return sendErrorEnvelope(r, err)
	}
	// Server sets the author from the session; ignore any author_user_id in the payload.
	params.AuthorUserID = null.IntFrom(auser.ID)
	article, err := app.knowledge.CreateArticle(params)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	// F12: trigger embed workflow when the new row is already
	// published. Create-as-draft path no-ops; the embed fires on
	// subsequent publish.
	app.kickEmbedIfPublished(article)
	return r.SendEnvelope(article)
}

func handleUpdateKnowledgeArticle(r *fastglue.Request) error {
	var (
		app    = r.Context.(*App)
		params = models.UpdateArticleParams{}
	)
	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if err != nil || id == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}
	if err := r.Decode(&params, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), err.Error(), envelope.InputError)
	}
	if err := validateArticleUpdate(app, params); err != nil {
		return sendErrorEnvelope(r, err)
	}
	article, err := app.knowledge.UpdateArticle(id, params)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	// F12: revision bumped by UPDATE; embed if currently published.
	app.kickEmbedIfPublished(article)
	return r.SendEnvelope(article)
}

func handlePublishKnowledgeArticle(r *fastglue.Request) error {
	var app = r.Context.(*App)
	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if err != nil || id == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}
	article, err := app.knowledge.PublishArticle(id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	// F12: publish flips status + bumps revision; embed the new state.
	app.kickEmbedIfPublished(article)
	return r.SendEnvelope(article)
}

func handleUnpublishKnowledgeArticle(r *fastglue.Request) error {
	var app = r.Context.(*App)
	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if err != nil || id == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}
	article, err := app.knowledge.UnpublishArticle(id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	// F12: article is no longer queryable — drop its vectors so the
	// hybrid search stops surfacing it. Folder move (below) deliberately
	// does NOT trigger this; folder_id is metadata, not embedded content.
	app.unembedArticle(article.ID)
	return r.SendEnvelope(article)
}

func handleMoveKnowledgeArticle(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req = articleMoveRequest{}
	)
	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if err != nil || id == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), err.Error(), envelope.InputError)
	}
	if req.FolderID == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.Ts("globals.messages.empty", "name", "`folder_id`"), nil, envelope.InputError)
	}
	article, err := app.knowledge.MoveArticle(id, req.FolderID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(article)
}

func handleDeleteKnowledgeArticle(r *fastglue.Request) error {
	var app = r.Context.(*App)
	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if err != nil || id == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}
	if err := app.knowledge.DeleteArticle(id); err != nil {
		return sendErrorEnvelope(r, err)
	}
	// F12: drop orphan vectors. articles_index_state.article_id has
	// ON DELETE CASCADE; embeddings rows are content_type-agnostic
	// (no FK) so the unembed sweep cleans them up explicitly.
	app.unembedArticle(id)
	return r.SendEnvelope(true)
}

// ---------------------------------------------------------------------------
// Validation helpers
// ---------------------------------------------------------------------------

func validateFolder(app *App, req folderRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return envelope.NewError(envelope.InputError, app.i18n.Ts("globals.messages.empty", "name", "`name`"), nil)
	}
	if !slugRE.MatchString(req.Slug) {
		return envelope.NewError(envelope.InputError, app.i18n.T("validation.invalidSlug"), nil)
	}
	if len(req.Slug) > 255 {
		return envelope.NewError(envelope.InputError, app.i18n.T("validation.invalidSlug"), nil)
	}
	return nil
}

func validateArticleCreate(app *App, p models.CreateArticleParams) error {
	if p.FolderID == 0 {
		return envelope.NewError(envelope.InputError, app.i18n.Ts("globals.messages.empty", "name", "`folder_id`"), nil)
	}
	return validateArticleCommon(app, p.Title, p.Slug, p.Locale, p.Classification)
}

func validateArticleUpdate(app *App, p models.UpdateArticleParams) error {
	return validateArticleCommon(app, p.Title, p.Slug, p.Locale, p.Classification)
}

func validateArticleCommon(app *App, title, slug, locale, classification string) error {
	if strings.TrimSpace(title) == "" || len(title) > 500 {
		return envelope.NewError(envelope.InputError, app.i18n.Ts("globals.messages.empty", "name", "`title`"), nil)
	}
	if !slugRE.MatchString(slug) || len(slug) > 255 {
		return envelope.NewError(envelope.InputError, app.i18n.T("validation.invalidSlug"), nil)
	}
	if locale == "" || len(locale) > 8 {
		return envelope.NewError(envelope.InputError, app.i18n.Ts("globals.messages.empty", "name", "`locale`"), nil)
	}
	if _, ok := models.ValidClassifications[classification]; !ok {
		return envelope.NewError(envelope.InputError, app.i18n.T("validation.invalidClassification"), nil)
	}
	return nil
}

// parseInt is a tolerant parser used for query-string ints.
func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
