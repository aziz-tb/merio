package runner

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentmodels "github.com/aziz-tb/merio/internal/ai/agents/models"
	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/aziz-tb/merio/internal/ai/llm/preset"
	aitools "github.com/aziz-tb/merio/internal/ai/tools"
	"github.com/aziz-tb/merio/internal/ai/tools/builtin"
	"github.com/aziz-tb/merio/internal/ai/tools/resolver"
	"github.com/stretchr/testify/require"
	"github.com/zerodha/logf"
)

// scriptedProvider returns a fixed sequence of completion responses, one per
// Complete call, and records every request it received so the test can assert
// on what the runner sent (e.g. the tool list and the tool_result fed back).
type scriptedProvider struct {
	responses []llm.CompletionResponse
	calls     []llm.CompletionRequest
}

func (p *scriptedProvider) Complete(_ context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	i := len(p.calls)
	p.calls = append(p.calls, req)
	if i >= len(p.responses) {
		// Defensive: end the turn cleanly rather than loop forever.
		return llm.CompletionResponse{StopReason: llm.StopReasonEndTurn}, nil
	}
	return p.responses[i], nil
}

func (p *scriptedProvider) StreamComplete(ctx context.Context, req llm.CompletionRequest, _ func(string)) (llm.CompletionResponse, error) {
	return p.Complete(ctx, req)
}

func (p *scriptedProvider) Name() string { return "scripted" }

// fakeAgentLoader returns the same agent for any id.
type fakeAgentLoader struct{ agent agentmodels.Agent }

func (f fakeAgentLoader) Get(int) (agentmodels.Agent, error) { return f.agent, nil }

// fakeSubAgentInvoker records the call and returns a canned report.
type fakeSubAgentInvoker struct {
	report SubAgentReport
	calls  []SubAgentCall
	parent SubAgentParentContext
}

func (f *fakeSubAgentInvoker) InvokeSubAgent(_ context.Context, parent SubAgentParentContext, call SubAgentCall) (SubAgentReport, error) {
	f.calls = append(f.calls, call)
	f.parent = parent
	return f.report, nil
}

// TestRun_ComposerCallsSubAgent proves the Slice 1 call boundary end to
// end at the runner level: a composer RUN (not an ai_agents row) gets the
// call_subagent tool, calling it routes to the invoker, the invoker's report is
// fed back as the tool result, and the composer composes a draft from it.
func TestRun_ComposerCallsSubAgent(t *testing.T) {
	draft := "Your open balance is 80€; we'll send the Q2 invoice again."
	prov := &scriptedProvider{responses: []llm.CompletionResponse{
		// Turn 1: composer delegates to the billing sub-agent.
		{StopReason: llm.StopReasonToolUse, Content: []llm.ContentBlock{{
			Type:      llm.ContentBlockTypeToolUse,
			ToolUseID: "tu_call_1",
			ToolName:  callSubagentToolName,
			ToolInput: map[string]any{"agent": "billing", "brief": "check the customer's balance"},
		}}},
		// Turn 2: composer commits the final reply via the terminal answer
		// decision. Slice 2 — composers terminate via answer/escalate, not
		// produce_draft.
		{StopReason: llm.StopReasonToolUse, Content: []llm.ContentBlock{{
			Type:      llm.ContentBlockTypeToolUse,
			ToolUseID: "tu_answer_1",
			ToolName:  answerToolName,
			ToolInput: map[string]any{"message": draft},
		}}},
	}}

	invoker := &fakeSubAgentInvoker{report: SubAgentReport{
		ReportText:  "open balance is 80€; Q2 invoice unpaid",
		ToolOutputs: []SubAgentToolOutput{},
	}}

	lo := logf.New(logf.Opts{Writer: io.Discard})
	r, err := New(Opts{
		Provider:   prov,
		Agents:     fakeAgentLoader{}, // never consulted: a composer run uses the platform config, not a row
		Resolver:   resolver.New(nil, builtin.Default()),
		Dispatcher: &aitools.Dispatcher{}, // never invoked: call_subagent + answer are intercepted
		PresetReg:  testPresetRegistry(t),
		Lo:         &lo,
	})
	require.NoError(t, err)

	resp, err := r.Run(context.Background(), RunRequest{
		InboundMessage:  "What's my outstanding balance?",
		TraceID:         "trace-orch-1",
		InboxID:         1,
		ConversationID:  42,
		IsComposer:  true,
		SubAgentInvoker: invoker,
	})
	require.NoError(t, err)

	// Composer completed by composing the reply from the report and
	// committing it via the terminal answer decision, running as the synthetic
	// platform composer (not an ai_agents row).
	require.Equal(t, StatusComplete, resp.Status)
	require.NotNil(t, resp.Draft)
	require.Equal(t, draft, *resp.Draft)
	require.Equal(t, composerAgentSlug, resp.AgentSlug)
	require.True(t, resp.IsComposer)
	require.NotNil(t, resp.Decision)
	require.Equal(t, DecisionKindAnswer, resp.Decision.Kind)
	require.Empty(t, resp.Decision.InternalReason)

	// The invoker was called exactly once with the composer's brief, and
	// the parent context flowed through (so the sub-agent inherits scope).
	require.Len(t, invoker.calls, 1)
	require.Equal(t, "billing", invoker.calls[0].AgentRef)
	require.Equal(t, "check the customer's balance", invoker.calls[0].Brief)
	require.Equal(t, "tu_call_1", invoker.calls[0].CallID)
	require.Equal(t, 42, invoker.parent.ConversationID)
	require.Equal(t, "trace-orch-1", invoker.parent.TraceID)

	// call_subagent is logged as a runner-owned tool carrying the report.
	var found *ToolCallLog
	for i := range resp.ToolCalls {
		if resp.ToolCalls[i].ToolID == callSubagentToolName {
			found = &resp.ToolCalls[i]
		}
	}
	require.NotNil(t, found, "expected a call_subagent tool log")
	require.Equal(t, "runner", found.ToolKind)
	require.Equal(t, "success", found.Status)
	report, ok := found.Data.(SubAgentReport)
	require.True(t, ok, "call_subagent data should be a SubAgentReport")
	require.Contains(t, report.ReportText, "open balance is 80€")
	require.NotNil(t, report.ToolOutputs)
	require.Empty(t, report.ToolOutputs)

	// The composer's first turn was offered call_subagent + the terminal
	// decision tools, and NOT produce_draft.
	require.Len(t, prov.calls, 2)
	require.True(t, hasTool(prov.calls[0].Tools, callSubagentToolName), "composer should be offered call_subagent")
	require.True(t, hasTool(prov.calls[0].Tools, answerToolName), "composer should be offered answer")
	require.True(t, hasTool(prov.calls[0].Tools, escalateToolName), "composer should be offered escalate")
	require.False(t, hasTool(prov.calls[0].Tools, produceDraftToolName), "composer must NOT be offered produce_draft")
	// ...and the report was fed back as the tool_result on the second turn.
	require.Contains(t, toolResultText(prov.calls[1].Messages), "open balance is 80€")
}

