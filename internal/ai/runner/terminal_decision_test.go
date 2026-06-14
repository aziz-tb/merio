package runner

import (
	"context"
	"io"
	"testing"

	"github.com/aziz-tb/merio/internal/ai/llm"
	aitools "github.com/aziz-tb/merio/internal/ai/tools"
	"github.com/aziz-tb/merio/internal/ai/tools/builtin"
	"github.com/aziz-tb/merio/internal/ai/tools/resolver"
	"github.com/stretchr/testify/require"
	"github.com/zerodha/logf"
)

// newComposerTestRunner wires a Runner with a scripted provider for
// composer terminal-decision tests. Mirrors the setup in
// call_subagent_test.go; the agent loader is never consulted because an
// composer run uses the synthetic platform config, not an ai_agents row.
func newComposerTestRunner(t *testing.T, prov *scriptedProvider) *Runner {
	t.Helper()
	lo := logf.New(logf.Opts{Writer: io.Discard})
	r, err := New(Opts{
		Provider:   prov,
		Agents:     fakeAgentLoader{},
		Resolver:   resolver.New(nil, builtin.Default()),
		Dispatcher: &aitools.Dispatcher{},
		PresetReg:  testPresetRegistry(t),
		Lo:         &lo,
	})
	require.NoError(t, err)
	return r
}

// TestRun_ComposerEscalateCompletes proves the ESCALATE terminal decision:
// the run completes flagged for a human, the internal reason is recorded, and
// the optional customer holding message rides in Draft.
func TestRun_ComposerEscalateCompletes(t *testing.T) {
	prov := &scriptedProvider{responses: []llm.CompletionResponse{
		{StopReason: llm.StopReasonToolUse, Content: []llm.ContentBlock{{
			Type:      llm.ContentBlockTypeToolUse,
			ToolUseID: "tu_esc_1",
			ToolName:  escalateToolName,
			ToolInput: map[string]any{
				"internal_reason":  "customer threatens legal action (BNetzA)",
				"customer_message": "Thanks — a colleague will get back to you shortly.",
			},
		}}},
	}}
	r := newComposerTestRunner(t, prov)

	resp, err := r.Run(context.Background(), RunRequest{
		InboundMessage: "I will report you to the regulator!",
		TraceID:        "trace-esc-1",
		InboxID:        1,
		ConversationID: 7,
		IsComposer:     true,
	})
	require.NoError(t, err)

	require.Equal(t, StatusComplete, resp.Status)
	require.True(t, resp.IsComposer)
	require.NotNil(t, resp.Decision)
	require.Equal(t, DecisionKindEscalate, resp.Decision.Kind)
	require.Equal(t, "customer threatens legal action (BNetzA)", resp.Decision.InternalReason)
	// The optional holding message is the customer-facing text, so it rides in Draft.
	require.NotNil(t, resp.Draft)
	require.Equal(t, "Thanks — a colleague will get back to you shortly.", *resp.Draft)

	require.True(t, hasTool(prov.calls[0].Tools, answerToolName), "composer should be offered answer")
	require.True(t, hasTool(prov.calls[0].Tools, escalateToolName), "composer should be offered escalate")
	require.False(t, hasTool(prov.calls[0].Tools, produceDraftToolName), "composer must NOT be offered produce_draft")
}

// TestRun_ComposerEscalateWithoutHoldingMessage confirms an escalation with
// no customer_message leaves Draft nil (nothing goes to the customer).
func TestRun_ComposerEscalateWithoutHoldingMessage(t *testing.T) {
	prov := &scriptedProvider{responses: []llm.CompletionResponse{
		{StopReason: llm.StopReasonToolUse, Content: []llm.ContentBlock{{
			Type:      llm.ContentBlockTypeToolUse,
			ToolUseID: "tu_esc_2",
			ToolName:  escalateToolName,
			ToolInput: map[string]any{"internal_reason": "needs a human"},
		}}},
	}}
	r := newComposerTestRunner(t, prov)

	resp, err := r.Run(context.Background(), RunRequest{
		InboundMessage: "hello", TraceID: "trace-esc-2", InboxID: 1, IsComposer: true,
	})
	require.NoError(t, err)
	require.Equal(t, StatusComplete, resp.Status)
	require.NotNil(t, resp.Decision)
	require.Equal(t, DecisionKindEscalate, resp.Decision.Kind)
	require.Equal(t, "needs a human", resp.Decision.InternalReason)
	require.Nil(t, resp.Draft)
}

