package capabilities

import (
	"encoding/json"
	"fmt"
)

// paramInt extracts an integer-typed parameter. JSON numbers always
// round-trip as float64 from json.Unmarshal into map[string]any, so we
// accept float64 / int / int64 and reject anything else. Returns
// (value, true) when present, (0, false) when absent.
func paramInt(params map[string]any, key string) (int, bool) {
	v, ok := params[key]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case float32:
		return int(n), true
	}
	return 0, false
}

// paramString extracts a string-typed parameter. Returns (value, true)
// when present, ("", false) when absent or empty.
func paramString(params map[string]any, key string) (string, bool) {
	v, ok := params[key]
	if !ok || v == nil {
		return "", false
	}
	if s, ok := v.(string); ok && s != "" {
		return s, true
	}
	return "", false
}

// decodeCustomAttributes decodes a user's custom_attributes JSONB into a
// map[string]any so JSON-marshalling the capability output produces a
// proper object rather than a base64-encoded string. Returns nil on
// parse failure (we'd rather drop the field than crash the agent run).
func decodeCustomAttributes(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

// paramIntSlice extracts an array of ints. Returns nil + nil error when
// the key is absent. Errors when the key is present but not an array of
// numbers.
func paramIntSlice(params map[string]any, key string) ([]int, error) {
	v, ok := params[key]
	if !ok || v == nil {
		return nil, nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", key)
	}
	out := make([]int, 0, len(arr))
	for i, e := range arr {
		switch n := e.(type) {
		case int:
			out = append(out, n)
		case int64:
			out = append(out, int(n))
		case float64:
			out = append(out, int(n))
		default:
			return nil, fmt.Errorf("%s[%d] is not a number (%T)", key, i, e)
		}
	}
	return out, nil
}
