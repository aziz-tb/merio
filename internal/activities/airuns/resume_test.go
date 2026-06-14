package airuns

import (
	"context"
	"testing"
	"time"

	"github.com/aziz-tb/merio/internal/ai/runner"
	convmodels "github.com/aziz-tb/merio/internal/conversation/models"
	"github.com/stretchr/testify/require"
)

// fakeResumeRunner records each RunRequest and returns scripted responses, so
// runResume's orchestration (which run gets which Resume) is asserted without a
// live Runner — the actual execute/inject mechanisms are covered in the runner
// package (Slice 4a).
type fakeResumeRunner struct {
	reqs  []runner.RunRequest
	resps []runner.RunResponse
}

func (f *fakeResumeRunner) Run(_ context.Context, req runner.RunRequest) (runner.RunResponse, error) {
	f.reqs = append(f.reqs, req)
	i := len(f.reqs) - 1
	if i < len(f.resps) {
		return f.resps[i], nil
	}
	return runner.RunResponse{Status: runner.StatusComplete}, nil
}

func strptr(s string) *string { return &s }
func intptr(i int) *int       { return &i }

// TestGateCreateParams covers the gate→record mapping for both origins: a
// sub-agent gate (parent + agent set) and an orchestrator-own gate (neither).
func TestGateCreateParams(t *testing.T) {
	t.Run("sub-agent gate carries parent + agent", func(t *testing.T) {
		pw := &runner.ProposedWrite{
			ToolID: 7, ToolSlug: "cancel", ToolUseID: "w1", Inputs: map[string]any{"id": 1},
			GatedTraceID: "sub-trace", ParentTraceID: "comp-trace", AgentID: intptr(3),
		}
		p, ok := gateCreateParams(pw, 5)
		require.True(t, ok)
		require.Equal(t, int64(5), p.ConversationID)
		require.Equal(t, "sub-trace", p.GatedRunTraceID)
		require.NotNil(t, p.ParentTraceID)
		require.Equal(t, "comp-trace", *p.ParentTraceID)
		require.NotNil(t, p.AgentID)
		require.Equal(t, 3, *p.AgentID)
		require.Equal(t, 7, p.ToolID)
		require.JSONEq(t, `{"id":1}`, string(p.ProposedInputs))
	})
	t.Run("orchestrator-own gate has no parent or agent", func(t *testing.T) {
		p, ok := gateCreateParams(&runner.ProposedWrite{ToolID: 8, GatedTraceID: "comp-trace", ToolUseID: "w2"}, 5)
		require.True(t, ok)
		require.Nil(t, p.ParentTraceID)
		require.Nil(t, p.AgentID)
	})
	t.Run("nil / missing gated trace -> not ok", func(t *testing.T) {
		_, ok := gateCreateParams(nil, 5)
		require.False(t, ok)
		_, ok = gateCreateParams(&runner.ProposedWrite{ToolID: 1}, 5)
		require.False(t, ok)
	})
}

// TestRunResume_SubAgentTwoStep is the contract-sensitive flow: a sub-agent
// gate resumes the sub-agent (executing the approved write), its report is fed
// back into the composer at the call_subagent point, and the composer composes.
func TestRunResume_SubAgentTwoStep(t *testing.T) {
	report := "Billing: balance is 80€, Q2 invoice unpaid."
	answer := "Your balance is 80€; the Q2 invoice is unpaid."
	subID := 7

	fr := &fakeResumeRunner{resps: []runner.RunResponse{
		// Step 1 — sub-agent resume produces its report (draft).
		{Status: runner.StatusComplete, Draft: strptr(report)},
		// Step 2 — composer resume composes from the injected report.
		{Status: runner.StatusComplete, Draft: strptr(answer),
			Decision: &runner.TerminalDecision{Kind: runner.DecisionKindAnswer}},
	}}

	resp, err := runResume(context.Background(), fr, nil, resumeInputs{
		subAgentGate:  true,
		agentID:       intptr(subID),
		toolSlug:      "cancel_contract",
		toolUseID:     "w1",
		inputs:        map[string]any{"id": 1},
		gatedTrace:    []runner.TurnTrace{{TurnIndex: 0, Text: "sub-agent reasoning"}},
		composerTrace: []runner.TurnTrace{{TurnIndex: 0, Text: "composer delegated to billing"}},
		resumeTraceID: "resume-1",
		seed:          resumeSeed{conversationID: 5, inboxID: 1, inbound: "balance?"},
	})
	require.NoError(t, err)

	// The composer composed the final answer from the resumed report.
	require.Equal(t, runner.StatusComplete, resp.Status)
	require.NotNil(t, resp.Decision)
	require.Equal(t, runner.DecisionKindAnswer, resp.Decision.Kind)
	require.Equal(t, answer, *resp.Draft)

	require.Len(t, fr.reqs, 2, "two runs: sub-agent resume then composer resume")

	// Step 1: the sub-agent resume executes the approved write on the sub trace.
	sub := fr.reqs[0]
	require.Equal(t, subID, sub.AgentID)
	require.False(t, sub.IsComposer)
	require.NotNil(t, sub.Resume)
	require.True(t, sub.Resume.ExecuteWrite)
	require.False(t, sub.Resume.Rejected)
	require.Equal(t, "cancel_contract", sub.Resume.ToolName)
	require.Equal(t, 1, sub.Resume.Inputs["id"])
	require.Equal(t, "sub-agent reasoning", sub.Resume.PriorTrace[0].Text)

	// Step 2: the composer resume injects the sub-agent's report at call_subagent.
	comp := fr.reqs[1]
	require.True(t, comp.IsComposer)
	require.NotNil(t, comp.Resume)
	require.Equal(t, runner.CallSubagentToolName, comp.Resume.ToolName)
	require.NotNil(t, comp.Resume.InjectedResult)
	require.Contains(t, *comp.Resume.InjectedResult, "80€", "the resumed report flowed into the composer")
	require.Equal(t, "composer delegated to billing", comp.Resume.PriorTrace[0].Text)
}

