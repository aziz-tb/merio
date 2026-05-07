package worklog

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/abhinavxd/libredesk/internal/worklog/models"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// These tests require a live Postgres reachable via the DATABASE_URL env var.
// CI will skip them until the workflow grows a postgres service step (see
// B1-WORKLOG-PLAN.md §7). Run locally with:
//
//	DATABASE_URL='postgres://libredesk:libredesk@localhost:5432/libredesk_test?sslmode=disable' \
//	  go test ./internal/worklog/...
//
// The DB must already have the libredesk schema applied (worklogs table, plus
// users + conversations + their dependencies for FK satisfaction).

func mustOpenDB(t *testing.T) *sqlx.DB {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping worklog DB tests")
	}
	db, err := sqlx.Connect("postgres", url)
	if err != nil {
		t.Fatalf("failed to connect to %s: %v", url, err)
	}
	return db
}

// seedConversation inserts a minimum-viable conversation + its dependencies and
// returns the new conversation id + uuid + the contact user id. Cleanup is
// handled by the caller via t.Cleanup.
func seedConversation(t *testing.T, db *sqlx.DB) (convID int64, convUUID string, userID int64) {
	t.Helper()

	// Pick an existing inbox + status; both are seeded on install.
	var inboxID, statusID int
	if err := db.Get(&inboxID, `SELECT id FROM inboxes LIMIT 1`); err != nil {
		t.Fatalf("no inboxes seeded; install the app first: %v", err)
	}
	if err := db.Get(&statusID, `SELECT id FROM conversation_statuses ORDER BY id LIMIT 1`); err != nil {
		t.Fatalf("no conversation_statuses seeded; install the app first: %v", err)
	}

	// Throw-away contact.
	var contactID int64
	err := db.Get(&contactID, `
		INSERT INTO users (type, first_name, email)
		VALUES ('contact', 'wlog-test', $1)
		RETURNING id
	`, time.Now().UnixNano())
	if err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM users WHERE id = $1`, contactID) })

	// Throw-away conversation. Cascades wipe worklogs on delete.
	err = db.QueryRow(`
		INSERT INTO conversations (contact_id, inbox_id, status_id)
		VALUES ($1, $2, $3)
		RETURNING id, uuid
	`, contactID, inboxID, statusID).Scan(&convID, &convUUID)
	if err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM conversations WHERE id = $1`, convID) })

	return convID, convUUID, contactID
}

func newManager(t *testing.T, db *sqlx.DB) *Manager {
	t.Helper()
	m, err := New(Opts{DB: db, Lo: noopLogger(), I18n: noopI18n(t)})
	if err != nil {
		t.Fatalf("worklog.New: %v", err)
	}
	return m
}

func TestStartWorklog_InsertsRowAndReturnsID(t *testing.T) {
	db := mustOpenDB(t)
	convID, _, _ := seedConversation(t, db)
	m := newManager(t, db)

	ctxPayload := json.RawMessage(`{"reason":"test"}`)
	id, err := m.StartWorklog(context.Background(), convID, nil, models.ActionTypeStatus, ctxPayload)
	if err != nil {
		t.Fatalf("StartWorklog: %v", err)
	}
	if id == 0 {
		t.Fatalf("expected non-zero worklog id")
	}

	var (
		gotConv int64
		gotType string
		gotCtx  json.RawMessage
	)
	if err := db.QueryRow(
		`SELECT conversation_id, action_type, context FROM worklogs WHERE id = $1`, id,
	).Scan(&gotConv, &gotType, &gotCtx); err != nil {
		t.Fatalf("readback: %v", err)
	}
	if gotConv != convID || gotType != models.ActionTypeStatus {
		t.Fatalf("readback mismatch: conv=%d type=%s", gotConv, gotType)
	}
}

func TestStartWorklog_RejectsInvalidActionType(t *testing.T) {
	db := mustOpenDB(t)
	convID, _, _ := seedConversation(t, db)
	m := newManager(t, db)

	if _, err := m.StartWorklog(context.Background(), convID, nil, "bogus", nil); err == nil {
		t.Fatalf("expected CHECK constraint violation for invalid action_type")
	}
}

