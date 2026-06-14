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
	// Blank import registers the builtin capabilities (incl. kb_search) into the
	// default registry via their init() blocks — same as production wiring.
	_ "github.com/aziz-tb/merio/internal/ai/tools/builtin/capabilities"
	"github.com/aziz-tb/merio/internal/ai/tools/resolver"
	"github.com/stretchr/testify/require"
	"github.com/zerodha/logf"
)

// TestRun_Composer_GeneralCapability proves Slice 5 Stage E2: a composer run
// given a general KB scope is resolved kb_search through the SAME path sub-agents
// use, carries the answer-vs-delegate instruction, and answers a general question
// DIRECTLY (no delegation) — while its tool set stays bounded to general + the
// platform terminal/delegation tools (no domain tools, no produce_draft).
func TestRun_Composer_GeneralCapability(t *testing.T) {
	prov := &scriptedProvider{responses: []llm.CompletionResponse{
		// Answer the general question directly — no call_subagent.
		{StopReason: llm.StopReasonToolUse, Content: []llm.ContentBlock{{
			Type: llm.ContentBlockTypeToolUse, ToolUseID: "tu_answer", ToolName: answerToolName,
			ToolInput: map[string]any{"message": "We're open 9–17, Monday to Friday."},
		}}},
	}}
	lo := logf.New(logf.Opts{Writer: io.Discard})
	r, err := New(Opts{
		Provider:   prov,
		Agents:     fakeAgentLoader{agent: agentmodels.Agent{ID: 1, Slug: "x", Enabled: true, MaxIterations: 5, UpdatedAt: time.Now()}},
		Resolver:   resolver.New(nil, builtin.Default()),
		Dispatcher: &aitools.Dispatcher{},
		PresetReg:  testPresetRegistry(t),
		Lo:         &lo,
	})
	require.NoError(t, err)

	invoker := &fakeSubAgentInvoker{}
	resp, err := r.Run(context.Background(), RunRequest{
		InboundMessage:  "what are your opening hours?",
		TraceID:         "t-cap",
		InboxID:         1,
		ConversationID:  5,
		IsComposer:      true,
		SubAgentInvoker: invoker, // delegation is AVAILABLE...
		ComposerKBScope: json.RawMessage(`{"folder_ids":[1],"article_ids":[]}`),
	})
	require.NoError(t, err)
	require.Equal(t, StatusComplete, resp.Status)
	require.NotNil(t, resp.Decision)
	require.Equal(t, DecisionKindAnswer, resp.Decision.Kind)

	// ...yet the general question was answered DIRECTLY: no sub-agent invoked.
	require.Empty(t, invoker.calls, "a general question is answered directly, not delegated")

	// The general KB scope gave the composer kb_search (resolved like a sub-agent's
	// tools), alongside the platform answer/escalate/call_subagent — and never
	// produce_draft (composer-only terminal path).
	tools := prov.calls[0].Tools
	require.True(t, hasTool(tools, "kb_search"), "a general KB scope yields kb_search")
	require.True(t, hasTool(tools, callSubagentToolName))
	require.True(t, hasTool(tools, answerToolName))
	require.True(t, hasTool(tools, escalateToolName))
	require.False(t, hasTool(tools, produceDraftToolName))

	// The answer-vs-delegate platform default is in the composer's prompt.
	require.Contains(t, systemText(prov.calls[0].Messages), "ANSWER DIRECTLY")
}
