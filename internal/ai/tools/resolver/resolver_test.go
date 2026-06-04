package resolver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aziz-tb/merio/internal/ai/tools/builtin"
	"github.com/stretchr/testify/require"
)

type fakeCapability struct {
	id string
}

func (f fakeCapability) ID() string                   { return f.id }
func (f fakeCapability) Name() string                 { return f.id }
func (f fakeCapability) Description() string          { return f.id }
func (f fakeCapability) InputSchema() map[string]any  { return map[string]any{"type": "object"} }
func (f fakeCapability) OutputSchema() map[string]any { return nil }
func (f fakeCapability) Category() string             { return "read" }
func (f fakeCapability) Execute(context.Context, map[string]any, *builtin.ToolRuntimeContext) (any, error) {
	return nil, nil
}

func TestResolveAutoIncludesKBSearchWhenScopeIsNonEmpty(t *testing.T) {
	reg := builtin.NewRegistry()
	require.NoError(t, reg.Register(fakeCapability{id: "kb_search"}))
	r := New(nil, reg)

	out, drops, err := r.Resolve(json.RawMessage(`[]`), builtin.KBScope{FolderIDs: []int{7}})

	require.NoError(t, err)
	require.Nil(t, drops)
	require.Len(t, out, 1)
	require.Equal(t, "kb_search", out[0].ID())
}

func TestResolveDoesNotAutoIncludeKBSearchWhenScopeIsEmpty(t *testing.T) {
	reg := builtin.NewRegistry()
	require.NoError(t, reg.Register(fakeCapability{id: "kb_search"}))
	r := New(nil, reg)

	out, drops, err := r.Resolve(json.RawMessage(`[]`), builtin.KBScope{})

	require.NoError(t, err)
	require.Nil(t, drops)
	require.Empty(t, out)
}

func TestResolveDoesNotDuplicateExplicitKBSearch(t *testing.T) {
	reg := builtin.NewRegistry()
	require.NoError(t, reg.Register(fakeCapability{id: "kb_search"}))
	r := New(nil, reg)

	out, drops, err := r.Resolve(
		json.RawMessage(`[{"kind":"internal","id":"kb_search"}]`),
		builtin.KBScope{FolderIDs: []int{7}},
	)

	require.NoError(t, err)
	require.Nil(t, drops)
	require.Len(t, out, 1)
	require.Equal(t, "kb_search", out[0].ID())
}
