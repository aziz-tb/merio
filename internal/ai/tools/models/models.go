// Package models holds DB-wire and domain types for the AI tools
// configuration table (ai_tools). ai_tools holds only the AI-facing
// metadata for Python/JS custom tools; the code itself lives in the
// functions table, linked by function_id (Unified Executor Framework
// batch). Internal capabilities (kb_search, etc.) live in Go code under
// internal/ai/tools/builtin/.
package models

import (
	"encoding/json"
	"time"
)

// Tool is the DB-wire representation of an ai_tools row joined with its
// linked functions row.
//
// InputParameters and OutputSchema are stored as raw JSONB on ai_tools.
// The code/language/packages live on the linked functions row; the
// dispatcher runs the tool via function.Manager.Run(tool.Slug). For the
// admin API the linked code is reconstructed into Implementation so the
// request/response JSON shape is unchanged from before the framework batch.
//
//   - InputParameters: array of {key, name, type, required, visibility,
//     description, options}. Extra fields are ignored for compatibility
//     with older saved JSON shapes.
//   - OutputSchema: JSON Schema describing the tool's structured output
//   - Implementation: {code, language, packages} — reconstructed from the
//     functions row on reads, parsed into the functions row on writes. NOT
//     an ai_tools column (db:"-").
type Tool struct {
	ID              int             `db:"id" json:"id"`
	CreatedAt       time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time       `db:"updated_at" json:"updated_at"`
	Name            string          `db:"name" json:"name"`
	Slug            string          `db:"slug" json:"slug"`
	Description     string          `db:"description" json:"description"`
	InputParameters json.RawMessage `db:"input_parameters" json:"input_parameters"`
	OutputSchema    json.RawMessage `db:"output_schema" json:"output_schema"`
	Category        string          `db:"category" json:"category"`
	TimeoutMS       int             `db:"timeout_ms" json:"timeout_ms"`
	Async           bool            `db:"async" json:"async"`
	Enabled         bool            `db:"enabled" json:"enabled"`
	FunctionID      int64           `db:"function_id" json:"function_id"`

	// Implementation is the {code, language, packages} view of the linked
	// functions row — reconstructed on reads, consumed on writes. Not a
	// column (db:"-"); the handler splits it into the functions row.
	Implementation json.RawMessage `db:"-" json:"implementation"`

	// Transient columns selected from the JOINed functions row on read
	// paths; used to build Implementation. Not part of the JSON contract.
	FnCode     string `db:"fn_code" json:"-"`
	FnLanguage string `db:"fn_language" json:"-"`
	FnPackages string `db:"fn_packages" json:"-"`
}

// PythonScriptImpl is the structured shape of `implementation` — the
// {code, language, packages} view of a tool's linked functions row.
type PythonScriptImpl struct {
	Code     string `json:"code"`
	Language string `json:"language"`
	Packages string `json:"packages"`
}
