package runner

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	agentmodels "github.com/aziz-tb/merio/internal/ai/agents/models"
	"github.com/aziz-tb/merio/internal/ai/llm"
	aitools "github.com/aziz-tb/merio/internal/ai/tools"
	"github.com/aziz-tb/merio/internal/ai/tools/builtin"
	toolmodels "github.com/aziz-tb/merio/internal/ai/tools/models"
	"github.com/aziz-tb/merio/internal/ai/tools/resolver"
	execModels "github.com/aziz-tb/merio/internal/exec/runner/models"
	"github.com/stretchr/testify/require"
	"github.com/zerodha/logf"
)

// fakeResolver returns a fixed ResolvedTool list, bypassing the DB so gate +
// resume tests can include a custom write tool.
type fakeResolver struct{ tools []resolver.ResolvedTool }

func (f fakeResolver) Resolve(json.RawMessage, builtin.KBScope) ([]resolver.ResolvedTool, map[int]string, error) {
	return f.tools, nil, nil
}

// fakeDispatcher records every Dispatch call (to assert a write executes
// exactly once and with which inputs) and returns a canned success.
type fakeDispatcher struct {
	calls  []aitools.DispatchRequest
	result *execModels.Result
}

func (f *fakeDispatcher) Dispatch(_ context.Context, req aitools.DispatchRequest) (*execModels.Result, error) {
	f.calls = append(f.calls, req)
	if f.result != nil {
		return f.result, nil
	}
	return &execModels.Result{Kind: execModels.KindSuccess, Data: json.RawMessage(`{"status":"cancelled"}`), ExecutionID: "exec-1"}, nil
}

func customWriteTool(id int, slug string) resolver.ResolvedTool {
	return resolver.ResolvedTool{
		Kind:   agentmodels.ToolRefKindCustom,
		Custom: &toolmodels.Tool{ID: id, Slug: slug, Name: slug, Category: "write", Enabled: true},
	}
}

func toolUseBlock(useID, name string, input map[string]any) llm.ContentBlock {
	return llm.ContentBlock{Type: llm.ContentBlockTypeToolUse, ToolUseID: useID, ToolName: name, ToolInput: input}
}

// newApprovalRunner builds a Runner with fake resolver + dispatcher so the gate
// and resume paths can run without a DB. The loaded agent (used only for
// non-composer runs) is enabled.
func newApprovalRunner(t *testing.T, prov *scriptedProvider, res ToolResolver, disp ToolDispatcher) *Runner {
	t.Helper()
	lo := logf.New(logf.Opts{Writer: io.Discard})
	r, err := New(Opts{
		Provider:   prov,
		Agents:     fakeAgentLoader{agent: agentmodels.Agent{ID: 2, Slug: "billing", Enabled: true, Prompt: "You are billing.", MaxIterations: 5, UpdatedAt: time.Now()}},
		Resolver:   res,
		Dispatcher: disp,
		PresetReg:  testPresetRegistry(t),
		Lo:         &lo,
	})
	require.NoError(t, err)
	return r
}

// TestFirstGatedToolUse covers gate detection (pure): a slug in the approval
// set that resolves to a custom tool yields a ProposedWrite carrying the FK
// tool_id; anything else returns nil.
func TestFirstGatedToolUse(t *testing.T) {
	resolved := map[string]resolver.ResolvedTool{
		"cancel_contract": customWriteTool(7, "cancel_contract"),
		"lookup_balance":  customWriteTool(8, "lookup_balance"),
	}
	approval := []string{"cancel_contract"}

	t.Run("gated slug -> proposed write with tool_id", func(t *testing.T) {
		pw := firstGatedToolUse([]llm.ContentBlock{toolUseBlock("tu1", "cancel_contract", map[string]any{"reason": "x"})}, resolved, approval)
		require.NotNil(t, pw)
		require.Equal(t, 7, pw.ToolID)
		require.Equal(t, "cancel_contract", pw.ToolSlug)
		require.Equal(t, "tu1", pw.ToolUseID)
		require.Equal(t, "x", pw.Inputs["reason"])
	})
	t.Run("non-gated slug -> nil", func(t *testing.T) {
		require.Nil(t, firstGatedToolUse([]llm.ContentBlock{toolUseBlock("tu2", "lookup_balance", nil)}, resolved, approval))
	})
	t.Run("gated slug not resolved (phantom) -> nil", func(t *testing.T) {
		require.Nil(t, firstGatedToolUse([]llm.ContentBlock{toolUseBlock("tu3", "cancel_contract", nil)}, map[string]resolver.ResolvedTool{}, approval))
	})
	t.Run("empty approval set -> nil", func(t *testing.T) {
		require.Nil(t, firstGatedToolUse([]llm.ContentBlock{toolUseBlock("tu4", "cancel_contract", nil)}, resolved, nil))
	})
}