// TestRunResume_SubAgentReject proves a rejected sub-agent write is fed back as
// a rejection (no execution) and the composer still composes.
func TestRunResume_SubAgentReject(t *testing.T) {
	fr := &fakeResumeRunner{resps: []runner.RunResponse{
		{Status: runner.StatusComplete, Draft: strptr("sub could not cancel")},
		{Status: runner.StatusComplete, Draft: strptr("A colleague will follow up."),
			Decision: &runner.TerminalDecision{Kind: runner.DecisionKindAnswer}},
	}}
	_, err := runResume(context.Background(), fr, nil, resumeInputs{
		subAgentGate: true, agentID: intptr(7),
		toolSlug: "cancel_contract", rejected: true, rejectionReason: "needs sign-off",
		gatedTrace: []runner.TurnTrace{{}}, composerTrace: []runner.TurnTrace{{}},
		resumeTraceID: "r", seed: resumeSeed{},
	})
	require.NoError(t, err)
	require.Len(t, fr.reqs, 2)
	require.True(t, fr.reqs[0].Resume.Rejected)
	require.False(t, fr.reqs[0].Resume.ExecuteWrite)
	require.Equal(t, "needs sign-off", fr.reqs[0].Resume.RejectionReason)
}

// TestRunResume_ComposerOwn proves an orchestrator-own write is a single-loop
// resume (IsComposer, execute the write, continue) — no second run.
func TestRunResume_ComposerOwn(t *testing.T) {
	fr := &fakeResumeRunner{resps: []runner.RunResponse{
		{Status: runner.StatusComplete, Draft: strptr("done"),
			Decision: &runner.TerminalDecision{Kind: runner.DecisionKindAnswer}},
	}}
	_, err := runResume(context.Background(), fr, nil, resumeInputs{
		subAgentGate: false, agentID: nil, // orchestrator-own
		toolSlug: "issue_credit", inputs: map[string]any{"amount": 10},
		gatedTrace: []runner.TurnTrace{{}}, resumeTraceID: "r", seed: resumeSeed{},
	})
	require.NoError(t, err)
	require.Len(t, fr.reqs, 1, "orchestrator-own write is a single resume")
	require.True(t, fr.reqs[0].IsComposer)
	require.Equal(t, 0, fr.reqs[0].AgentID)
	require.True(t, fr.reqs[0].Resume.ExecuteWrite)
	require.Equal(t, "issue_credit", fr.reqs[0].Resume.ToolName)
}

// TestRunResume_Held proves a staleness HOLD on an orchestrator-own write feeds
// the held notice (no execution) on a single resume — the 4c failure-mode fix:
// a customer message during the approval window holds the write.
func TestRunResume_Held(t *testing.T) {
	fr := &fakeResumeRunner{resps: []runner.RunResponse{
		{Status: runner.StatusComplete, Draft: strptr("re-checked — not cancelling"),
			Decision: &runner.TerminalDecision{Kind: runner.DecisionKindAnswer}},
	}}
	_, err := runResume(context.Background(), fr, nil, resumeInputs{
		subAgentGate: false, agentID: nil, // orchestrator-own
		toolSlug: "cancel_contract", inputs: map[string]any{"id": 1},
		held: true, heldReason: `the customer replied while it awaited approval: "wait, don't cancel"`,
		gatedTrace: []runner.TurnTrace{{}}, resumeTraceID: "r", seed: resumeSeed{},
	})
	require.NoError(t, err)
	require.Len(t, fr.reqs, 1)
	require.True(t, fr.reqs[0].IsComposer)
	require.NotNil(t, fr.reqs[0].Resume)
	require.True(t, fr.reqs[0].Resume.Held)
	require.False(t, fr.reqs[0].Resume.ExecuteWrite, "a held write must NOT execute")
	require.False(t, fr.reqs[0].Resume.Rejected)
	require.Contains(t, fr.reqs[0].Resume.HeldReason, "don't cancel")
}

