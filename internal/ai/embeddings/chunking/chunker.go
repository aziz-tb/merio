// Package chunking implements the locked F12 chunking strategy:
// heading-aware primary split → recursive structural fallback for
// oversized sections → merge of undersized siblings.
//
// The recursive fallback never splits mid-word or mid-sentence, which
// preserves German compound words (Abschlagszahlung) and sentence
// semantics where character-based splitters would break both.
package chunking

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// Config controls chunk size targets in tokens (estimated via
// tokenize.EstimateTokens). Defaults at DefaultConfig().
type Config struct {
	// TargetTokens is the size each chunk aims for during a recursive
	// split, softened by structural boundaries (never breaks a sentence
	// to hit it exactly).
	TargetTokens int

	// OverlapTokens is reserved for a future sliding-window fallback;
	// currently unused — structural splits are non-overlapping.
	OverlapTokens int

	// MaxTokens is the threshold above which a section is recursively
	// split. Sections at or below this size pass through unchanged.
	MaxTokens int

	// MinTokens is the threshold below which a section is merged with an
	// adjacent sibling, to avoid vector-DB noise from near-empty chunks.
	MinTokens int
}

// DefaultConfig returns the locked v1 config from the F12 strategy doc.
func DefaultConfig() Config {
	return Config{
		TargetTokens:  512,
		OverlapTokens: 50,
		MaxTokens:     800,
		MinTokens:     100,
	}
}

// Chunk is one logical unit of text to embed with provenance.
type Chunk struct {
	// Index is the chunk's position within the source (0, 1, 2, …),
	// stable across re-runs on the same input.
	Index int

	// Text is the actual content to embed.
	Text string

	// SourcePath is the heading breadcrumb this chunk came from (e.g.
	// "## Payment Terms > ### Late Fees"; empty for atomic content). It
	// surfaces in the kb_search response so operators see why a chunk
	// matched.
	SourcePath string

	// ContentHash is sha256(Text) hex-encoded. Stored on the embeddings
	// row for per-chunk drift detection across re-indexes.
	ContentHash string
}

// section is the internal type after the heading-aware split — it
// retains its position in the heading hierarchy for SourcePath rendering.
type section struct {
	text string
	path string
}

// headingRE matches ATX headings (#, ##, … level 1-6) at line start.
// Setext (underlined ===) headings are not handled.
var headingRE = regexp.MustCompile(`(?m)^(#{1,6})\s+(.+?)\s*$`)

// ChunkMarkdown splits a Markdown article using the locked strategy:
//  1. Split on headings into sections with a hierarchical SourcePath.
//  2. Recursively split any section over MaxTokens; pass the rest through.
//  3. Merge sections under MinTokens into the next neighbor (which keeps
//     the earlier section's heading context).
//  4. Renumber Index and compute ContentHash.
func ChunkMarkdown(text string, cfg Config) []Chunk {
	cfg = withDefaults(cfg)
	sections := splitByHeading(text)
	var chunks []Chunk
	for _, sec := range sections {
		if EstimateTokens(sec.text) > cfg.MaxTokens {
			chunks = append(chunks, recursiveSplit(sec, cfg)...)
		} else {
			chunks = append(chunks, Chunk{Text: sec.text, SourcePath: sec.path})
		}
	}
	chunks = mergeSmall(chunks, cfg)
	return finalize(chunks)
}

// ChunkAtomicText is the non-Markdown path for single-block text: skips
// the heading step and recursively splits only if over MaxTokens.
func ChunkAtomicText(text string, cfg Config) []Chunk {
	cfg = withDefaults(cfg)
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if EstimateTokens(text) <= cfg.MaxTokens {
		return finalize([]Chunk{{Text: text}})
	}
	return finalize(recursiveSplit(section{text: text}, cfg))
}

func withDefaults(c Config) Config {
	d := DefaultConfig()
	if c.TargetTokens == 0 {
		c.TargetTokens = d.TargetTokens
	}
	if c.OverlapTokens == 0 {
		c.OverlapTokens = d.OverlapTokens
	}
	if c.MaxTokens == 0 {
		c.MaxTokens = d.MaxTokens
	}
	if c.MinTokens == 0 {
		c.MinTokens = d.MinTokens
	}
	return c
}

// splitByHeading walks the text linearly, accumulating each region
// between headings as its own section and computing the breadcrumb
// path. Content before the first heading (preamble) gets path="".
func splitByHeading(text string) []section {
	matches := headingRE.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return []section{{text: strings.TrimSpace(text)}}
	}
	var out []section
	var stack []heading

	// Capture preamble (content before first heading) if any.
	first := matches[0][0]
	if pre := strings.TrimSpace(text[:first]); pre != "" {
		out = append(out, section{text: pre})
	}

	for i, m := range matches {
		level := m[3] - m[2] // length of '#' run
		title := text[m[4]:m[5]]
		// Pop stack to (level - 1)
		for len(stack) > 0 && stack[len(stack)-1].level >= level {
			stack = stack[:len(stack)-1]
		}
		stack = append(stack, heading{level: level, title: title})

		// Body of this section runs from end-of-heading-line to start
		// of next heading (or end of text).
		bodyStart := m[1]
		var bodyEnd int
		if i+1 < len(matches) {
			bodyEnd = matches[i+1][0]
		} else {
			bodyEnd = len(text)
		}
		body := strings.TrimSpace(text[bodyStart:bodyEnd])
		// Prepend the heading text to the body — it carries keyword
		// signal for the keyword half of hybrid search.
		body = stack[len(stack)-1].title + "\n\n" + body
		body = strings.TrimSpace(body)

		path := makePath(stack)
		out = append(out, section{text: body, path: path})
	}
	return out
}

