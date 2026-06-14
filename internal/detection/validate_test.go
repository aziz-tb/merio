package detection

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func leaf(source, path, op string) ConditionNode {
	return ConditionNode{Leaf: &Condition{Source: source, Path: path, Operator: op, Value: "x"}}
}

func TestValidateConditionTree(t *testing.T) {
	sources := []string{SourceTicket, SourceCustomer, SourceContract, SourceTags}

	t.Run("nil tree is valid", func(t *testing.T) {
		require.NoError(t, ValidateConditionTree(nil, sources))
	})

	t.Run("valid nested tree", func(t *testing.T) {
		tree := &ConditionTree{Op: LogicalAnd, Children: []ConditionNode{
			leaf(SourceTicket, "subject", OpContains),
			{Tree: &ConditionTree{Op: LogicalOr, Children: []ConditionNode{
				leaf(SourceCustomer, "email", OpEquals),
				leaf(SourceTags, "", OpIn), // tags carries no path
			}}},
		}}
		require.NoError(t, ValidateConditionTree(tree, sources))
	})

	t.Run("invalid source rejected", func(t *testing.T) {
		tree := &ConditionTree{Op: LogicalAnd, Children: []ConditionNode{
			leaf("inbox", "id", OpEquals), // inbox not yet an allowed source (Slice 5)
		}}
		err := ValidateConditionTree(tree, sources)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid condition source")
	})

	t.Run("invalid logical operator rejected", func(t *testing.T) {
		tree := &ConditionTree{Op: "XOR", Children: nil}
		require.Error(t, ValidateConditionTree(tree, sources))
	})

	t.Run("invalid comparison operator rejected", func(t *testing.T) {
		tree := &ConditionTree{Op: LogicalAnd, Children: []ConditionNode{
			leaf(SourceTicket, "subject", "~=")},
		}
		require.Error(t, ValidateConditionTree(tree, sources))
	})

	t.Run("non-tags source requires a path", func(t *testing.T) {
		tree := &ConditionTree{Op: LogicalAnd, Children: []ConditionNode{
			leaf(SourceTicket, "", OpEquals)},
		}
		err := ValidateConditionTree(tree, sources)
		require.Error(t, err)
		require.Contains(t, err.Error(), "requires a path")
	})

	t.Run("tags must not carry a path", func(t *testing.T) {
		tree := &ConditionTree{Op: LogicalAnd, Children: []ConditionNode{
			leaf(SourceTags, "vip", OpIn)},
		}
		err := ValidateConditionTree(tree, sources)
		require.Error(t, err)
		require.Contains(t, err.Error(), "must not carry a path")
	})

	t.Run("empty node rejected", func(t *testing.T) {
		tree := &ConditionTree{Op: LogicalAnd, Children: []ConditionNode{{}}}
		require.Error(t, ValidateConditionTree(tree, sources))
	})
}