// TestRun_ComposerFailSafeEscalatesOnEndTurn proves the Layer-1 fail-safe:
// a composer that just stops (end_turn) with no decision and no error is
// upgraded from a silent incomplete to a committed escalation.
func TestRun_ComposerFailSafeEscalatesOnEndTurn(t *testing.T) {
	prov := &scriptedProvider{responses: []llm.CompletionResponse{
		{StopReason: llm.StopReasonEndTurn, Content: []llm.ContentBlock{{
			Type: llm.ContentBlockTypeText, Text: "I think I'm done here.",
		}}},
	}}
	r := newComposerTestRunner(t, prov)

	resp, err := r.Run(context.Background(), RunRequest{
		InboundMessage: "hi", TraceID: "trace-fs-1", InboxID: 1, IsComposer: true,
	})
	require.NoError(t, err) // fail-safe never surfaces an error
	require.Equal(t, StatusComplete, resp.Status)
	require.True(t, resp.IsComposer)
	require.NotNil(t, resp.Decision)
	require.Equal(t, DecisionKindEscalate, resp.Decision.Kind)
	require.Contains(t, resp.Decision.InternalReason, "without a decision")
	require.Nil(t, resp.Draft)
}

// TestRun_ComposerFailSafeEscalatesOnMaxIterations proves the fail-safe also
// catches a composer that keeps delegating and never decides — the loop
// exhausts and the run escalates instead of ending as a stuck ticket.
func TestRun_ComposerFailSafeEscalatesOnMaxIterations(t *testing.T) {
	call := llm.CompletionResponse{StopReason: llm.StopReasonToolUse, Content: []llm.ContentBlock{{
		Type:      llm.ContentBlockTypeToolUse,
		ToolUseID: "tu_call",
		ToolName:  callSubagentToolName,
		ToolInput: map[string]any{"agent": "billing", "brief": "look again"},
	}}}
	prov := &scriptedProvider{responses: []llm.CompletionResponse{call, call}}
	r := newComposerTestRunner(t, prov)
	invoker := &fakeSubAgentInvoker{report: SubAgentReport{
		ReportText: "still unclear", ToolOutputs: []SubAgentToolOutput{},
	}}

	resp, err := r.Run(context.Background(), RunRequest{
		InboundMessage:  "help",
		TraceID:         "trace-fs-2",
		InboxID:         1,
		IsComposer:      true,
		MaxIterations:   2,
		SubAgentInvoker: invoker,
	})
	require.NoError(t, err)
	require.Equal(t, StatusComplete, resp.Status)
	require.NotNil(t, resp.Decision)
	require.Equal(t, DecisionKindEscalate, resp.Decision.Kind)
	require.Contains(t, resp.Decision.InternalReason, "max_iterations")
	require.Len(t, invoker.calls, 2) // delegated twice, then the fail-safe escalated
}

// TestRun_ComposerMalformedAnswerFedBackThenCompletes proves a botched
// terminal call (answer with an empty message) is NOT escalated: the validation
// error is fed back as the tool result and the composer corrects itself on
// the next turn, completing normally. A formatting slip never reaches a human.
func TestRun_ComposerMalformedAnswerFedBackThenCompletes(t *testing.T) {
	good := "Here is your balance: 80€."
	prov := &scriptedProvider{responses: []llm.CompletionResponse{
		// Turn 1: malformed answer (empty message).
		{StopReason: llm.StopReasonToolUse, Content: []llm.ContentBlock{{
			Type: llm.ContentBlockTypeToolUse, ToolUseID: "tu_bad", ToolName: answerToolName,
			ToolInput: map[string]any{"message": "   "},
		}}},
		// Turn 2: corrected answer.
		{StopReason: llm.StopReasonToolUse, Content: []llm.ContentBlock{{
			Type: llm.ContentBlockTypeToolUse, ToolUseID: "tu_good", ToolName: answerToolName,
			ToolInput: map[string]any{"message": good},
		}}},
	}}
	r := newComposerTestRunner(t, prov)

	resp, err := r.Run(context.Background(), RunRequest{
		InboundMessage: "balance?", TraceID: "trace-fix-1", InboxID: 1, IsComposer: true,
	})
	require.NoError(t, err)
	require.Equal(t, StatusComplete, resp.Status)
	require.NotNil(t, resp.Decision)
	require.Equal(t, DecisionKindAnswer, resp.Decision.Kind)
	require.NotNil(t, resp.Draft)
	require.Equal(t, good, *resp.Draft)
	// The model got two turns: the malformed one was fed back, then corrected.
	require.Len(t, prov.calls, 2)
	// Turn 2 saw the validation error as a tool_result (reject-feeds-back).
	require.Contains(t, toolResultText(prov.calls[1].Messages), "non-empty")
}

