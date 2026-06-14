package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zerodha/logf"

	agentmodels "github.com/aziz-tb/merio/internal/ai/agents/models"
	"github.com/aziz-tb/merio/internal/ai/embeddings"
	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/aziz-tb/merio/internal/ai/llm/preset"
	aitools "github.com/aziz-tb/merio/internal/ai/tools"
	"github.com/aziz-tb/merio/internal/ai/tools/builtin"
	"github.com/aziz-tb/merio/internal/ai/tools/resolver"
	"github.com/aziz-tb/merio/internal/ai/tools/translator"
	"github.com/aziz-tb/merio/internal/ai/usage/redact"
	"github.com/aziz-tb/merio/internal/contract"
	"github.com/aziz-tb/merio/internal/conversation"
	"github.com/aziz-tb/merio/internal/customer"
	execModels "github.com/aziz-tb/merio/internal/exec/runner/models"
	"github.com/aziz-tb/merio/internal/knowledge"
	"github.com/aziz-tb/merio/internal/user"
)

// AgentLoader is the slice of aiagents.Manager the runner needs.
// Defined as an interface so tests can substitute a simple fake without
// spinning up a Postgres connection. Production uses *aiagents.Manager
// directly (it implements this).
type AgentLoader interface {
	Get(id int) (agentmodels.Agent, error)
}

// ToolResolver resolves an agent's tools JSONB into ResolvedTools. Defined as
// an interface (like AgentLoader) so tests substitute a fake; production wires
// *resolver.Resolver.
type ToolResolver interface {
	Resolve(rawTools json.RawMessage, kbScope builtin.KBScope) ([]resolver.ResolvedTool, map[int]string, error)
}

// ToolDispatcher dispatches one resolved tool call through the unified
// dispatcher. Interface so tests substitute a fake (e.g. to assert the approval
// resume executes a write exactly once); production wires *aitools.Dispatcher.
type ToolDispatcher interface {
	Dispatch(ctx context.Context, req aitools.DispatchRequest) (*execModels.Result, error)
}

// Defaults applied when RunRequest fields are zero. v1 hardcoded; the
// test endpoint exposes them so admins can simulate budget paths.
const (
	defaultMaxIterations    = 5
	defaultTotalTokenBudget = 50000
	defaultDeadline         = 60 * time.Second
)

// Sentinel errors returned alongside RunResponse{Status: StatusFailed}.
// Callers errors.Is against these for HTTP status mapping.
var (
	ErrAgentNotFound                    = errors.New("runner: agent not found")
	ErrAgentDisabled                    = errors.New("runner: agent is disabled")
	ErrAgentVerificationInsufficient    = aitools.ErrAgentVerificationInsufficient
	agentVerificationInsufficientReason = "agent requires a verified customer; conversation is not sufficiently verified"
)

// UsageRecorder is the runner's view of the ai_usage write surface.
// Production wires *usage.Manager; tests can substitute a counting
// fake. Defined as an interface (not an import of the usage package)
// so the runner package doesn't pull a DB-touching dependency just
// for the type assertion.
type UsageRecorder interface {
	Record(ctx context.Context, row UsageRow) error
}

// UsageRow is the runner's projection of usage.Row — the subset of
// fields the runner can populate from inside its loop, before handing
// off to the persistence layer. The handler layer fills in
// ConversationID / AgentExecutionID / UserID via the same Row shape
// when it calls Record() itself (chunk 8 — rewrite manager wrapper).
//
// Kept here so the runner doesn't import internal/ai/usage; the app
// and worker adapters translate this to usage.Row at the boundary.
type UsageRow struct {
	TraceID             string
	CallSequenceInTrace int
	InboxID             int
	ConversationID      *int64
	AgentID             *int
	UserID              *int64

	Provider   string
	Model      string
	PresetSlug string

	Purpose    string
	StopReason string

	InputTokens       int
	OutputTokens      int
	CachedInputTokens int
	CacheMarkersUsed  int

	LatencyMs  int
	RetryCount int

	Outcome string

	ProviderRequestID *string
	ProviderMetadata  []byte

	RequestPayload  []byte
	ResponsePayload []byte
}

// Opts wires every dependency the runner needs. All required except
// Usage, which the runner tolerates as nil for tests / migration
// windows where ai_usage isn't yet wired.
type Opts struct {
	Provider     llm.LLMProvider
	Agents       AgentLoader
	Resolver     ToolResolver
	Dispatcher   ToolDispatcher
	PresetReg    *preset.Registry
	Lo           *logf.Logger
	Events       EventPublisher
	Knowledge    *knowledge.Manager
	Conversation *conversation.Manager
	User         *user.Manager
	Customer     *customer.Manager
	Contract     *contract.Manager
	Usage        UsageRecorder
}

// Runner is the LLM↔tool loop. Construct once at app startup; every
// agent run uses the same instance.
type Runner struct {
	opts Opts
}

// New builds a Runner. Returns an error when required deps are missing.
func New(opts Opts) (*Runner, error) {
	if opts.Provider == nil {
		return nil, errors.New("runner: Provider is nil")
	}
	if opts.Agents == nil {
		return nil, errors.New("runner: Agents manager is nil")
	}
	if opts.Resolver == nil {
		return nil, errors.New("runner: Resolver is nil")
	}
	if opts.Dispatcher == nil {
		return nil, errors.New("runner: Dispatcher is nil")
	}
	if opts.PresetReg == nil {
		return nil, errors.New("runner: PresetReg is nil")
	}
	if opts.Lo == nil {
		return nil, errors.New("runner: Lo is nil")
	}
	return &Runner{opts: opts}, nil
}