// TestRunResume_SubAgentHeld proves the hold propagates through the 2-step: the
// sub-agent step holds (does not execute) the approved write, and the composer
// still composes from the resumed report. The orchestration is unchanged — only
// the copied Resume carries Held.
func TestRunResume_SubAgentHeld(t *testing.T) {
	fr := &fakeResumeRunner{resps: []runner.RunResponse{
		{Status: runner.StatusComplete, Draft: strptr("held the cancel; awaiting guidance")},
		{Status: runner.StatusComplete, Draft: strptr("You asked to keep it — no changes made."),
			Decision: &runner.TerminalDecision{Kind: runner.DecisionKindAnswer}},
	}}
	_, err := runResume(context.Background(), fr, nil, resumeInputs{
		subAgentGate: true, agentID: intptr(7),
		toolSlug: "cancel_contract", inputs: map[string]any{"id": 1},
		held: true, heldReason: `the customer replied while it awaited approval: "keep my plan"`,
		gatedTrace:    []runner.TurnTrace{{}},
		composerTrace: []runner.TurnTrace{{}},
		resumeTraceID: "r", seed: resumeSeed{},
	})
	require.NoError(t, err)
	require.Len(t, fr.reqs, 2)
	// Step 1: the sub-agent HOLDS the write (no execute).
	require.True(t, fr.reqs[0].Resume.Held)
	require.False(t, fr.reqs[0].Resume.ExecuteWrite)
	require.Contains(t, fr.reqs[0].Resume.HeldReason, "keep my plan")
	// Step 2: composer still composes from the (held) report.
	require.True(t, fr.reqs[1].IsComposer)
	require.Equal(t, runner.CallSubagentToolName, fr.reqs[1].Resume.ToolName)
	require.NotNil(t, fr.reqs[1].Resume.InjectedResult)
}

// TestGapAndHistoryHelpers covers the pure 4c logic: gap detection (contact
// messages after the approval), the held reason, and history mapping.
func TestGapAndHistoryHelpers(t *testing.T) {
	base := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	msgs := []convmodels.Message{
		{SenderType: convmodels.SenderTypeContact, Content: "please cancel", CreatedAt: base.Add(-time.Minute)},
		{SenderType: convmodels.SenderTypeAgent, Content: "on it", CreatedAt: base.Add(time.Minute)},
		{SenderType: convmodels.SenderTypeContact, TextContent: "wait, don't cancel", CreatedAt: base.Add(2 * time.Minute)},
	}

	// Gap = only the contact message AFTER the approval timestamp.
	gap := gapCustomerMessages(msgs, base)
	require.Len(t, gap, 1)
	require.Equal(t, "wait, don't cancel", messageText(gap[0]))
	require.Contains(t, formatGapReason(gap), "wait, don't cancel")

	// An agent message in the window is NOT a gap (only customer replies hold).
	require.Empty(t, gapCustomerMessages([]convmodels.Message{
		{SenderType: convmodels.SenderTypeAgent, Content: "fyi", CreatedAt: base.Add(time.Minute)},
	}, base))

	// History maps contact→customer, prefers TextContent, drops empties.
	hist := toRunnerHistory(msgs)
	require.Len(t, hist, 3)
	require.Equal(t, "customer", hist[0].Sender)
	require.Equal(t, "please cancel", hist[0].Text)
	require.Equal(t, "agent", hist[1].Sender)
	require.Equal(t, "wait, don't cancel", hist[2].Text)
	require.Empty(t, toRunnerHistory([]convmodels.Message{{SenderType: convmodels.SenderTypeContact, Content: "  "}}))
}

// TestRunResume_SingleAgent proves a single-agent write resumes that agent (not
// composer), single loop.
func TestRunResume_SingleAgent(t *testing.T) {
	fr := &fakeResumeRunner{resps: []runner.RunResponse{
		{Status: runner.StatusComplete, Draft: strptr("ok")},
	}}
	_, err := runResume(context.Background(), fr, nil, resumeInputs{
		subAgentGate: false, agentID: intptr(3), // single agent, no parent
		toolSlug: "update_address", inputs: map[string]any{"street": "x"},
		gatedTrace: []runner.TurnTrace{{}}, resumeTraceID: "r", seed: resumeSeed{},
	})
	require.NoError(t, err)
	require.Len(t, fr.reqs, 1)
	require.False(t, fr.reqs[0].IsComposer)
	require.Equal(t, 3, fr.reqs[0].AgentID)
}
