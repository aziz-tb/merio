package airuns

import (
	"encoding/json"

	"github.com/aziz-tb/merio/internal/ai/runner"
	"github.com/aziz-tb/merio/internal/approvals"
	"github.com/aziz-tb/merio/internal/composer"
	wfairuns "github.com/aziz-tb/merio/internal/workflows/airuns"
)

// approvalSlugs resolves the inbox's approval-required write-tool slugs from the
// single global composer config (Slice 3 resolver). Best-effort: a resolution
// failure yields no gating (the run proceeds) rather than failing the run.
func (a *Activities) approvalSlugs(inboxID int) []string {
	if a.Composer == nil {
		return nil
	}
	row, err := a.Composer.GetConfig()
	if err != nil {
		if a.Lo != nil {
			a.Lo.Warn("approval slugs: get composer config failed", "error", err)
		}
		return nil
	}
	cfg, err := row.Decode()
	if err != nil {
		if a.Lo != nil {
			a.Lo.Warn("approval slugs: decode composer config failed", "error", err)
		}
		return nil
	}
	return composer.Resolve(cfg, inboxID).Automation.ApprovalRequiredToolSlugs
}

// gateCreateParams maps a runner ProposedWrite to the approval-record insert.
// Pure — the activity supplies the conversation id from params. The identity
// (gated trace / parent / agent) is carried on the ProposedWrite so this works
// uniformly for an orchestrator-own gate and a bubbled sub-agent gate.
func gateCreateParams(pw *runner.ProposedWrite, conversationID int) (approvals.CreateParams, bool) {
	if pw == nil || pw.GatedTraceID == "" {
		return approvals.CreateParams{}, false
	}
	var parent *string
	if pw.ParentTraceID != "" {
		p := pw.ParentTraceID
		parent = &p
	}
	inputs, _ := json.Marshal(pw.Inputs)
	return approvals.CreateParams{
		ConversationID:  int64(conversationID),
		GatedRunTraceID: pw.GatedTraceID,
		ParentTraceID:   parent,
		AgentID:         pw.AgentID,
		ToolID:          pw.ToolID,
		ToolUseID:       pw.ToolUseID,
		ProposedInputs:  inputs,
	}, true
}

// persistGateApproval inserts the pending approval record for a gated run.
// Single owner = the activity; idempotent via the partial-unique index (ON
// CONFLICT), so a Temporal activity retry records the write exactly once.
// Best-effort: a failure is logged, not fatal (the run already paused).
func (a *Activities) persistGateApproval(params wfairuns.AgentRunWorkflowParams, pw *runner.ProposedWrite) {
	if a.Approvals == nil {
		return
	}
	p, ok := gateCreateParams(pw, params.ConversationID)
	if !ok {
		return
	}
	if _, err := a.Approvals.Create(p); err != nil && a.Lo != nil {
		a.Lo.Warn("failed to persist write approval", "trace_id", pw.GatedTraceID, "error", err)
	}
}
