package embeddings

import (
	"github.com/knadh/koanf/v2"
)

// DefaultMinRerankerScore is the reranker-score floor when embeddings.min_reranker_score is unset.
const DefaultMinRerankerScore = 0.3

// SearcherConfig holds the two retrieval thresholds; a zero MinSemanticSimilarity disables the semantic filter.
type SearcherConfig struct {
	MinSemanticSimilarity float64
	MinRerankerScore      float64
}

// SearcherConfigFromKoanf reads the thresholds; a nil koanf yields defaults (no semantic filter).
func SearcherConfigFromKoanf(ko *koanf.Koanf) SearcherConfig {
	cfg := SearcherConfig{
		MinRerankerScore: DefaultMinRerankerScore,
	}
	if ko == nil {
		return cfg
	}
	cfg.MinSemanticSimilarity = ko.Float64("embeddings.min_semantic_similarity")
	if ko.Exists("embeddings.min_reranker_score") {
		cfg.MinRerankerScore = ko.Float64("embeddings.min_reranker_score")
	}
	return cfg
}

// NewConfiguredSearcher builds a Searcher with config thresholds. When opt.Embed is nil it
// returns (nil, nil) — search disabled, not an error — so callers must nil-check the result.
func NewConfiguredSearcher(ko *koanf.Koanf, opt SearcherOpts) (*Searcher, error) {
	if opt.Embed == nil {
		return nil, nil
	}
	cfg := SearcherConfigFromKoanf(ko)
	opt.MinSemanticSimilarity = cfg.MinSemanticSimilarity
	opt.MinRerankerScore = cfg.MinRerankerScore

	s, err := NewSearcher(opt)
	if err != nil {
		return nil, err
	}
	opt.Lo.Info(
		"kb_search initialized",
		"min_semantic_similarity", cfg.MinSemanticSimilarity,
		"min_reranker_score", cfg.MinRerankerScore,
		"reranker", opt.Reranker != nil,
		"cache", opt.Cache != nil,
	)
	return s, nil
}
