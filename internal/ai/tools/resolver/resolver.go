// Package resolver turns an agent's `tools` JSONB array into a fully
// dereferenced []ResolvedTool that the agent runtime + dispatcher +
// translator all consume. v1's job is small but central: shape unification.
//
// The resolver doesn't dispatch — that's the dispatcher's job (chunk 10).
// It doesn't translate to provider format — that's the translator's job
// (chunk 11). It just resolves slugs/ids into a single typed list and
// reports anything that won't fit.
package resolver

import (
	"encoding/json"
	"fmt"
	"sort"

	agentmodels "github.com/aziz-tb/merio/internal/ai/agents/models"
	aitools "github.com/aziz-tb/merio/internal/ai/tools"
	"github.com/aziz-tb/merio/internal/ai/tools/builtin"
	toolmodels "github.com/aziz-tb/merio/internal/ai/tools/models"
)

// ResolvedTool is one fully dereferenced tool. Exactly one of Internal
// or Custom is set; Kind tells the caller which to read.
type ResolvedTool struct {
	Kind agentmodels.ToolRefKind

	// Internal is set when Kind == "internal".
	Internal builtin.InternalCapability

	// Custom is set when Kind == "custom". Pointer to a copy of the
	// ai_tools row at resolve time — callers must not mutate.
	Custom *toolmodels.Tool

	// ConfigOverride is the agent's per-tool override JSONB (raw). The
	// dispatcher / translator interprets it; the resolver just passes
	// it through. Empty when the agent didn't declare an override.
	ConfigOverride json.RawMessage
}

// ID returns the tool's stable identifier — internal capability id for
// internal tools, ai_tools.slug for custom tools. Used for log fields,
// metric labels, and tool-name compliance with Anthropic's naming regex.
func (r ResolvedTool) ID() string {
	switch r.Kind {
	case agentmodels.ToolRefKindInternal:
		if r.Internal != nil {
			return r.Internal.ID()
		}
	case agentmodels.ToolRefKindCustom:
		if r.Custom != nil {
			return r.Custom.Slug
		}
	}
	return ""
}

// Name returns the operator-facing display name for this tool. Built-ins
// provide it from code; custom tools use ai_tools.name.
func (r ResolvedTool) Name() string {
	switch r.Kind {
	case agentmodels.ToolRefKindInternal:
		if r.Internal != nil {
			return r.Internal.Name()
		}
	case agentmodels.ToolRefKindCustom:
		if r.Custom != nil {
			return r.Custom.Name
		}
	}
	return ""
}

// Category returns the dispatch category for this resolved tool.
// Built-ins declare their category in code; custom tools read the
// persisted ai_tools.category value with a safe default of "write".
func (r ResolvedTool) Category() string {
	switch r.Kind {
	case agentmodels.ToolRefKindInternal:
		if r.Internal != nil {
			return r.Internal.Category()
		}
	case agentmodels.ToolRefKindCustom:
		if r.Custom != nil && (r.Custom.Category == "read" || r.Custom.Category == "write") {
			return r.Custom.Category
		}
	}
	return "write"
}

// Resolver dereferences agent.tools refs to ResolvedTool. Holds the
// tool manager + capability registry; same instance is shared across
// agent runs.
type Resolver struct {
	tools    *aitools.Manager
	registry *builtin.Registry
}

// New builds a Resolver. registry defaults to builtin.Default() when nil.
func New(tools *aitools.Manager, registry *builtin.Registry) *Resolver {
	if registry == nil {
		registry = builtin.Default()
	}
	return &Resolver{tools: tools, registry: registry}
}

func isKBScopeEmpty(s builtin.KBScope) bool {
	return len(s.FolderIDs) == 0 && len(s.ArticleIDs) == 0
}

func hasResolvedTool(out []ResolvedTool, id string) bool {
	for _, tool := range out {
		if tool.ID() == id {
			return true
		}
	}
	return false
}

