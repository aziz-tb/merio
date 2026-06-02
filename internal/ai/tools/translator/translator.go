// Package translator turns resolved agent tools (the unified
// ResolvedTool type) into the provider-agnostic llm.ToolDef shape that
// the LLM provider then forwards to its SDK. v1 produces JSON Schema
// from two source shapes:
//
//   - InternalCapability — already declares JSON Schema (subset:
//     basic types, enum, nested object, array). Pass-through with a
//     defensive copy so caller mutations don't propagate.
//   - Custom Python tool — declares the F5 array-of-{key, type, ...}
//     shape (Enneo-derived). We map each entry to a JSON Schema property
//     and collect requireds.
//
// Tool names go on the wire to Anthropic, which enforces ^[a-zA-Z0-9_-]{1,64}$.
// Internal capability ids and ai_tools.slug values both comply by
// construction (slugify produces lowercase + dashes; capability ids are
// lowercase + underscores). The translator validates anyway and surfaces
// a typed error rather than letting the SDK reject silently.
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

// anthropicToolNameRegex mirrors the SDK's documented constraint. We
// validate at translate time to surface the failure here rather than
// inside the Anthropic translator.
var anthropicToolNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// Translate converts a list of ResolvedTool to llm.ToolDef. Stops at the
// first translation error so callers see a single failure message. An
// empty input produces an empty output with no error.
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

// TranslateOne is exposed for callers that already hold a single tool —
// the test endpoint, debug paths.
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

// translateInternal handles the easy case: capabilities declare proper
// JSON Schema already, just needs a deep copy + name/description pull.
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

// translateCustom maps the F5 array shape to JSON Schema. Each F5 type
// tag maps to a JSON Schema type:
//
//	STRING/TEXT/DATE/DATETIME → {"type":"string"}     (DATE adds format)
//	INTEGER/INT               → {"type":"integer"}
//	FLOAT/NUMBER              → {"type":"number"}
//	BOOL/BOOLEAN              → {"type":"boolean"}
//	LIST/ARRAY                → {"type":"array"}      (no item type in v1)
//	OBJECT/MAP                → {"type":"object"}
//
// Unknown tags become {"type":"string"} with a description noting the
// original tag — surfaces tool-config drift to the LLM rather than
// silently widening the schema.
//
// `options` (a list of allowed values from the F5 schema) becomes a JSON
// Schema `enum`. Nested objects: a parameter whose `type` is OBJECT can
// declare `properties` directly; the translator passes those through.
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

// customParamDef mirrors the F5 input_parameters JSONB shape. Only the
// fields the translator reads are mapped; extra fields are ignored.
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
		// Hint to the LLM that this is a date-shaped string. format is
		// not part of our v1 validator surface but Anthropic forwards it.
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
		// Only set if we didn't already populate it via the unknown-tag
		// branch above.
		if _, exists := prop["description"]; !exists {
			prop["description"] = d
		}
	}
	return prop
}

// deepCopy is a JSON-roundtrip clone for map[string]any. Used so a
// caller mutating the returned schema doesn't accidentally mutate the
// capability's internal copy.
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
