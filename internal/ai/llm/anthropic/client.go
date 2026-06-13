package anthropic

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/sony/gobreaker"
	"github.com/zerodha/logf"
	"golang.org/x/sync/semaphore"

	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/aziz-tb/merio/internal/ai/llm/catalog"
	"github.com/aziz-tb/merio/internal/ai/llm/preset"
)

// Tunables. Hardcoded for v1; if production data shows
// any of them are wrong we'll move to per-preset config.
const (
	defaultConcurrency  = 30
	defaultTotalTimeout = 120 * time.Second
	defaultHTTPTimeout  = 90 * time.Second

	breakerFailures = 5
	breakerInterval = 30 * time.Second
	breakerOpenFor  = 60 * time.Second
)

// Provider implements llm.LLMProvider against Anthropic's Messages API.
//
//   - Concurrency: a weighted semaphore caps in-flight calls; acquisition
//     respects the request context.
//   - Reliability: exponential backoff on transient errors (1s, 3s) wrapped
//     in a gobreaker that returns ErrCircuitOpen once upstream keeps failing.
//   - Caching: applies cache_control markers per the request when the preset
//     enables caching, within Anthropic's per-request marker cap.
type Provider struct {
	client   sdk.Client
	catalog  *catalog.Catalog
	registry *preset.Registry
	sem      *semaphore.Weighted
	breaker  *gobreaker.CircuitBreaker
	lo       *logf.Logger
	totalTO  time.Duration
	healthy  atomic.Bool
}

// Opts wires dependencies into Provider.New. APIKey is required (zero
// returns an error). Registry must be the same registry the rest of the
// app uses; it's how the provider resolves PresetSlug → Preset.
type Opts struct {
	APIKey      string
	Catalog     *catalog.Catalog
	Registry    *preset.Registry
	Lo          *logf.Logger
	Concurrency int
	// BaseURL overrides the SDK's default for tests; empty = SDK default.
	BaseURL string
	// HTTPClient overrides the SDK's default; empty = SDK default with
	// a 90s timeout. Tests use this to point at httptest.Server.
	HTTPClient *http.Client
}

// New builds a Provider. Returns an error when required deps are missing.
func New(opt Opts) (*Provider, error) {
	if opt.APIKey == "" {
		return nil, fmt.Errorf("anthropic: api key is empty")
	}
	if opt.Catalog == nil {
		return nil, fmt.Errorf("anthropic: model catalog is nil")
	}
	if opt.Registry == nil {
		return nil, fmt.Errorf("anthropic: preset registry is nil")
	}
	if opt.Lo == nil {
		return nil, fmt.Errorf("anthropic: logger is nil")
	}

	conc := opt.Concurrency
	if conc <= 0 {
		conc = defaultConcurrency
	}

	httpClient := opt.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}

	sdkOpts := []option.RequestOption{
		option.WithAPIKey(opt.APIKey),
		option.WithMaxRetries(0), // we own retry policy
		option.WithHTTPClient(httpClient),
	}
	if opt.BaseURL != "" {
		sdkOpts = append(sdkOpts, option.WithBaseURL(opt.BaseURL))
	}

	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "anthropic",
		Interval:    breakerInterval,
		Timeout:     breakerOpenFor,
		ReadyToTrip: func(counts gobreaker.Counts) bool { return counts.ConsecutiveFailures >= breakerFailures },
		OnStateChange: func(_ string, from, to gobreaker.State) {
			opt.Lo.Warn("anthropic.circuit_breaker.state",
				"from", from.String(),
				"to", to.String(),
			)
		},
	})

	p := &Provider{
		client:   sdk.NewClient(sdkOpts...),
		catalog:  opt.Catalog,
		registry: opt.Registry,
		sem:      semaphore.NewWeighted(int64(conc)),
		breaker:  cb,
		lo:       opt.Lo,
		totalTO:  defaultTotalTimeout,
	}
	p.healthy.Store(true)
	return p, nil
}