// scriptedWrite is a one-turn provider that emits a single custom write call.
func scriptedWrite(slug string, input map[string]any) *scriptedProvider {
	return &scriptedProvider{responses: []llm.CompletionResponse{
		{StopReason: llm.StopReasonToolUse, Content: []llm.ContentBlock{toolUseBlock("w1", slug, input)}},
	}}
}

// TestRun_Gate_ComposerWrite proves the gate ends a COMPOSER run before
// executing a gated write, with the proposal captured and nothing dispatched.
func TestRun_Gate_ComposerWrite(t *testing.T) {
	disp := &fakeDispatcher{}
	r := newApprovalRunner(t, scriptedWrite("cancel_contract", map[string]any{"id": 1}),
		fakeResolver{tools: []resolver.ResolvedTool{customWriteTool(7, "cancel_contract")}}, disp)

	resp, err := r.Run(context.Background(), RunRequest{
		InboundMessage: "cancel my contract", TraceID: "t-gate-c", InboxID: 1, ConversationID: 5,
		IsComposer:            true,
		ApprovalRequiredSlugs: []string{"cancel_contract"},
	})
	require.NoError(t, err)
	require.Equal(t, StatusAwaitingApproval, resp.Status)
	require.Nil(t, resp.Decision, "a gated run must NOT be escalated by the fail-safe")
	require.NotNil(t, resp.ProposedWrite)
	require.Equal(t, 7, resp.ProposedWrite.ToolID)
	require.Equal(t, "cancel_contract", resp.ProposedWrite.ToolSlug)
	require.Equal(t, 1, resp.ProposedWrite.Inputs["id"])
	require.Empty(t, disp.calls, "the gated write must NOT execute before approval")
}

// TestRun_Gate_SubAgentWrite proves the SAME gate fires for a sub-agent
// (non-composer) run — slug-driven and origin-agnostic.
func TestRun_Gate_SubAgentWrite(t *testing.T) {
	disp := &fakeDispatcher{}
	r := newApprovalRunner(t, scriptedWrite("cancel_contract", map[string]any{"id": 2}),
		fakeResolver{tools: []resolver.ResolvedTool{customWriteTool(7, "cancel_contract")}}, disp)

	resp, err := r.Run(context.Background(), RunRequest{
		AgentID: 2, InboundMessage: "cancel", TraceID: "t-gate-s", InboxID: 1, ConversationID: 5,
		ApprovalRequiredSlugs: []string{"cancel_contract"},
	})
	require.NoError(t, err)
	require.Equal(t, StatusAwaitingApproval, resp.Status)
	require.NotNil(t, resp.ProposedWrite)
	require.Equal(t, "cancel_contract", resp.ProposedWrite.ToolSlug)
	require.Empty(t, disp.calls)
}

