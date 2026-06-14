package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/stretchr/testify/require"
)

// systemText concatenates the text of every system message in a provider call,
// so a test can assert what the model was told.
func systemText(messages []llm.Message) string {
	var b strings.Builder
	for _, m := range messages {
		if m.Role != llm.RoleSystem {
			continue
		}
		for _, c := range m.Content {
			if c.Type == llm.ContentBlockTypeText {
				b.WriteString(c.Text)
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}

// firstSystemText returns the text of the FIRST system message — the assembled
// system prompt, before any context/history blocks.
func firstSystemText(messages []llm.Message) string {
	for _, m := range messages {
		if m.Role != llm.RoleSystem {
			continue
		}
		var b strings.Builder
		for _, c := range m.Content {
			if c.Type == llm.ContentBlockTypeText {
				b.WriteString(c.Text)
			}
		}
		return b.String()
	}
	return ""
}

// TestRun_Composer_UsesAssembledSystemPrompt proves the runner uses the caller's
// assembled ComposerSystemPrompt verbatim as the Composer's system message — the
// seam by which composer.BuildComposerSystemPrompt (skeleton + guidance sections
// + roster + decision) reaches the model. (The guidance→sections proof itself
// lives in the composer builder tests.)
func TestRun_Composer_UsesAssembledSystemPrompt(t *testing.T) {
	const assembled = "ASSEMBLED COMPOSER PROMPT\n## How to communicate\n- Address the customer with Sie."
	prov := answerTurn("Guten Tag.")
	r := newApprovalRunner(t, prov, fakeResolver{tools: nil}, &fakeDispatcher{})

	_, err := r.Run(context.Background(), RunRequest{
		InboundMessage: "hallo", TraceID: "t-csp", InboxID: 1, ConversationID: 5, IsComposer: true,
		SubAgentInvoker:      &fakeSubAgentInvoker{},
		ComposerSystemPrompt: assembled,
	})
	require.NoError(t, err)
	require.Len(t, prov.calls, 1)
	require.Equal(t, assembled, firstSystemText(prov.calls[0].Messages), "composer uses ComposerSystemPrompt verbatim")
}

// TestRun_Composer_FallsBackToFloor proves a Composer run with no assembled
// prompt (not built by the activity) uses the skeleton+decision floor — exactly
// MinimalComposerSystemPrompt, which is the builder's empty-input output.
func TestRun_Composer_FallsBackToFloor(t *testing.T) {
	prov := answerTurn("Hi.")
	r := newApprovalRunner(t, prov, fakeResolver{tools: nil}, &fakeDispatcher{})

	_, err := r.Run(context.Background(), RunRequest{
		InboundMessage: "hi", TraceID: "t-floor", InboxID: 1, ConversationID: 5, IsComposer: true,
		SubAgentInvoker: &fakeSubAgentInvoker{},
	})
	require.NoError(t, err)
	require.Equal(t, MinimalComposerSystemPrompt(ComposerDecisionInstructions()), firstSystemText(prov.calls[0].Messages))
}

// TestRun_ForceEscalate_ShortCircuits proves the deterministic escalation arm
// (Slice 5 Stage C): when a rule fired, the run escalates without calling the
// model at all — "regardless of model output". The provider is scripted to
// ANSWER, yet the run escalates and the model is never invoked.
func TestRun_ForceEscalate_ShortCircuits(t *testing.T) {
	prov := answerTurn("Sure, I can handle that for you.")
	r := newApprovalRunner(t, prov, fakeResolver{tools: nil}, &fakeDispatcher{})

	const reason = "Escalated by a deterministic escalation rule (1 matched)."
	resp, err := r.Run(context.Background(), RunRequest{
		InboundMessage: "this is a legal complaint", TraceID: "t-esc", InboxID: 1, ConversationID: 5, IsComposer: true,
		ForceEscalateReason: reason,
	})
	require.NoError(t, err)
	require.Equal(t, StatusComplete, resp.Status)
	require.NotNil(t, resp.Decision)
	require.Equal(t, DecisionKindEscalate, resp.Decision.Kind)
	require.Equal(t, reason, resp.Decision.InternalReason)
	require.Empty(t, prov.calls, "rule-fired escalation must NOT call the model")
}
