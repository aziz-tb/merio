package chunking

// EstimateTokens returns a rough token count (1 token ≈ 4 chars). Used
// only for fits-in-one-chunk vs needs-split decisions, so being off by
// 10-20% is fine — the recursive fallback splits further when needed.
// German compounds skew the ratio but /4 still yields reasonable sizes.
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return (len(text) + 3) / 4
}
