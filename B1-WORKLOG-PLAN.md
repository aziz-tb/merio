# B1 — Worklog Instrumentation: Implementation Plan

Status: **approved with edits**, awaiting code-gen kickoff.

Source spec: [PLAN.md §6.2 B1](PLAN.md). Whole-repo map: [AGENTS.md](AGENTS.md).

This file is the single source of truth for the B1 PR. It supersedes any earlier conversation. Re-read §0 (Decisions) and §10 (Hard-gate findings) before code-gen.

---

## 0. Decisions (locked in)

| # | Question | Decision |
|---|---|---|
| Q1 | Down-migration mechanism (framework is forward-only) | **(a)** Documented manual SQL as a comment block at the top of `internal/migrations/v2_3_0.go`. Not auto-executed. (Earlier draft used a separate `DOWN.sql` file but the repo has no precedent for that convention, so the comment-block form was preferred for consistency.) |
| Q2 | DB-backed test strategy | **(b)** `DATABASE_URL` env var; `t.Skip` when unset. **CI does not currently run Postgres** ([go.yml](.github/workflows/go.yml) has no `services:` block) — follow-up PR adds it. Tests are local-only until then. |
| Q3 | Forward-action hook placement | **(a)** Instrument call-site in [cmd/messages.go](cmd/messages.go) `handleForwardMessage`. No change to `QueueReply` signature. |
| Q4 | Permission persistence model | Confirmed via reads (§10). Permissions stored as `roles.permissions TEXT[]` ([schema.sql:118-126](schema.sql)); validated against the hardcoded `validPermissions` map in [internal/authz/models/models.go](internal/authz/models/models.go); seeded inline in [schema.sql:754-769](schema.sql). The migration **must** grant new permissions to the existing Admin role on upgraded instances — install-time seed only covers fresh installs. |
| Q5 | Conversation manager DI shape | Confirmed via reads (§10). `Manager` uses interface-stores; `New(...)` takes 14 positional args + `Opts` ([conversation.go:184-200](internal/conversation/conversation.go)); construction lives at [cmd/init.go:305](cmd/init.go), called from [cmd/main.go:228](cmd/main.go). Plan adds `worklogStore` as a 15th interface dependency. |
| Q6 | jsonb merge semantics in `EndWorklog` | Shallow merge via `||`. Documented in a comment on the `end` query. |
| Q7 | `automation_level` column | Provision **now**, NOT NULL DEFAULT `'L0'`, CHECK in `('L0','L1','L2','L3','L4','L5')`. **No writer exposed in this PR** — column is provisioned for the Wave-2 AI work to populate later. |

---

## 1. Files to be created

| Path | Purpose |
|---|---|
| `internal/worklog/worklog.go` | Manager: `New`, `StartWorklog`, `EndWorklog`, `GetWorklogsByConversation`, `GetWorklogsByUser`. Mirrors [internal/macro/macro.go](internal/macro/macro.go). |
| `internal/worklog/queries.sql` | Embedded SQL: `start`, `end`, `get-by-conversation`, `get-by-user`. |
| `internal/worklog/models/models.go` | `Worklog` struct, `ActionType*` constants, `AutomationLevel*` constants, `ListOpts`. |
| `internal/worklog/worklog_test.go` | Round-trip + multi-action + error-path tests; uses `DATABASE_URL`, `t.Skip`s otherwise. Style mirrors [internal/sla/calculator_test.go](internal/sla/calculator_test.go). |
| `internal/migrations/v2_3_0.go` | New `V2_3_0` migration: creates `worklogs` table; idempotently adds the three `worklog:*` permissions to existing seeded `Admin` role. Pattern matches [internal/migrations/v1.0.1.go](internal/migrations/v1.0.1.go). The rollback SQL is embedded as a comment block at the top of this file (no separate DOWN file — keeps the migrations directory uniform). |

---

## 2. Files to be modified

