package builtin

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// InternalCapability is the contract every Go-side capability implements:
// a stateless metadata bundle plus an Execute function. Per-run state
// (DB connections, manager handles) flows in via ToolRuntimeContext.
type InternalCapability interface {
	// ID returns the stable identifier the LLM and agent.tools array
	// reference (e.g., "kb_search"). Must match ^[a-z][a-z0-9_]*$.
	ID() string

	// Name returns the display name shown in admin UIs.
	Name() string

	// Description is the LLM-facing string included with every tool
	// definition; wording influences whether the model calls the tool.
	Description() string

	// InputSchema returns a JSON Schema (subset: object type, basic
	// property types, enum, required) for the accepted parameters.
	InputSchema() map[string]any

	// OutputSchema returns an optional JSON Schema for Execute's result,
	// surfaced to the LLM for planning follow-up calls. Nil when none applies.
	OutputSchema() map[string]any

	// Category returns the dispatch category, "read" or "write". Reads are
	// eligible for parallel dispatch within a turn; writes run sequentially
	// after all reads. Affects runtime dispatch only, not the tool schema.
	Category() string

	// Execute runs the capability against params and runtime context; the
	// returned value (struct/map/slice) is JSON-encoded for the LLM. On
	// failure, return an error (the dispatcher wraps it as execution_error).
	Execute(ctx context.Context, params map[string]any, rt *ToolRuntimeContext) (any, error)
}

// Registry is the concurrency-safe in-memory map of capability ID →
// InternalCapability. Registration happens at startup; Get/List run from
// any worker.
type Registry struct {
	mu   sync.RWMutex
	caps map[string]InternalCapability
}

// NewRegistry constructs an empty Registry. Tests use this for isolated
// registries; production uses the package singleton via RegisterInternal /
// Default().
func NewRegistry() *Registry {
	return &Registry{caps: map[string]InternalCapability{}}
}

// Register adds cap to the registry, returning an error on a nil cap,
// empty ID, or duplicate ID.
func (r *Registry) Register(cap InternalCapability) error {
	if cap == nil {
		return fmt.Errorf("builtin: cannot register nil capability")
	}
	id := cap.ID()
	if id == "" {
		return fmt.Errorf("builtin: capability has empty ID")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.caps[id]; dup {
		return fmt.Errorf("builtin: duplicate capability ID %q", id)
	}
	r.caps[id] = cap
	return nil
}

// Get returns the capability with the given id, or nil + false.
func (r *Registry) Get(id string) (InternalCapability, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.caps[id]
	return c, ok
}

// List returns all registered capabilities sorted by ID.
func (r *Registry) List() []InternalCapability {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.caps))
	for id := range r.caps {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]InternalCapability, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.caps[id])
	}
	return out
}

// IDs returns the registered capability IDs sorted. Helper for log lines
// and error messages that surface available IDs.
func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.caps))
	for id := range r.caps {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// defaultRegistry is the package singleton populated by init() blocks in
// .../builtin/capabilities/*. Tests use NewRegistry() to avoid global state.
var defaultRegistry = NewRegistry()

// Default returns the singleton registry that init() blocks register
// against. The agent runtime references it at startup and never mutates it.
func Default() *Registry { return defaultRegistry }

// RegisterInternal registers cap into the package singleton, for use by
// capability init() blocks:
//
//	func init() {
//	    builtin.RegisterInternal(&kbSearch{})
//	}
//
// Panics on duplicate IDs to surface the misconfiguration loudly at startup.
func RegisterInternal(cap InternalCapability) {
	if err := defaultRegistry.Register(cap); err != nil {
		panic(err)
	}
}

// ResetDefault resets the package singleton, for tests that exercise
// registration paths and need a clean state.
func ResetDefault() { defaultRegistry = NewRegistry() }