// Run executes one agent run. Always returns a populated RunResponse;
// the second return is the terminal error (if any). Run never panics
// — every panic source (LLM provider, tool dispatch, JSON encode) is
// either recovered locally or wrapped at a deeper boundary.
func (r *Runner) Run(ctx context.Context, req RunRequest) (finalResp RunResponse, finalErr error) {
	var err error
	req, err = applyDefaults(req)
	if err != nil {
		return RunResponse{Status: StatusFailed, PartialReason: err.Error()}, err
	}
	pub := r.eventPublisher(req)

	if strings.TrimSpace(req.InboundMessage) == "" {
		return RunResponse{Status: StatusFailed, PartialReason: "inbound_message is required"},
			errors.New("runner: inbound_message is required")
	}
	// A composer run has no ai_agents row (it uses the platform config),
	// so AgentID is required only for ordinary agent runs.
	if !req.IsComposer && req.AgentID <= 0 {
		return RunResponse{Status: StatusFailed, PartialReason: "agent_id is required"},
			errors.New("runner: agent_id is required")
	}

	// Wrap caller ctx with the deadline. callerCtx may have its own
	// deadline; whichever fires first wins.
	runCtx, cancel := context.WithTimeout(ctx, req.Deadline)
	defer cancel()

	startedAt := time.Now()

	// Load the agent. A composer run is NOT backed by an ai_agents row —
	// the composer is a platform component executed via the Runner — so it
	// uses the platform composer config instead of a DB lookup. See
	// call_subagent.go (TODO(slice-3): config comes from Composer).
	var agent agentmodels.Agent
	if req.IsComposer {
		agent = platformComposerAgent()
	} else {
		agent, err = r.opts.Agents.Get(req.AgentID)
		if err != nil {
			return RunResponse{Status: StatusFailed, PartialReason: "agent not found"}, fmt.Errorf("%w: %v", ErrAgentNotFound, err)
		}
	}
	if !agent.Enabled {
		return RunResponse{
			Status: StatusFailed, PartialReason: "agent is disabled", AgentSlug: agent.Slug,
		}, ErrAgentDisabled
	}
	if err := verifyAgentVerification(req, agent); err != nil {
		return RunResponse{
			Status:        StatusFailed,
			PartialReason: agentVerificationInsufficientReason,
			AgentSlug:     agent.Slug,
		}, err
	}
	r.emit(runCtx, req.TraceID, EventTypeRunStarted, RunStartedPayload{
		AgentID:        agent.ID,
		AgentSlug:      agent.Slug,
		ConversationID: req.ConversationID,
	}, pub)

	kbScope := decodeKBScope(agent.KBScope, r.opts.Lo, req.TraceID, agent.ID)

	// Resolve agent.tools to ResolvedTool. Drops are surfaced into
	// RunResponse so the operator can see why a declared tool wasn't
	// available (missing capability, empty kb_scope, disabled custom tool).
	resolved, drops, err := r.opts.Resolver.Resolve(agent.Tools, kbScope)
	if err != nil {
		return RunResponse{
			Status: StatusFailed, PartialReason: "tool resolution failed", AgentSlug: agent.Slug,
		}, fmt.Errorf("runner: resolve tools: %w", err)
	}

	// Translate resolved tools to provider-agnostic ToolDef.
	toolDefs, err := translator.Translate(resolved)
	if err != nil {
		return RunResponse{
			Status: StatusFailed, PartialReason: "tool translation failed", AgentSlug: agent.Slug, Drops: drops,
		}, fmt.Errorf("runner: translate tools: %w", err)
	}
	// Terminal-tool injection. Composers terminate via a real decision
	// (answer/escalate); every other agent terminates via produce_draft.
	// produce_draft is NOT offered to composers — a successful composer
	// run requires a terminal DECISION, not a draft. Gated on the run-level
	// IsComposer flag (the composer's identity comes from how the run is
	// started, not from a row). See terminal_decision.go.
	composerInstructions := ""
	if req.IsComposer {
		toolDefs = append(toolDefs, answerToolDef, escalateToolDef)
		composerInstructions = terminalDecisionInstructions

		// A composer run carrying a sub-agent invoker also gets
		// call_subagent. The decision tools above are always present (an
		// composer must be able to terminate even with no invoker); the
		// invoker only adds delegation. See call_subagent.go.
		if req.SubAgentInvoker != nil {
			toolDefs = append(toolDefs, callSubagentToolDef)
			composerInstructions = composerInstructions + "\n\n" + callSubagentInstructions
		} else {
			r.opts.Lo.Warn("runner: composer run has no sub-agent invoker; call_subagent unavailable",
				"trace_id", req.TraceID)
		}
	} else {
		toolDefs = append(toolDefs, produceDraftToolDef)
	}

	// Resolve preset for the agent's main reasoning purpose. v1 hardcodes
	// "reason" for every turn; once stage-specific purposes land
	// (extract_parameters mid-turn, etc.) this becomes per-turn.
	overrides := decodePresetOverrides(agent.PresetOverrides)
	pres, err := r.opts.PresetReg.Resolve(llm.PurposeReason, overrides)
	if err != nil {
		return RunResponse{
			Status: StatusFailed, PartialReason: "preset resolution failed", AgentSlug: agent.Slug, Drops: drops,
		}, fmt.Errorf("runner: resolve preset: %w", err)
	}

	// Build the runtime context handed to internal capabilities. The KB
	// scope is decoded once per run and threaded into every dispatch
	// (kb_search reads rt.KBScope to enforce intersection semantics).
	// A malformed kb_scope JSONB is treated as "no scope" — better to
	// surface a noisy log and continue than to fail the whole run on a
	// stored JSON shape we already validated at agent save time.
	kbCache := embeddings.NewSearchResultCache()
	rt := &builtin.ToolRuntimeContext{
		ConversationID:      req.ConversationID,
		AgentID:             agent.ID,
		ResolutionAttemptID: 0, // populated by F3 when it lands
		TraceID:             req.TraceID,
		VerificationLevel:   req.VerificationLevel,
		KBScope:             kbScope,
		KBSearchCache:       kbCache,
		Logf:                r.opts.Lo,
		Knowledge:           r.opts.Knowledge,
		Conversation:        r.opts.Conversation,
		User:                r.opts.User,
		Customer:            r.opts.Customer,
		Contract:            r.opts.Contract,
	}

	// resolvedByName lets the approval gate and the resume path find a
	// ResolvedTool by the slug the LLM emits as the tool name.
	resolvedByName := make(map[string]resolver.ResolvedTool, len(resolved))
	for _, tool := range resolved {
		resolvedByName[tool.ID()] = tool
	}

	var (
		trace       []TurnTrace
		toolCallLog []ToolCallLog
		totalUsage  llm.Usage
		llmLatency  int
		toolLatency int
		cacheHits   int
		iterations  int
	)

	// Build the initial messages. A resuming run reconstructs prior context
	// from the gated run's trace and pre-feeds the approved write's result
	// (executing it exactly once); a fresh run builds the normal system +
	// inbound seed. The cache marker manager seeds after the last system msg.
	var messages []llm.Message
	if req.Resume != nil {
		var extraTrace []TurnTrace
		var extraLogs []ToolCallLog
		messages, extraTrace, extraLogs = r.seedResumeMessages(runCtx, req, agent, composerInstructions, resolvedByName, rt, pub)
		trace = append(trace, extraTrace...)
		toolCallLog = append(toolCallLog, extraLogs...)
	} else {
		messages = buildInitialMessages(agent.Prompt, req.Context, req.ConversationHistory, req.InboundMessage, req.IsComposer, composerInstructions)
	}
	systemCount := countSystemMessages(messages)
	cacheMgr := newCacheMarkerManager(systemCount)

	// Per-agent iteration cap. The agent's max_iterations (F9 schema)
	// is one source; the request's MaxIterations override is another;
	// we use the agent's value as the default when the request didn't
	// explicitly override.
	maxIters := req.MaxIterations
	if maxIters == 0 {
		maxIters = agent.MaxIterations
	}
	if maxIters <= 0 {
		maxIters = defaultMaxIterations
	}

	// Composer fail-safe. Registered HERE — after every pre-loop config
	// failure has already returned StatusFailed — so a misconfigured
	// composer fails loud, but once the agentic loop starts the run is
	// structurally incapable of ending without a terminal decision: any exit
	// that did not reach answer/escalate (model stopped, max-iterations,
	// refusal, between-turns deadline, token budget) defaults to ESCALATE (fail
	// safe to a human), never a silent incomplete that drops the ticket. Error
	// returns (finalErr != nil) are deliberately left untouched here — the
	// activity boundary owns the retry-vs-escalate-vs-fail-loud decision so we
	// don't cannibalize Temporal's retry net. See terminal_decision.go.
	if req.IsComposer {
		defer func() {
			finalResp.IsComposer = true
			// A run paused for write approval is a legitimate terminal state —
			// leave it; the fail-safe only catches genuine no-decision exits.
			if finalErr == nil && finalResp.Decision == nil && finalResp.Status != StatusAwaitingApproval {
				finalResp = synthesizeEscalation(finalResp, composerNoDecisionReason(finalResp))
			}
		}()
	}

	// Loop. Pre-flight checks run at the TOP of each iteration so we
	// never start a turn we can't afford.
	for iter := 0; iter < maxIters; iter++ {
		iterations = iter + 1
		r.emit(runCtx, req.TraceID, EventTypeTurnStarted, TurnStartedPayload{
			TurnIndex: iter,
		}, pub)

		// Deadline pre-flight (between turns; the LLM call itself
		// honors runCtx independently).
		if err := runCtx.Err(); err != nil {
			r.emitRunLog(req.TraceID, agent, iterations, totalUsage, "deadline_or_cancel", err.Error())
			resp := r.buildResponse(agent, nil, trace, toolCallLog, totalUsage, llmLatency, toolLatency, cacheHits, cacheMgr.Count(), iterations, drops)
			if errors.Is(err, context.DeadlineExceeded) {
				resp.Status = StatusIncomplete
				resp.PartialReason = incompleteWithoutProduceDraft("deadline reached between turns")
				return resp, nil
			}
			resp.Status = StatusFailed
			resp.PartialReason = "context cancelled: " + err.Error()
			return resp, err
		}

		// Token budget pre-flight.
		spent := totalUsage.InputTokens + totalUsage.OutputTokens
		if spent >= req.TotalTokenBudget {
			resp := r.buildResponse(agent, nil, trace, toolCallLog, totalUsage, llmLatency, toolLatency, cacheHits, cacheMgr.Count(), iterations-1, drops)
			resp.Status = StatusIncomplete
			resp.PartialReason = incompleteWithoutProduceDraft(
				fmt.Sprintf("token budget %d exceeded (spent %d before turn %d)", req.TotalTokenBudget, spent, iter),
			)
			r.emitRunLog(req.TraceID, agent, iterations-1, totalUsage, string(StatusIncomplete), resp.PartialReason)
			return resp, nil
		}

		// Build the completion request for this turn.
		compReq := llm.CompletionRequest{
			Purpose:             llm.PurposeReason,
			PresetSlug:          pres.Slug,
			Messages:            messages,
			Tools:               toolDefs,
			ThinkingEnabled:     agent.ExtendedThinkingEnabled,
			CachedBlocks:        cacheMgr.Markers(),
			TraceID:             req.TraceID,
			AgentID:             agent.ID,
			ResolutionAttemptID: 0,
		}

		turnStart := time.Now()
		resp, err := r.opts.Provider.Complete(runCtx, compReq)
		turnLatencyMs := int(time.Since(turnStart).Milliseconds())
		llmLatency += turnLatencyMs

		if err != nil {
			// Provider already retried transients; this is terminal.
			// Record a failure row anyway — ops needs to see the call
			// happened even when it failed. Outcome maps from the
			// error class.
			r.recordUsage(runCtx, req, compReq, agent, pres, iter+1, resp, turnLatencyMs, outcomeForError(err), err)
			out := r.buildResponse(agent, nil, trace, toolCallLog, totalUsage, llmLatency, toolLatency, cacheHits, cacheMgr.Count(), iterations, drops)
			out.Status = StatusFailed
			out.PartialReason = "provider error: " + err.Error()
			r.emitRunLog(req.TraceID, agent, iterations, totalUsage, "provider_error", err.Error())
			return out, err
		}

		// Successful call — record one ai_usage row per turn. Best-
		// effort: persistence failures are logged inside recordUsage
		// and never abort the run.
		r.recordUsage(runCtx, req, compReq, agent, pres, iter+1, resp, turnLatencyMs, "success", nil)

		// Update tracking. Anthropic returns CachedTokens separately
		// from InputTokens (we surface both).
		totalUsage.InputTokens += resp.Usage.InputTokens
		totalUsage.OutputTokens += resp.Usage.OutputTokens
		totalUsage.CachedTokens += resp.Usage.CachedTokens
		if resp.Usage.CachedTokens > 0 {
			cacheHits++
		}
		turnText := llm.ExtractText(resp.Content)
		turnThinking := llm.ExtractThinkingBlocks(resp.Content)
		turnToolUses := llm.ExtractToolUses(resp.Content)
		thinkingTrace := make([]ThinkingBlockTrace, 0, len(turnThinking))
		for _, b := range turnThinking {
			thinkingTrace = append(thinkingTrace, ThinkingBlockTrace{
				Type:      string(b.Type),
				Thinking:  b.Text,
				Signature: b.Signature,
				Data:      b.Data,
			})
		}

		// Record the turn BEFORE acting on stop_reason — partial paths
		// still leave a trail.
		turn := TurnTrace{
			TurnIndex:      iter,
			StopReason:     resp.StopReason,
			Text:           turnText,
			ThinkingBlocks: thinkingTrace,
			LatencyMs:      turnLatencyMs,
			Usage:          resp.Usage,
		}
		r.emit(runCtx, req.TraceID, EventTypeTurnCompleted, TurnCompletedPayload{
			TurnIndex:  iter,
			StopReason: string(resp.StopReason),
			LatencyMs:  turnLatencyMs,
		}, pub)

		// Stop-reason dispatch table. See package doc for the contract;
		// each branch is intentional, do NOT collapse them.
		switch resp.StopReason {
		case llm.StopReasonEndTurn:
			trace = append(trace, turn)
			r.emitRunLog(req.TraceID, agent, iterations, totalUsage, string(StatusIncomplete), "end_turn without produce_draft")
			out := r.buildResponse(agent, nil, trace, toolCallLog, totalUsage, llmLatency, toolLatency, cacheHits, cacheMgr.Count(), iterations, drops)
			out.Status = StatusIncomplete
			out.PartialReason = incompleteWithoutProduceDraft("agent ended with stop_reason=end_turn")
			return out, nil

		case llm.StopReasonStopSequence:
			// Configured stop — preset author asked the model to halt
			// here. Without produce_draft the run is still incomplete.
			trace = append(trace, turn)
			r.emitRunLog(req.TraceID, agent, iterations, totalUsage, string(StatusIncomplete), "stop_sequence without produce_draft")
			out := r.buildResponse(agent, nil, trace, toolCallLog, totalUsage, llmLatency, toolLatency, cacheHits, cacheMgr.Count(), iterations, drops)
			out.Status = StatusIncomplete
			out.PartialReason = incompleteWithoutProduceDraft("agent ended with stop_reason=stop_sequence")
			return out, nil

		case llm.StopReasonMaxTokens:
			trace = append(trace, turn)
			r.emitRunLog(req.TraceID, agent, iterations, totalUsage, string(StatusIncomplete), "max_tokens without produce_draft")
			out := r.buildResponse(agent, nil, trace, toolCallLog, totalUsage, llmLatency, toolLatency, cacheHits, cacheMgr.Count(), iterations, drops)
			out.Status = StatusIncomplete
			out.PartialReason = incompleteWithoutProduceDraft("agent ended with stop_reason=max_tokens")
			return out, nil

		case llm.StopReasonRefusal:
			trace = append(trace, turn)
			r.emitRunLog(req.TraceID, agent, iterations, totalUsage, string(StatusIncomplete), "refusal without produce_draft")
			out := r.buildResponse(agent, nil, trace, toolCallLog, totalUsage, llmLatency, toolLatency, cacheHits, cacheMgr.Count(), iterations, drops)
			out.Status = StatusIncomplete
			out.PartialReason = incompleteWithoutProduceDraft("model refused to respond")
			return out, nil

		case llm.StopReasonToolUse:
			// Composers terminate via answer/escalate (checked first; they
			// have no produce_draft). A non-decision tool turn (e.g.
			// call_subagent) falls through to normal dispatch below.
			if req.IsComposer {
				if handled, decision, decisionLogs, partialReason := maybeHandleTerminalDecisionTurn(turnToolUses); handled {
					turn.ToolCalls = decisionLogs
					trace = append(trace, turn)
					toolCallLog = append(toolCallLog, decisionLogs...)
					if decision != nil {
						out := r.buildResponse(agent, decisionDraft(decision), trace, toolCallLog, totalUsage, llmLatency, toolLatency, cacheHits, cacheMgr.Count(), iterations, drops)
						out.Status = StatusComplete
						out.Decision = &TerminalDecision{Kind: decision.kind, InternalReason: decision.internalReason}
						if decision.kind == DecisionKindAnswer {
							r.emit(runCtx, req.TraceID, EventTypeDraftCommitted, DraftCommittedPayload{
								TurnIndex: iter,
							}, pub)
						}
						r.emitRunLog(req.TraceID, agent, iterations, totalUsage, string(StatusComplete), "terminal decision: "+decision.kind)
						return out, nil
					}

					// Malformed terminal call (empty message, missing reason, or
					// not the only tool in its turn). Feed the validation error
					// back as the tool result so the composer can correct
					// itself on the next turn — the same reject-feeds-back pattern
					// as call_subagent. The run does NOT end and does NOT
					// re-dispatch: max-iterations bounds the retries and the
					// fail-safe escalates a persistent failure, so a formatting
					// slip never escalates a ticket to a human.
					messages = append(messages,
						llm.Message{Role: llm.RoleAssistant, Content: resp.Content},
						llm.Message{Role: llm.RoleUser, Content: terminalDecisionErrorResults(turnToolUses, partialReason)},
					)
					cacheMgr.AddMarkerAfter(len(messages) - 1)
					continue
				}
			} else if handled, draft, produceLogs, partialReason := maybeHandleProduceDraftTurn(turnToolUses); handled {
				turn.ToolCalls = produceLogs
				trace = append(trace, turn)
				toolCallLog = append(toolCallLog, produceLogs...)
				out := r.buildResponse(agent, draft, trace, toolCallLog, totalUsage, llmLatency, toolLatency, cacheHits, cacheMgr.Count(), iterations, drops)
				if draft != nil {
					r.emit(runCtx, req.TraceID, EventTypeDraftCommitted, DraftCommittedPayload{
						TurnIndex: iter,
					}, pub)
					r.emitRunLog(req.TraceID, agent, iterations, totalUsage, string(StatusComplete), "produce_draft committed")
					out.Status = StatusComplete
				} else {
					r.emitRunLog(req.TraceID, agent, iterations, totalUsage, string(StatusIncomplete), partialReason)
					out.Status = StatusIncomplete
					out.PartialReason = partialReason
				}
				return out, nil
			}

			// Approval gate (slug-driven, origin-agnostic): if the model wants
			// to dispatch a tool whose slug is in the approval-required set,
			// STOP before executing — record the proposed write and end the run
			// awaiting human approval. A resuming run executes it. The same
			// check covers a composer write and a sub-agent write because both
			// run through this one loop. See approval.go.
			if gated := firstGatedToolUse(turnToolUses, resolvedByName, req.ApprovalRequiredSlugs); gated != nil {
				// Stamp the run identity so the activity persists this gate
				// uniformly. The bubble-up path keeps the sub-agent's identity
				// (set here in its own run) and the invoker adds ParentTraceID.
				gated.GatedTraceID = req.TraceID
				gated.AgentID = agentIDPtr(agent.ID)
				turn.ToolCalls = []ToolCallLog{newAwaitingApprovalLog(*gated)}
				trace = append(trace, turn)
				toolCallLog = append(toolCallLog, turn.ToolCalls...)
				out := r.buildResponse(agent, nil, trace, toolCallLog, totalUsage, llmLatency, toolLatency, cacheHits, cacheMgr.Count(), iterations, drops)
				out.Status = StatusAwaitingApproval
				out.ProposedWrite = gated
				r.emitRunLog(req.TraceID, agent, iterations, totalUsage, string(StatusAwaitingApproval), "awaiting approval for tool "+gated.ToolSlug)
				return out, nil
			}

			// Dispatch the remaining non-terminal tool_use blocks.
			// dispatchOneTool wraps each call in panic recovery +
			// per-tool timeout.
			toolStart := time.Now()
			results := r.dispatchAllTools(
				runCtx,
				turnToolUses,
				resolved,
				rt,
				req.InvokerUserID,
				req.ConversationID,
				req.VerificationLevel,
				req.TraceID,
				iter,
				pub,
				req.SubAgentInvoker,
				subAgentParentContext(req),
			)
			toolLatency += int(time.Since(toolStart).Milliseconds())
			turn.ToolCalls = results
			trace = append(trace, turn)
			toolCallLog = append(toolCallLog, results...)

			// Sub-agent approval bubble-up: if a call_subagent's sub-agent
			// paused at an approval-gated write, the composer run ends awaiting
			// approval too — it can't compose without the sub-agent's real
			// result. The gated run is the sub-agent's (one record, persisted by
			// the invoker); the composer's ProposedWrite mirrors it for the UI.
			if pw := bubbledApproval(results); pw != nil {
				out := r.buildResponse(agent, nil, trace, toolCallLog, totalUsage, llmLatency, toolLatency, cacheHits, cacheMgr.Count(), iterations, drops)
				out.Status = StatusAwaitingApproval
				out.ProposedWrite = pw
				r.emitRunLog(req.TraceID, agent, iterations, totalUsage, string(StatusAwaitingApproval), "sub-agent awaiting approval for tool "+pw.ToolSlug)
				return out, nil
			}

			// Append assistant turn (with text + tool_use blocks) and
			// the tool_result block(s) to messages for the next iteration.
			assistantMsg := llm.Message{Role: llm.RoleAssistant, Content: resp.Content}
			toolResultBlocks := make([]llm.ContentBlock, 0, len(results))
			for i, res := range results {
				tu := turnToolUses[i]
				resultText := encodeToolResult(res)
				toolResultBlocks = append(toolResultBlocks, llm.ContentBlock{
					Type:            llm.ContentBlockTypeToolResult,
					ResultToolUseID: tu.ToolUseID,
					ResultContent:   resultText,
					ResultIsError:   res.Status != "success",
				})
			}
			toolResultMsg := llm.Message{Role: llm.RoleUser, Content: toolResultBlocks}
			messages = append(messages, assistantMsg, toolResultMsg)

			// Cumulative cache marker — the just-appended tool_result
			// message becomes part of the stable prefix on the next
			// request. Index points at the tool_result message (the
			// last we appended).
			cacheMgr.AddMarkerAfter(len(messages) - 1)
			continue

		case llm.StopReasonPauseTurn:
			// Newer Anthropic models occasionally signal "I'm thinking,
			// continue without dispatching tools." Append the assistant
			// turn so context grows; max_iterations bounds recurrence.
			trace = append(trace, turn)
			messages = append(messages, llm.Message{Role: llm.RoleAssistant, Content: resp.Content})
			continue

		default:
			// Unknown stop_reason. Log loudly so ops sees it, then
			// decide based on whether we have text to surface.
			trace = append(trace, turn)
			r.opts.Lo.Warn("runner.unknown_stop_reason",
				"trace_id", req.TraceID,
				"agent_id", agent.ID,
				"agent_slug", agent.Slug,
				"iteration", iter,
				"stop_reason", string(resp.StopReason),
			)
			out := r.buildResponse(agent, nil, trace, toolCallLog, totalUsage, llmLatency, toolLatency, cacheHits, cacheMgr.Count(), iterations, drops)
			out.Status = StatusIncomplete
			out.PartialReason = incompleteWithoutProduceDraft("unknown stop_reason: " + string(resp.StopReason))
			return out, nil
		}
	}

	// Loop exited via max_iterations.
	r.emitRunLog(req.TraceID, agent, iterations, totalUsage, string(StatusIncomplete), "max iterations without produce_draft")
	out := r.buildResponse(agent, nil, trace, toolCallLog, totalUsage, llmLatency, toolLatency, cacheHits, cacheMgr.Count(), iterations, drops)
	out.Status = StatusIncomplete
	out.PartialReason = incompleteWithoutProduceDraft(fmt.Sprintf("hit max_iterations=%d", maxIters))
	_ = startedAt
	return out, nil
}