| Path | Change |
|---|---|
| [cmd/upgrade.go](cmd/upgrade.go) | Append `{"v2.3.0", migrations.V2_3_0}` to `migList` (after the `v2.2.0` entry around line 44). |
| [schema.sql](schema.sql) | (a) Add `worklogs` table + indexes + checks at the end of the table-creation block (after `user_notifications`, before the seed `INSERT`s). (b) Update both seeded role rows at lines 754-769: append `worklog:view_own` to the `Agent` row, append `worklog:view_own,worklog:view_team,worklog:view_all` to the `Admin` row. Keeps fresh installs in sync with migrated instances. |
| [internal/authz/models/models.go](internal/authz/models/models.go) | Add three Go constants `PermWorklogViewOwn`, `PermWorklogViewTeam`, `PermWorklogViewAll`; add the three string keys to `validPermissions`. **This is mandatory** — without it, `role.filterValidPermissions` ([role.go:150](internal/role/role.go)) drops the permissions and roles cannot hold them. |
| [internal/conversation/conversation.go](internal/conversation/conversation.go) | (a) Add the `worklogStore` interface block (alongside the other store interfaces around lines 107-167). (b) Add `worklogStore worklogStore` field on `Manager` (after `webhookStore`, line 72). (c) Add a `worklogStore` parameter to `New(...)` (after `webhook`, before `dispatcher`, line 198). (d) Wire it in the `Manager{...}` literal (line 226-250). (e) Inside `UpdateConversationStatus` ([conversation.go:824](internal/conversation/conversation.go)): wrap the body with `Start/End` (action_type=`status`, context: `{previous_status, new_status, snooze_until}`). |
| [internal/conversation/message.go](internal/conversation/message.go) | (a) Inside `QueueReply` ([message.go:450](internal/conversation/message.go)): wrap body with `Start/End` (action_type=`write`, context: `{inbox_id, recipients_count, has_attachments}`). (b) Inside `SendPrivateNote` ([message.go:392](internal/conversation/message.go)): wrap body with `Start/End` (action_type=`note`, context: `{mentions_count}`). |
| [cmd/messages.go](cmd/messages.go) | Inside `handleForwardMessage` (around line 294): instrument with `Start/End` via `app.worklog` (action_type=`forward`, context: `{original_message_uuid, to_count, has_comment}`). Q3 — the forward hook lives at the call site, not inside `QueueReply`. |
| [cmd/init.go](cmd/init.go) | (a) Add `initWorklog(...)` builder (mirror of e.g. `initMacro` if it exists, otherwise `initAutoAssigner` at [cmd/init.go:618](cmd/init.go)). (b) Update `initConversations` signature ([init.go:282-299](cmd/init.go)) to accept a `*worklog.Manager` (or the interface). (c) Pass it through to `conversation.New(...)` at [init.go:305](cmd/init.go). |
| [cmd/main.go](cmd/main.go) | (a) Add `worklog *worklog.Manager` field on `App` (next to `conversation`, around [main.go:96](cmd/main.go)). (b) Construct it before `initConversations` is called at [main.go:228](cmd/main.go). (c) Pass it into the updated `initConversations(...)` call at [main.go:228](cmd/main.go). |
| [frontend/apps/main/src/constants/permissions.js](frontend/apps/main/src/constants/permissions.js) | Add `WORKLOG_VIEW_OWN`, `WORKLOG_VIEW_TEAM`, `WORKLOG_VIEW_ALL`. |

**No file is deleted. No existing exported function signature is removed.** The two signature changes (`conversation.New`, `initConversations`) are additive — every existing call site is updated in the same PR.

---

## 3. Up migration — `internal/migrations/v2_3_0.go`

