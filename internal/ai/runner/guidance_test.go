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

// TestRun_Composer_InjectsGuidance proves assembled guidance prose reaches the
// Composer's system prompt (Slice 5 Stage B) — the seam by which an inbox-scoped
// item (e.g. Sie vs Du) shapes the reply.
func TestRun_Composer_InjectsGuidance(t *testing.T) {
	prov := answerTurn("Guten Tag.")
	r := newApprovalRunner(t, prov, fakeResolver{tools: nil}, &fakeDispatcher{})

	_, err := r.Run(context.Background(), RunRequest{
		InboundMessage: "hallo", TraceID: "t-guid", InboxID: 1, ConversationID: 5, IsComposer: true,
		GuidanceProse: "Operator guidance for this conversation (apply where relevant):\n- Address the customer with Sie.",
	})
	require.NoError(t, err)
	require.Len(t, prov.calls, 1)
	require.Contains(t, systemText(prov.calls[0].Messages), "Address the customer with Sie.")
}

// TestRun_Composer_NoGuidance_NoBlock proves an empty GuidanceProse injects
// nothing (no stray guidance header).
func TestRun_Composer_NoGuidance_NoBlock(t *testing.T) {
	prov := answerTurn("Hi.")
	r := newApprovalRunner(t, prov, fakeResolver{tools: nil}, &fakeDispatcher{})

	_, err := r.Run(context.Background(), RunRequest{
		InboundMessage: "hi", TraceID: "t-noguid", InboxID: 1, ConversationID: 5, IsComposer: true,
	})
	require.NoError(t, err)
	require.NotContains(t, systemText(prov.calls[0].Messages), "Operator guidance")
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
