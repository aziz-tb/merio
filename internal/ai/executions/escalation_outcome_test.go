package executions

import (
	"testing"

	"github.com/aziz-tb/merio/internal/ai/executions/models"
	"github.com/aziz-tb/merio/internal/ai/runner"
	"github.com/stretchr/testify/require"
)

func strptr(s string) *string { return &s }

// TestEscalationOutcomeParams covers the pure decision->outcome mapping that
// PersistCompleteRun uses to record a composer escalation for human
// review. Only an ESCALATE decision produces an outcome row; answers, drafts,
// and ordinary runs do not.
func TestEscalationOutcomeParams(t *testing.T) {
	tests := []struct {
		name     string
		resp     runner.RunResponse
		wantOK   bool
		wantRow  models.AgentExecutionOutcome // only Outcome + reason checked
		wantNoOp bool
	}{
		{
			name: "escalate with reason -> escalated outcome",
			resp: runner.RunResponse{Decision: &runner.TerminalDecision{
				Kind: runner.DecisionKindEscalate, InternalReason: "needs a human",
			}},
			wantOK: true,
		},
		{
			name:     "answer -> no outcome (left for operator / auto-send)",
			resp:     runner.RunResponse{Decision: &runner.TerminalDecision{Kind: runner.DecisionKindAnswer}},
			wantNoOp: true,
		},
		{
			name:     "no decision (ordinary/sub-agent run) -> no outcome",
			resp:     runner.RunResponse{},
			wantNoOp: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			op, ok := escalationOutcomeParams(99, tc.resp)
			if tc.wantNoOp {
				require.False(t, ok)
				return
			}
			require.True(t, ok)
			require.Equal(t, int64(99), op.AgentExecutionID)
			require.Equal(t, models.OutcomeEscalated, op.Outcome)
			require.Equal(t, strptr("needs a human"), op.OutcomeReason)
			require.Nil(t, op.MessageID)         // platform-committed, no message
			require.Nil(t, op.CommittedByUserID) // platform-committed, no user
		})
	}
}

// TestEscalationOutcomeParams_BlankReason confirms a blank internal reason maps
// to a NULL outcome_reason rather than an empty string.
func TestEscalationOutcomeParams_BlankReason(t *testing.T) {
	op, ok := escalationOutcomeParams(1, runner.RunResponse{Decision: &runner.TerminalDecision{
		Kind: runner.DecisionKindEscalate, InternalReason: "   ",
	}})
	require.True(t, ok)
	require.Nil(t, op.OutcomeReason)
}
