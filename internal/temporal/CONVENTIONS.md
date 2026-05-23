# Temporal conventions

The load-bearing rules for every Temporal workflow + activity in the Libredesk codebase. Read once, internalize. These conventions exist because Temporal has sharp edges (determinism, versioning, replay) that bite hard if ignored.

## Package layout

```
internal/
  workflows/<domain>/      -- workflow definitions (DETERMINISTIC)
    *.go                   -- WorkflowFunc + input/output types + version tags
  activities/<domain>/     -- activity implementations (NORMAL Go)
    activities.go          -- Activities struct (dependency-injected)
    *.go                   -- one file per activity, method on Activities receiver
  temporal/                -- infrastructure
    client.go, worker.go, config.go, queues.go, search_attributes.go,
    interceptors.go, sdklogger.go

cmd/
  temporal_worker.go       -- worker binary entry point
```

**Workflows and activities live in sibling top-level packages, never co-mingled.** This separation is the structural enforcement of the determinism rules below. Importing workflow helpers into an activity is fine; importing activity code into a workflow is the bug-causing mistake — keeping the packages separated makes it visible.

## Naming

| Thing | Pattern | Example |
|---|---|---|
| Workflow type | `<Verb><Noun>Workflow` | `EmbedArticleWorkflow` |
| Workflow ID | `<workflow-type-slug>:<unique-key>[:v<version>]` | `embed-article:123:v4` |
| Activity type | `<Verb><Noun>` (no "Activity" suffix) | `GenerateEmbedding` |
| Task queue | snake-case constant in `temporal/queues.go` | `Queue_embeddings = "embeddings"` |
| Version tag | descriptive string, not a number | `"add-pii-redaction"` |

**Workflow IDs MUST be deterministic for idempotency.** Same logical work → same workflow ID. Reusing an existing ID with `WorkflowIDReusePolicy_AllowDuplicateFailedOnly` is the standard idempotent-start pattern.

## Activity authoring

Every activity is a method on an `Activities` struct that holds dependencies:

```go
type Activities struct {
    DB                *sqlx.DB
    EmbeddingProvider embeddings.Provider
    Redactor          *redact.Redactor
    Usage             *usage.Manager
    Log               *logf.Logger
}

func (a *Activities) GenerateEmbedding(ctx context.Context, in *GenerateInput) (*GenerateOutput, error) {
    // ...
}
```

Activity inputs/outputs are typed struct pointers — never raw scalars or `interface{}`. Struct evolution is forwards-compatible (Temporal serializes via JSON by default).

## Determinism rules (workflow code only)

Workflow code must replay deterministically from history. Forbidden:

| Forbidden | Use instead |
|---|---|
| `time.Now()` | `workflow.Now(ctx)` |
| `rand.Int()` | `workflow.SideEffect` or pass as activity input |
| `go func() { ... }` | `workflow.Go(ctx, ...)` |
| Direct DB / HTTP / file I/O | put it in an activity |
| Reading env vars / system state | put it in an activity |
| Timer channels via `select` | `workflow.NewTimer` / `workflow.NewSelector` |
| Iterating a Go map | sort keys first, then iterate |

These violations don't always fail in tests (the workflow runs once); they fail on **replay** in production, with confusing errors. Treat any determinism violation as a P0 bug.

## Idempotency at every layer

1. **Workflow ID is the idempotency key.** Same logical operation, same ID, same outcome.
2. **Activities are idempotent.** Calling twice with the same input produces the same observable result. Use DB unique constraints (e.g. the `ai_usage` `(trace_id, call_sequence, created_at)` index pattern) to make this concrete.
3. **DB writes use `ON CONFLICT DO NOTHING`** or unique constraints.

## Timeouts and retries

Every activity has **explicit** timeouts and retry policies. Defaults are dangerous; explicit is safe.

```go
opts := workflow.ActivityOptions{
    StartToCloseTimeout:    30 * time.Second, // one attempt
    ScheduleToCloseTimeout: 5 * time.Minute,  // total wall-clock incl. retries
    RetryPolicy: &temporal.RetryPolicy{
        InitialInterval:    1 * time.Second,
        MaximumInterval:    1 * time.Minute,
        BackoffCoefficient: 2.0,
        MaximumAttempts:    5,
    },
}
ctx = workflow.WithActivityOptions(ctx, opts)
```