func TestStartWorklog_AcceptsNilUserID(t *testing.T) {
	db := mustOpenDB(t)
	convID, _, _ := seedConversation(t, db)
	m := newManager(t, db)

	id, err := m.StartWorklog(context.Background(), convID, nil, models.ActionTypeWrite, nil)
	if err != nil {
		t.Fatalf("StartWorklog: %v", err)
	}
	var u sql.NullInt64
	if err := db.QueryRow(`SELECT user_id FROM worklogs WHERE id = $1`, id).Scan(&u); err != nil {
		t.Fatalf("readback: %v", err)
	}
	if u.Valid {
		t.Fatalf("expected user_id to be NULL, got %d", u.Int64)
	}
}

func TestStartWorklog_FailsOnUnknownConversation(t *testing.T) {
	db := mustOpenDB(t)
	m := newManager(t, db)

	if _, err := m.StartWorklog(context.Background(), -1, nil, models.ActionTypeStatus, nil); err == nil {
		t.Fatalf("expected FK violation for unknown conversation")
	}
}

func TestStartWorklogByUUID_NoOpOnUnknownUUID(t *testing.T) {
	db := mustOpenDB(t)
	m := newManager(t, db)

	id, err := m.StartWorklogByUUID(context.Background(), "00000000-0000-0000-0000-000000000000", nil, models.ActionTypeStatus, nil)
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows for unknown uuid, got id=%d err=%v", id, err)
	}
}