// TestRun_ComposerPersistentMalformedEscalates proves the guard: if the
// composer keeps emitting malformed terminal calls, the loop is bounded by
// max-iterations and the fail-safe escalates — the ticket is never silently
// stuck, even though a single slip would not have escalated.
func TestRun_ComposerPersistentMalformedEscalates(t *testing.T) {
	bad := llm.CompletionResponse{StopReason: llm.StopReasonToolUse, Content: []llm.ContentBlock{{
		Type: llm.ContentBlockTypeToolUse, ToolUseID: "tu_bad", ToolName: answerToolName,
		ToolInput: map[string]any{"message": ""},
	}}}
	prov := &scriptedProvider{responses: []llm.CompletionResponse{bad, bad}}
	r := newComposerTestRunner(t, prov)

	resp, err := r.Run(context.Background(), RunRequest{
		InboundMessage: "hi", TraceID: "trace-fix-2", InboxID: 1, IsComposer: true,
		MaxIterations: 2,
	})
	require.NoError(t, err)
	require.Equal(t, StatusComplete, resp.Status)
	require.NotNil(t, resp.Decision)
	require.Equal(t, DecisionKindEscalate, resp.Decision.Kind)
	require.Contains(t, resp.Decision.InternalReason, "max_iterations")
	require.Len(t, prov.calls, 2) // tried twice (fed back once), then the fail-safe escalated
}

// TestMaybeHandleTerminalDecisionTurn unit-tests the decision parser/validator
// directly, including the validation paths the in-loop handler ends the run on.
func TestMaybeHandleTerminalDecisionTurn(t *testing.T) {
	answerTU := func(input map[string]any) llm.ContentBlock {
		return llm.ContentBlock{Type: llm.ContentBlockTypeToolUse, ToolUseID: "a", ToolName: answerToolName, ToolInput: input}
	}
	escalateTU := func(input map[string]any) llm.ContentBlock {
		return llm.ContentBlock{Type: llm.ContentBlockTypeToolUse, ToolUseID: "e", ToolName: escalateToolName, ToolInput: input}
	}
	otherTU := llm.ContentBlock{Type: llm.ContentBlockTypeToolUse, ToolUseID: "o", ToolName: "kb_search", ToolInput: map[string]any{}}

	t.Run("no terminal tool -> not handled", func(t *testing.T) {
		handled, d, _, _ := maybeHandleTerminalDecisionTurn([]llm.ContentBlock{otherTU})
		require.False(t, handled)
		require.Nil(t, d)
	})
	t.Run("answer success", func(t *testing.T) {
		handled, d, logs, reason := maybeHandleTerminalDecisionTurn([]llm.ContentBlock{answerTU(map[string]any{"message": "hello"})})
		require.True(t, handled)
		require.NotNil(t, d)
		require.Equal(t, DecisionKindAnswer, d.kind)
		require.Equal(t, "hello", d.customerText)
		require.Empty(t, reason)
		require.Len(t, logs, 1)
		require.Equal(t, "success", logs[0].Status)
	})
	t.Run("escalate success with holding message", func(t *testing.T) {
		handled, d, _, _ := maybeHandleTerminalDecisionTurn([]llm.ContentBlock{escalateTU(map[string]any{"internal_reason": "r", "customer_message": "hold"})})
		require.True(t, handled)
		require.NotNil(t, d)
		require.Equal(t, DecisionKindEscalate, d.kind)
		require.Equal(t, "r", d.internalReason)
		require.Equal(t, "hold", d.customerText)
	})
	t.Run("answer missing message -> validation error, no decision", func(t *testing.T) {
		handled, d, logs, reason := maybeHandleTerminalDecisionTurn([]llm.ContentBlock{answerTU(map[string]any{})})
		require.True(t, handled)
		require.Nil(t, d)
		require.NotEmpty(t, reason)
		require.Equal(t, "validation_error", logs[0].Status)
	})
	t.Run("escalate missing internal_reason -> validation error", func(t *testing.T) {
		handled, d, _, reason := maybeHandleTerminalDecisionTurn([]llm.ContentBlock{escalateTU(map[string]any{})})
		require.True(t, handled)
		require.Nil(t, d)
		require.NotEmpty(t, reason)
	})
	t.Run("terminal tool not alone -> validation error", func(t *testing.T) {
		handled, d, _, reason := maybeHandleTerminalDecisionTurn([]llm.ContentBlock{answerTU(map[string]any{"message": "x"}), otherTU})
		require.True(t, handled)
		require.Nil(t, d)
		require.Contains(t, reason, "only tool call")
	})
	t.Run("two terminal tools -> validation error", func(t *testing.T) {
		handled, d, _, reason := maybeHandleTerminalDecisionTurn([]llm.ContentBlock{
			answerTU(map[string]any{"message": "x"}),
			escalateTU(map[string]any{"internal_reason": "r"}),
		})
		require.True(t, handled)
		require.Nil(t, d)
		require.Contains(t, reason, "only tool call")
	})
}
