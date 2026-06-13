// Package redact rewrites a JSON payload (LLM request or response) into a
// PII-stripped copy safe to store in ai_usage.request_payload /
// ai_usage.response_payload. Two layers:
//
//  1. Pattern-based redaction over every string value, at any nesting.
//  2. Field-aware redaction for known PII keys (e.g. first_name / address
//     in a get_customer_data tool result) that aren't shaped like PII.
//
// Bias is intentionally aggressive: over-redaction only hurts debuggability,
// under-redaction leaks PII into audit rows (DSGVO Art. 32), so false
// positives are acceptable.
package redact

import (
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"unicode"
)

// Sentinel replacements, kept short so audit rows stay human-readable.
const (
	SentinelEmail    = "[EMAIL]"
	SentinelIBAN     = "[IBAN]"
	SentinelPhone    = "[PHONE]"
	SentinelContract = "[CONTRACT_ID]"
	SentinelCustomer = "[CUSTOMER_ID]"
	SentinelMeter    = "[METER_ID]"
	SentinelName     = "[NAME]"
	SentinelAddress  = "[ADDRESS]"
	SentinelCustom   = "[REDACTED]"
)

// piiFields maps JSON object keys whose values are PII to their sentinel,
// regardless of the value's literal shape. Keys are canonical snake_case;
// normalizeFieldName folds incoming snake_case, camelCase, and PascalCase
// into them, so matching is effectively case-insensitive.
var piiFields = map[string]string{
	"first_name":     SentinelName,
	"last_name":      SentinelName,
	"full_name":      SentinelName,
	"name":           SentinelName,
	"address":        SentinelAddress,
	"street":         SentinelAddress,
	"postal_code":    SentinelAddress,
	"zip":            SentinelAddress,
	"city":           SentinelAddress,
	"email":          SentinelEmail,
	"phone":          SentinelPhone,
	"phone_number":   SentinelPhone,
	"iban":           SentinelIBAN,
	"contract_id":    SentinelContract,
	"customer_id":    SentinelCustomer,
	"kundennummer":   SentinelCustomer,
	"vertragsnummer": SentinelContract,
	"zaehlernummer":  SentinelMeter,
	"meter_number":   SentinelMeter,
}

// Pattern order matters: IBAN is matched before phone, since an IBAN can
// look like a long phone-shaped string.
type compiledPattern struct {
	re       *regexp.Regexp
	sentinel string
}

var (
	patterns     []compiledPattern
	patternsOnce sync.Once
)

func loadPatterns() {
	patternsOnce.Do(func() {
		patterns = []compiledPattern{
			// Email, RFC 5322 simplified; not fully validated since false
			// positives are fine.
			{
				re:       regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
				sentinel: SentinelEmail,
			},
			// IBAN with optional internal spaces. Loose on length so non-DE
			// IBANs (FR, NL, AT, CH) match too.
			{
				re:       regexp.MustCompile(`\b[A-Z]{2}\s?\d{2}(?:\s?[A-Z0-9]){12,30}\b`),
				sentinel: SentinelIBAN,
			},
			// Phone: leading + or 0 then 8+ digits with optional spaces or
			// dashes, e.g. +49 170 1234567 or 01701234567. Tight enough to
			// avoid plain numeric strings like ticket IDs.
			{
				re:       regexp.MustCompile(`(?:\+\d{1,3}[\s\-]?|\b0)(?:\d[\s\-]?){8,13}\d\b`),
				sentinel: SentinelPhone,
			},
			// Contract IDs: ALL-CAPS prefix + dash + 4+ digits, e.g. CNT-12345.
			{
				re:       regexp.MustCompile(`\b(?:CNT|VERT|KD|VK|VTR|ZN)-\d{4,}\b`),
				sentinel: SentinelContract,
			},
		}
	})
}

// Bytes redacts a raw JSON payload, applying field-aware redaction to known
// PII keys and pattern-based redaction to every other string value. Returns
// the input unchanged if it doesn't parse as JSON.
func Bytes(in []byte) []byte {
	if len(in) == 0 {
		return in
	}
	loadPatterns()
	var v interface{}
	if err := json.Unmarshal(in, &v); err != nil {
		return in
	}
	walk(&v, "")
	out, err := json.Marshal(v)
	if err != nil {
		return in
	}
	return out
}

// String redacts a free-form (non-JSON) string using patterns only; with no
// surrounding key context, field-aware redaction doesn't apply.
func String(s string) string {
	loadPatterns()
	return applyPatterns(s)
}

// walk recurses through a parsed JSON value, applying field-aware rules at
// object keys and pattern rules at leaf strings. parentKey is the normalized
// key of the current value in its parent object, empty at the root and on
// array elements.
func walk(v *interface{}, parentKey string) {
	if sentinel, ok := piiFields[parentKey]; ok && isRedactableFieldValue(*v) {
		*v = sentinel
		return
	}
	switch x := (*v).(type) {
	case string:
		*v = applyPatterns(x)
	case map[string]interface{}:
		for k, child := range x {
			lk := normalizeFieldName(k)
			walk(&child, lk)
			x[k] = child
		}
	case []interface{}:
		for i := range x {
			walk(&x[i], "")
		}
	}
}

func isRedactableFieldValue(v interface{}) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(x) != ""
	case []interface{}:
		return len(x) > 0
	case map[string]interface{}:
		return len(x) > 0
	default:
		return true
	}
}

func normalizeFieldName(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	runes := []rune(key)
	var b strings.Builder
	lastWasUnderscore := false
	for i, r := range runes {
		if r == '_' || r == '-' || unicode.IsSpace(r) {
			if b.Len() > 0 && !lastWasUnderscore {
				b.WriteByte('_')
				lastWasUnderscore = true
			}
			continue
		}
		if unicode.IsUpper(r) {
			if b.Len() > 0 && !lastWasUnderscore && shouldInsertFieldUnderscore(runes, i) {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(unicode.ToLower(r))
		}
		lastWasUnderscore = false
	}
	return strings.Trim(b.String(), "_")
}

func shouldInsertFieldUnderscore(runes []rune, i int) bool {
	if i == 0 {
		return false
	}
	prev := runes[i-1]
	if prev == '_' || prev == '-' || unicode.IsSpace(prev) {
		return false
	}
	if unicode.IsLower(prev) || unicode.IsDigit(prev) {
		return true
	}
	if unicode.IsUpper(prev) && i+1 < len(runes) {
		next := runes[i+1]
		if next == 's' && i+2 == len(runes) {
			return false
		}
		return unicode.IsLower(next)
	}
	return false
}

// applyPatterns replaces each pattern's matches with its sentinel, in order.
// Later patterns see the already-redacted string, so no double-redaction.
func applyPatterns(s string) string {
	for _, p := range patterns {
		s = p.re.ReplaceAllString(s, p.sentinel)
	}
	return s
}