// TestRun_Gate_SubAgentBubblesUp proves a sub-agent that paused for approval
// ends the COMPOSER run awaiting approval too (origin-agnostic), surfacing the
// sub-agent's proposed write — and the fail-safe does NOT escalate it.
func TestRun_Gate_SubAgentBubblesUp(t *testing.T) {
	prov := &scriptedProvider{responses: []llm.CompletionResponse{
		{StopReason: llm.StopReasonToolUse, Content: []llm.ContentBlock{
			toolUseBlock("call1", callSubagentToolName, map[string]any{"agent": "billing", "brief": "cancel it"}),
		}},
	}}
	invoker := &fakeSubAgentInvoker{report: SubAgentReport{
		ReportText:       "paused for approval",
		ToolOutputs:      []SubAgentToolOutput{},
		AwaitingApproval: &ProposedWrite{ToolID: 7, ToolSlug: "cancel_contract", ToolUseID: "w1", Inputs: map[string]any{"id": 1}},
	}}
	r := newApprovalRunner(t, prov, fakeResolver{}, &fakeDispatcher{})

	resp, err := r.Run(context.Background(), RunRequest{
		InboundMessage: "cancel", TraceID: "t-bubble", InboxID: 1, ConversationID: 5,
		IsComposer: true, SubAgentInvoker: invoker,
	})
	require.NoError(t, err)
	require.Equal(t, StatusAwaitingApproval, resp.Status)
	require.NotNil(t, resp.ProposedWrite)
	require.Equal(t, "cancel_contract", resp.ProposedWrite.ToolSlug)
	require.Nil(t, resp.Decision, "a bubbled-up approval must not be escalated by the fail-safe")
}

// answerTurn is a one-turn provider that emits an answer terminal decision.
func answerTurn(message string) *scriptedProvider {
	return &scriptedProvider{responses: []llm.CompletionResponse{
		{StopReason: llm.StopReasonToolUse, Content: []llm.ContentBlock{
			toolUseBlock("ans", answerToolName, map[string]any{"message": message}),
		}},
	}}
}

// TestRun_Resume_Approve_ExecutesOnce proves the resume executes the approved
// write exactly once (with its inputs), feeds the output back, and the loop
// continues to a decision.
func TestRun_Resume_Approve_ExecutesOnce(t *testing.T) {
	answer := "Your contract has been cancelled."
	prov := answerTurn(answer)
	disp := &fakeDispatcher{}
	r := newApprovalRunner(t, prov,
		fakeResolver{tools: []resolver.ResolvedTool{customWriteTool(7, "cancel_contract")}}, disp)

	resp, err := r.Run(context.Background(), RunRequest{
		InboundMessage: "cancel", TraceID: "t-approve", InboxID: 1, ConversationID: 5, IsComposer: true,
		Resume: &ResumeContext{
			PriorTrace:   []TurnTrace{{TurnIndex: 0, Text: "Customer asked to cancel."}},
			ToolName:     "cancel_contract",
			ToolUseID:    "w1",
			Inputs:       map[string]any{"id": 1},
			ExecuteWrite: true,
		},
	})
	require.NoError(t, err)
	require.Equal(t, StatusComplete, resp.Status)
	require.NotNil(t, resp.Decision)
	require.Equal(t, DecisionKindAnswer, resp.Decision.Kind)
	require.Equal(t, answer, *resp.Draft)

	// Executed exactly once, with the approved inputs, against the right tool.
	require.Len(t, disp.calls, 1)
	require.Equal(t, "cancel_contract", disp.calls[0].Slug)
	require.Equal(t, 1, disp.calls[0].Parameters["id"])
	// The write output is in the run's tool log (→ tool_outputs) and was fed
	// back as the tool result the model composed from.
	require.Equal(t, "success", resp.ToolCalls[0].Status)
	require.Contains(t, toolResultText(prov.calls[0].Messages), "cancelled")
}

// TestRun_Resume_Edit_ExecutesWithEditedInputs proves edit executes the write
// with the operator's edited inputs.
func TestRun_Resume_Edit_ExecutesWithEditedInputs(t *testing.T) {
	disp := &fakeDispatcher{}
	r := newApprovalRunner(t, answerTurn("done"),
		fakeResolver{tools: []resolver.ResolvedTool{customWriteTool(7, "cancel_contract")}}, disp)

	_, err := r.Run(context.Background(), RunRequest{
		InboundMessage: "cancel", TraceID: "t-edit", InboxID: 1, ConversationID: 5, IsComposer: true,
		Resume: &ResumeContext{
			PriorTrace:   []TurnTrace{{TurnIndex: 0}},
			ToolName:     "cancel_contract",
			Inputs:       map[string]any{"id": 1, "effective_date": "2026-07-01"}, // operator-edited
			ExecuteWrite: true,
		},
	})
	require.NoError(t, err)
	require.Len(t, disp.calls, 1)
	require.Equal(t, "2026-07-01", disp.calls[0].Parameters["effective_date"])
}