```go
package migrations

import (
    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V2_3_0 provisions the worklog instrumentation table (B1) and grants its
// permissions to the existing Admin role on upgraded instances. Idempotent.
func V2_3_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
    // 1. Create the worklogs table. IF NOT EXISTS guards make it safe to re-run.
    _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS worklogs (
            id                BIGSERIAL PRIMARY KEY,
            conversation_id   BIGINT NOT NULL
                              REFERENCES conversations(id)
                              ON DELETE CASCADE ON UPDATE CASCADE,
            user_id           BIGINT NULL
                              REFERENCES users(id)
                              ON DELETE SET NULL ON UPDATE CASCADE,
            started_at        TIMESTAMPTZ NOT NULL,
            ended_at          TIMESTAMPTZ NULL,
            duration_seconds  INT NULL,
            action_type       VARCHAR(32) NOT NULL,
            automation_level  VARCHAR(2) NOT NULL DEFAULT 'L0',
            context           JSONB NULL,
            created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),

            CONSTRAINT constraint_worklogs_on_action_type
                CHECK (action_type IN ('write', 'status', 'note', 'forward')),
            CONSTRAINT constraint_worklogs_on_automation_level
                CHECK (automation_level IN ('L0','L1','L2','L3','L4','L5')),
            CONSTRAINT constraint_worklogs_on_duration_non_negative
                CHECK (duration_seconds IS NULL OR duration_seconds >= 0),
            CONSTRAINT constraint_worklogs_on_ended_at_after_started_at
                CHECK (ended_at IS NULL OR ended_at >= started_at)
        );

        CREATE INDEX IF NOT EXISTS index_worklogs_on_conversation_id
            ON worklogs (conversation_id);
        CREATE INDEX IF NOT EXISTS index_worklogs_on_user_id
            ON worklogs (user_id);
        CREATE INDEX IF NOT EXISTS index_worklogs_on_started_at
            ON worklogs (started_at);
        CREATE INDEX IF NOT EXISTS index_worklogs_on_action_type
            ON worklogs (action_type);
    `)
    if err != nil {
        return err
    }

    // 2. Grant the new permissions to existing seeded roles. The install-time
    //    seed in schema.sql covers fresh installs; this branch covers upgraded
    //    instances. array_append is a no-op when the value is already present
    //    (we use a NOT-contains guard to keep it strictly idempotent).
    _, err = db.Exec(`
        UPDATE roles
        SET    permissions = permissions || ARRAY['worklog:view_own']::TEXT[]
        WHERE  name = 'Agent'
          AND  NOT ('worklog:view_own' = ANY(permissions));
    `)
    if err != nil {
        return err
    }

    _, err = db.Exec(`
        UPDATE roles
        SET    permissions = permissions || ARRAY[
                   'worklog:view_own',
                   'worklog:view_team',
                   'worklog:view_all'
               ]::TEXT[]
        WHERE  name = 'Admin'
          AND  NOT ('worklog:view_all' = ANY(permissions));
    `)
    return err
}
```

### Design notes

- `action_type VARCHAR(32) + CHECK` rather than a Postgres `ENUM`. Adding values later is a single SQL statement instead of a `CREATE TYPE` migration. If you'd rather keep the codebase enum-uniform, swap to a typed enum and I'll rework the file.
- `ON DELETE CASCADE` on `conversation_id` matches `conversation_messages` and `conversation_drafts` ([schema.sql:276, 312](schema.sql)).
- `ON DELETE SET NULL` on `user_id` matches `last_message_sender_id` etc. on `conversations` ([schema.sql:244](schema.sql)). Worklogs retain analytics value after agent deletion.
- `automation_level VARCHAR(2) DEFAULT 'L0'` — provisioned per Q7. No code path writes anything but the default in this PR; the column exists so the Wave-2 AI work doesn't need a backfilling migration.
- The role-update branches are guarded by a `NOT ('...' = ANY(permissions))` check to be strictly idempotent under the existing forward-only migration model.

---

## 4. Down migration — comment block in `internal/migrations/v2_3_0.go` (manual, not auto-executed)

The libredesk migration framework is forward-only and the repo has no precedent for separate down files, so the rollback SQL lives as a comment block at the top of `v2_3_0.go`. Operators paste it into psql against the same database, then redeploy a build that no longer contains the worklog code.

```sql
BEGIN;
UPDATE roles
SET    permissions = ARRAY(
           SELECT UNNEST(permissions)
           EXCEPT SELECT UNNEST(ARRAY[
               'worklog:view_own',
               'worklog:view_team',
               'worklog:view_all'
           ])
       );
