// Package voyage implements embeddings.Provider against the Voyage AI
// embedding API (https://docs.voyageai.com/docs/embeddings), with the same
// reliability stack as internal/ai/llm/anthropic: bounded concurrency,
// per-call timeout, circuit breaker, exponential-backoff retry.
//
// Auth is a bearer token; key and endpoint resolve through secret.Resolver
// when configured, falling back to static Opts.
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

	"github.com/aziz-tb/merio/internal/secret"
)

const (
	defaultEndpoint = "https://api.voyageai.com/v1/embeddings"

	// modelVersionTag stamps Model() with a version date so mixed-model
	// rows coexist under the schema's unique index keyed on embedding_model.
	modelVersionTag = "2025-01"

	defaultModel        = "voyage-3-large"
	defaultDimensions   = 1024
	defaultMaxBatch     = 128
	defaultConcurrency  = 10
	defaultTotalTimeout = 30 * time.Second

	// 5 consecutive failures open the breaker for 60s; while open, calls
	// fail fast with errCircuitOpen so callers can stamp
	// ai_usage.outcome='circuit_open'.
	breakerFailures = 5
	breakerOpenFor  = 60 * time.Second

	// 3 total attempts (initial + 2 retries) with 1s then 3s backoff.
	retryAttempts = 3
	retryBackoff1 = 1 * time.Second
	retryBackoff2 = 3 * time.Second
)

// Provider implements embeddings.Provider against Voyage's REST API.
type Provider struct {
	apiKey       string
	endpoint     string
	resolver     *secret.Resolver
	model        string
	dimensions   int
	maxBatch     int
	totalTimeout time.Duration

	httpClient *http.Client
	sem        *semaphore.Weighted
	breaker    *gobreaker.CircuitBreaker
	lo         *logf.Logger
}

// Opts wires the provider's dependencies. APIKey + Lo are required;
// zero-value fields take the defaults above.
type Opts struct {
	APIKey       string
	Endpoint     string
	Resolver     *secret.Resolver
	Model        string
	Dimensions   int
	Concurrency  int
	MaxBatch     int
	TotalTimeout time.Duration
	HTTPClient   *http.Client
	Lo           *logf.Logger
}

// New builds a Voyage provider, erroring when required fields are missing.
func New(opt Opts) (*Provider, error) {
	if opt.APIKey == "" && opt.Resolver == nil {
		return nil, errors.New("voyage: APIKey is required")
	}
	if opt.Lo == nil {
		return nil, errors.New("voyage: Lo is required")
	}
	if opt.Endpoint == "" {
		opt.Endpoint = defaultEndpoint
	}
	if opt.Model == "" {
		opt.Model = defaultModel
	}
	if opt.Dimensions == 0 {
		opt.Dimensions = defaultDimensions
	}
	if opt.MaxBatch == 0 {
		opt.MaxBatch = defaultMaxBatch
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
		Name:        "voyage-embeddings",
		MaxRequests: 1,
		Interval:    0,
		Timeout:     breakerOpenFor,
		ReadyToTrip: func(c gobreaker.Counts) bool { return c.ConsecutiveFailures >= breakerFailures },
		OnStateChange: func(_ string, from, to gobreaker.State) {
			opt.Lo.Warn("voyage embeddings breaker state change", "from", from.String(), "to", to.String())
		},
	})

	return &Provider{
		apiKey:       opt.APIKey,
		endpoint:     opt.Endpoint,
		resolver:     opt.Resolver,
		model:        opt.Model,
		dimensions:   opt.Dimensions,
		maxBatch:     opt.MaxBatch,
		totalTimeout: opt.TotalTimeout,
		httpClient:   opt.HTTPClient,
		sem:          semaphore.NewWeighted(int64(opt.Concurrency)),
		breaker:      cb,
		lo:           opt.Lo,
	}, nil
}

// Embed batches the input slice into chunks of MaxBatchSize and returns
// embeddings in the same order. One round-trip per batch.
func (p *Provider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	out := make([][]float32, 0, len(texts))
	for i := 0; i < len(texts); i += p.maxBatch {
		end := i + p.maxBatch
		if end > len(texts) {
			end = len(texts)
		}
		vecs, err := p.embedBatch(ctx, texts[i:end])
		if err != nil {
			return nil, err
		}
		out = append(out, vecs...)
	}
	return out, nil
}

// Model returns the canonical project-namespaced model identifier.
func (p *Provider) Model() string { return "voyage/" + p.model + "@" + modelVersionTag }

