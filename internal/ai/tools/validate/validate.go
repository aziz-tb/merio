// Package validate is the shared parameter validator used by both the
// HTTP manual-invoke handler and the unified dispatcher's runtime path.
// Sharing the implementation guarantees that ops sees identical error
// messages regardless of how a tool was invoked — no "this works in the
// admin tester but fails in the agent run" surprises.
//
// The validator is intentionally shallow: it checks required-ness and
// the F5 type tags (STRING, INTEGER, FLOAT, BOOL, DATE, LIST, OBJECT)
// against the raw param map. Deep schema validation (nested object
// shape, enum values, string formats) is the dispatched code's
// responsibility — the Python SDK and the internal capabilities both
// surface their own typed errors.
package validate

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Params is a JSON-decoded parameter map (json.Unmarshal into
// map[string]any). All number fields arrive as float64 unless the
// caller pre-converted; the validator handles both shapes.
type Params = map[string]any

// ParameterDef is one entry in a tool's input_parameters JSONB array.
// Only the fields the validator needs are mapped — extra fields
// (description, options, etc.) are ignored.
type ParameterDef struct {
	Key      string `json:"key"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

// Schema validates rawSchema (a tool's input_parameters JSONB) against
// params. Returns nil on success, a descriptive error on the first
// validation failure. A nil / empty / malformed schema is accept-any
// (matches the existing F9 behavior so a tool-config bug doesn't lock
// out admins).
func Schema(rawSchema json.RawMessage, params Params) error {
	defs, ok := decodeSchema(rawSchema)
	if !ok {
		return nil
	}
	if params == nil {
		params = Params{}
	}
	for _, def := range defs {
		v, present := params[def.Key]
		if def.Required && (!present || v == nil) {
			return fmt.Errorf("required parameter %q is missing", def.Key)
		}
		if !present || v == nil {
			continue
		}
		if err := checkType(def, v); err != nil {
			return err
		}
	}
	return nil
}

// SchemaTyped is the same as Schema but takes already-decoded ParameterDef.
// Used by callers that hold the typed schema (the agent runtime decodes
// once and validates many).
func SchemaTyped(defs []ParameterDef, params Params) error {
	if len(defs) == 0 {
		return nil
	}
	if params == nil {
		params = Params{}
	}
	for _, def := range defs {
		v, present := params[def.Key]
		if def.Required && (!present || v == nil) {
			return fmt.Errorf("required parameter %q is missing", def.Key)
		}
		if !present || v == nil {
			continue
		}
		if err := checkType(def, v); err != nil {
			return err
		}
	}
	return nil
}

// decodeSchema decodes the input_parameters JSONB. Returns (nil, false)
// when the schema is empty / null / malformed — the caller treats that
// as accept-any.
func decodeSchema(raw json.RawMessage) ([]ParameterDef, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return nil, false
	}
	var defs []ParameterDef
	if err := json.Unmarshal(raw, &defs); err != nil {
		return nil, false
	}
	return defs, true
}

// checkType verifies v matches the F5 type tag in def.Type. Unknown
// type tags are accept-any (admins may declare custom labels we don't
// pre-enumerate).
func checkType(def ParameterDef, v any) error {
	t := strings.ToUpper(strings.TrimSpace(def.Type))
	switch t {
	case "", "STRING", "TEXT":
		if _, ok := v.(string); !ok {
			return typeMismatch(def.Key, "string", v)
		}
	case "INTEGER", "INT":
		switch n := v.(type) {
		case float64:
			// JSON numbers come in as float64; require integral value.
			if n != float64(int64(n)) {
				return typeMismatch(def.Key, "integer", v)
			}
		case int, int32, int64:
			// ok
		default:
			return typeMismatch(def.Key, "integer", v)
		}
	case "FLOAT", "NUMBER":
		switch v.(type) {
		case float32, float64, int, int32, int64:
			// ok
		default:
			return typeMismatch(def.Key, "number", v)
		}
	case "BOOL", "BOOLEAN":
		if _, ok := v.(bool); !ok {
			return typeMismatch(def.Key, "boolean", v)
		}
	case "DATE", "DATETIME":
		if _, ok := v.(string); !ok {
			return typeMismatch(def.Key, "date string", v)
		}
	case "LIST", "ARRAY":
		if _, ok := v.([]any); !ok {
			return typeMismatch(def.Key, "array", v)
		}
	case "OBJECT", "MAP":
		if _, ok := v.(map[string]any); !ok {
			return typeMismatch(def.Key, "object", v)
		}
	}
	return nil
}

func typeMismatch(key, expected string, got any) error {
	return fmt.Errorf("parameter %q expected %s, got %T", key, expected, got)
}