DROP INDEX IF EXISTS index_worklogs_on_action_type;
DROP INDEX IF EXISTS index_worklogs_on_started_at;
DROP INDEX IF EXISTS index_worklogs_on_user_id;
DROP INDEX IF EXISTS index_worklogs_on_conversation_id;
DROP TABLE IF EXISTS worklogs;
COMMIT;
```

The Go-side cleanup (revert `validPermissions`, revert role seed in `schema.sql`, revert `permissions.js`) is part of the redeploy, not part of the SQL.

---

## 5. Manager struct + method signatures

### `internal/worklog/models/models.go`

```go
package models

import (
    "encoding/json"
    "time"
)

// Action types stored in worklogs.action_type. Mirrors the CHECK constraint.
const (
    ActionTypeWrite   = "write"
    ActionTypeStatus  = "status"
    ActionTypeNote    = "note"
    ActionTypeForward = "forward"
)

// Automation levels stored in worklogs.automation_level. Defaults to L0; no
// writer exposed in this PR — Wave-2 AI work populates the higher levels.
const (
    AutomationLevelL0 = "L0"
    AutomationLevelL1 = "L1"
    AutomationLevelL2 = "L2"
    AutomationLevelL3 = "L3"
    AutomationLevelL4 = "L4"
    AutomationLevelL5 = "L5"
)

// Worklog is a single ticket-touching action recorded for analytics + reporting.
type Worklog struct {
    ID              int64           `db:"id"               json:"id"`
    ConversationID  int64           `db:"conversation_id"  json:"conversation_id"`
    UserID          *int64          `db:"user_id"          json:"user_id"`
    StartedAt       time.Time       `db:"started_at"       json:"started_at"`
    EndedAt         *time.Time      `db:"ended_at"         json:"ended_at"`
    DurationSeconds *int            `db:"duration_seconds" json:"duration_seconds"`
    ActionType      string          `db:"action_type"      json:"action_type"`
    AutomationLevel string          `db:"automation_level" json:"automation_level"`
    Context         json.RawMessage `db:"context"          json:"context"`
    CreatedAt       time.Time       `db:"created_at"       json:"created_at"`
}

// ListOpts paginates list reads. Filters live in the queries on
// purpose-built indexes; this struct is paging only for now.
type ListOpts struct {
    Page     int
    PageSize int
}
```

### `internal/worklog/worklog.go`

```go
package worklog

import (
    "context"
    "database/sql"
    "embed"
    "encoding/json"

    "github.com/abhinavxd/libredesk/internal/dbutil"
    "github.com/abhinavxd/libredesk/internal/envelope"
    "github.com/abhinavxd/libredesk/internal/worklog/models"
    "github.com/jmoiron/sqlx"
    "github.com/knadh/go-i18n"
    "github.com/zerodha/logf"
)

var (
    //go:embed queries.sql
    efs embed.FS
)

type Manager struct {
    q    queries
    lo   *logf.Logger
    i18n *i18n.I18n
}

type queries struct {
    Start             *sqlx.Stmt `query:"start"`
    End               *sqlx.Stmt `query:"end"`
    GetByConversation *sqlx.Stmt `query:"get-by-conversation"`
    GetByUser         *sqlx.Stmt `query:"get-by-user"`
}

type Opts struct {
    DB   *sqlx.DB
    Lo   *logf.Logger
    I18n *i18n.I18n
}

func New(opts Opts) (*Manager, error)

// StartWorklog inserts a row with started_at = NOW() and returns its id.
// userID may be nil for system-initiated actions.
// contextPayload may be nil. ctx is honored for cancellation.
func (m *Manager) StartWorklog(
    ctx context.Context,
    convID int64,
    userID *int64,
    actionType string,
    contextPayload json.RawMessage,
) (int64, error)

