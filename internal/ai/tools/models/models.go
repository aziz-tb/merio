// Package models holds DB-wire and domain types for the AI tools
// configuration table (ai_tools). Tools come in two flavors:
// go_builtin (in-process Go function calls) and python_script
// (Python source embedded directly, dispatched through the F6
// sandbox).
package models

import (
	"encoding/json"
	"time"
)

// Type values for the ai_tools.type column. Enforced by a CHECK
// constraint at the DB layer.
const (
	TypeGoBuiltin    = "go_builtin"
	TypePythonScript = "python_script"
)

// Category values for the ai_tools.category column. Enforced by a
// CHECK constraint at the DB layer. Categories are coarse — the
// runtime layer (F5 tool registry) applies finer-grained policy
// per category.
const (
	CategoryRead      = "read"
	CategoryWrite     = "write"
	CategoryDecision  = "decision"
	CategoryUniversal = "universal"
)

// Tool is the DB-wire representation of an ai_tools row.
//
// InputParameters, OutputSchema, and Implementation are stored as raw
// JSONB and unmarshalled at the handler / dispatcher layer:
//
//   - InputParameters: array of {key, name, type, source, source_key,
//     required, visibility, description, options}
//   - OutputSchema: JSON Schema describing the tool's structured output
//   - Implementation:
//       go_builtin    → {function: "Name"}
//       python_script → {code, language, packages}
type Tool struct {
	ID              int             `db:"id" json:"id"`
	CreatedAt       time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time       `db:"updated_at" json:"updated_at"`
	Name            string          `db:"name" json:"name"`
	Slug            string          `db:"slug" json:"slug"`
	Description     string          `db:"description" json:"description"`
	Type            string          `db:"type" json:"type"`
	Category        string          `db:"category" json:"category"`
	InputParameters json.RawMessage `db:"input_parameters" json:"input_parameters"`
	OutputSchema    json.RawMessage `db:"output_schema" json:"output_schema"`
	Implementation  json.RawMessage `db:"implementation" json:"implementation"`
	MinLegitimation int             `db:"min_legitimation" json:"min_legitimation"`
	TimeoutMS       int             `db:"timeout_ms" json:"timeout_ms"`
	Async           bool            `db:"async" json:"async"`
	Enabled         bool            `db:"enabled" json:"enabled"`
}

// GoBuiltinImpl is the structured shape of `implementation` when
// type=go_builtin.
type GoBuiltinImpl struct {
	Function string `json:"function"`
}

// PythonScriptImpl is the structured shape of `implementation` when
// type=python_script. Code is embedded directly per the F9 spec
// (Enneo-style; one row = one tool = one place to look).
type PythonScriptImpl struct {
	Code     string `json:"code"`
	Language string `json:"language"`
	Packages string `json:"packages"`
}