// Name reports the provider identifier.
func (p *Provider) Name() string { return "anthropic" }

// Complete runs one Messages.New call with retry + circuit + concurrency
// + caching applied. See package docs for the policy details.
func (p *Provider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	if req.PresetSlug == "" {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: PresetSlug is required")
	}
	pre, ok := p.registry.Get(req.PresetSlug)
	if !ok {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: %w (slug=%q)", llm.ErrPresetNotFound, req.PresetSlug)
	}
	if !pre.Enabled {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: preset %q is disabled", req.PresetSlug)
	}

	params, err := reqToSDKParams(req, pre)
	if err != nil {
		return llm.CompletionResponse{}, err
	}
	if modelMeta, ok := p.catalog.Get(pre.Provider, pre.Model); ok {
		if req.ThinkingEnabled {
			if thinking, ok := thinkingConfigForModel(modelMeta); ok {
				params.Thinking = thinking
			} else {
				p.lo.Warn("anthropic.extended_thinking.unsupported_model",
					"model", pre.Model,
				)
			}
		}
	} else if req.ThinkingEnabled {
		p.lo.Warn("anthropic.extended_thinking.model_missing_from_catalog",
			"model", pre.Model,
		)
	}
	// Strip sampling knobs that Anthropic rejects under thinking
	// (temperature != 1, top_k set, top_p outside [0.95, 1.0]). Noop
	// when thinking is disabled.
	params = sanitizeForThinking(params, p.lo, pre.Model)
	markers := applyCacheMarkers(&params, req, pre)

	cl := &callLog{
		startedAt:    time.Now(),
		traceID:      req.TraceID,
		purpose:      string(req.Purpose),
		preset:       pre.Slug,
		model:        pre.Model,
		agentID:      req.AgentID,
		resAttemptID: req.ResolutionAttemptID,
		cacheMarkers: markers,
	}

	// Total timeout covers acquire + retry + breaker; the SDK's HTTP timeout
	// is a separate, smaller per-attempt window.
	callCtx, cancel := context.WithTimeout(ctx, p.totalTO)
	defer cancel()

	if err := p.sem.Acquire(callCtx, 1); err != nil {
		cl.emitFailure(p.lo, err)
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: semaphore acquire: %w", err)
	}
	defer p.sem.Release(1)

	resAny, brErr := p.breaker.Execute(func() (any, error) {
		msg, retryErr, attempts := withRetry(callCtx, func(c context.Context) (*sdk.Message, error) {
			return p.client.Messages.New(c, params)
		})
		cl.attempts = attempts
		if retryErr != nil {
			return nil, retryErr
		}
		return msg, nil
	})

	if brErr != nil {
		// Translate gobreaker's open-circuit error to our sentinel for
		// callers; everything else passes through.
		if brErr == gobreaker.ErrOpenState || brErr == gobreaker.ErrTooManyRequests {
			cl.emitFailure(p.lo, llm.ErrCircuitOpen)
			return llm.CompletionResponse{}, llm.ErrCircuitOpen
		}
		cl.emitFailure(p.lo, brErr)
		return llm.CompletionResponse{}, brErr
	}

	msg, ok := resAny.(*sdk.Message)
	if !ok || msg == nil {
		err := fmt.Errorf("anthropic: provider returned nil message")
		cl.emitFailure(p.lo, err)
		return llm.CompletionResponse{}, err
	}

	out := sdkToCompletionResponse(msg, pre, markers, cl.attempts)
	cl.emitSuccess(p.lo, out)
	return out, nil
}

// StreamComplete is the streaming variant; not yet implemented (returns
// ErrStreamingNotImplemented).
func (p *Provider) StreamComplete(_ context.Context, _ llm.CompletionRequest, _ func(string)) (llm.CompletionResponse, error) {
	return llm.CompletionResponse{}, llm.ErrStreamingNotImplemented
}
