// Package models contains the data types for the knowledge base manager:
// folders form a tree, articles live inside a folder. Bodies are stored in
// three formats (HTML, TipTap JSON, Markdown) so downstream RAG embedding can
// consume Markdown without a follow-up migration.
package models

import (
	"encoding/json"
	"time"

	"github.com/volatiletech/null/v9"
)

// Classification values for kb_articles.classification. Enforced by a
// CHECK constraint in the schema; mirrored here so handlers can validate
// before round-tripping to the DB.
const (
	ClassificationFAQ    = "faq"
	ClassificationHowTo  = "how_to"
	ClassificationPolicy = "policy"
	ClassificationNews   = "news"
	ClassificationOther  = "other"
)

// Status values for kb_articles.status. Binary: a draft becomes published
// via PublishArticle (which sets published_at) and back via UnpublishArticle
// (which clears it).
const (
	StatusDraft     = "draft"
	StatusPublished = "published"
)

// Locales supported by the v1 admin UI. The schema does not enforce this
// list — the frontend hardcodes the allowed set; server-side validation
// only checks length. Adding more locales is a frontend change.
const (
	LocaleEN = "en"
	LocaleDE = "de"
	LocaleFR = "fr"
)

// ValidClassifications enumerates accepted values for handler validation.
var ValidClassifications = map[string]struct{}{
	ClassificationFAQ:    {},
	ClassificationHowTo:  {},
	ClassificationPolicy: {},
	ClassificationNews:   {},
	ClassificationOther:  {},
}

// ValidStatuses enumerates accepted values for handler validation.
var ValidStatuses = map[string]struct{}{
	StatusDraft:     {},
	StatusPublished: {},
}

// Folder represents a kb_folders row. ParentID is null.Int because the
// root level uses NULL parents (no sentinel root row).
type Folder struct {
	ID        int       `db:"id"         json:"id"`
	ParentID  null.Int  `db:"parent_id"  json:"parent_id"`
	Name      string    `db:"name"       json:"name"`
	Slug      string    `db:"slug"       json:"slug"`
	SortOrder int       `db:"sort_order" json:"sort_order"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// Article represents a kb_articles row.
type Article struct {
	ID             int             `db:"id"               json:"id"`
	FolderID       int             `db:"folder_id"        json:"folder_id"`
	Slug           string          `db:"slug"             json:"slug"`
	Title          string          `db:"title"            json:"title"`
	Locale         string          `db:"locale"           json:"locale"`
	Classification string          `db:"classification"   json:"classification"`
	Status         string          `db:"status"           json:"status"`
	IsAIGroundable bool            `db:"is_ai_groundable" json:"is_ai_groundable"`
	BodyHTML       string          `db:"body_html"        json:"body_html"`
	BodyTiptapJSON json.RawMessage `db:"body_tiptap_json" json:"body_tiptap_json"`
	BodyMarkdown   string          `db:"body_markdown"    json:"body_markdown"`
	AuthorUserID   null.Int        `db:"author_user_id"   json:"author_user_id"`
	PublishedAt    null.Time       `db:"published_at"     json:"published_at"`
	CreatedAt      time.Time       `db:"created_at"       json:"created_at"`
	UpdatedAt      time.Time       `db:"updated_at"       json:"updated_at"`
}

// CreateArticleParams is the payload for CreateArticle. AuthorUserID is set
// by the handler from the authenticated session; it is not accepted from
// the request body.
type CreateArticleParams struct {
	FolderID       int             `json:"folder_id"`
	Title          string          `json:"title"`
	Slug           string          `json:"slug"`
	Locale         string          `json:"locale"`
	Classification string          `json:"classification"`
	IsAIGroundable bool            `json:"is_ai_groundable"`
	BodyHTML       string          `json:"body_html"`
	BodyTiptapJSON json.RawMessage `json:"body_tiptap_json"`
	BodyMarkdown   string          `json:"body_markdown"`
	AuthorUserID   null.Int        `json:"-"`
}

// UpdateArticleParams is the payload for UpdateArticle. FolderID and
// AuthorUserID are intentionally omitted: folder changes go through
// MoveArticle, and authorship is set on create only.
type UpdateArticleParams struct {
	Title          string          `json:"title"`
	Slug           string          `json:"slug"`
	Locale         string          `json:"locale"`
	Classification string          `json:"classification"`
	IsAIGroundable bool            `json:"is_ai_groundable"`
	BodyHTML       string          `json:"body_html"`
	BodyTiptapJSON json.RawMessage `json:"body_tiptap_json"`
	BodyMarkdown   string          `json:"body_markdown"`
}

// ListArticlesOpts controls ListArticles. All filters are optional; an
// empty/null value means "any". Search is ILIKE on title only — no body
// search, no full-text in v1.
type ListArticlesOpts struct {
	FolderID        null.Int
	Classifications []string
	Statuses        []string
	Locale          null.String
	Search          null.String
	Page            int
	PageSize        int
}
