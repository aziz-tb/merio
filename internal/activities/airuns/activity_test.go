package airuns

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	agentmodels "github.com/abhinavxd/libredesk/internal/ai/agents/models"
	executions "github.com/abhinavxd/libredesk/internal/ai/executions"
	executionmodels "github.com/abhinavxd/libredesk/internal/ai/executions/models"
	"github.com/abhinavxd/libredesk/internal/ai/llm"
	"github.com/abhinavxd/libredesk/internal/ai/llm/fake"
	"github.com/abhinavxd/libredesk/internal/ai/llm/preset"
	"github.com/abhinavxd/libredesk/internal/ai/runner"
	runnerevents "github.com/abhinavxd/libredesk/internal/ai/runner/events"
	aitools "github.com/abhinavxd/libredesk/internal/ai/tools"
	"github.com/abhinavxd/libredesk/internal/ai/tools/builtin"
	"github.com/abhinavxd/libredesk/internal/ai/tools/resolver"
	wfairuns "github.com/abhinavxd/libredesk/internal/workflows/airuns"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/zerodha/logf"
)

type stubActivityAgentLoader struct {
	agent agentmodels.Agent
}

func (s stubActivityAgentLoader) Get(id int) (agentmodels.Agent, error) {
	return s.agent, nil
}

func TestRunAgentActivity_ComposerPublishesCompletedResponse(t *testing.T) {
	client := newTestRedis(t)
	lo := logf.New(logf.Opts{Level: logf.ErrorLevel})
	pub, err := runnerevents.NewRedisStreamPublisher(client, &lo, time.Minute)
	require.NoError(t, err)

	provider := fake.New()
	provider.RegisterStatic(llm.PurposeReason, llm.CompletionResponse{
		StopReason: llm.StopReasonToolUse,
		Content: []llm.ContentBlock{{
			Type:      llm.ContentBlockTypeToolUse,
			ToolUseID: "toolu_pd_1",
			ToolName:  "produce_draft",
			ToolInput: map[string]any{"draft": "done"},
		}},
		Usage: llm.Usage{InputTokens: 10, OutputTokens: 5},
	})

	acts := &Activities{
		Runner:     newActivityTestRunner(t, provider),
		Executions: &executions.Manager{},
		Events:     pub,
		Lo:         &lo,
	}

	params := wfairuns.AgentRunWorkflowParams{
		TraceID:        "trace-composer",
		Source:         executionmodels.SourceComposer,
		AgentID:        12,
		ConversationID: 0,
		InboundMessage: "hi",
		InvokerUserID:  1,
		EnqueuedAt:     time.Now().UTC(),
	}

	out, err := acts.RunAgentActivity(context.Background(), params)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, "trace-composer", out.TraceID)
	require.True(t, out.DraftPresent)

	events := decodeStreamEvents(t, client, params.TraceID)
	require.NotEmpty(t, events)
	require.Equal(t, runner.EventTypeRunCompleted, events[len(events)-1].Type)

	var payload runner.RunCompletedPayload
	require.NoError(t, json.Unmarshal(events[len(events)-1].Payload, &payload))
	require.Equal(t, executionmodels.SourceComposer, payload.Source)
	require.NotNil(t, payload.Response)
	require.NotNil(t, payload.Response.Draft)
	require.Equal(t, "done", *payload.Response.Draft)
}

func TestRunAgentActivity_PipelineUsesNoopPublisher(t *testing.T) {
	client := newTestRedis(t)
	lo := logf.New(logf.Opts{Level: logf.ErrorLevel})
	pub, err := runnerevents.NewRedisStreamPublisher(client, &lo, time.Minute)
	require.NoError(t, err)

	provider := fake.New()
	provider.RegisterStatic(llm.PurposeReason, llm.CompletionResponse{
		StopReason: llm.StopReasonToolUse,
		Content: []llm.ContentBlock{{
			Type:      llm.ContentBlockTypeToolUse,
			ToolUseID: "toolu_pd_1",
			ToolName:  "produce_draft",
			ToolInput: map[string]any{"draft": "done"},
		}},
		Usage: llm.Usage{InputTokens: 10, OutputTokens: 5},
	})

	acts := &Activities{
		Runner:     newActivityTestRunner(t, provider),
		Executions: &executions.Manager{},
		Events:     pub,
		Lo:         &lo,
	}

	params := wfairuns.AgentRunWorkflowParams{
		TraceID:        "trace-pipeline",
		Source:         executionmodels.SourcePipeline,
		AgentID:        12,
		ConversationID: 0,
		InboundMessage: "hi",
		EnqueuedAt:     time.Now().UTC(),
	}

	out, err := acts.RunAgentActivity(context.Background(), params)
	require.NoError(t, err)
	require.NotNil(t, out)

	exists, err := client.Exists(context.Background(), runnerevents.StreamKey(params.TraceID)).Result()
	require.NoError(t, err)
	require.EqualValues(t, 0, exists)
}

