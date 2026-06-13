// Package models holds DB-wire and domain types for the ai_tools table.
// ai_tools stores only AI-facing metadata for custom tools; the code lives
// in the functions table, linked by function_id. Internal capabilities
// (kb_search, etc.) live in Go under internal/ai/tools/builtin/.
package models

import (
	"encoding/json"
	"time"
)

// Tool is the DB-wire representation of an ai_tools row joined with its
// linked functions row. The dispatcher runs the tool via
// function.Manager.Run(tool.Slug).
//
// InputParameters and OutputSchema are raw JSONB. InputParameters is an
// array of parameter objects; extra fields are ignored for compatibility
// with older saved JSON shapes.
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
	// functions row — reconstructed on reads, split back into that row on
	// writes. Not an ai_tools column (db:"-").
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
