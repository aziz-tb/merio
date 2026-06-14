package runner

import (
	"context"
	"io"
	"testing"
	"time"

	agentmodels "github.com/aziz-tb/merio/internal/ai/agents/models"
	"github.com/aziz-tb/merio/internal/ai/llm"
	aitools "github.com/aziz-tb/merio/internal/ai/tools"
	"github.com/aziz-tb/merio/internal/ai/tools/builtin"
	"github.com/aziz-tb/merio/internal/ai/tools/resolver"
	"github.com/stretchr/testify/require"
	"github.com/zerodha/logf"
)

// TestRun_NoVerificationGate pins the Slice 5 Stage D behavior change: the
// hardcoded run-failing verification gate (verifyAgentVerification, on
// agent.MinVerification) is GONE. An under-verified run — VerificationLevel
// BELOW the agent's MinVerification — now PROCEEDS and produces a reply (which a
// verification-conditioned guidance item steers toward "ask for verification")
// instead of failing with ErrAgentVerificationInsufficient.
//
// Safety that used to ride on this gate now lives elsewhere: consequential
// writes are gated by approval (Slice 4); customer-data reads stay scoped to the
// conversation's linked (verified) customer/contract.
func TestRun_NoVerificationGate(t *testing.T) {
	prov := &scriptedProvider{responses: []llm.CompletionResponse{
		{StopReason: llm.StopReasonToolUse, Content: []llm.ContentBlock{{
			Type:      llm.ContentBlockTypeToolUse,
			ToolUseID: "tu_draft_1",
			ToolName:  "produce_draft",
			ToolInput: map[string]any{"draft": "To help with your account, please verify your identity first."},
		}}},
	}}
	// Agent demands verification level 2; the conversation is unverified (0) —
	// exactly the case the old gate failed.
	loader := fakeAgentLoader{agent: agentmodels.Agent{
		ID: 3, Slug: "billing", Enabled: true, MinVerification: 2,
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
		AgentID: 3, InboundMessage: "cancel my contract", TraceID: "t-noverif", InboxID: 1, ConversationID: 5,
		VerificationLevel: 0, // below MinVerification — the old gate would have failed here
	})
	require.NoError(t, err, "the removed gate must no longer fail an under-verified run")
	require.Equal(t, StatusComplete, resp.Status)
	require.NotNil(t, resp.Draft)
	require.Contains(t, *resp.Draft, "verify your identity")
}
