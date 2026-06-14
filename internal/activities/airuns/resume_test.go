package airuns

import (
	"context"
	"testing"

	"github.com/aziz-tb/merio/internal/ai/runner"
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
