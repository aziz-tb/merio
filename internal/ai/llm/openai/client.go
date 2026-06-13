// Package openai implements the llm provider for OpenAI's Chat Completions API.
package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	sdk "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/sony/gobreaker"
	"github.com/zerodha/logf"
	"golang.org/x/sync/semaphore"

	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/aziz-tb/merio/internal/ai/llm/catalog"
	"github.com/aziz-tb/merio/internal/ai/llm/openaitranslate"
	"github.com/aziz-tb/merio/internal/ai/llm/preset"
	"github.com/aziz-tb/merio/internal/secret"
)

const (
	defaultConcurrency  = 30
	defaultTotalTimeout = 120 * time.Second
	defaultHTTPTimeout  = 90 * time.Second

	breakerFailures = 5
	breakerInterval = 30 * time.Second
	breakerOpenFor  = 60 * time.Second

	openAIAPIKeySecret = "OPENAI_API_KEY"
)

// Provider implements llm.LLMProvider against OpenAI's Chat Completions API
// using the official Go SDK. It has no Azure routing; Foundry calls are handled
// by the Azure provider.
type Provider struct {
	apiKey     string
	resolver   *secret.Resolver
	baseURL    string
	httpClient *http.Client
	catalog    *catalog.Catalog
	registry   *preset.Registry
	sem        *semaphore.Weighted
	breaker    *gobreaker.CircuitBreaker
	lo         *logf.Logger
	totalTO    time.Duration
	healthy    atomic.Bool
}

// Opts holds the dependencies and tuning passed to New.
type Opts struct {
	APIKey      string
	Resolver    *secret.Resolver
	Catalog     *catalog.Catalog
	Registry    *preset.Registry
	Lo          *logf.Logger
	Concurrency int
	BaseURL     string
	HTTPClient  *http.Client
}

// New builds an OpenAI Provider. It errors if no API key or resolver is set, or if the
// catalog, registry, or logger is nil.
func New(opt Opts) (*Provider, error) {
	if opt.APIKey == "" && opt.Resolver == nil {
		return nil, fmt.Errorf("openai: api key is empty")
	}
	if opt.Catalog == nil {
		return nil, fmt.Errorf("openai: model catalog is nil")
	}
	if opt.Registry == nil {
		return nil, fmt.Errorf("openai: preset registry is nil")
	}
	if opt.Lo == nil {
		return nil, fmt.Errorf("openai: logger is nil")
	}

	conc := opt.Concurrency
	if conc <= 0 {
		conc = defaultConcurrency
	}
	httpClient := opt.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}

	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "openai",
		Interval:    breakerInterval,
		Timeout:     breakerOpenFor,
		ReadyToTrip: func(counts gobreaker.Counts) bool { return counts.ConsecutiveFailures >= breakerFailures },
		OnStateChange: func(_ string, from, to gobreaker.State) {
			opt.Lo.Warn("openai.circuit_breaker.state", "from", from.String(), "to", to.String())
		},
	})

	p := &Provider{
		apiKey:     opt.APIKey,
		resolver:   opt.Resolver,
		baseURL:    opt.BaseURL,
		httpClient: httpClient,
		catalog:    opt.Catalog,
		registry:   opt.Registry,
		sem:        semaphore.NewWeighted(int64(conc)),
		breaker:    cb,
		lo:         opt.Lo,
		totalTO:    defaultTotalTimeout,
	}
	p.healthy.Store(true)
	return p, nil
}

// Name returns the provider's name.
func (p *Provider) Name() string { return "openai" }