// EndWorklog stamps ended_at = NOW(), computes duration_seconds at the DB,
// and shallow-merges `extra` into the existing context jsonb (right-side wins
// on key conflict — Postgres `||` operator semantics).
// Idempotent only by guard: a second call is a no-op because the UPDATE has
// `AND ended_at IS NULL`.
func (m *Manager) EndWorklog(
    ctx context.Context,
    worklogID int64,
    extra json.RawMessage,
) error

func (m *Manager) GetWorklogsByConversation(
    ctx context.Context,
    convID int64,
    opts models.ListOpts,
) ([]models.Worklog, error)

func (m *Manager) GetWorklogsByUser(
    ctx context.Context,
    userID int64,
    opts models.ListOpts,
) ([]models.Worklog, error)
```

Error returns use `envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)` for unexpected DB errors and `envelope.NewError(envelope.NotFoundError, m.i18n.T("validation.notFoundWorklog"), nil)` where applicable — matches the pattern in [internal/macro/macro.go](internal/macro/macro.go). New i18n key `validation.notFoundWorklog` will be added to [i18n/en.json](i18n/en.json).

### `internal/worklog/queries.sql`

```sql
-- name: start
INSERT INTO worklogs (conversation_id, user_id, started_at, action_type, context)
VALUES ($1, $2, NOW(), $3, $4)
RETURNING id;

-- name: end
-- Shallow-merge extra into context: right-side wins on key conflict
-- (Postgres `||` operator semantics). Caller is expected to hand off
-- a flat JSON object; nested merge is not performed.
UPDATE worklogs
SET    ended_at         = NOW(),
       duration_seconds = GREATEST(0, EXTRACT(EPOCH FROM (NOW() - started_at))::int),
       context          = COALESCE(context, '{}'::jsonb) || COALESCE($2::jsonb, '{}'::jsonb)
WHERE  id = $1
  AND  ended_at IS NULL;

-- name: get-by-conversation
SELECT id, conversation_id, user_id, started_at, ended_at, duration_seconds,
       action_type, automation_level, context, created_at
FROM   worklogs
WHERE  conversation_id = $1
ORDER  BY started_at DESC
LIMIT  $2 OFFSET $3;

-- name: get-by-user
SELECT id, conversation_id, user_id, started_at, ended_at, duration_seconds,
       action_type, automation_level, context, created_at
