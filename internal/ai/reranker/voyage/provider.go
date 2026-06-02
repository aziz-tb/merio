// Package voyage implements reranker.Provider against Voyage AI's rerank
// endpoint. Same reliability stack as the embedding provider
// (internal/ai/embeddings/voyage): semaphore + breaker + retry + total
// timeout.
//
// API reference: https://docs.voyageai.com/docs/reranker
package voyage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sony/gobreaker"
	"github.com/zerodha/logf"
	"golang.org/x/sync/semaphore"

	"github.com/aziz-tb/merio/internal/ai/reranker"
)

const (
	defaultEndpoint     = "https://api.voyageai.com/v1/rerank"
	modelVersionTag     = "2025-01"
	defaultModel        = "rerank-2"
	defaultConcurrency  = 5
	defaultTotalTimeout = 30 * time.Second
	breakerFailures     = 5
	breakerOpenFor      = 60 * time.Second
	retryAttempts       = 3
	retryBackoff1       = 1 * time.Second
	retryBackoff2       = 3 * time.Second
)

// Provider implements reranker.Provider against Voyage's REST API.
type Provider struct {
	apiKey       string
	endpoint     string
	model        string
	totalTimeout time.Duration

	httpClient *http.Client
	sem        *semaphore.Weighted
	breaker    *gobreaker.CircuitBreaker
	lo         *logf.Logger
}

// Opts wires the provider's dependencies.
type Opts struct {
	APIKey       string
	Endpoint     string
	Model        string
	Concurrency  int
	TotalTimeout time.Duration
	HTTPClient   *http.Client
	Lo           *logf.Logger
}

// New builds a Voyage reranker provider.
func New(opt Opts) (*Provider, error) {
	if opt.APIKey == "" {
		return nil, errors.New("voyage reranker: APIKey is required")
	}
	if opt.Lo == nil {
		return nil, errors.New("voyage reranker: Lo is required")
	}
	if opt.Endpoint == "" {
		opt.Endpoint = defaultEndpoint
	}
	if opt.Model == "" {
		opt.Model = defaultModel
	}
	if opt.Concurrency == 0 {
		opt.Concurrency = defaultConcurrency
	}
	if opt.TotalTimeout == 0 {
		opt.TotalTimeout = defaultTotalTimeout
	}
	if opt.HTTPClient == nil {
		opt.HTTPClient = &http.Client{Timeout: opt.TotalTimeout + 5*time.Second}
	}
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "voyage-rerank",
		MaxRequests: 1,
		Timeout:     breakerOpenFor,
		ReadyToTrip: func(c gobreaker.Counts) bool { return c.ConsecutiveFailures >= breakerFailures },
		OnStateChange: func(_ string, from, to gobreaker.State) {
			opt.Lo.Warn("voyage rerank breaker state change", "from", from.String(), "to", to.String())
		},
	})
	return &Provider{
		apiKey:       opt.APIKey,
		endpoint:     opt.Endpoint,
		model:        opt.Model,
		totalTimeout: opt.TotalTimeout,
		httpClient:   opt.HTTPClient,
		sem:          semaphore.NewWeighted(int64(opt.Concurrency)),
		breaker:      cb,
		lo:           opt.Lo,
	}, nil
}

// Model returns the project-namespaced model id.
func (p *Provider) Model() string { return "voyage/" + p.model + "@" + modelVersionTag }
func (p *Provider) Name() string  { return "voyage" }

// Rerank scores `candidates` against `query` and returns up to topK
// results in descending relevance order.
func (p *Provider) Rerank(ctx context.Context, query string, candidates []string, topK int) ([]reranker.Result, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	if topK <= 0 || topK > len(candidates) {
		topK = len(candidates)
	}

	ctx, cancel := context.WithTimeout(ctx, p.totalTimeout)
	defer cancel()

	if err := p.sem.Acquire(ctx, 1); err != nil {
		return nil, fmt.Errorf("voyage rerank: semaphore: %w", err)
	}
	defer p.sem.Release(1)

	resultI, err := p.breaker.Execute(func() (any, error) {
		return p.callWithRetry(ctx, query, candidates, topK)
	})
	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) {
			return nil, errCircuitOpen
		}
		return nil, err
	}
	return resultI.([]reranker.Result), nil
}

type rerankRequest struct {
	Query           string   `json:"query"`
	Documents       []string `json:"documents"`
	Model           string   `json:"model"`
	TopK            int      `json:"top_k"`
	ReturnDocuments bool     `json:"return_documents,omitempty"`
	Truncation      bool     `json:"truncation,omitempty"`
}

type rerankResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"data"`
	Model string `json:"model"`
}

type apiError struct {
	statusCode int
	detail     string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("voyage rerank: status %d: %s", e.statusCode, e.detail)
}

func (p *Provider) callWithRetry(ctx context.Context, query string, candidates []string, topK int) ([]reranker.Result, error) {
	var lastErr error
	backoffs := []time.Duration{0, retryBackoff1, retryBackoff2}
	for attempt := 0; attempt < retryAttempts; attempt++ {
		if backoffs[attempt] > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoffs[attempt]):
			}
		}
		result, err := p.callOnce(ctx, query, candidates, topK)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !isTransient(err) {
			return nil, err
		}
		p.lo.Warn("voyage rerank call failed (will retry)", "attempt", attempt+1, "error", err)
	}
	return nil, fmt.Errorf("voyage rerank: exhausted retries: %w", lastErr)
}

func (p *Provider) callOnce(ctx context.Context, query string, candidates []string, topK int) ([]reranker.Result, error) {
	body := rerankRequest{
		Query:      query,
		Documents:  candidates,
		Model:      p.model,
		TopK:       topK,
		Truncation: true,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("voyage rerank: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("voyage rerank: new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("voyage rerank: do: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("voyage rerank: read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &apiError{statusCode: resp.StatusCode, detail: strings.TrimSpace(string(respBody))}
	}

	var parsed rerankResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("voyage rerank: unmarshal: %w", err)
	}
	out := make([]reranker.Result, len(parsed.Data))
	for i, d := range parsed.Data {
		out[i] = reranker.Result{Index: d.Index, RelevanceScore: d.RelevanceScore}
	}
	return out, nil
}

var errCircuitOpen = errors.New("voyage rerank: circuit open")

// IsCircuitOpen reports whether err is the rerank breaker-open sentinel.
func IsCircuitOpen(err error) bool { return errors.Is(err, errCircuitOpen) }

func isTransient(err error) bool {
	var ae *apiError
	if errors.As(err, &ae) {
		if ae.statusCode == 429 {
			return true
		}
		if ae.statusCode >= 500 && ae.statusCode <= 599 {
			return true
		}
		return false
	}
	return true
}
