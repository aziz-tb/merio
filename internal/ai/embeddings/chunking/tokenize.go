package chunking

// EstimateTokens returns a rough token count for `text`. Approximation:
// 1 token ≈ 4 characters of typical Latin-script prose. Voyage uses a
// similar tokenizer to OpenAI's tiktoken; exact counts aren't needed for
// chunking decisions — only to choose between "fits in one chunk" vs
// "needs to be split." The cost of being off by 10-20% is recovered by
// the recursive structural fallback (it splits further only when needed).
//
// German compound words skew the ratio slightly (one word can hit 30+
// characters). Empirically /4 still gives reasonable chunk sizes on
// German energy-domain text. Revisit when an actual tokenizer port
// (tiktoken-go) becomes a justified dependency.
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return (len(text) + 3) / 4
}