func TestRunAgentActivity_FailedRunPublishesSanitizedError(t *testing.T) {
	client := newTestRedis(t)
	lo := logf.New(logf.Opts{Level: logf.ErrorLevel})
	pub, err := runnerevents.NewRedisStreamPublisher(client, &lo, time.Minute)
	require.NoError(t, err)

	provider := fake.New()
	provider.RegisterError(llm.PurposeReason, llm.ErrInvalidRequest)

	acts := &Activities{
		Runner:     newActivityTestRunner(t, provider),
		Executions: &executions.Manager{},
		Events:     pub,
		Lo:         &lo,
	}

	params := wfairuns.AgentRunWorkflowParams{
		TraceID:        "trace-failed",
		Source:         executionmodels.SourceComposer,
		AgentID:        12,
		ConversationID: 0,
		InboundMessage: "hi",
		EnqueuedAt:     time.Now().UTC(),
	}

	out, err := acts.RunAgentActivity(context.Background(), params)
	require.Error(t, err)
	require.Nil(t, out)

	events := decodeStreamEvents(t, client, params.TraceID)
	require.NotEmpty(t, events)
	require.Equal(t, runner.EventTypeRunFailed, events[len(events)-1].Type)

	var payload runner.RunFailedPayload
	require.NoError(t, json.Unmarshal(events[len(events)-1].Payload, &payload))
	require.Equal(t, "Request validation failed", payload.Error)
	require.Equal(t, executionmodels.SourceComposer, payload.Source)
}

func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr := miniredis.RunT(t)
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func decodeStreamEvents(t *testing.T, client *redis.Client, traceID string) []runner.RunEvent {
	t.Helper()
	entries, err := client.XRange(context.Background(), runnerevents.StreamKey(traceID), "-", "+").Result()
	require.NoError(t, err)

	out := make([]runner.RunEvent, 0, len(entries))
	for _, entry := range entries {
		raw, _ := entry.Values["json"].(string)
		var ev runner.RunEvent
		require.NoError(t, json.Unmarshal([]byte(raw), &ev))
		out = append(out, ev)
	}
	return out
}

func newActivityTestRunner(t *testing.T, provider *fake.Provider) *runner.Runner {
	t.Helper()

	presetReg := testActivityPresetRegistry(t)
	lo := logf.New(logf.Opts{Level: logf.ErrorLevel})
	reg := builtin.NewRegistry()
	dispatcher := aitools.NewDispatcherWithRegistry(nil, nil, reg)
	res := resolver.New(nil, reg)

	out, err := runner.New(runner.Opts{
		Provider:   provider,
		Agents:     stubActivityAgentLoader{agent: testActivityAgent()},
		Resolver:   res,
		Dispatcher: dispatcher,
		PresetReg:  presetReg,
		Lo:         &lo,
	})
	require.NoError(t, err)
	return out
}

func testActivityAgent() agentmodels.Agent {
	return agentmodels.Agent{
		ID:                      12,
		Name:                    "Test Agent",
		Slug:                    "test-agent",
		Prompt:                  "Bearbeite die Kundenanfrage.",
		Enabled:                 true,
		MaxIterations:           5,
		ExtendedThinkingEnabled: true,
		UpdatedAt:               time.Now().UTC(),
	}
}

func testActivityPresetRegistry(t *testing.T) *preset.Registry {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ai_presets.json")
	body := `{
		"presets": [
			{
				"slug": "test-sonnet",
				"name": "Test Sonnet",
				"description": "test preset",
				"provider": "anthropic",
				"model": "claude-sonnet-4-6",
				"temperature": 0.1,
				"max_tokens": 512,
				"caching_enabled": false,
				"caching_extended_ttl": false,
				"enabled": true
			}
		],
		"purpose_mappings": {
			"classify": "test-sonnet",
			"summarize": "test-sonnet",
			"reason": "test-sonnet",
			"synthesize": "test-sonnet",
			"redact_pii": "test-sonnet",
			"brief": "test-sonnet",
			"classification": "test-sonnet",
			"extract_parameters": "test-sonnet"
		}
	}`
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	reg, err := preset.Load(path, nil)
	require.NoError(t, err)
	return reg
}
