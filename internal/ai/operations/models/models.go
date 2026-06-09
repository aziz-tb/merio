// Package models holds the domain + DB-wire types for ai_operations, the
// unified per-logical-AI-operation outcome audit. One row per
// logical operation (a tag classification, a future agent validation, an
// OCR pass, …); the underlying LLM provider call(s) live in ai_usage and
// are joined via trace_id.
package models

import (
	"encoding/json"
	"time"

	"github.com/volatiletech/null/v9"
)

// operation_type values. No DB CHECK enum — managed here so a new
// operation type ships without a schema migration. Keep these in sync
// with the payload structs under internal/aioperations/payloads/.
const (
	TypeTagClassification      = "tag_classification"
	TypeAgentValidation        = "agent_validation"
	TypeCustomerIdentification = "customer_identification"
	// Future: TypeSummarizeMessage, TypeOCRExtract, TypeSentimentAnalyze, …
)

// status values. Mirrors the CHECK constraint on ai_operations.status.
const (
	StatusRunning = "running"
	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusSkipped = "skipped"
)

// triggered_by values. Mirrors the CHECK constraint on
// ai_operations.triggered_by.
const (
	TriggeredByAuto   = "auto"
	TriggeredByManual = "manual"
)

// Operation is the DB-wire representation of an ai_operations row.
// Payload is raw JSONB whose shape is determined by OperationType and
// documented under internal/aioperations/payloads/.
type Operation struct {
	ID                int64           `db:"id" json:"id"`
	CreatedAt         time.Time       `db:"created_at" json:"created_at"`
	OperationType     string          `db:"operation_type" json:"operation_type"`
	ConversationID    null.Int64      `db:"conversation_id" json:"conversation_id"`
	TriggeredBy       string          `db:"triggered_by" json:"triggered_by"`
	TriggeredByUserID null.Int64      `db:"triggered_by_user_id" json:"triggered_by_user_id"`
	TraceID           string          `db:"trace_id" json:"trace_id"`
	Status            string          `db:"status" json:"status"`
	ErrorMessage      null.String     `db:"error_message" json:"error_message"`
	Payload           json.RawMessage `db:"payload" json:"payload"`
}
