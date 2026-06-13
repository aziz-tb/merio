// Package resolver dereferences an agent's `tools` JSONB array into a typed
// []ResolvedTool. It resolves slugs/ids only — it does not dispatch (dispatcher)
// or translate to provider format (translator).
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
// internal tools, ai_tools.slug for custom tools. Used as the provider-facing
// tool name, so it must satisfy Anthropic's naming regex.
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

// Resolver dereferences agent.tools refs to ResolvedTool. Safe to share
// across agent runs.
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

// Resolve turns agent.Tools (raw JSONB) into a list of ResolvedTool. Refs are
// dropped (not errored) when an internal capability is unregistered/missing, a
// custom tool is missing/disabled, or kb_scope is empty for kb_search.
//
// Returns the resolved list plus a drops map keyed by source-element index for
// diagnostics. Errors only on malformed agent.Tools shape, which the agent
// handler validates at save time.
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
			// Only runtime-conditional drop today. If a second one lands,
			// add an availability hook to the capability interface rather
			// than more hardcoded tool-id checks here.
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
			// No per-tool gating; agent-level verification covers data access.
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

	// Sort by ID so the LLM sees tools in a deterministic position across calls.
	sort.SliceStable(out, func(a, b int) bool { return out[a].ID() < out[b].ID() })

	if len(drops) == 0 {
		drops = nil
	}
	return out, drops, nil
}