// Resolve turns agent.Tools (raw JSONB) into a list of ResolvedTool.
// Tool-level verification gating was dropped (agents gate their tools), so
// the only drops here are unregistered/missing internal capabilities,
// empty kb_scope for kb_search, and missing/disabled custom tools —
// operators don't want disabled tools firing.
//
// Returns the resolved list plus a "drops" map keyed by the source
// element's index for diagnostics. Errors only on malformed agent.Tools
// shape (which the agent handler validates at save time, so this should
// be unreachable from the runtime path).
func (r *Resolver) Resolve(rawTools json.RawMessage, kbScope builtin.KBScope) ([]ResolvedTool, map[int]string, error) {
	var refs []agentmodels.ToolRef
	if len(rawTools) > 0 && string(rawTools) != "null" && string(rawTools) != "[]" {
		if err := json.Unmarshal(rawTools, &refs); err != nil {
			return nil, nil, fmt.Errorf("resolver: agent.tools is not a valid array: %w", err)
		}
	}

	out := make([]ResolvedTool, 0, len(refs)+1)
	drops := make(map[int]string)

	// Pre-fetch all custom tool ids in a single query — avoids N+1.
	customIDs := make([]int, 0, len(refs))
	for _, ref := range refs {
		if ref.Kind == agentmodels.ToolRefKindCustom {
			if id, ok := ref.CustomID(); ok && id > 0 {
				customIDs = append(customIDs, id)
			}
		}
	}
	customByID := map[int]*toolmodels.Tool{}
	if len(customIDs) > 0 && r.tools != nil {
		fetched, err := r.tools.GetByIDs(customIDs)
		if err != nil {
			return nil, nil, fmt.Errorf("resolver: load custom tools: %w", err)
		}
		for i := range fetched {
			t := fetched[i]
			customByID[t.ID] = &t
		}
	}

	for i, ref := range refs {
		switch ref.Kind {
		case agentmodels.ToolRefKindInternal:
			cid, ok := ref.InternalID()
			if !ok || cid == "" {
				drops[i] = "internal ref missing string id"
				continue
			}
			cap, found := r.registry.Get(cid)
			if !found {
				drops[i] = "internal capability not registered: " + cid
				continue
			}
			// This is the first scope/runtime-conditional resolver drop.
			// The builtin registry currently exposes only static metadata
			// (id, name, schemas). When a second case lands, add an
			// availability hook on the capability interface instead of
			// growing more hardcoded tool-id checks here.
			if cid == "kb_search" && isKBScopeEmpty(kbScope) {
				drops[i] = "kb_scope is empty"
				continue
			}
			out = append(out, ResolvedTool{
				Kind:           agentmodels.ToolRefKindInternal,
				Internal:       cap,
				ConfigOverride: ref.ConfigOverride,
			})

		case agentmodels.ToolRefKindCustom:
			id, ok := ref.CustomID()
			if !ok || id <= 0 {
				drops[i] = "custom ref missing positive integer id"
				continue
			}
			t, found := customByID[id]
			if !found {
				drops[i] = fmt.Sprintf("ai_tools.id=%d not found", id)
				continue
			}
			if !t.Enabled {
				drops[i] = fmt.Sprintf("ai_tools.id=%d disabled", id)
				continue
			}
			// Custom tools are no longer gated per tool; agent-level
			// verification covers the data-access decision upstream.
			out = append(out, ResolvedTool{
				Kind:           agentmodels.ToolRefKindCustom,
				Custom:         t,
				ConfigOverride: ref.ConfigOverride,
			})

		default:
			drops[i] = fmt.Sprintf("unknown kind %q", ref.Kind)
		}
	}

	if !isKBScopeEmpty(kbScope) && !hasResolvedTool(out, "kb_search") {
		if cap, found := r.registry.Get("kb_search"); found {
			out = append(out, ResolvedTool{
				Kind:     agentmodels.ToolRefKindInternal,
				Internal: cap,
			})
		}
	}

	// Stable order — sort by ID so the LLM sees tools in a deterministic
	// position across calls. Agent prompts that say "use tool X" benefit
	// from positional consistency.
	sort.SliceStable(out, func(a, b int) bool { return out[a].ID() < out[b].ID() })

	if len(drops) == 0 {
		drops = nil
	}
	return out, drops, nil
}