func TestEndWorklog_ComputesDurationSeconds(t *testing.T) {
	db := mustOpenDB(t)
	convID, _, _ := seedConversation(t, db)
	m := newManager(t, db)

	id, err := m.StartWorklog(context.Background(), convID, nil, models.ActionTypeWrite, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	if err := m.EndWorklog(context.Background(), id, nil); err != nil {
		t.Fatal(err)
	}

	var dur sql.NullInt64
	if err := db.QueryRow(`SELECT duration_seconds FROM worklogs WHERE id = $1`, id).Scan(&dur); err != nil {
		t.Fatal(err)
	}
	if !dur.Valid || dur.Int64 < 1 {
		t.Fatalf("expected duration_seconds >= 1, got valid=%v val=%d", dur.Valid, dur.Int64)
	}
}

func TestEndWorklog_MergesContextExtra(t *testing.T) {
	db := mustOpenDB(t)
	convID, _, _ := seedConversation(t, db)
	m := newManager(t, db)

	id, err := m.StartWorklog(context.Background(), convID, nil, models.ActionTypeWrite,
		json.RawMessage(`{"a":1,"b":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.EndWorklog(context.Background(), id, json.RawMessage(`{"b":99,"c":3}`)); err != nil {
		t.Fatal(err)
	}

	var got json.RawMessage
	if err := db.QueryRow(`SELECT context FROM worklogs WHERE id = $1`, id).Scan(&got); err != nil {
		t.Fatal(err)
	}
	var parsed map[string]int
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("unmarshal: %v (raw=%s)", err, string(got))
	}
	if parsed["a"] != 1 || parsed["b"] != 99 || parsed["c"] != 3 {
		t.Fatalf("expected shallow-merge {a:1,b:99,c:3}, got %v", parsed)
	}
}

func TestEndWorklog_NoOpOnAlreadyEnded(t *testing.T) {
	db := mustOpenDB(t)
	convID, _, _ := seedConversation(t, db)
	m := newManager(t, db)

	id, err := m.StartWorklog(context.Background(), convID, nil, models.ActionTypeNote, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.EndWorklog(context.Background(), id, nil); err != nil {
		t.Fatal(err)
	}
	var firstEnd time.Time
	if err := db.QueryRow(`SELECT ended_at FROM worklogs WHERE id = $1`, id).Scan(&firstEnd); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := m.EndWorklog(context.Background(), id, nil); err != nil {
		t.Fatal(err)
	}
	var secondEnd time.Time
	if err := db.QueryRow(`SELECT ended_at FROM worklogs WHERE id = $1`, id).Scan(&secondEnd); err != nil {
		t.Fatal(err)
	}
	if !firstEnd.Equal(secondEnd) {
		t.Fatalf("expected ended_at unchanged after second End, got %v then %v", firstEnd, secondEnd)
	}
}

func TestGetWorklogsByConversation_RoundTrip(t *testing.T) {
	db := mustOpenDB(t)
	a, _, _ := seedConversation(t, db)
	b, _, _ := seedConversation(t, db)
	m := newManager(t, db)

	for _, conv := range []int64{a, a, a, b} {
		if _, err := m.StartWorklog(context.Background(), conv, nil, models.ActionTypeStatus, nil); err != nil {
			t.Fatal(err)
		}
	}

	got, err := m.GetWorklogsByConversation(context.Background(), a, models.ListOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 worklogs for a, got %d", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i].StartedAt.After(got[i-1].StartedAt) {
			t.Fatalf("expected DESC by started_at; row %d > row %d", i, i-1)
		}
	}
}

func TestGetWorklogsByUser_RoundTrip(t *testing.T) {
	db := mustOpenDB(t)
	convID, _, contactID := seedConversation(t, db)
	m := newManager(t, db)

	uid := contactID
	for i := 0; i < 4; i++ {
		if _, err := m.StartWorklog(context.Background(), convID, &uid, models.ActionTypeWrite, nil); err != nil {
			t.Fatal(err)
		}
	}

	got, err := m.GetWorklogsByUser(context.Background(), uid, models.ListOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 worklogs for user, got %d", len(got))
	}
}

func TestMultiActionConversation(t *testing.T) {
	db := mustOpenDB(t)
	convID, _, _ := seedConversation(t, db)
	m := newManager(t, db)

	for _, at := range []string{
		models.ActionTypeStatus,
		models.ActionTypeWrite,
		models.ActionTypeNote,
		models.ActionTypeForward,
	} {
		id, err := m.StartWorklog(context.Background(), convID, nil, at, nil)
		if err != nil {
			t.Fatalf("Start %s: %v", at, err)
		}
		if err := m.EndWorklog(context.Background(), id, nil); err != nil {
			t.Fatalf("End %s: %v", at, err)
		}
	}

	got, err := m.GetWorklogsByConversation(context.Background(), convID, models.ListOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(got))
	}
	for _, w := range got {
		if w.EndedAt == nil {
			t.Fatalf("row %d has nil ended_at", w.ID)
		}
		if w.DurationSeconds == nil {
			t.Fatalf("row %d has nil duration_seconds", w.ID)
		}
	}
}

func TestPagination(t *testing.T) {
	db := mustOpenDB(t)
	convID, _, _ := seedConversation(t, db)
	m := newManager(t, db)

	for i := 0; i < 25; i++ {
		if _, err := m.StartWorklog(context.Background(), convID, nil, models.ActionTypeStatus, nil); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		page    int
		perPage int
		want    int
	}{
		{1, 10, 10},
		{2, 10, 10},
		{3, 10, 5},
	}
	for _, c := range cases {
		got, err := m.GetWorklogsByConversation(
			context.Background(), convID,
			models.ListOpts{Page: c.page, PageSize: c.perPage},
		)
		if err != nil {
			t.Fatalf("page=%d: %v", c.page, err)
		}
		if len(got) != c.want {
			t.Fatalf("page=%d: want %d rows, got %d", c.page, c.want, len(got))
		}
	}
}

func TestAutomationLevelDefaultsToL0(t *testing.T) {
	db := mustOpenDB(t)
	convID, _, _ := seedConversation(t, db)
	m := newManager(t, db)

	id, err := m.StartWorklog(context.Background(), convID, nil, models.ActionTypeStatus, nil)
	if err != nil {
		t.Fatal(err)
	}
	var lvl string
	if err := db.QueryRow(`SELECT automation_level FROM worklogs WHERE id = $1`, id).Scan(&lvl); err != nil {
		t.Fatal(err)
	}
	if lvl != models.AutomationLevelL0 {
		t.Fatalf("expected automation_level=L0, got %q", lvl)
	}
}
