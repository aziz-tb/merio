package detection

import (
	"fmt"
	"strings"
)

// validLogicalOps is the set of operators a ConditionTree node may use.
var validLogicalOps = map[string]bool{
	LogicalAnd: true,
	LogicalOr:  true,
	LogicalNot: true,
}

// validOperators is the set of comparison operators a leaf Condition may use.
var validOperators = map[string]bool{
	OpEquals: true, OpNotEquals: true,
	OpGreaterThan: true, OpGreaterOrEqual: true,
	OpLessThan: true, OpLessOrEqual: true,
	OpContains: true, OpNotContains: true, OpContainsRegex: true,
	OpIn: true, OpNotIn: true,
	OpIsNull: true, OpIsNotNull: true,
}

// ValidateConditionTree checks a condition tree's shape for save-time
// validation: every node uses a known logical operator, and every leaf
// references an allowed source, uses a known comparison operator, and carries a
// path (except SourceTags, which has none).
//
// allowedSources is passed in so callers can scope the source set — composer
// escalation rules and guidance conditions use the four detection sources today,
// and Slice 5 extends this with the stored verification level. Pure and
// side-effect-free; reused wherever a ConditionTree is authored (composer
// config now; Slices 5/7 surfaces later). A nil tree is valid (no conditions).
func ValidateConditionTree(tree *ConditionTree, allowedSources []string) error {
	if tree == nil {
		return nil
	}
	allowed := make(map[string]bool, len(allowedSources))
	for _, s := range allowedSources {
		allowed[s] = true
	}
	return validateTree(tree, allowed)
}

func validateTree(tree *ConditionTree, allowed map[string]bool) error {
	if tree == nil {
		return nil
	}
	if !validLogicalOps[tree.Op] {
		return fmt.Errorf("invalid logical operator %q (expected AND, OR, or NOT)", tree.Op)
	}
	for i := range tree.Children {
		node := tree.Children[i]
		switch {
		case node.Tree != nil && node.Leaf != nil:
			return fmt.Errorf("condition node %d has both a subtree and a leaf", i)
		case node.Tree != nil:
			if err := validateTree(node.Tree, allowed); err != nil {
				return err
			}
		case node.Leaf != nil:
			if err := validateLeaf(node.Leaf, allowed); err != nil {
				return err
			}
		default:
			return fmt.Errorf("condition node %d is empty (neither subtree nor leaf)", i)
		}
	}
	return nil
}

func validateLeaf(c *Condition, allowed map[string]bool) error {
	if !allowed[c.Source] {
		return fmt.Errorf("invalid condition source %q", c.Source)
	}
	if !validOperators[c.Operator] {
		return fmt.Errorf("invalid condition operator %q", c.Operator)
	}
	// Tags carries no path; every other source requires one.
	if c.Source == SourceTags {
		if strings.TrimSpace(c.Path) != "" {
			return fmt.Errorf("condition source %q must not carry a path", SourceTags)
		}
		return nil
	}
	if strings.TrimSpace(c.Path) == "" {
		return fmt.Errorf("condition source %q requires a path", c.Source)
	}
	return nil
}
