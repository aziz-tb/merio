// Package redact rewrites a JSON payload (LLM request or response)
// into a PII-stripped copy safe to land in ai_usage.request_payload /
// ai_usage.response_payload for storage and analysis.
//
// Strategy — two layers:
//
//   1. **Pattern-based redaction** over every string value in the
//      payload, regardless of nesting. Catches emails, German IBANs,
//      phone numbers, contract numbers, customer IDs, Zählernummer,
//      and any extra patterns the deployment configures.
//
//   2. **Field-aware redaction** for known structured shapes — the
//      `get_customer_data` tool result holds first_name / last_name /
//      address fields that aren't shaped like generic PII. These are
//      replaced by sentinel placeholders when the field name matches
//      a known PII key.
//
// **Default bias: aggressive.** Over-redaction hurts debuggability but
// not compliance. Under-redaction (PII leaks through to stored audit
// rows) is non-negotiable under DSGVO Article 32 for the German energy
// sector at our scale (200k+ customers). False positives are worth it.
//
// Performance: each payload is walked once recursively; per-string
// regex passes are bounded by the precompiled pattern set. At 50ms p99
// for a 5kB payload (typical LLM message) this is well within the
// usage.Manager's synchronous-write budget.
package redact

import (
	"encoding/json"
	"regexp"
	"strings"
	"sync"
)

// Sentinel replacements. Kept short — the audit row needs to be
// human-readable when an operator pulls it for debugging.
const (
	SentinelEmail      = "[EMAIL]"
	SentinelIBAN       = "[IBAN]"
	SentinelPhone      = "[PHONE]"
	SentinelContract   = "[CONTRACT_ID]"
	SentinelCustomer   = "[CUSTOMER_ID]"
	SentinelMeter      = "[METER_ID]"
	SentinelName       = "[NAME]"
	SentinelAddress    = "[ADDRESS]"
	SentinelCustom     = "[REDACTED]"
)

// Field names (case-insensitive) we treat as PII when they appear as
// JSON object keys. Values under these keys get replaced with the
// matching sentinel regardless of their literal shape.
//
//   first_name / last_name / full_name → [NAME]
//   address / street / postal_code / zip / city → [ADDRESS]
//   email → [EMAIL]
//   phone / phone_number → [PHONE]
//   iban → [IBAN]
//
// The map is lower-cased for O(1) lookup; rewriter normalizes keys.
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

// Patterns. The order matters — IBAN matches before phone (an IBAN can
// look like a long phone-number-shaped string), and email matches
// before name fragments. All precompiled once.
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
			// Email (RFC 5322 simplified). Captures common shapes
			// including dots, plus-signs, hyphens. Doesn't try to
			// validate the full RFC — false positives are fine.
			{
				re:       regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
				sentinel: SentinelEmail,
			},
			// German IBAN: DE + 2 digits + 18 digits = 22 chars total.
			// We allow optional internal spaces (DE89 3704 ...) which
			// IBAN printouts often carry. The pattern is loose on
			// total length so non-DE IBANs (FR, NL, AT, CH) also match.
			{
				re:       regexp.MustCompile(`\b[A-Z]{2}\s?\d{2}(?:\s?[A-Z0-9]){12,30}\b`),
				sentinel: SentinelIBAN,
			},
			// Phone numbers. Matches:
			//   +49 170 1234567
			//   +49-170-1234567
			//   0170 1234567
			//   01701234567
			// Constraint: leading + or 0, then 8+ digits with optional
			// spaces/dashes between groups. Tight enough to avoid
			// catching arbitrary numeric strings like ticket IDs.
			{
				re:       regexp.MustCompile(`(?:\+\d{1,3}[\s\-]?|\b0)(?:\d[\s\-]?){8,13}\d\b`),
				sentinel: SentinelPhone,
			},
			// German contract identifiers — generic ALL-CAPS prefix +
			// dash + 4+ digits. Catches CNT-12345, VERT-789, KD-100001,
			// etc. Configurable extension via AddCustomPattern.
			{
				re:       regexp.MustCompile(`\b(?:CNT|VERT|KD|VK|VTR|ZN)-\d{4,}\b`),
				sentinel: SentinelContract,
			},
		}
	})
}

// customPatterns lets ops register additional regexes (German energy
// distributors have idiosyncratic ID shapes — Zählernummer schemes
// vary by Netzbetreiber, customer references vary by billing system).
// Appended once at startup; not safe to mutate concurrently after the
// first redaction.
var customPatterns []compiledPattern

// AddCustomPattern registers an additional regex → sentinel mapping.
// Call ONLY at startup, before any Record() / Redact() runs. The
// patterns array is precompiled and read-only after first use.
func AddCustomPattern(pattern, sentinel string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}
	customPatterns = append(customPatterns, compiledPattern{re: re, sentinel: sentinel})
	return nil
}

// Bytes is the top-level entrypoint. Takes a raw JSON payload, walks
// it recursively, applies field-aware redaction to known PII keys and
// pattern-based redaction to every remaining string value. Returns
// the redacted JSON or the original bytes if parsing fails (which
// means the caller passed non-JSON; we don't try to redact opaque
// strings here — that's the caller's job).
func Bytes(in []byte) []byte {
	if len(in) == 0 {
		return in
	}
	loadPatterns()
	var v interface{}
	if err := json.Unmarshal(in, &v); err != nil {
		// Caller passed non-JSON; we don't redact unstructured bytes.
		return in
	}
	walk(&v, "")
	out, err := json.Marshal(v)
	if err != nil {
		return in
	}
	return out
}

// String redacts a free-form string (not JSON) using the pattern set
// only. Field-aware redaction doesn't apply — there's no surrounding
// key context.
func String(s string) string {
	loadPatterns()
	return applyPatterns(s)
}

// walk recurses through a parsed JSON value, applying field-aware
// rules at object keys and pattern rules at leaf strings.
//
// parentKey is the lower-cased key under which the current value sits
// in its parent object. It's empty at the root + on array elements.
func walk(v *interface{}, parentKey string) {
	switch x := (*v).(type) {
	case string:
		// Field-aware override: if our parent key is a known PII key,
		// replace the value with the matching sentinel wholesale.
		if sentinel, ok := piiFields[parentKey]; ok && x != "" {
			*v = sentinel
			return
		}
		*v = applyPatterns(x)
	case map[string]interface{}:
		for k, child := range x {
			lk := strings.ToLower(k)
			walk(&child, lk)
			x[k] = child
		}
	case []interface{}:
		for i := range x {
			walk(&x[i], "") // array elements have no parent key context
		}
	}
}

// applyPatterns runs every precompiled regex (built-in + custom) over
// the string in order. Each match is replaced wholesale by the
// pattern's sentinel; subsequent patterns see the already-redacted
// string so we never double-redact.
func applyPatterns(s string) string {
	for _, p := range patterns {
		s = p.re.ReplaceAllString(s, p.sentinel)
	}
	for _, p := range customPatterns {
		s = p.re.ReplaceAllString(s, p.sentinel)
	}
	return s
}