**Different activity classes get different policies:**

- **External API calls** (LLM, embeddings, ERP, KB): transient failures common — aggressive retry (5+ attempts, exponential backoff).
- **DB writes**: failures usually permanent — fewer retries (2-3), faster failure.
- **Side-effect tools** (refund, bank-data change): consider idempotency keys + tighter retry caps; the wrong retry policy double-charges customers.

## Search attributes

Every workflow attaches search attributes at start time. Registered once with Temporal (one-time setup; see `internal/temporal/search_attributes.go`).

| Name | Type | Purpose |
|---|---|---|
| `trace_id` | Keyword | Correlate with `ai_usage` / `script_executions` |
| `inbox_id` | Int | Per-inbox filtering |
| `workflow_kind` | Keyword | Category — `embedding`, `pipeline`, `sla`, … |
| `entity_type` | Keyword | The thing being processed — `kb_article`, `ticket`, … |
| `entity_id` | Int | Numeric id of that entity |

## Workflow versioning

**The single most important convention.** When workflow code changes in a way that affects replay:

```go
v := workflow.GetVersion(ctx, "add-pii-redaction", workflow.DefaultVersion, 1)
if v == workflow.DefaultVersion {
    return oldLogic(ctx, input)
}
return newLogic(ctx, input)
```

Rules:

- **Never delete a `GetVersion` block** while any workflow that ran the old branch might still be replaying. Wait until past the namespace retention window.
- Use **descriptive string IDs**: `"add-pii-redaction"`, not `"v2"`.
- Keep workflow function signatures backward-compatible — adding optional fields to input structs is safe; removing or renaming is not.
- Log every versioning decision in code comments at the `GetVersion` site (what changed, when, why).

## Graceful degradation when Temporal is disabled

Workflow-starting code paths check `app.temporal != nil`. The fallback is one of two — never silent no-op:

1. **Sync fallback** when the work is reasonable to do inline (e.g., embedding one article ≈ 200-500ms — acceptable in dev). Document the latency cost where the fallback lives.
2. **Refuse to register the endpoint / log "feature disabled — Temporal required" at startup** for features that genuinely can't function without it (durable SLA timers, multi-step pipelines).

**Never** start a workflow path that does nothing when Temporal is off. That is a footgun.

## Failure handling

- Aggressive retry → give up after `MaximumAttempts` → workflow marked Failed → visible in Temporal UI.
- Failed workflows surface to operators via Temporal UI search. Trace correlation via `trace_id` ties them to `ai_usage` rows.
- No separate DLQ table in v1. Failed pipeline runs surface on the conversation UI (per F3 plan) with a "Retry AI processing" button.
- Treat `failed workflows / hour > N` as an on-call signal when a metrics stack lands.

## Things that look fine but aren't

- **Calling `time.Sleep` in a workflow** — breaks replay. Use `workflow.Sleep` or `workflow.NewTimer`.
- **Returning errors with stack-trace formatting** — Temporal serializes errors; rich types may round-trip lossily. Use plain `fmt.Errorf("descriptor: %w", err)`.
- **Capturing variables by closure inside `workflow.Go`** — same hazards as goroutines in normal Go, plus determinism: only capture variables you control fully.
- **Throwing panics for assertion failures** — Temporal catches panics and treats them as workflow task failures (retry forever by default). Return a non-retryable error instead.

## Reviewer checklist (when reviewing a workflow PR)

- [ ] Workflow ID is deterministic from inputs (no `uuid.New()` or `time.Now()` in the start path).
- [ ] All activity invocations have explicit `ActivityOptions` (timeout + retry).
- [ ] No `time.Now()`, `rand.*`, `os.Getenv` inside the workflow function.
- [ ] `GetVersion` used for any behavior change since the last release.
- [ ] Search attributes attached at start time.
- [ ] Activity functions are idempotent OR the workflow uses `WorkflowIDReusePolicy` correctly to prevent double-execution.
- [ ] Tests use `testsuite.WorkflowTestSuite` for workflow logic; plain Go tests for activity logic.
