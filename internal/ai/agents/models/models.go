// Package models holds DB-wire and domain types for the AI agent
// configuration table (ai_agents). The detection JSONB column reuses the
// universal detection AST so one condition language is maintained across
// the platform.
package models

import (
	"encoding/json"
	"time"
)

// Agent is the DB-wire representation of an ai_agents row. Every ai_agents row
// is a sub-agent (user-authored: prompt, tools, detection, knowledge); the
// orchestrator is a platform component, not a row (see internal/ai/runner).
//
// Detection, Tools, and PresetOverrides are stored as raw JSONB and
// unmarshalled at the handler/runtime layer where the structured shape matters:
//
//   - Detection unmarshals into internal/detection.Detection by the validation
//     runtime. New agents get the structured DefaultDetection scaffold from
//     internal/ai/agents so the visual builder has usable data.
//   - Tools unmarshals into []ToolRef
//   - PresetOverrides unmarshals into map[string]string (purpose → preset slug)
//
// Description allows NULL in the DB but the API and handlers always
// pass a non-NULL string (matching the automation_rules precedent).
type Agent struct {
	ID                      int             `db:"id" json:"id"`
	CreatedAt               time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt               time.Time       `db:"updated_at" json:"updated_at"`
	Name                    string          `db:"name" json:"name"`
	Slug                    string          `db:"slug" json:"slug"`
	Description             string          `db:"description" json:"description"`
	Prompt                  string          `db:"prompt" json:"prompt"`
	Detection               json.RawMessage `db:"detection" json:"detection"`
	Tools                   json.RawMessage `db:"tools" json:"tools"`
	KBScope                 json.RawMessage `db:"kb_scope" json:"kb_scope"`
	PresetOverrides         json.RawMessage `db:"preset_overrides" json:"preset_overrides"`
	MaxIterations           int             `db:"max_iterations" json:"max_iterations"`
	ExtendedThinkingEnabled bool            `db:"extended_thinking_enabled" json:"extended_thinking_enabled"`
	MinVerification         int             `db:"min_verification" json:"min_verification"`
	Enabled                 bool            `db:"enabled" json:"enabled"`
}

// KBScope is the structured shape of the `kb_scope` JSONB column on an
// Agent. FolderIDs whitelists kb_folders.id values; ArticleIDs whitelists
// kb_articles.id values that may live in folders outside the folder
// whitelist (additive). Empty scope = no KB access for this agent —
// kb_search returns empty results without consulting the index.
type KBScope struct {
	FolderIDs  []int `json:"folder_ids"`
	ArticleIDs []int `json:"article_ids"`
}

// ToolRefKind enumerates the two flavors of tool reference an agent
// can carry. Internal capabilities live in code under
// internal/ai/tools/builtin/; custom tools are ai_tools rows (Python
// scripts after the F5 schema cleanup).
type ToolRefKind string

const (
	ToolRefKindInternal ToolRefKind = "internal"
	ToolRefKindCustom   ToolRefKind = "custom"
)

// ToolRef is one entry in the agent's `tools` JSONB array. The `id`
// field is decoded as raw JSON so we can carry both numeric ids (custom
// tools, ai_tools.id) and string ids (internal capability IDs) without
// fighting the type system.
//
// Use the typed accessors CustomID() / InternalID() rather than reading
// ID directly — they handle the Kind→type mapping consistently.
type ToolRef struct {
	Kind           ToolRefKind     `json:"kind"`
	ID             json.RawMessage `json:"id"`
	ConfigOverride json.RawMessage `json:"config_override,omitempty"`
}

// CustomID returns the ai_tools.id for a custom-kind ref, or (0, false)
// if Kind is not Custom or ID isn't a JSON number.
func (r ToolRef) CustomID() (int, bool) {
	if r.Kind != ToolRefKindCustom {
		return 0, false
	}
	var id int
	if err := json.Unmarshal(r.ID, &id); err != nil {
		return 0, false
	}
	return id, true
}

// InternalID returns the internal capability id for an internal-kind
// ref, or ("", false) if Kind is not Internal or ID isn't a JSON string.
func (r ToolRef) InternalID() (string, bool) {
	if r.Kind != ToolRefKindInternal {
		return "", false
	}
	var id string
	if err := json.Unmarshal(r.ID, &id); err != nil {
		return "", false
	}
	return id, true
}