// Complete runs one chat completion: it resolves the request's preset, enforces the
// concurrency limit and circuit breaker, retries transient failures, and returns typed llm
// errors (ErrPresetNotFound, ErrAuth, ErrCircuitOpen, ...).
func (p *Provider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	if req.PresetSlug == "" {
		return llm.CompletionResponse{}, fmt.Errorf("openai: PresetSlug is required")
	}
	pre, ok := p.registry.Get(req.PresetSlug)
	if !ok {
		return llm.CompletionResponse{}, fmt.Errorf("openai: %w (slug=%q)", llm.ErrPresetNotFound, req.PresetSlug)
	}
	if !pre.Enabled {
		return llm.CompletionResponse{}, fmt.Errorf("openai: preset %q is disabled", req.PresetSlug)
	}

	modelMeta, _ := p.catalog.Get(pre.Provider, pre.Model)
	params, err := openaitranslate.RequestToParams(req, pre, modelMeta)
	if err != nil {
		return llm.CompletionResponse{}, err
	}

	cl := &callLog{
		provider:     p.Name(),
		startedAt:    time.Now(),
		traceID:      req.TraceID,
		purpose:      string(req.Purpose),
		preset:       pre.Slug,
		model:        pre.Model,
		agentID:      req.AgentID,
		resAttemptID: req.ResolutionAttemptID,
	}

	callCtx, cancel := context.WithTimeout(ctx, p.totalTO)
	defer cancel()

	if err := p.sem.Acquire(callCtx, 1); err != nil {
		cl.emitFailure(p.lo, err)
		return llm.CompletionResponse{}, fmt.Errorf("openai: semaphore acquire: %w", err)
	}
	defer p.sem.Release(1)

	resAny, brErr := p.breaker.Execute(func() (any, error) {
		apiKey := p.resolveAPIKey(callCtx)
		if apiKey == "" {
			return nil, llm.ErrAuth
		}
		client := p.newClient(apiKey)
		resp, retryErr, attempts := withRetry(callCtx, func(c context.Context) (*sdk.ChatCompletion, error) {
			return client.Chat.Completions.New(c, params)
		})
		cl.attempts = attempts
		if retryErr != nil {
			return nil, retryErr
		}
		return resp, nil
	})

	if brErr != nil {
		if brErr == gobreaker.ErrOpenState || brErr == gobreaker.ErrTooManyRequests {
			cl.emitFailure(p.lo, llm.ErrCircuitOpen)
			return llm.CompletionResponse{}, llm.ErrCircuitOpen
		}
		cl.emitFailure(p.lo, brErr)
		return llm.CompletionResponse{}, brErr
	}

	resp, ok := resAny.(*sdk.ChatCompletion)
	if !ok || resp == nil {
		err := fmt.Errorf("openai: provider returned nil completion")
		cl.emitFailure(p.lo, err)
		return llm.CompletionResponse{}, err
	}
	out := openaitranslate.ResponseToCompletionResponse(resp, pre, p.Name(), cl.attempts)
	cl.emitSuccess(p.lo, out)
	return out, nil
}

// StreamComplete is not implemented for OpenAI; it always returns llm.ErrStreamingNotImplemented.
func (p *Provider) StreamComplete(_ context.Context, _ llm.CompletionRequest, _ func(string)) (llm.CompletionResponse, error) {
	return llm.CompletionResponse{}, llm.ErrStreamingNotImplemented
}

func (p *Provider) resolveAPIKey(ctx context.Context) string {
	if p.resolver != nil {
		if v := p.resolver.Resolve(ctx, openAIAPIKeySecret); v != "" {
			return v
		}
	}
	return p.apiKey
}

func (p *Provider) newClient(apiKey string) sdk.Client {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithMaxRetries(0),
		option.WithHTTPClient(p.httpClient),
	}
	if p.baseURL != "" {
		opts = append(opts, option.WithBaseURL(p.baseURL))
	}
	return sdk.NewClient(opts...)
}

type callLog struct {
	provider     string
	startedAt    time.Time
	traceID      string
	purpose      string
	preset       string
	model        string
	agentID      int
	resAttemptID int
	attempts     int
}

func (c *callLog) emitSuccess(lo *logf.Logger, resp llm.CompletionResponse) {
	lo.Info(c.provider+".complete",
		"trace_id", c.traceID,
		"purpose", c.purpose,
		"preset", c.preset,
		"model", c.model,
		"agent_id", c.agentID,
		"resolution_attempt_id", c.resAttemptID,
		"latency_ms", int(time.Since(c.startedAt).Milliseconds()),
		"input_tokens", resp.Usage.InputTokens,
		"output_tokens", resp.Usage.OutputTokens,
		"cached_tokens", resp.Usage.CachedTokens,
		"stop_reason", string(resp.StopReason),
		"attempts", c.attempts,
		"outcome", "success",
	)
}

func (c *callLog) emitFailure(lo *logf.Logger, err error) {
	statusCode := 0
	requestID := ""
	message := ""
	var apiErr *sdk.Error
	if errors.As(err, &apiErr) {
		statusCode = apiErr.StatusCode
		message = apiErr.Message
		if apiErr.Response != nil {
			requestID = apiErr.Response.Header.Get("x-request-id")
		}
	}

	lo.Error(c.provider+".complete",
		"trace_id", c.traceID,
		"purpose", c.purpose,
		"preset", c.preset,
		"model", c.model,
		"agent_id", c.agentID,
		"resolution_attempt_id", c.resAttemptID,
		"latency_ms", int(time.Since(c.startedAt).Milliseconds()),
		"attempts", c.attempts,
		"outcome", "error",
		"error", err.Error(),
		"api_status_code", statusCode,
		"api_error_message", message,
		"api_request_id", requestID,
	)
}
