// Package models holds DB-wire and domain types for the AI tools
// configuration table (ai_tools). After the F5 schema cleanup
// (chunk 6 of the F7+F5 batch), ai_tools holds only Python custom tools;
// internal capabilities (kb_search, etc.) live in Go code under
// internal/ai/tools/builtin/.
package models

import (
	"encoding/json"
	"time"
)

// Tool is the DB-wire representation of an ai_tools row.
//
// InputParameters, OutputSchema, and Implementation are stored as raw
// JSONB and unmarshalled at the handler / dispatcher layer:
//
//   - InputParameters: array of {key, name, type, source, source_key,
//     required, visibility, description, options}
//   - OutputSchema: JSON Schema describing the tool's structured output
//   - Implementation: {code, language, packages} — Python source embedded
//     directly per the F9 spec (Enneo-style; one row = one tool = one
//     place to look).
type Tool struct {
	ID              int             `db:"id" json:"id"`
	CreatedAt       time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time       `db:"updated_at" json:"updated_at"`
	Name            string          `db:"name" json:"name"`
	Slug            string          `db:"slug" json:"slug"`
	Description     string          `db:"description" json:"description"`
	InputParameters json.RawMessage `db:"input_parameters" json:"input_parameters"`
	OutputSchema    json.RawMessage `db:"output_schema" json:"output_schema"`
	Implementation  json.RawMessage `db:"implementation" json:"implementation"`
	Category        string          `db:"category" json:"category"`
	MinLegitimation int             `db:"min_legitimation" json:"min_legitimation"`
	TimeoutMS       int             `db:"timeout_ms" json:"timeout_ms"`
	Async           bool            `db:"async" json:"async"`
	Enabled         bool            `db:"enabled" json:"enabled"`
}

// PythonScriptImpl is the structured shape of `implementation`. Code is
// embedded directly per the F9 spec (Enneo-style; one row = one tool =
// one place to look).
type PythonScriptImpl struct {
	Code     string `json:"code"`
	Language string `json:"language"`
	Packages string `json:"packages"`
}
