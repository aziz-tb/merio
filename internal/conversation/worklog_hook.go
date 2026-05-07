package conversation

import (
	"context"
	"encoding/json"

	wlmodels "github.com/abhinavxd/libredesk/internal/worklog/models"
)

// startWorklog opens a worklog entry for an action on the given conversation
// (by int64 ID) and returns the new id. Errors are logged and swallowed —
// instrumentation must never alter action semantics. A returned id of 0 means
// "no worklog row was created" and is safe to pass to endWorklog (no-op).
func (c *Manager) startWorklog(convID int, userID int, actionType string, ctxFields map[string]any) int64 {
	if c.worklogStore == nil || convID <= 0 {
		return 0
	}
	payload := marshalContext(ctxFields)
	uid := userPtr(userID)
	id, err := c.worklogStore.StartWorklog(context.Background(), int64(convID), uid, actionType, payload)
	if err != nil {
		c.lo.Warn("worklog start failed", "error", err, "action_type", actionType, "conversation_id", convID)
		return 0
	}
	return id
}

// startWorklogByUUID is startWorklog using a conversation UUID — useful when
// the int ID isn't already loaded at the call site (QueueReply, SendPrivateNote).
func (c *Manager) startWorklogByUUID(convUUID string, userID int, actionType string, ctxFields map[string]any) int64 {
	if c.worklogStore == nil || convUUID == "" {
		return 0
	}
	payload := marshalContext(ctxFields)
	uid := userPtr(userID)
	id, err := c.worklogStore.StartWorklogByUUID(context.Background(), convUUID, uid, actionType, payload)
	if err != nil {
		// sql.ErrNoRows is silent: a worklog for a non-existent conversation
		// would have failed anyway, and this matches the no-op contract.
		if err.Error() != "sql: no rows in result set" {
			c.lo.Warn("worklog start failed", "error", err, "action_type", actionType, "conversation_uuid", convUUID)
		}
		return 0
	}
	return id
}

// endWorklog closes a worklog. errReason is non-nil only when the underlying
// action failed; in that case its message is merged into the row's context.
// A worklogID of 0 (start failed/skipped) is a no-op.
func (c *Manager) endWorklog(worklogID int64, errReason error) {
	if c.worklogStore == nil || worklogID == 0 {
		return
	}
	var extra json.RawMessage
	if errReason != nil {
		if b, err := json.Marshal(map[string]any{"error": errReason.Error()}); err == nil {
			extra = b
		}
	}
	if err := c.worklogStore.EndWorklog(context.Background(), worklogID, extra); err != nil {
		c.lo.Warn("worklog end failed", "error", err, "worklog_id", worklogID)
	}
}

func marshalContext(fields map[string]any) json.RawMessage {
	if len(fields) == 0 {
		return nil
	}
	b, err := json.Marshal(fields)
	if err != nil {
		return nil
	}
	return b
}

func userPtr(id int) *int64 {
	if id <= 0 {
		return nil
	}
	v := int64(id)
	return &v
}

// Force a package-level reference to the worklog models so a grep for the
// import alias in this file always lands on something — preserves the
// explicit dependency for readers.
var _ = wlmodels.ActionTypeWrite
