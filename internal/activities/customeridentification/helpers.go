package customeridentification

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func asMap(v any) (map[string]any, bool) {
	if m, ok := v.(map[string]any); ok {
		return m, true
	}
	return nil, false
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		v, ok := m[key]
		if !ok || v == nil {
			continue
		}
		switch x := v.(type) {
		case string:
			return strings.TrimSpace(x)
		case fmt.Stringer:
			return strings.TrimSpace(x.String())
		case float64:
			if x == float64(int64(x)) {
				return strconv.FormatInt(int64(x), 10)
			}
			return strconv.FormatFloat(x, 'f', -1, 64)
		case int:
			return strconv.Itoa(x)
		case int64:
			return strconv.FormatInt(x, 10)
		case json.Number:
			return x.String()
		}
	}
	return ""
}

func rawJSONFrom(m map[string]any, keys ...string) json.RawMessage {
	for _, key := range keys {
		v, ok := m[key]
		if !ok || v == nil {
			continue
		}
		if raw, ok := v.(json.RawMessage); ok {
			return raw
		}
		b, err := json.Marshal(v)
		if err == nil && string(b) != "null" {
			return b
		}
	}
	return nil
}