// TestRun_Resume_Reject_DoesNotExecute proves reject feeds the rejection back
// (no execution) and the agent continues without the write.
func TestRun_Resume_Reject_DoesNotExecute(t *testing.T) {
	disp := &fakeDispatcher{}
	r := newApprovalRunner(t, answerTurn("I can't cancel that automatically; a colleague will follow up."),
		fakeResolver{tools: []resolver.ResolvedTool{customWriteTool(7, "cancel_contract")}}, disp)

	resp, err := r.Run(context.Background(), RunRequest{
		InboundMessage: "cancel", TraceID: "t-reject", InboxID: 1, ConversationID: 5, IsComposer: true,
		Resume: &ResumeContext{
			PriorTrace:      []TurnTrace{{TurnIndex: 0}},
			ToolName:        "cancel_contract",
			Inputs:          map[string]any{"id": 1},
			Rejected:        true,
			RejectionReason: "needs manager sign-off",
		},
	})
	require.NoError(t, err)
	require.Equal(t, StatusComplete, resp.Status)
	require.Empty(t, disp.calls, "a rejected write must NOT execute")
	require.Equal(t, "rejected", resp.ToolCalls[0].Status)
}

// TestRun_Resume_Held_DoesNotExecute proves an approved-but-HELD write (the
// situation changed during the approval window) feeds a neutral reconsider
// notice carrying the customer's words and does NOT execute — the agent
// re-reasons with the change in view instead of acting on a stale approval.
func TestRun_Resume_Held_DoesNotExecute(t *testing.T) {
	prov := answerTurn("I see you asked to keep your plan — I've made no changes.")
	disp := &fakeDispatcher{}
	r := newApprovalRunner(t, prov,
		fakeResolver{tools: []resolver.ResolvedTool{customWriteTool(7, "cancel_contract")}}, disp)

	resp, err := r.Run(context.Background(), RunRequest{
		InboundMessage: "cancel", TraceID: "t-held", InboxID: 1, ConversationID: 5, IsComposer: true,
		Resume: &ResumeContext{
			PriorTrace: []TurnTrace{{TurnIndex: 0}},
			ToolName:   "cancel_contract",
			Inputs:     map[string]any{"id": 1},
			Held:       true,
			HeldReason: `the customer replied while it awaited approval: "wait, don't cancel"`,
		},
	})
	require.NoError(t, err)
	require.Equal(t, StatusComplete, resp.Status)
	require.Empty(t, disp.calls, "a held write must NOT execute")
	require.Equal(t, "held", resp.ToolCalls[0].Status)
	// The neutral hold notice (with the customer's words) was fed to the model.
	fed := toolResultText(prov.calls[0].Messages)
	require.Contains(t, fed, "HELD")
	require.Contains(t, fed, "don't cancel")
}

// TestRun_Resume_Inject_FeedsReport proves the inject mode (used to feed a
// resumed sub-agent's report back into the composer) feeds the content with no
// dispatch and the loop continues.
func TestRun_Resume_Inject_FeedsReport(t *testing.T) {
	report := "Billing: balance is 80€; Q2 invoice unpaid."
	disp := &fakeDispatcher{}
	r := newApprovalRunner(t, answerTurn("Your balance is 80€."),
		fakeResolver{tools: nil}, disp)

	resp, err := r.Run(context.Background(), RunRequest{
		InboundMessage: "balance?", TraceID: "t-inject", InboxID: 1, ConversationID: 5, IsComposer: true,
		Resume: &ResumeContext{
			PriorTrace:     []TurnTrace{{TurnIndex: 0, Text: "Delegating to billing."}},
			ToolName:       callSubagentToolName,
			Inputs:         map[string]any{"agent": "billing"},
			InjectedResult: &report,
		},
	})
	require.NoError(t, err)
	require.Equal(t, StatusComplete, resp.Status)
	require.Empty(t, disp.calls)
}
