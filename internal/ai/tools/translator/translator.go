// Package translator converts resolved agent tools (ResolvedTool) into the
// provider-agnostic llm.ToolDef shape. Two source shapes produce JSON Schema:
//
//   - InternalCapability — already JSON Schema; pass-through with a defensive
//     copy so caller mutations don't propagate.
//   - Custom Python tool — the F5 array-of-{key, type, ...} shape, mapped
//     per entry to a property with collected requireds.
//
// Tool names go on the wire to Anthropic (^[a-zA-Z0-9_-]{1,64}$); the
// translator validates and returns a typed error rather than letting the SDK
// reject silently.
package translator

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	agentmodels "github.com/aziz-tb/merio/internal/ai/agents/models"
	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/aziz-tb/merio/internal/ai/tools/resolver"
)

// anthropicToolNameRegex mirrors the SDK's documented tool-name constraint.
var anthropicToolNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// Translate converts ResolvedTools to llm.ToolDefs, stopping at the first
// translation error. Empty input yields nil output and no error.
func Translate(tools []resolver.ResolvedTool) ([]llm.ToolDef, error) {
	if len(tools) == 0 {
		return nil, nil
	}
	out := make([]llm.ToolDef, 0, len(tools))
	for i, t := range tools {
		def, err := translateOne(t)
		if err != nil {
			return nil, fmt.Errorf("translator: tool[%d]: %w", i, err)
		}
		out = append(out, def)
	}
	return out, nil
}

// TranslateOne converts a single ResolvedTool (test endpoint, debug paths).
func TranslateOne(t resolver.ResolvedTool) (llm.ToolDef, error) {
	return translateOne(t)
}

func translateOne(t resolver.ResolvedTool) (llm.ToolDef, error) {
	switch t.Kind {
	case agentmodels.ToolRefKindInternal:
		return translateInternal(t)
	case agentmodels.ToolRefKindCustom:
		return translateCustom(t)
	default:
		return llm.ToolDef{}, fmt.Errorf("unknown kind %q", t.Kind)
	}
}

// translateInternal copies a capability's already-declared JSON Schema and
// pulls its name/description.
func translateInternal(t resolver.ResolvedTool) (llm.ToolDef, error) {
	if t.Internal == nil {
		return llm.ToolDef{}, fmt.Errorf("internal capability is nil")
	}
	name := t.Internal.ID()
	if !anthropicToolNameRegex.MatchString(name) {
		return llm.ToolDef{}, fmt.Errorf("capability id %q violates Anthropic name regex", name)
	}
	schema := deepCopy(t.Internal.InputSchema())
	if schema == nil {
		schema = map[string]any{"type": "object"}
	}
	return llm.ToolDef{
		Name:        name,
		Description: strings.TrimSpace(t.Internal.Description()),
		InputSchema: schema,
	}, nil
}

// translateCustom maps the F5 array shape to a JSON Schema object (per-param
// mapping in paramToProperty). Empty description falls back to the tool name.
func translateCustom(t resolver.ResolvedTool) (llm.ToolDef, error) {
	if t.Custom == nil {
		return llm.ToolDef{}, fmt.Errorf("custom tool is nil")
	}
	name := t.Custom.Slug
	if !anthropicToolNameRegex.MatchString(name) {
		return llm.ToolDef{}, fmt.Errorf("tool slug %q violates Anthropic name regex", name)
	}

	defs, _ := decodeCustomParams(t.Custom.InputParameters)

	props := map[string]any{}
	required := []string{}
	for _, def := range defs {
		key := strings.TrimSpace(def.Key)
		if key == "" {
			continue
		}
		props[key] = paramToProperty(def)
		if def.Required {
			required = append(required, key)
		}
	}
	schema := map[string]any{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		schema["required"] = required
	}

	desc := strings.TrimSpace(t.Custom.Description)
	if desc == "" {
		desc = t.Custom.Name
	}
	return llm.ToolDef{
		Name:        name,
		Description: desc,
		InputSchema: schema,
	}, nil
}

// customParamDef is the subset of the F5 input_parameters JSONB shape the
// translator reads.
type customParamDef struct {
	Key         string          `json:"key"`
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	Required    bool            `json:"required"`
	Description string          `json:"description"`
	Options     []any           `json:"options"`
	Properties  json.RawMessage `json:"properties"`
}

func decodeCustomParams(raw json.RawMessage) ([]customParamDef, bool) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" || string(raw) == "null" {
		return nil, false
	}
	var defs []customParamDef
	if err := json.Unmarshal(raw, &defs); err != nil {
		return nil, false
	}
	return defs, true
}

// paramToProperty turns one F5 parameter def into one JSON Schema
// property. Description / enum / nested object are surfaced when present.
func paramToProperty(def customParamDef) map[string]any {
	prop := map[string]any{}

	t := strings.ToUpper(strings.TrimSpace(def.Type))
	switch t {
	case "INTEGER", "INT":
		prop["type"] = "integer"
	case "FLOAT", "NUMBER":
		prop["type"] = "number"
	case "BOOL", "BOOLEAN":
		prop["type"] = "boolean"
	case "LIST", "ARRAY":
		prop["type"] = "array"
	case "OBJECT", "MAP":
		prop["type"] = "object"
		// Nested properties: pass through if the F5 def carries them.
		if len(def.Properties) > 0 {
			var nested map[string]any
			if err := json.Unmarshal(def.Properties, &nested); err == nil && nested != nil {
				prop["properties"] = nested
			}
		}
	case "DATE", "DATETIME":
		prop["type"] = "string"
		// format isn't in the v1 validator surface but Anthropic forwards it.
		if t == "DATE" {
			prop["format"] = "date"
		} else {
			prop["format"] = "date-time"
		}
	case "", "STRING", "TEXT":
		prop["type"] = "string"
	default:
		// Unknown F5 type — degrade to string + describe the original tag
		// so the LLM doesn't silently accept arbitrary structures.
		prop["type"] = "string"
		hint := "originally declared as F5 type " + def.Type
		if d := strings.TrimSpace(def.Description); d != "" {
			prop["description"] = d + " (" + hint + ")"
		} else {
			prop["description"] = hint
		}
	}

	// Enum surfacing — Anthropic supports `enum` natively for any type.
	if len(def.Options) > 0 {
		prop["enum"] = append([]any{}, def.Options...)
	}

	if d := strings.TrimSpace(def.Description); d != "" {
		// Don't clobber the description set by the unknown-tag branch.
		if _, exists := prop["description"]; !exists {
			prop["description"] = d
		}
	}
	return prop
}

// deepCopy JSON-roundtrip clones a map so a caller mutating the returned
// schema can't reach the capability's internal copy.
func deepCopy(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	b, err := json.Marshal(in)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}