func verifyAgentVerification(req RunRequest, agent agentmodels.Agent) error {
	if req.VerificationLevel < agent.MinVerification {
		return ErrAgentVerificationInsufficient
	}
	return nil
}

// applyDefaults fills zero-value knobs with their canonical defaults and
// validates caller-owned invariants.
//
// TraceID is intentionally NOT synthesized here anymore. The caller
// must generate it before invoking Run() so the same id can also anchor
// workflow ids, Redis event streams, HTTP responses, and any
// out-of-run observability recorded before the runner starts.
func applyDefaults(req RunRequest) (RunRequest, error) {
	if req.MaxIterations < 0 {
		req.MaxIterations = 0
	}
	if req.TotalTokenBudget <= 0 {
		req.TotalTokenBudget = defaultTotalTokenBudget
	}
	if req.Deadline <= 0 {
		req.Deadline = defaultDeadline
	}
	if strings.TrimSpace(req.TraceID) == "" {
		return RunRequest{}, errors.New("runner: TraceID is required; caller must generate before invoking runner.Run")
	}
	return req, nil
}

// buildInitialMessages constructs the messages slice for turn 0.
//
// Order:
//  1. Agent prompt (system message) — always present unless blank.
//  2. Pre-prepared context (system message) — caller-supplied JSON
//     payload. v1 placeholder; pipeline stage 8 refines into structured
//     per-agent views.
//  3. Conversation history (system message) — current ticket's prior
//     messages, formatted under a "## Conversation history (current
//     ticket)" header. Skipped when history is empty.
//  4. Inbound message (user turn) — the customer message the agent
//     should draft a reply to.
//
// Distinct from the get_conversation_history capability: that surfaces
// PRIOR conversations from the same customer; the history system block
// here is the CURRENT ticket's running context.
func buildInitialMessages(systemPrompt string, ctxData map[string]any, history []ConversationMessage, inbound string, isComposer bool, composerInstructions string) []llm.Message {
	out := make([]llm.Message, 0, 4)

	// Composers terminate via answer/escalate, so their runtime prompt
	// carries the terminal-decision (+ call_subagent) instructions and NOT the
	// produce_draft rule. Every other agent gets the produce_draft rule and is
	// byte-for-byte unchanged.
	var runtimePrompt string
	if isComposer {
		runtimePrompt = joinPromptSections(strings.TrimSpace(systemPrompt), composerInstructions)
	} else {
		runtimePrompt = buildRuntimeSystemPrompt(systemPrompt)
	}
	if strings.TrimSpace(runtimePrompt) != "" {
		out = append(out, llm.Message{
			Role: llm.RoleSystem,
			Content: []llm.ContentBlock{{
				Type: llm.ContentBlockTypeText,
				Text: runtimePrompt,
			}},
		})
	}

	if len(ctxData) > 0 {
		// JSON-stringified placeholder. F3 / pipeline stage 8 replaces
		// this with structured per-agent views.
		body, err := json.MarshalIndent(ctxData, "", "  ")
		if err == nil {
			out = append(out, llm.Message{
				Role: llm.RoleSystem,
				Content: []llm.ContentBlock{{
					Type: llm.ContentBlockTypeText,
					Text: "Context:\n" + string(body),
				}},
			})
		}
	}

	if formatted := formatConversationHistory(history); formatted != "" {
		out = append(out, llm.Message{
			Role: llm.RoleSystem,
			Content: []llm.ContentBlock{{
				Type: llm.ContentBlockTypeText,
				Text: formatted,
			}},
		})
	}

	out = append(out, llm.Message{
		Role: llm.RoleUser,
		Content: []llm.ContentBlock{{
			Type: llm.ContentBlockTypeText,
			Text: inbound,
		}},
	})
	return out
}

