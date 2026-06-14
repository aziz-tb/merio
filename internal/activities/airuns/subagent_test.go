package airuns

import (
	"context"
	"errors"
	"io"
	"testing"

	agentmodels "github.com/aziz-tb/merio/internal/ai/agents/models"
	executionmodels "github.com/aziz-tb/merio/internal/ai/executions/models"
	"github.com/aziz-tb/merio/internal/ai/runner"
	wfairuns "github.com/aziz-tb/merio/internal/workflows/airuns"
	"github.com/stretchr/testify/require"
	"github.com/zerodha/logf"
)

type fakeResolver struct {
	agent agentmodels.Agent
	err   error
}

func (f fakeResolver) GetBySlug(string) (agentmodels.Agent, error) { return f.agent, f.err }

type fakeSubRunner struct {
	resp   runner.RunResponse
	err    error
	gotReq runner.RunRequest
}

func (f *fakeSubRunner) Run(_ context.Context, req runner.RunRequest) (runner.RunResponse, error) {
	f.gotReq = req
	return f.resp, f.err
}

type recordingPersister struct {
	completeCalls int
	failedCalls   int
	lastParams    wfairuns.AgentRunWorkflowParams
}

func (p *recordingPersister) PersistCompleteRun(_ context.Context, params wfairuns.AgentRunWorkflowParams, _ runner.RunResponse) (int64, error) {
	p.completeCalls++
	p.lastParams = params
	return 1, nil
}

func (p *recordingPersister) PersistFailedRun(_ context.Context, params wfairuns.AgentRunWorkflowParams, _ runner.RunResponse, _ string) (int64, error) {
	p.failedCalls++
	p.lastParams = params
	return 2, nil
}

func newInvoker(run subRunner, res subAgentResolver, per subRunPersister) *subAgentInvoker {
	lo := logf.New(logf.Opts{Writer: io.Discard})
	return &subAgentInvoker{runner: run, agents: res, executions: per, lo: &lo}
}

// TestSubAgentInvoker_PersistsCompleteSubRun is the "trace shows both runs"
// guarantee: a successful sub-agent run persists a SECOND agent_executions row
// with source="subagent" and the derived child trace id, while only the report
// (reads summarized; typed-empty tool_outputs) flows back to the composer.
func TestSubAgentInvoker_PersistsCompleteSubRun(t *testing.T) {
	draft := "open balance is 80€"
	run := &fakeSubRunner{resp: runner.RunResponse{Status: runner.StatusComplete, Draft: &draft}}
	persister := &recordingPersister{}
	inv := newInvoker(run,
		fakeResolver{agent: agentmodels.Agent{ID: 7, Slug: "billing", Enabled: true}},
		persister,
	)

	parent := runner.SubAgentParentContext{ConversationID: 42, InboxID: 3, VerificationLevel: 2, InvokerUserID: 9, TraceID: "trace-orch"}
	report, err := inv.InvokeSubAgent(context.Background(), parent, runner.SubAgentCall{
		AgentRef: "billing", Brief: "check the balance", CallID: "tu_call_1",
	})
	require.NoError(t, err)

	// Report contract: reads summarized in report_text; typed empty tool_outputs.
	require.Equal(t, draft, report.ReportText)
	require.NotNil(t, report.ToolOutputs)
	require.Empty(t, report.ToolOutputs)

	// The sub-agent run inherited parent scope, got a derived child trace id,
	// and carried NO invoker / not-composer (recursion impossible).
	require.Equal(t, 7, run.gotReq.AgentID)
	require.Equal(t, "check the balance", run.gotReq.InboundMessage)
	require.Equal(t, 42, run.gotReq.ConversationID)
	require.Equal(t, "trace-orch:sub:tu_call_1", run.gotReq.TraceID)
	require.Nil(t, run.gotReq.SubAgentInvoker)
	require.False(t, run.gotReq.IsComposer)

	// The full sub-agent trace persisted as a COMPLETE run, source=subagent.
	require.Equal(t, 1, persister.completeCalls)
	require.Equal(t, 0, persister.failedCalls)
	require.Equal(t, executionmodels.SourceSubagent, persister.lastParams.Source)
	require.Equal(t, "trace-orch:sub:tu_call_1", persister.lastParams.TraceID)
	require.Equal(t, 7, persister.lastParams.AgentID)
	require.Equal(t, 42, persister.lastParams.ConversationID)
}

// TestSubAgentInvoker_PersistsFailedSubRun confirms a failed sub-run persists
// via PersistFailedRun and still returns a non-empty, reasoning-friendly report.
func TestSubAgentInvoker_PersistsFailedSubRun(t *testing.T) {
	run := &fakeSubRunner{resp: runner.RunResponse{Status: runner.StatusFailed}, err: errors.New("provider down")}
	persister := &recordingPersister{}
	inv := newInvoker(run,
		fakeResolver{agent: agentmodels.Agent{ID: 7, Slug: "billing", Enabled: true}},
		persister,
	)

	report, err := inv.InvokeSubAgent(context.Background(),
		runner.SubAgentParentContext{ConversationID: 42, TraceID: "t"},
		runner.SubAgentCall{AgentRef: "billing", Brief: "x", CallID: "c"},
	)
	require.NoError(t, err)
	require.Contains(t, report.ReportText, "could not complete")
	require.NotEmpty(t, report.ReportText)
	require.Equal(t, 1, persister.failedCalls)
	require.Equal(t, 0, persister.completeCalls)
	require.Equal(t, executionmodels.SourceSubagent, persister.lastParams.Source)
}

// TestSubAgentInvoker_UnknownSlugIsRecoverableError confirms an unknown
// sub-agent slug surfaces as an error (which the runner turns into a tool error
// the composer can re-reason about) and persists nothing.
func TestSubAgentInvoker_UnknownSlugIsRecoverableError(t *testing.T) {
	persister := &recordingPersister{}
	inv := newInvoker(&fakeSubRunner{}, fakeResolver{err: errors.New("not found")}, persister)

	_, err := inv.InvokeSubAgent(context.Background(),
		runner.SubAgentParentContext{},
		runner.SubAgentCall{AgentRef: "ghost", Brief: "x", CallID: "c"},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "ghost")
	require.Equal(t, 0, persister.completeCalls)
	require.Equal(t, 0, persister.failedCalls)
}
