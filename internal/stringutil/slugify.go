package stringutil

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// slugifyTransformer composes NFKD normalization with combining-mark
// removal so accented characters (Café) collapse to their ASCII base
// forms (cafe). Built once at package init; safe to share — transform
// chains are immutable.
var slugifyTransformer = transform.Chain(
	norm.NFKD,
	runes.Remove(runes.In(unicode.Mn)), // strip combining marks
	norm.NFC,
)

// SlugRegex is the shape every persisted slug must match. Lowercase
// ASCII alphanumerics and dashes only; no leading or trailing dash; no
// double dashes (one or more single dashes between groups). Same shape
// is enforced for AI agents (cmd/ai_agents.go) and custom tools
// (cmd/ai_tools.go); centralizing here keeps the two in sync.
var SlugRegex = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ErrSlugImmutableMessage is the canonical human-readable message used
// when a PUT body's slug doesn't match the row's existing slug. Slugs
// are permanent code references — every dispatcher lookup, every
// JSONB-stored tool ref, every agent prompt that mentions a slug breaks
// if it changes after creation.
const ErrSlugImmutableMessage = "slug is permanent and cannot be changed after creation"

// Pre-compiled regexes used by Slugify. Compiled once at package init.
var (
	// Inserts a dash between camelCase boundaries:
	//   - "echoTwoFields" → "echo-Two-Fields" (then lowercased to "echo-two-fields")
	//   - "ABCdef"        → "AB-Cdef"           (handle the run-of-uppercase case)
	// We need TWO substitutions: lowercase→Uppercase boundary, and
	// Uppercase-run→Uppercase+lowercase boundary. Order matters: the
	// second pattern must run after the first so we don't double-insert.
	camelLowerUpper = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	camelAcronym    = regexp.MustCompile(`([A-Z]+)([A-Z][a-z])`)

	// Anything left that isn't [a-z0-9] becomes a dash, then we collapse
	// runs and trim. ASCII-only at this point thanks to the NFKD pass.
	nonSlugChar = regexp.MustCompile(`[^a-z0-9]+`)
)

// Slugify converts a free-form name to a URL-safe slug.
//
// Pipeline (deterministic, idempotent on already-slugged input):
//  1. NFKD-normalize and strip combining marks. Accented Latin chars
//     fold to ASCII (Café → Cafe, ß stays ß and gets dropped later).
//  2. Insert dashes at camelCase boundaries (echoTwoFields →
//     echo-Two-Fields), including acronym runs (HTTPRequest →
//     HTTP-Request).
//  3. Lowercase.
//  4. Replace every run of non-[a-z0-9] characters with a single dash.
//  5. Trim leading/trailing dashes.
//
// Empty input → empty slug; characters that can't be folded to ASCII
// (e.g., non-Latin scripts) are dropped — callers may need to provide
// an explicit slug in that case.
//
// Examples:
//
//	"echoTwoFields"   → "echo-two-fields"
//	"Café Faktura"    → "cafe-faktura"
//	"My Tool!! Foo"   → "my-tool-foo"
//	"HTTPRequest"     → "http-request"
//	"set_meter_reading" → "set-meter-reading"
func Slugify(s string) string {
	// 1. ASCII-fold accented characters via NFKD.
	folded, _, _ := transform.String(slugifyTransformer, s)

	// 2. Split camelCase boundaries (do this BEFORE lowercasing so we
	//    can still see the case).
	folded = camelAcronym.ReplaceAllString(folded, "$1-$2")
	folded = camelLowerUpper.ReplaceAllString(folded, "$1-$2")

	// 3. Lowercase.
	folded = strings.ToLower(folded)

	// 4. Replace non-slug runs with single dash.
	folded = nonSlugChar.ReplaceAllString(folded, "-")

	// 5. Trim leading/trailing dashes.
	return strings.Trim(folded, "-")
}
