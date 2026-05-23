# ai_operations payloads

Each `ai_operations.operation_type` has a payload struct in this package
describing the shape of the row's `payload` JSONB column.

## Convention

Adding a new AI operation type:

1. Add the `operation_type` constant in `internal/aioperations/models/models.go`.
2. Add a `<operation>.go` file here with the payload struct + `Marshal()` /
   `Unmarshal<Operation>()` helpers (mirror `tag_classification.go`).
3. Write the row via `aioperations.Manager.Insert` with the marshaled payload.

**No schema migration is needed** to support a new operation type — the
`operation_type` column has no DB CHECK enum and `payload` is schemaless
JSONB. The Go structs are the contract.

## What does NOT go in payload

- Token counts, model, latency, cost → these live in `ai_usage`, joined to
  the operation by `trace_id` (one operation → N LLM calls).
- Heavyweight agent-run state (drafts, status machine, staleness) → that
  stays in `agent_executions`; agent runs are not mirrored into
  `ai_operations`.

## Current payloads

| operation_type | struct |
|----------------|--------|
| `tag_classification` | `TagClassificationPayload` |
