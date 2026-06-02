// Package customerdata contains small JSON helpers shared by the customer and
// contract cache normalizers. The helpers keep ERP-authored JSON key order
// intact for Zone 2 pass-through data.
package customerdata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Object decodes a JSON object into raw per-key values. Number values stay as
// bytes until callers explicitly coerce them.
func Object(raw json.RawMessage) (map[string]json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// UnwrapObject returns a nested object when raw has a key such as "customer"
// or "contract"; otherwise it returns raw unchanged.
func UnwrapObject(raw json.RawMessage, key string) (json.RawMessage, map[string]json.RawMessage, error) {
	data, err := Object(raw)
	if err != nil {
		return nil, nil, err
	}
	if len(data) == 0 {
		return raw, data, nil
	}
	if nested, ok := data[key]; ok && isObject(nested) {
		nestedData, err := Object(nested)
		return nested, nestedData, err
	}
	return raw, data, nil
}

// BuildRemainder removes Zone 1 keys from an object while preserving the
// original order and casing of the remaining fields.
func BuildRemainder(raw json.RawMessage, zone1Keys map[string]bool) json.RawMessage {
	var out bytes.Buffer
	out.WriteByte('{')

	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return json.RawMessage(`{}`)
	}

	first := true
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return json.RawMessage(`{}`)
		}
		key, ok := keyTok.(string)
		if !ok {
			return json.RawMessage(`{}`)
		}

		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return json.RawMessage(`{}`)
		}
		if zone1Keys[key] {
			continue
		}
		if !first {
			out.WriteByte(',')
		}
		first = false
		keyJSON, _ := json.Marshal(key)
		out.Write(keyJSON)
		out.WriteByte(':')
		out.Write(value)
	}
	out.WriteByte('}')
	return out.Bytes()
}

func FirstRaw(data map[string]json.RawMessage, keys ...string) json.RawMessage {
	for _, key := range keys {
		raw, ok := data[key]
		if !ok || len(raw) == 0 || string(raw) == "null" {
			continue
		}
		return raw
	}
	return nil
}

func FirstString(data map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		raw, ok := data[key]
		if !ok || len(raw) == 0 || string(raw) == "null" {
			continue
		}
		if s, ok := stringFromRaw(raw); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func FirstBool(data map[string]json.RawMessage, keys ...string) (bool, bool, error) {
	for _, key := range keys {
		raw, ok := data[key]
		if !ok || len(raw) == 0 || string(raw) == "null" {
			continue
		}
		var b bool
		if err := json.Unmarshal(raw, &b); err == nil {
			return b, true, nil
		}
		if s, ok := stringFromRaw(raw); ok {
			parsed, err := strconv.ParseBool(strings.TrimSpace(s))
			if err != nil {
				return false, true, err
			}
			return parsed, true, nil
		}
		return false, true, fmt.Errorf("expected boolean")
	}
	return false, false, nil
}

func FirstStringSlice(data map[string]json.RawMessage, keys ...string) ([]string, bool, error) {
	for _, key := range keys {
		raw, ok := data[key]
		if !ok || len(raw) == 0 || string(raw) == "null" {
			continue
		}
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, true, err
		}
		out := make([]string, 0, len(values))
		for _, item := range values {
			if s, ok := stringFromRaw(item); ok {
				if s = strings.TrimSpace(s); s != "" {
					out = append(out, s)
				}
			}
		}
		return out, true, nil
	}
	return nil, false, nil
}

func stringFromRaw(raw json.RawMessage) (string, bool) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, true
	}
	var n json.Number
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&n); err == nil {
		return n.String(), true
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return strconv.FormatBool(b), true
	}
	return "", false
}

func isObject(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) >= 2 && raw[0] == '{'
}