// formatConversationHistory renders the current ticket's prior messages
// into a labeled system block. Sender values outside {"customer", "agent"}
// fall back to "Other:" so a misclassified row doesn't crash the runner.
// Returns "" when history is empty so the caller can skip the system
// message entirely.
func formatConversationHistory(history []ConversationMessage) string {
	if len(history) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Conversation history (current ticket)\n\n")
	for _, m := range history {
		label := "Other"
		switch strings.ToLower(strings.TrimSpace(m.Sender)) {
		case "customer":
			label = "Customer"
		case "agent":
			label = "Agent"
		}
		b.WriteString(label)
		b.WriteString(": ")
		b.WriteString(strings.TrimSpace(m.Text))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func countSystemMessages(msgs []llm.Message) int {
	n := 0
	for _, m := range msgs {
		if m.Role == llm.RoleSystem {
			n++
			continue
		}
		break
	}
	return n
}

// decodeKBScope unmarshals ai_agents.kb_scope into the runtime shape
// the kb_search capability reads. Validation already ran at agent save
// time; the runner just decodes. A malformed shape (shouldn't happen on
// a write that went through the handler) returns a zero KBScope and
// logs at WARN so ops can spot drift between schema and code.
func decodeKBScope(raw json.RawMessage, lo *logf.Logger, traceID string, agentID int) builtin.KBScope {
	if len(raw) == 0 {
		return builtin.KBScope{}
	}
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" || s == "{}" {
		return builtin.KBScope{}
	}
	var scope struct {
		FolderIDs  []int `json:"folder_ids"`
		ArticleIDs []int `json:"article_ids"`
	}
	if err := json.Unmarshal(raw, &scope); err != nil {
		if lo != nil {
			lo.Warn("runner: kb_scope decode failed, falling back to empty scope",
				"trace_id", traceID, "agent_id", agentID, "error", err)
		}
		return builtin.KBScope{}
	}
	return builtin.KBScope{FolderIDs: scope.FolderIDs, ArticleIDs: scope.ArticleIDs}
}

// decodePresetOverrides decodes the agent's preset_overrides JSONB.
// Returns nil on empty/null/malformed — Resolve handles that as "use
// platform defaults."
func decodePresetOverrides(raw json.RawMessage) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" || s == "{}" {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

// encodeToolResult serializes a ToolCallLog into the string the LLM
// gets back as the tool_result content. Uses JSON for structured
// outputs so the model can parse fields; passes string outputs through
// unchanged.
func encodeToolResult(log ToolCallLog) string {
	if log.Status == "success" {
		switch v := log.Data.(type) {
		case nil:
			return ""
		case string:
			return v
		case json.RawMessage:
			if len(v) == 0 || string(v) == "null" {
				return ""
			}
			return string(v)
		default:
			b, err := json.Marshal(v)
			if err != nil {
				return fmt.Sprintf("(unserializable result: %v)", err)
			}
			return string(b)
		}
	}
	// Error path — the model needs the failure reason to plan its
	// next move (retry with different params, give up, etc.).
	body := map[string]any{
		"status":    log.Status,
		"code":      nil,
		"error":     log.Error,
		"retryable": log.Retryable,
	}
	if log.Code != "" {
		body["code"] = log.Code
	}
	if log.Recovered {
		body["recovered_from_panic"] = true
	}
	b, _ := json.Marshal(body)
	return string(b)
}

// buildResponse fills out a RunResponse with the aggregate fields.
// Status is left zero so the caller (the loop branches above) sets it
// — keeps the "what to set" decision in one place.
func (r *Runner) buildResponse(
	agent agentmodels.Agent,
	draft *string,
	trace []TurnTrace,
	toolCallLog []ToolCallLog,
	usage llm.Usage,
	llmLatencyMs int,
	toolLatencyMs int,
	cacheHits int,
	_ int, // marker count — reserved for downstream metrics surface
	iterations int,
	drops map[int]string,
) RunResponse {
	return RunResponse{
		Draft:      draft,
		Iterations: iterations,
		ToolCalls:  toolCallLog,
		Trace:      trace,
		Usage:      usage,
		Metrics: RunMetrics{
			TotalLatencyMs: llmLatencyMs + toolLatencyMs,
			LLMLatencyMs:   llmLatencyMs,
			ToolLatencyMs:  toolLatencyMs,
			CacheHits:      cacheHits,
			CachedTokens:   usage.CachedTokens,
		},
		AgentSlug:      agent.Slug,
		AgentUpdatedAt: agent.UpdatedAt,
		Drops:          drops,
	}
}

// emitRunLog writes a single structured INFO line per terminal Run
// outcome. Per-turn / per-tool logs are emitted by the provider and
// dispatcher respectively at lower layers; this is the run-level
// summary so ops can pull "agent X had Y outcome at trace Z" cleanly.
func (r *Runner) emitRunLog(traceID string, agent agentmodels.Agent, iterations int, usage llm.Usage, status, msg string) {
	r.opts.Lo.Info("runner.run.terminal",
		"trace_id", traceID,
		"agent_id", agent.ID,
		"agent_slug", agent.Slug,
		"iterations", iterations,
		"input_tokens", usage.InputTokens,
		"output_tokens", usage.OutputTokens,
		"cached_tokens", usage.CachedTokens,
		"status", status,
		"reason", msg,
	)
}

// recordUsage writes one ai_usage row per LLM provider call. Best-
// effort: the runner never aborts a run because persistence failed.
// Skips silently when no UsageRecorder is wired (test contexts) or
// when InboxID is zero (the caller didn't stamp it — see
// cmd/ai_run.go for the resolution path).
//
// The call_sequence_in_trace counter is the iteration index (1-based),
// which doubles as a per-trace identifier when retries land. The
// idempotency invariant on (trace_id, call_sequence, created_at) is
// safe because created_at is stamped here once per call.
func (r *Runner) recordUsage(
	ctx context.Context,
	req RunRequest,
	compReq llm.CompletionRequest,
	agent agentmodels.Agent,
	pres preset.Preset,
	callSeq int,
	resp llm.CompletionResponse,
	turnLatencyMs int,
	outcome string,
	callErr error,
) {
	if r.opts.Usage == nil {
		return
	}
	if req.InboxID <= 0 {
		// Handler/caller forgot to stamp. Log loudly so the gap is
		// visible in dev/ops without breaking the run.
		r.opts.Lo.Warn("runner: skipping ai_usage write (InboxID is zero)",
			"trace_id", req.TraceID, "agent_id", agent.ID)
		return
	}
	row := UsageRow{
		TraceID:             req.TraceID,
		CallSequenceInTrace: callSeq,
		InboxID:             req.InboxID,
		Provider:            coalesceString(resp.Provider, resp.Observability.Provider, pres.Provider),
		Model:               coalesceString(resp.Model, pres.Model),
		PresetSlug:          coalesceString(resp.PresetUsed, pres.Slug),
		Purpose:             string(llm.PurposeReason),
		StopReason:          string(resp.StopReason),
		InputTokens:         resp.Usage.InputTokens,
		OutputTokens:        resp.Usage.OutputTokens,
		CachedInputTokens:   resp.Usage.CachedTokens,
		CacheMarkersUsed:    resp.CacheMetrics.Markers,
		LatencyMs:           turnLatencyMs,
		RetryCount:          resp.Observability.RetryCount,
		Outcome:             outcome,
		ProviderMetadata:    []byte(resp.Observability.ProviderMetadata),
		RequestPayload:      redactedJSON(compReq),
	}
	if hasCompletionResponsePayload(resp) {
		row.ResponsePayload = redactedJSON(resp)
	}
	if resp.Observability.ProviderRequestID != "" {
		id := resp.Observability.ProviderRequestID
		row.ProviderRequestID = &id
	}
	if req.ConversationID > 0 {
		cid := int64(req.ConversationID)
		row.ConversationID = &cid
	}
	if agent.ID > 0 {
		aid := agent.ID
		row.AgentID = &aid
	}
	if req.InvokerUserID > 0 {
		uid := int64(req.InvokerUserID)
		row.UserID = &uid
	}
	if err := r.opts.Usage.Record(ctx, row); err != nil {
		// Log and continue. Audit gaps are recoverable from provider
		// logs; failing the user's run because of an audit miss is
		// worse than the gap.
		r.opts.Lo.Warn("runner: ai_usage write failed",
			"trace_id", req.TraceID, "agent_id", agent.ID, "seq", callSeq,
			"error", err)
	}
	_ = callErr // accepted for future error-class enrichment; not used yet
}

func redactedJSON(v any) []byte {
	raw, err := json.Marshal(v)
	if err != nil || len(raw) == 0 {
		return nil
	}
	return redact.Bytes(raw)
}

func hasCompletionResponsePayload(resp llm.CompletionResponse) bool {
	return resp.StopReason != "" ||
		len(resp.Content) > 0 ||
		resp.Usage.InputTokens != 0 ||
		resp.Usage.OutputTokens != 0 ||
		resp.Usage.CachedTokens != 0 ||
		resp.Provider != "" ||
		resp.PresetUsed != "" ||
		resp.Model != "" ||
		resp.Observability.Provider != "" ||
		resp.Observability.ProviderRequestID != "" ||
		len(resp.Observability.ProviderMetadata) > 0
}

func coalesceString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// outcomeForError maps a provider error to one of the ai_usage outcome
// values. Mirrors the constants in internal/ai/usage. Unknown errors
// fall through to "other" rather than "success" — analytics queries
// that filter on outcome != 'success' should catch them.
func outcomeForError(err error) string {
	if err == nil {
		return "success"
	}
	switch {
	case errors.Is(err, llm.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, llm.ErrAuth):
		return "auth_error"
	case errors.Is(err, llm.ErrRateLimited):
		return "rate_limited"
	case errors.Is(err, llm.ErrCircuitOpen):
		return "circuit_open"
	case errors.Is(err, llm.ErrInvalidRequest):
		return "invalid_request"
	default:
		return "other"
	}
}