func makePath(stack []heading) string {
	parts := make([]string, len(stack))
	for i, h := range stack {
		parts[i] = strings.Repeat("#", h.level) + " " + h.title
	}
	return strings.Join(parts, " > ")
}

type heading struct {
	level int
	title string
}

// recursiveSplit handles one section larger than MaxTokens. Tries each
// structural splitter in order; if a split produces chunks that all fit
// (≤MaxTokens), returns them. Otherwise recurses on the still-too-large
// chunks with the next-level splitter.
func recursiveSplit(sec section, cfg Config) []Chunk {
	splitters := []func(string) []string{
		splitByParagraph,
		splitBySentence,
		splitByClause,
		splitByWhitespace,
	}
	return recursiveSplitWith(sec.text, cfg, splitters, 0, sec.path)
}

func recursiveSplitWith(text string, cfg Config, splitters []func(string) []string, depth int, path string) []Chunk {
	if depth >= len(splitters) {
		// Last-resort: emit as-is even if over MaxTokens. Guards against
		// pathological input; the whitespace splitter normally prevents it.
		return []Chunk{{Text: text, SourcePath: path}}
	}
	parts := splitters[depth](text)
	if len(parts) <= 1 {
		// Splitter found no boundaries — try next splitter.
		return recursiveSplitWith(text, cfg, splitters, depth+1, path)
	}

	// Pack consecutive parts into chunks until the next part would push
	// us over MaxTokens.
	var packed []string
	var cur strings.Builder
	curTokens := 0
	for _, p := range parts {
		pTokens := EstimateTokens(p)
		if pTokens > cfg.MaxTokens {
			// This single part is still too large — flush the buffer,
			// then recurse on the part with the next splitter level.
			if cur.Len() > 0 {
				packed = append(packed, cur.String())
				cur.Reset()
				curTokens = 0
			}
			for _, c := range recursiveSplitWith(p, cfg, splitters, depth+1, path) {
				packed = append(packed, c.Text)
			}
			continue
		}
		if curTokens+pTokens > cfg.TargetTokens && cur.Len() > 0 {
			packed = append(packed, cur.String())
			cur.Reset()
			curTokens = 0
		}
		if cur.Len() > 0 {
			cur.WriteString(splitterJoinChar(depth))
		}
		cur.WriteString(p)
		curTokens += pTokens
	}
	if cur.Len() > 0 {
		packed = append(packed, cur.String())
	}

	out := make([]Chunk, 0, len(packed))
	for _, p := range packed {
		out = append(out, Chunk{Text: strings.TrimSpace(p), SourcePath: path})
	}
	return out
}

// splitterJoinChar returns the boundary string to re-insert between
// packed parts (indexed by splitter depth) so the text reads naturally.
func splitterJoinChar(depth int) string {
	switch depth {
	case 0:
		return "\n\n"
	case 1:
		return " "
	case 2:
		return ", "
	default:
		return " "
	}
}

func splitByParagraph(text string) []string {
	parts := regexp.MustCompile(`\n\s*\n`).Split(text, -1)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// sentenceEndRE matches terminating punctuation, whitespace, then an
// uppercase letter — the uppercase requirement avoids splitting on
// abbreviations followed by lowercase (e.g. German "z. B.").
var sentenceEndRE = regexp.MustCompile(`([.!?])\s+([A-ZÄÖÜ])`)

func splitBySentence(text string) []string {
	// Insert a sentinel between sentences, then split on it.
	withSentinel := sentenceEndRE.ReplaceAllString(text, "$1\x00$2")
	parts := strings.Split(withSentinel, "\x00")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

var clauseRE = regexp.MustCompile(`[,;]\s+`)

func splitByClause(text string) []string {
	parts := clauseRE.Split(text, -1)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func splitByWhitespace(text string) []string {
	return strings.Fields(text)
}

// mergeSmall merges each chunk below MinTokens into the next chunk (or
// the previous if there's no next), keeping the larger neighbor's path.
func mergeSmall(chunks []Chunk, cfg Config) []Chunk {
	if len(chunks) <= 1 {
		return chunks
	}
	out := make([]Chunk, 0, len(chunks))
	for i := 0; i < len(chunks); i++ {
		c := chunks[i]
		if EstimateTokens(c.Text) >= cfg.MinTokens {
			out = append(out, c)
			continue
		}
		// Too small: merge forward, falling back to backward if forward
		// overflows MaxTokens or there is no next chunk.
		if i+1 < len(chunks) {
			next := chunks[i+1]
			merged := c.Text + "\n\n" + next.Text
			if EstimateTokens(merged) <= cfg.MaxTokens {
				chunks[i+1] = Chunk{Text: merged, SourcePath: next.SourcePath}
				continue
			}
		}
		if len(out) > 0 {
			prev := out[len(out)-1]
			merged := prev.Text + "\n\n" + c.Text
			if EstimateTokens(merged) <= cfg.MaxTokens {
				out[len(out)-1] = Chunk{Text: merged, SourcePath: prev.SourcePath}
				continue
			}
		}
		// Neither merge fits — keep the small chunk rather than drop it.
		out = append(out, c)
	}
	return out
}

// finalize assigns sequential Index values and computes ContentHash.
func finalize(chunks []Chunk) []Chunk {
	for i := range chunks {
		chunks[i].Index = i
		sum := sha256.Sum256([]byte(chunks[i].Text))
		chunks[i].ContentHash = hex.EncodeToString(sum[:])
	}
	return chunks
}
