package runner

import (
	"context"
	"errors"
	"fmt"
	"time"

	agentmodels "github.com/aziz-tb/merio/internal/ai/agents/models"
	"github.com/aziz-tb/merio/internal/ai/llm"
	aitools "github.com/aziz-tb/merio/internal/ai/tools"
	"github.com/aziz-tb/merio/internal/ai/tools/builtin"
	"github.com/aziz-tb/merio/internal/ai/tools/resolver"
	convmodels "github.com/aziz-tb/merio/internal/conversation/models"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

// Per-tool wall-clock timeouts. Internal capabilities run in-process
// and finish in milliseconds; the 15s guard catches buggy capabilities
// that loop. Custom tools dispatch through the F6 sandbox; the
// dispatcher already honors ai_tools.timeout_ms (defaulting to 30s),
// but we wrap the whole dispatch (including sandbox round-trip) in a
// safety ceiling.
const (
	internalToolTimeout       = 15 * time.Second
	customToolTimeoutFallback = 30 * time.Second
	maxParallelReads          = 5
)

type plannedToolCall struct {
	idx int
	tu  llm.ContentBlock
}

// dispatchAllTools runs read tools in parallel and writes sequentially
// afterward, preserving the model-emitted order in the returned slice.
// Per-call panic recovery + per-tool timeout live inside dispatchOneTool.
func (r *Runner) dispatchAllTools(
	ctx context.Context,
	turnToolUses []llm.ContentBlock,
	resolved []resolver.ResolvedTool,
	rt *builtin.ToolRuntimeContext,
	invokerUserID int,
	conversationID int,
	verificationLevel int,
	traceID string,
	turnIndex int,
	pub EventPublisher,
	subInvoker SubAgentInvoker,
	subParent SubAgentParentContext,
) []ToolCallLog {
	if len(turnToolUses) == 0 {
		return nil
	}

	logs := make([]ToolCallLog, len(turnToolUses))
	resolvedByName := make(map[string]resolver.ResolvedTool, len(resolved))
	for _, tool := range resolved {
		resolvedByName[tool.ID()] = tool
	}

	var reads, writes []plannedToolCall
	for i, tu := range turnToolUses {
		plan := plannedToolCall{idx: i, tu: tu}
		// call_subagent is the composer seam, not a dispatcher tool: it
		// routes to the sub-agent invoker (in dispatchOneTool) and always runs
		// sequentially — a future multi-sub-agent turn must not parallelize
		// world-changing sub-agent calls.
		if tu.ToolName == callSubagentToolName {
			writes = append(writes, plan)
			continue
		}
		category := "write"
		if tool, ok := resolvedByName[tu.ToolName]; ok {
			category = tool.Category()
		}
		if category == "read" {
			reads = append(reads, plan)
			continue
		}
		writes = append(writes, plan)
	}

	if len(reads) > 0 {
		sem := semaphore.NewWeighted(maxParallelReads)
		g, gctx := errgroup.WithContext(ctx)
		for _, plan := range reads {
			plan := plan
			g.Go(func() error {
				if err := sem.Acquire(gctx, 1); err != nil {
					logs[plan.idx] = unavailableToolLog(plan.tu, err)
					return nil
				}
				defer sem.Release(1)
				logs[plan.idx] = r.dispatchOneTool(
					gctx, plan.tu, resolvedByName, rt,
					invokerUserID, conversationID, verificationLevel,
					traceID, turnIndex, pub,
					subInvoker, subParent,
				)
				return nil
			})
		}
		_ = g.Wait()
	}

	for _, plan := range writes {
		logs[plan.idx] = r.dispatchOneTool(
			ctx, plan.tu, resolvedByName, rt,
			invokerUserID, conversationID, verificationLevel,
			traceID, turnIndex, pub,
			subInvoker, subParent,
		)
	}

	return logs
}

// dispatchOneTool runs one tool_use block through the dispatcher.
// Wraps the whole call in a deferred panic recover so a buggy
// capability or Python tool can't crash the runner goroutine.
//
// Resolution: the LLM emits tool_use with a `Name` field. We look up
// the matching ResolvedTool by name to figure out which kind to dispatch
// as. If the LLM hallucinates a tool name we didn't register, return
// an unavailable result rather than crashing.
func (r *Runner) dispatchOneTool(
	ctx context.Context,
	tu llm.ContentBlock,
	resolvedByName map[string]resolver.ResolvedTool,
	rt *builtin.ToolRuntimeContext,
	invokerUserID int,
	conversationID int,
	verificationLevel int,
	traceID string,
	turnIndex int,
	pub EventPublisher,
	subInvoker SubAgentInvoker,
	subParent SubAgentParentContext,
) (logEntry ToolCallLog) {
	// Composer seam: call_subagent is not a dispatcher tool — route it to
	// the sub-agent invoker. Handled before the timing/defer setup so it owns
	// its own events. See call_subagent.go.
	if tu.ToolName == callSubagentToolName {
		return r.handleCallSubagent(ctx, tu, subInvoker, subParent, traceID, turnIndex, pub)
	}

	startedAt := time.Now().UTC()
	logEntry = ToolCallLog{
		ToolID:    tu.ToolName,
		Input:     tu.ToolInput,
		StartedAt: startedAt,
	}

	// Panic recovery wraps the entire dispatch path. The deferred
	// closure also computes DurationMs so the audit always has a
	// timing field even on the panic path.
	defer func() {
		finishedAt := time.Now().UTC()
		logEntry.FinishedAt = finishedAt
		logEntry.DurationMs = finishedAt.Sub(startedAt).Milliseconds()
		if rec := recover(); rec != nil {
			traceID := ""
			agentID := 0
			if rt != nil {
				traceID = rt.TraceID
				agentID = rt.AgentID
			}
			r.opts.Lo.Error("runner.dispatch.panic",
				"trace_id", traceID,
				"agent_id", agentID,
				"tool_id", tu.ToolName,
				"panic", fmt.Sprintf("%v", rec),
			)
			logEntry.Recovered = true
			logEntry.Status = "execution_error"
			logEntry.Error = fmt.Sprintf("internal panic during dispatch: %v", rec)
		}
		r.emit(ctx, traceID, EventTypeToolCompleted, ToolCompletedPayload{
			TurnIndex:  turnIndex,
			ToolID:     tu.ToolName,
			ToolName:   logEntry.ToolName,
			ToolKind:   logEntry.ToolKind,
			Status:     logEntry.Status,
			DurationMs: logEntry.DurationMs,
			Data:       logEntry.Data,
			Error:      logEntry.Error,
		}, pub)
	}()

	// Match the LLM's tool name to one of the resolved tools. The
	// translator emits internal capability id and ai_tools.slug as the
	// Anthropic tool name, both unique within a run.
	found, ok := resolvedByName[tu.ToolName]
	if !ok {
		// LLM hallucinated a tool name. Surface as unavailable so the
		// model can adjust on the next turn.
		logEntry.ToolKind = "unknown"
		logEntry.Status = "unavailable"
		logEntry.Error = fmt.Sprintf("tool %q not registered for this agent", tu.ToolName)
		return logEntry
	}

	logEntry.ToolKind = string(found.Kind)
	logEntry.ToolName = found.Name()
	r.emit(ctx, traceID, EventTypeToolDispatched, ToolDispatchedPayload{
		TurnIndex: turnIndex,
		ToolID:    tu.ToolName,
		ToolName:  logEntry.ToolName,
		ToolKind:  logEntry.ToolKind,
		Input:     tu.ToolInput,
	}, pub)

	// Per-tool timeout. Internal: 15s. Custom: ai_tools.timeout_ms +
	// a small buffer (the dispatcher itself already enforces the tool's
	// declared timeout via the F6 runner; this is a belt-and-braces cap).
	timeout := internalToolTimeout
	if found.Kind == agentmodels.ToolRefKindCustom && found.Custom != nil {
		ms := found.Custom.TimeoutMS
		if ms <= 0 {
			ms = int(customToolTimeoutFallback / time.Millisecond)
		}
		timeout = time.Duration(ms)*time.Millisecond + 5*time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build the dispatch request. Slug semantics differ by kind:
	// internal → capability id; custom → ai_tools.slug.
	dispatchReq := aitools.DispatchRequest{
		Kind:              found.Kind,
		Slug:              found.ID(),
		InvokerUserID:     invokerUserID,
		VerificationLevel: verificationLevel,
		Parameters:        tu.ToolInput,
		ConversationID:    conversationID,
	}
	if found.Kind == agentmodels.ToolRefKindInternal {
		dispatchReq.RuntimeCtx = rt
	}

	result, err := r.opts.Dispatcher.Dispatch(callCtx, dispatchReq)
	if err != nil {
		logEntry.Status = classifyDispatcherError(err)
		logEntry.Error = err.Error()
		return logEntry
	}
	if result == nil {
		logEntry.Status = "execution_error"
		logEntry.Error = "dispatcher returned nil result without error"
		return logEntry
	}

	// On success, copy the dispatcher's typed result into our log.
	logEntry.ExecutionID = result.ExecutionID
	if result.Kind == "success" {
		logEntry.Status = "success"
		logEntry.Data = result.Data
		if found.Category() == "write" {
			r.invalidateLinkedCaches(conversationID, traceID, tu.ToolName)
		}
	} else {
		logEntry.Status = string(result.Kind)
		logEntry.Code = result.Code
		logEntry.Retryable = result.Retryable
		if result.Message != "" {
			logEntry.Error = result.Message
		} else {
			logEntry.Error = string(result.Kind)
		}
	}
	return logEntry
}

func (r *Runner) invalidateLinkedCaches(conversationID int, traceID, toolID string) {
	if r == nil || conversationID <= 0 || r.opts.Conversation == nil {
		return
	}
	conv, err := r.opts.Conversation.GetConversation(conversationID, "", "")
	if err != nil {
		r.opts.Lo.Warn("runner.cache_invalidation.load_conversation_failed",
			"trace_id", traceID,
			"conversation_id", conversationID,
			"tool_id", toolID,
			"error", err)
		return
	}
	r.invalidateCachesForConversation(conv, traceID, toolID)
}

func (r *Runner) invalidateCachesForConversation(conv convmodels.Conversation, traceID, toolID string) {
	if r == nil {
		return
	}
	if conv.ContractID.Valid && r.opts.Contract != nil {
		if err := r.opts.Contract.InvalidateCache(conv.ContractID.String); err != nil {
			r.opts.Lo.Warn("runner.cache_invalidation.contract_failed",
				"trace_id", traceID,
				"conversation_id", conv.ID,
				"contract_id", conv.ContractID.String,
				"tool_id", toolID,
				"error", err)
		}
	}
	if conv.CustomerID.Valid && r.opts.Customer != nil {
		if err := r.opts.Customer.InvalidateCache(conv.CustomerID.String); err != nil {
			r.opts.Lo.Warn("runner.cache_invalidation.customer_failed",
				"trace_id", traceID,
				"conversation_id", conv.ID,
				"customer_id", conv.CustomerID.String,
				"tool_id", toolID,
				"error", err)
		}
	}
}

func unavailableToolLog(tu llm.ContentBlock, err error) ToolCallLog {
	startedAt := time.Now().UTC()
	finishedAt := time.Now().UTC()
	return ToolCallLog{
		ToolKind:   "unknown",
		ToolID:     tu.ToolName,
		Input:      tu.ToolInput,
		Status:     "unavailable",
		Error:      err.Error(),
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		DurationMs: finishedAt.Sub(startedAt).Milliseconds(),
	}
}

// classifyDispatcherError maps a dispatcher sentinel error to one of the
// 4 ToolCallLog.Status values. Same scheme the manual-invoke handler
// uses for HTTP codes; consistency keeps the audit shape coherent
// across surfaces.
func classifyDispatcherError(err error) string {
	switch {
	case errors.Is(err, aitools.ErrToolNotFound):
		return "unavailable"
	case errors.Is(err, aitools.ErrToolDisabled):
		return "unavailable"
	case errors.Is(err, aitools.ErrAgentVerificationInsufficient):
		return "unavailable"
	case errors.Is(err, aitools.ErrParamsInvalid):
		return "validation_error"
	case errors.Is(err, aitools.ErrCapabilityFailed):
		return "execution_error"
	case errors.Is(err, aitools.ErrImplInvalid):
		return "execution_error"
	case aitools.IsRunnerErrSandboxUnavailable(err):
		return "unavailable"
	}
	return "execution_error"
}