FROM   worklogs
WHERE  user_id = $1
ORDER  BY started_at DESC
LIMIT  $2 OFFSET $3;
```

### Conversation manager interface dependency

In [internal/conversation/conversation.go](internal/conversation/conversation.go) (alongside the existing `webhookStore`, `statusStore`, etc.):

```go
type worklogStore interface {
    StartWorklog(ctx context.Context, convID int64, userID *int64, actionType string, contextPayload json.RawMessage) (int64, error)
    EndWorklog(ctx context.Context, worklogID int64, extra json.RawMessage) error
}
```

Using an interface keeps the `conversation` package importable in tests without pulling in the worklog package (matches the pattern of every other store).

### Hook shape (status example)

```go
// inside UpdateConversationStatus, before the existing return paths:
wlID, _ := c.worklogStore.StartWorklog(
    context.Background(),
    int64(conversationBeforeChange.ID),
    int64Ptr(actor.ID),
    wmodels.ActionTypeStatus,
    mustJSON(map[string]any{
        "previous_status": oldStatus,
        "new_status":      status,
    }),
)
defer func() {
    extra := json.RawMessage(`{}`)
    if err != nil {
        extra = mustJSON(map[string]any{"error": err.Error()})
    }
    _ = c.worklogStore.EndWorklog(context.Background(), wlID, extra)
}()
```

`mustJSON` and `int64Ptr` are tiny helpers in `internal/conversation/util.go` (or inline). Errors from `StartWorklog`/`EndWorklog` are logged but **never** propagated — instrumentation must not change action semantics. Same shape for the four hook sites.

### Forward hook (Q3, in [cmd/messages.go](cmd/messages.go))

```go
// inside handleForwardMessage, immediately after parsing the request and
// resolving conversationUUID/orig:
wlID, _ := app.worklog.StartWorklog(
    r.RequestCtx,
    int64(conv.ID),
    int64Ptr(user.ID),
    wmodels.ActionTypeForward,
    mustJSON(map[string]any{
        "original_message_uuid": orig.UUID,
        "to_count":              len(req.To),
        "has_comment":           strings.TrimSpace(req.Comment) != "",
    }),
)
defer app.worklog.EndWorklog(context.Background(), wlID, nil)
```

---

## 6. Tests

`internal/worklog/worklog_test.go` — `t.Skip` when `DATABASE_URL` is unset. Uses a per-test transaction that rolls back at the end (no shared mutable state between tests).

1. `TestStartWorklog_InsertsRowAndReturnsID` — happy path, valid `action_type`.
2. `TestStartWorklog_RejectsInvalidActionType` — DB CHECK constraint surfaces an error.
3. `TestStartWorklog_AcceptsNilUserID` — system-initiated action.
4. `TestStartWorklog_FailsOnUnknownConversation` — FK violation on bogus `conversation_id`.
5. `TestEndWorklog_ComputesDurationSeconds` — Start → sleep ≥1s → End → assert `duration_seconds ≥ 1`.
6. `TestEndWorklog_MergesContextExtra` — confirm `||` shallow-merge semantics; right-side overrides on key conflict.
7. `TestEndWorklog_NoOpOnAlreadyEnded` — second End is silently dropped by the `AND ended_at IS NULL` guard.
8. `TestGetWorklogsByConversation_RoundTrip` — insert N worklogs across two conversations; fetch one; assert count + ordering DESC by `started_at`.
9. `TestGetWorklogsByUser_RoundTrip` — same shape, by user.
10. `TestMultiActionConversation` — single conversation, all four action types end-to-end; assert four rows, durations recorded, ordering correct.
11. `TestPagination` — insert 25 rows; `PageSize=10` returns 10/10/5.
12. `TestAutomationLevelDefaultsToL0` — fresh row has `automation_level = 'L0'`.

Test helper `mustOpenDB(t)` reads `DATABASE_URL`; on miss, `t.Skip("set DATABASE_URL to run worklog DB tests")`.

---

## 7. CI follow-up (out of scope for B1, tracked here)

[.github/workflows/go.yml](.github/workflows/go.yml) currently runs `make test` against no DB. Follow-up PR after B1 lands:

```yaml
services:
  postgres:
    image: postgres:16-alpine
    env:
      POSTGRES_USER: libredesk
      POSTGRES_PASSWORD: libredesk
      POSTGRES_DB: libredesk_test
    ports: ['5432:5432']
    options: >-
      --health-cmd "pg_isready -U libredesk"
      --health-interval 10s --health-timeout 5s --health-retries 5
env:
  DATABASE_URL: postgres://libredesk:libredesk@localhost:5432/libredesk_test?sslmode=disable
```

…plus a step that runs the install/migration flow against the empty DB before `make test`. Filed as a comment in the B1 PR description, not blocking B1 merge.

---

## 8. Permission integration map

| Layer | Where | Change |
|---|---|---|
| Backend constant + validator | [internal/authz/models/models.go](internal/authz/models/models.go) | Add 3 `PermWorklog*` constants + 3 keys in `validPermissions`. |
| DB seed (fresh installs) | [schema.sql:754-769](schema.sql) | Append to Agent + Admin role permission arrays. |
| DB seed (upgraded instances) | `internal/migrations/v2_3_0.go` | Idempotent `UPDATE roles SET permissions = permissions ‖ ...`. |
| Frontend constant | [frontend/apps/main/src/constants/permissions.js](frontend/apps/main/src/constants/permissions.js) | Add 3 JS keys. |
| Future read endpoints (NOT in this PR) | [cmd/handlers.go](cmd/handlers.go) | Documented in §9 below; gated by `perm("worklog:view_team", ...)`. |

Casbin enforces on the `obj:act` split — `worklog:view_team` becomes `(obj=worklog, act=view_team)` per [internal/authz/authz.go:78-83](internal/authz/authz.go).

---

## 9. Future hookup points (not in this PR — documented for the Reports work)

The manager methods exist in this PR but are **not** exposed via HTTP. When the Reports work lands, here's where each piece plugs in:

```go
// cmd/handlers.go — alongside existing routes:
g.GET("/api/conversations/:uuid/worklogs", perm("worklog:view_team", handleGetConversationWorklogs))
g.GET("/api/users/:id/worklogs",          perm("worklog:view_team", handleGetUserWorklogs))
g.GET("/api/reports/worklogs",            perm("worklog:view_all",  handleGetWorklogReport))

