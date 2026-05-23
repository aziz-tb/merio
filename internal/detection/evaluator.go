package detection

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/zerodha/logf"
)

var regexCache sync.Map // map[string]*regexp.Regexp

// EvaluateRules evaluates a ConditionTree against the context without invoking
// AI. It is exported for consumers that need rules-only pre-filtering.
func EvaluateRules(tree *ConditionTree, ctx EvaluationContext, lo *logf.Logger) bool {
	return evaluateRules(tree, ctx, lo)
}

// evaluateRules evaluates a ConditionTree against the context.
func evaluateRules(tree *ConditionTree, ctx EvaluationContext, lo *logf.Logger) bool {
	if tree == nil || len(tree.Children) == 0 {
		return false
	}
	return evalTree(tree, ctx, lo)
}

func evalTree(tree *ConditionTree, ctx EvaluationContext, lo *logf.Logger) bool {
	switch tree.Op {
	case LogicalAnd:
		for _, child := range tree.Children {
			if !evalNode(child, ctx, lo) {
				return false
			}
		}
		return true
	case LogicalOr:
		for _, child := range tree.Children {
			if evalNode(child, ctx, lo) {
				return true
			}
		}
		return false
	case LogicalNot:
		if len(tree.Children) != 1 {
			if lo != nil {
				lo.Warn("NOT op requires exactly one child", "children_count", len(tree.Children))
			}
			return false
		}
		return !evalNode(tree.Children[0], ctx, lo)
	}
	if lo != nil {
		lo.Warn("unknown logical op", "op", tree.Op)
	}
	return false
}

func evalNode(node ConditionNode, ctx EvaluationContext, lo *logf.Logger) bool {
	if node.Tree != nil {
		return evalTree(node.Tree, ctx, lo)
	}
	if node.Leaf != nil {
		return evalLeaf(*node.Leaf, ctx, lo)
	}
	return false
}

func evalLeaf(c Condition, ctx EvaluationContext, lo *logf.Logger) bool {
	if c.Source == SourceTags && c.Operator != OpIn && c.Operator != OpNotIn {
		if lo != nil {
			lo.Warn("tags source only supports in/not in", "operator", c.Operator)
		}
		return false
	}
	val, exists := ResolvePath(ctx, c.Source, c.Path)
	matched, err := applyOperator(c.Operator, val, exists, c.Value)
	if err != nil {
		if lo != nil {
			lo.Warn("operator evaluation error", "operator", c.Operator, "source", c.Source, "path", c.Path, "error", err)
		}
		return false
	}
	return matched
}

func applyOperator(op string, actual any, exists bool, expected any) (bool, error) {
	switch op {
	case OpIsNull:
		return !exists || isNilOrEmpty(actual), nil
	case OpIsNotNull:
		return exists && !isNilOrEmpty(actual), nil
	}

	if !exists {
		return false, nil
	}

	switch op {
	case OpEquals:
		return equalsAny(actual, expected), nil
	case OpNotEquals:
		return !equalsAny(actual, expected), nil
	case OpGreaterThan:
		return compareNumeric(actual, expected, func(a, b float64) bool { return a > b })
	case OpGreaterOrEqual:
		return compareNumeric(actual, expected, func(a, b float64) bool { return a >= b })
	case OpLessThan:
		return compareNumeric(actual, expected, func(a, b float64) bool { return a < b })
	case OpLessOrEqual:
		return compareNumeric(actual, expected, func(a, b float64) bool { return a <= b })
	case OpContains:
		return containsAny(actual, expected)
	case OpNotContains:
		matched, err := containsAny(actual, expected)
		return !matched, err
	case OpContainsRegex:
		return containsRegex(actual, expected)
	case OpIn:
		return inList(actual, expected)
	case OpNotIn:
		matched, err := inList(actual, expected)
		return !matched, err
	}
	return false, fmt.Errorf("unknown operator: %s", op)
}

func isNilOrEmpty(v any) bool {
	if v == nil {
		return true
	}
	switch x := v.(type) {
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	case []string:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	}
	return false
}

func equalsAny(a, b any) bool {
	return toLowerString(a) == toLowerString(b)
}

func toLowerString(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return strings.ToLower(x)
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case bool:
		return strconv.FormatBool(x)
	}
	return strings.ToLower(fmt.Sprintf("%v", v))
}

func compareNumeric(a, b any, cmp func(float64, float64) bool) (bool, error) {
	af, err := toFloat64(a)
	if err != nil {
		return false, fmt.Errorf("actual not numeric: %w", err)
	}
	bf, err := toFloat64(b)
	if err != nil {
		return false, fmt.Errorf("expected not numeric: %w", err)
	}
	return cmp(af, bf), nil
}

func toFloat64(v any) (float64, error) {
	if v == nil {
		return 0, fmt.Errorf("nil")
	}
	switch x := v.(type) {
	case float64:
		return x, nil
	case int:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case json.Number:
		return x.Float64()
	case string:
		return strconv.ParseFloat(strings.TrimSpace(x), 64)
	}
	return 0, fmt.Errorf("not numeric: %T", v)
}

func containsAny(actual, expected any) (bool, error) {
	actualStr := normalizeWhitespace(strings.ToLower(toString(actual)))

	switch exp := expected.(type) {
	case string:
		for _, segment := range strings.Split(exp, ",") {
			if containsSegment(actualStr, segment) {
				return true, nil
			}
		}
		return false, nil
	case []any:
		for _, item := range exp {
			if containsSegment(actualStr, toString(item)) {
				return true, nil
			}
		}
		return false, nil
	case []string:
		for _, item := range exp {
			if containsSegment(actualStr, item) {
				return true, nil
			}
		}
		return false, nil
	}
	return false, fmt.Errorf("contains: unsupported expected type %T", expected)
}

func containsSegment(actual, segment string) bool {
	segment = normalizeWhitespace(strings.ToLower(strings.TrimSpace(segment)))
	return segment != "" && strings.Contains(actual, segment)
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func containsRegex(actual, expected any) (bool, error) {
	pattern := toString(expected)
	if pattern == "" {
		return false, fmt.Errorf("contains regex: empty pattern")
	}
	if cached, ok := regexCache.Load(pattern); ok {
		return cached.(*regexp.Regexp).MatchString(toString(actual)), nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, fmt.Errorf("contains regex: compile failed: %w", err)
	}
	regexCache.Store(pattern, re)
	return re.MatchString(toString(actual)), nil
}

func inList(actual, expected any) (bool, error) {
	expectedSet := collectStrings(expected)
	if len(expectedSet) == 0 {
		return false, nil
	}
	actualVals := collectStrings(actual)
	if len(actualVals) == 0 {
		actualVals = []string{toLowerString(actual)}
	}
	for _, av := range actualVals {
		for _, ev := range expectedSet {
			if av == ev {
				return true, nil
			}
		}
	}
	return false, nil
}

func collectStrings(v any) []string {
	var out []string
	switch x := v.(type) {
	case []string:
		for _, s := range x {
			if trimmed := strings.ToLower(strings.TrimSpace(s)); trimmed != "" {
				out = append(out, trimmed)
			}
		}
	case []any:
		for _, item := range x {
			if trimmed := toLowerString(item); trimmed != "" {
				out = append(out, trimmed)
			}
		}
	case string:
		for _, s := range strings.Split(x, ",") {
			if trimmed := strings.ToLower(strings.TrimSpace(s)); trimmed != "" {
				out = append(out, trimmed)
			}
		}
	}
	return out
}