// TestRun_NonComposerHasNoCallSubagent confirms a normal agent run is never
// offered call_subagent, even when an invoker is attached — the runner gates
// injection on the run-level IsComposer flag, not on invoker-presence.
func TestRun_NonComposerHasNoCallSubagent(t *testing.T) {
	prov := &scriptedProvider{responses: []llm.CompletionResponse{
		{StopReason: llm.StopReasonToolUse, Content: []llm.ContentBlock{{
			Type:      llm.ContentBlockTypeToolUse,
			ToolUseID: "tu_draft_1",
			ToolName:  "produce_draft",
			ToolInput: map[string]any{"draft": "hello"},
		}}},
	}}
	loader := fakeAgentLoader{agent: agentmodels.Agent{
		ID: 2, Slug: "billing", Enabled: true,
		Prompt: "You are billing.", MaxIterations: 5, UpdatedAt: time.Now(),
	}}
	lo := logf.New(logf.Opts{Writer: io.Discard})
	r, err := New(Opts{
		Provider:   prov,
		Agents:     loader,
		Resolver:   resolver.New(nil, builtin.Default()),
		Dispatcher: &aitools.Dispatcher{},
		PresetReg:  testPresetRegistry(t),
		Lo:         &lo,
	})
	require.NoError(t, err)

	resp, err := r.Run(context.Background(), RunRequest{
		AgentID: 2, InboundMessage: "hi", TraceID: "trace-sub-1", InboxID: 1,
		// IsComposer deliberately false; invoker present but must be ignored.
		SubAgentInvoker: &fakeSubAgentInvoker{},
	})
	require.NoError(t, err)
	require.Equal(t, StatusComplete, resp.Status)
	require.Len(t, prov.calls, 1)
	require.False(t, hasTool(prov.calls[0].Tools, callSubagentToolName), "normal run must not be offered call_subagent")
	// Regression: a normal run still terminates via produce_draft (Slice 1) and
	// is never offered the composer-only terminal decision tools.
	require.True(t, hasTool(prov.calls[0].Tools, produceDraftToolName), "normal run should be offered produce_draft")
	require.False(t, hasTool(prov.calls[0].Tools, answerToolName), "normal run must not be offered answer")
	require.False(t, hasTool(prov.calls[0].Tools, escalateToolName), "normal run must not be offered escalate")
	require.False(t, resp.IsComposer)
	require.Nil(t, resp.Decision)
}

func hasTool(tools []llm.ToolDef, name string) bool {
	for _, t := range tools {
		if t.Name == name {
			return true
		}
	}
	return false
}

func toolResultText(msgs []llm.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		for _, c := range m.Content {
			if c.Type == llm.ContentBlockTypeToolResult {
				b.WriteString(c.ResultContent)
			}
		}
	}
	return b.String()
}

// testPresetRegistry builds a minimal in-memory preset registry from a temp
// file (catalog nil => model-existence checks skipped). It maps every known
// purpose to one preset so Resolve("reason", …) succeeds; the scripted
// provider ignores the preset's contents.
func testPresetRegistry(t *testing.T) *preset.Registry {
	t.Helper()
	mappings := map[string]string{}
	for _, p := range llm.KnownPurposes {
		mappings[string(p)] = "test-preset"
	}
	file := map[string]any{
		"presets": []map[string]any{{
			"slug":        "test-preset",
			"name":        "Test",
			"provider":    "anthropic",
			"model":       "test-model",
			"temperature": 0,
			"max_tokens":  1024,
			"enabled":     true,
		}},
		"purpose_mappings": mappings,
	}
	b, err := json.Marshal(file)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "ai_presets.json")
	require.NoError(t, os.WriteFile(path, b, 0o600))
	reg, err := preset.Load(path, nil)
	require.NoError(t, err)
	return reg
}