// cmd/worklog.go (new file in the Reports PR):
func handleGetConversationWorklogs(r *fastglue.Request) error { /* uses app.worklog.GetWorklogsByConversation */ }
func handleGetUserWorklogs(r *fastglue.Request) error         { /* uses app.worklog.GetWorklogsByUser */ }
```

`worklog:view_own` is reserved for the agent's own self-service "what did I do today" view (filters `user_id = current_user.id`).

---

## 10. Hard-gate findings (Q4 + Q5)

These were verified by reading the files; documenting in case anyone touches this plan later.

### Permissions architecture

- Permissions are **not** a separate table. They're a `TEXT[]` column on `roles` ([schema.sql:118-126](schema.sql)).
- Validity is enforced by a **hardcoded Go map** `validPermissions` in [internal/authz/models/models.go](internal/authz/models/models.go). Anything not in this map is silently dropped by `role.filterValidPermissions` ([role.go:150-160](internal/role/role.go)). Adding a new permission to the frontend without updating this map is a no-op.
- Default roles are seeded inline at [schema.sql:754-769](schema.sql). This runs **only on fresh installs**. No existing migration ever updates seeded role rows on upgraded instances — B1's migration is the first one that needs to.
- Casbin model in [internal/authz/authz.go:30-42](internal/authz/authz.go) splits permission strings on `:` into `(obj, act)`. `worklog:view_team` enforces as `(sub=user_id, obj=worklog, act=view_team)`.

### Conversation manager DI

- `Manager` struct at [internal/conversation/conversation.go:60-88](internal/conversation/conversation.go); each store dependency is an unexported interface defined in the same file (lines 107-167).
- `New(...)` is positional with 14 arguments + `Opts` ([conversation.go:184-200](internal/conversation/conversation.go)).
- Construction lives in `initConversations(...)` at [cmd/init.go:282-311](cmd/init.go), called from [cmd/main.go:228](cmd/main.go).
- The plan adds `worklogStore` as a 15th positional argument to `New` and threads it through `initConversations`. Both call sites are updated in this PR.

---

## 11. Acceptance criteria

- [ ] `go test ./...` passes with `DATABASE_URL` set; cleanly skips when unset.
- [ ] Existing `internal/conversation` tests still pass (no semantic change to action paths).
- [ ] Fresh install (`make build` + first-run): Admin role has `worklog:view_*`; Agent role has `worklog:view_own`.
- [ ] Upgrade path: existing instance running pre-v2.3.0, after migration runs, the same statement holds.
- [ ] Worklog row appears in `worklogs` for every: status change, agent reply via `QueueReply`, private note via `SendPrivateNote`, forward via `handleForwardMessage`. Verified by manual click-through against a dev DB.
- [ ] Errored actions still produce a worklog with `ended_at` set and an `error` key in `context`.
- [ ] `automation_level` defaults to `'L0'` on every row; no other value is written by this PR.
- [ ] Down SQL in the `v2_3_0.go` header comment reverses the up migration cleanly on a test DB.

---

*Re-read §0 and §10 before code-gen. Once approved, the file order to write in is: models → queries.sql → worklog.go → worklog_test.go → migration (with rollback comment block) → schema.sql → permissions.js + authz/models → conversation hooks → cmd/messages.go forward hook → init.go + main.go wiring.*