// Dimensions is the configured output vector size; it must equal the
// embeddings column width (halfvec(1024)) or inserts fail.
func (p *Provider) Dimensions() int { return p.dimensions }

// MaxBatchSize returns the per-call input cap Embed batches against.
func (p *Provider) MaxBatchSize() int { return p.maxBatch }

// Name is the provider tag ("voyage") stamped on logs and ai_usage rows.
func (p *Provider) Name() string { return "voyage" }

// embedBatch handles one batch through the full reliability stack:
// total timeout → semaphore → circuit breaker → retry.
func (p *Provider) embedBatch(ctx context.Context, batch []string) ([][]float32, error) {
	ctx, cancel := context.WithTimeout(ctx, p.totalTimeout)
	defer cancel()

	if err := p.sem.Acquire(ctx, 1); err != nil {
		return nil, fmt.Errorf("voyage: semaphore: %w", err)
	}
	defer p.sem.Release(1)

	resultI, err := p.breaker.Execute(func() (any, error) {
		return p.callWithRetry(ctx, batch)
	})
	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) {
			return nil, errCircuitOpen
		}
		return nil, err
	}
	return resultI.([][]float32), nil
}

// callWithRetry is the retry loop. Transient errors (5xx, 429, network
// failures) retry with backoff; permanent errors (4xx other than 429,
// auth) fail fast.
func (p *Provider) callWithRetry(ctx context.Context, batch []string) ([][]float32, error) {
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
		vecs, err := p.callOnce(ctx, batch)
		if err == nil {
			return vecs, nil
		}
		lastErr = err
		if !isTransient(err) {
			return nil, err
		}
		p.lo.Warn("voyage embed call failed (will retry)", "attempt", attempt+1, "error", err)
	}
	return nil, fmt.Errorf("voyage: exhausted retries: %w", lastErr)
}

type embedRequest struct {
	Input           []string `json:"input"`
	Model           string   `json:"model"`
	InputType       string   `json:"input_type,omitempty"`
	Truncation      bool     `json:"truncation,omitempty"`
	OutputDimension int      `json:"output_dimension,omitempty"`
	OutputDtype     string   `json:"output_dtype,omitempty"`
}

type embedResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

type apiError struct {
	statusCode int
	detail     string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("voyage: status %d: %s", e.statusCode, e.detail)
}

// callOnce is one HTTP round-trip; no retry, no breaker — just request +
// response parsing + status-based error classification.
func (p *Provider) callOnce(ctx context.Context, batch []string) ([][]float32, error) {
	body := embedRequest{
		Input:           batch,
		Model:           p.model,
		InputType:       "document",
		Truncation:      true,
		OutputDimension: p.dimensions,
		OutputDtype:     "float",
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("voyage: marshal: %w", err)
	}

	apiKey := p.apiKey
	endpoint := p.endpoint
	if p.resolver != nil {
		if resolved := p.resolver.Resolve(ctx, "VOYAGE_API_KEY"); resolved != "" {
			apiKey = resolved
		}
		if resolved := p.resolver.Resolve(ctx, "VOYAGE_ENDPOINT"); resolved != "" {
			endpoint = resolved
		}
	}
	if apiKey == "" {
		return nil, errors.New("voyage: API key is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("voyage: new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("voyage: do: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("voyage: read body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &apiError{statusCode: resp.StatusCode, detail: strings.TrimSpace(string(respBody))}
	}

	var parsed embedResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("voyage: unmarshal: %w", err)
	}
	if len(parsed.Data) != len(batch) {
		return nil, fmt.Errorf("voyage: expected %d embeddings, got %d", len(batch), len(parsed.Data))
	}

	// The API may not return entries in input order — sort by index.
	out := make([][]float32, len(batch))
	for _, item := range parsed.Data {
		if item.Index < 0 || item.Index >= len(out) {
			return nil, fmt.Errorf("voyage: response index %d out of range", item.Index)
		}
		out[item.Index] = item.Embedding
	}
	return out, nil
}

// errCircuitOpen is the typed sentinel returned when the breaker is open.
// Callers map this onto ai_usage.outcome='circuit_open'.
var errCircuitOpen = errors.New("voyage: circuit open")

// IsCircuitOpen reports whether err is the breaker-open sentinel.
func IsCircuitOpen(err error) bool { return errors.Is(err, errCircuitOpen) }

// isTransient classifies an error as worth retrying. 429 + 5xx and any
// network error qualify. 4xx other than 429 do not.
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
	return true // network error, timeout, etc.
}
