package embeddings

import (
	"github.com/knadh/koanf/v2"
)

const DefaultMinRerankerScore = 0.3

type SearcherConfig struct {
	MinSemanticSimilarity float64
	MinRerankerScore      float64
}

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
