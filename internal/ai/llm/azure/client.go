// Package azure implements the llm provider for Azure Foundry, routing OpenAI-family
// deployments through the OpenAI SDK and Claude deployments through direct Anthropic HTTP.
package azure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
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

	azureAPIKeySecret   = "AZURE_API_KEY"
	azureEndpointSecret = "AZURE_FOUNDRY_ENDPOINT"

	foundryAnthropicVersion = "2023-06-01"
	foundryAnthropicAuth    = "x-api-key"
)

// Provider routes Azure Foundry model deployments to the correct internal
// surface. OpenAI-family deployments use the OpenAI Go SDK on /openai/v1.
// Claude deployments use direct HTTP on /anthropic/v1/messages because the
// Anthropic Go SDK does not support Azure Foundry.
type Provider struct {
	apiKey     string
	endpoint   string
	resolver   *secret.Resolver
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
	Endpoint    string
	Resolver    *secret.Resolver
	Catalog     *catalog.Catalog
	Registry    *preset.Registry
	Lo          *logf.Logger
	Concurrency int
	HTTPClient  *http.Client
}

// New builds an Azure Foundry Provider. It errors if neither an API key/endpoint nor a
// resolver is set, or if the catalog, registry, or logger is nil.
func New(opt Opts) (*Provider, error) {
	if opt.Resolver == nil && opt.APIKey == "" {
		return nil, fmt.Errorf("azure: api key is empty")
	}
	if opt.Resolver == nil && opt.Endpoint == "" {
		return nil, fmt.Errorf("azure: endpoint is empty")
	}
	if opt.Catalog == nil {
		return nil, fmt.Errorf("azure: model catalog is nil")
	}
	if opt.Registry == nil {
		return nil, fmt.Errorf("azure: preset registry is nil")
	}
	if opt.Lo == nil {
		return nil, fmt.Errorf("azure: logger is nil")
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
		Name:        "azure",
		Interval:    breakerInterval,
		Timeout:     breakerOpenFor,
		ReadyToTrip: func(counts gobreaker.Counts) bool { return counts.ConsecutiveFailures >= breakerFailures },
		OnStateChange: func(_ string, from, to gobreaker.State) {
			opt.Lo.Warn("azure.circuit_breaker.state", "from", from.String(), "to", to.String())
		},
	})

	p := &Provider{
		apiKey:     opt.APIKey,
		endpoint:   strings.TrimRight(opt.Endpoint, "/"),
		resolver:   opt.Resolver,
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
func (p *Provider) Name() string { return "azure" }

// Complete runs one completion, dispatching by model family — Claude via direct Anthropic
// HTTP, OpenAI-family via the OpenAI SDK — under the concurrency limit and circuit breaker,
// and returns typed llm errors (ErrPresetNotFound, ErrAuth, ErrCircuitOpen, ...).
func (p *Provider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	if req.PresetSlug == "" {
		return llm.CompletionResponse{}, fmt.Errorf("azure: PresetSlug is required")
	}
	pre, ok := p.registry.Get(req.PresetSlug)
	if !ok {
		return llm.CompletionResponse{}, fmt.Errorf("azure: %w (slug=%q)", llm.ErrPresetNotFound, req.PresetSlug)
	}
	if !pre.Enabled {
		return llm.CompletionResponse{}, fmt.Errorf("azure: preset %q is disabled", req.PresetSlug)
	}
	modelMeta, _ := p.catalog.Get(pre.Provider, pre.Model)
	family, err := modelFamily(pre.Model, modelMeta)
	if err != nil {
		return llm.CompletionResponse{}, err
	}

	cl := &callLog{
		provider:     p.Name(),
		family:       family,
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
		return llm.CompletionResponse{}, fmt.Errorf("azure: semaphore acquire: %w", err)
	}
	defer p.sem.Release(1)

	resAny, brErr := p.breaker.Execute(func() (any, error) {
		apiKey := p.resolveAPIKey(callCtx)
		endpoint := p.resolveEndpoint(callCtx)
		if apiKey == "" || endpoint == "" {
			return nil, llm.ErrAuth
		}
		switch family {
		case modelFamilyClaude:
			params, err := reqToAnthropicParams(req, pre, modelMeta, p.lo)
			if err != nil {
				return nil, err
			}
			cl.cacheMarkers = applyAnthropicCacheMarkers(&params, req, pre)
			resp, retryErr, attempts := withRetry(callCtx, func(c context.Context) (any, error) {
				return p.doAnthropic(c, endpoint, apiKey, params)
			})
			cl.attempts = attempts
			if retryErr != nil {
				return nil, retryErr
			}
			return resp, nil
		case modelFamilyOpenAI:
			params, err := openaitranslate.RequestToParams(req, pre, modelMeta)
			if err != nil {
				return nil, err
			}
			client := p.newOpenAIClient(endpoint, apiKey)
			resp, retryErr, attempts := withRetry(callCtx, func(c context.Context) (any, error) {
				return client.Chat.Completions.New(c, params)
			})
			cl.attempts = attempts
			if retryErr != nil {
				return nil, retryErr
			}
			return resp, nil
		default:
			return nil, fmt.Errorf("azure: unsupported model family %q", family)
		}
	})

	if brErr != nil {
		if brErr == gobreaker.ErrOpenState || brErr == gobreaker.ErrTooManyRequests {
			cl.emitFailure(p.lo, llm.ErrCircuitOpen)
			return llm.CompletionResponse{}, llm.ErrCircuitOpen
		}
		cl.emitFailure(p.lo, brErr)
		return llm.CompletionResponse{}, brErr
	}

	switch family {
	case modelFamilyClaude:
		resp, ok := resAny.(*anthropicResponse)
		if !ok || resp == nil {
			err := fmt.Errorf("azure: anthropic returned nil response")
			cl.emitFailure(p.lo, err)
			return llm.CompletionResponse{}, err
		}
		out := anthropicToCompletionResponse(resp, pre, cl.cacheMarkers, cl.attempts)
		cl.emitSuccess(p.lo, out)
		return out, nil
	case modelFamilyOpenAI:
		resp, ok := resAny.(*sdk.ChatCompletion)
		if !ok || resp == nil {
			err := fmt.Errorf("azure: openai returned nil completion")
			cl.emitFailure(p.lo, err)
			return llm.CompletionResponse{}, err
		}
		out := openaitranslate.ResponseToCompletionResponse(resp, pre, p.Name(), cl.attempts)
		cl.emitSuccess(p.lo, out)
		return out, nil
	default:
		err := fmt.Errorf("azure: unsupported model family %q", family)
		cl.emitFailure(p.lo, err)
		return llm.CompletionResponse{}, err
	}
}

// StreamComplete is not implemented for Azure; it always returns llm.ErrStreamingNotImplemented.
func (p *Provider) StreamComplete(_ context.Context, _ llm.CompletionRequest, _ func(string)) (llm.CompletionResponse, error) {
	return llm.CompletionResponse{}, llm.ErrStreamingNotImplemented
}

func (p *Provider) resolveAPIKey(ctx context.Context) string {
	if p.resolver != nil {
		if v := p.resolver.Resolve(ctx, azureAPIKeySecret); v != "" {
			return v
		}
	}
	return p.apiKey
}

func (p *Provider) resolveEndpoint(ctx context.Context) string {
	if p.resolver != nil {
		if v := p.resolver.Resolve(ctx, azureEndpointSecret); v != "" {
			return strings.TrimRight(v, "/")
		}
	}
	return p.endpoint
}

func (p *Provider) newOpenAIClient(endpoint string, apiKey string) sdk.Client {
	return sdk.NewClient(
		option.WithBaseURL(openAIBaseURL(endpoint)),
		option.WithAPIKey(apiKey),
		option.WithHeaderDel("authorization"),
		option.WithHeader("api-key", apiKey),
		option.WithMaxRetries(0),
		option.WithHTTPClient(p.httpClient),
	)
}

func (p *Provider) doAnthropic(ctx context.Context, endpoint string, apiKey string, params anthropicRequest) (*anthropicResponse, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("azure anthropic: marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicMessagesURL(endpoint), bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("azure anthropic: build request: %w", err)
	}
	httpReq.Header.Set("content-type", "application/json")
	httpReq.Header.Set(foundryAnthropicAuth, apiKey)
	httpReq.Header.Set("anthropic-version", foundryAnthropicVersion)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("azure anthropic: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, anthropicHTTPError{StatusCode: resp.StatusCode, Body: body}
	}

	var parsed anthropicResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("azure anthropic: parse response: %w", err)
	}
	return &parsed, nil
}

func openAIBaseURL(endpoint string) string {
	base := strings.TrimRight(endpoint, "/")
	if strings.HasSuffix(base, "/openai/v1") {
		return base
	}
	if strings.HasSuffix(base, "/openai") {
		return base + "/v1"
	}
	return base + "/openai/v1"
}

func anthropicMessagesURL(endpoint string) string {
	base := strings.TrimRight(endpoint, "/")
	if strings.HasSuffix(base, "/anthropic/v1/messages") {
		return base
	}
	if strings.HasSuffix(base, "/anthropic/v1") {
		return base + "/messages"
	}
	if strings.HasSuffix(base, "/anthropic") {
		return base + "/v1/messages"
	}
	return base + "/anthropic/v1/messages"
}

func classifyError(err error) (retryable bool, sentinel error) {
	if err == nil {
		return false, nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false, err
	}

	var sdkErr *sdk.Error
	if errors.As(err, &sdkErr) {
		return classifyStatus(sdkErr.StatusCode, err)
	}
	var httpErr anthropicHTTPError
	if errors.As(err, &httpErr) {
		return classifyStatus(httpErr.StatusCode, err)
	}

	return true, err
}

func classifyStatus(status int, err error) (bool, error) {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return false, llm.ErrAuth
	case status == http.StatusTooManyRequests:
		return true, llm.ErrRateLimited
	case status >= 500 && status < 600:
		return true, err
	case status >= 400 && status < 500:
		return false, llm.ErrInvalidRequest
	default:
		return false, err
	}
}

func withRetry(ctx context.Context, fn func(ctx context.Context) (any, error)) (any, error, int) {
	var (
		resp    any
		lastErr error
		attempt int
	)
	for attempt = 0; ; attempt++ {
		resp, lastErr = fn(ctx)
		if lastErr == nil {
			return resp, nil, attempt + 1
		}
		retryable, sentinel := classifyError(lastErr)
		if !retryable {
			if sentinel != nil {
				return nil, sentinel, attempt + 1
			}
			return nil, lastErr, attempt + 1
		}
		if attempt >= len(retryDelays) {
			if sentinel != nil && errors.Is(sentinel, llm.ErrRateLimited) {
				return nil, llm.ErrRateLimited, attempt + 1
			}
			return nil, lastErr, attempt + 1
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err(), attempt + 1
		case <-time.After(retryDelays[attempt]):
		}
	}
}

var retryDelays = []time.Duration{1 * time.Second, 3 * time.Second}

type anthropicHTTPError struct {
	StatusCode int
	Body       []byte
}

func (e anthropicHTTPError) Error() string {
	return fmt.Sprintf("azure anthropic: status=%d body=%s", e.StatusCode, strings.TrimSpace(string(e.Body)))
}

type callLog struct {
	provider     string
	family       string
	startedAt    time.Time
	traceID      string
	purpose      string
	preset       string
	model        string
	agentID      int
	resAttemptID int
	cacheMarkers int
	attempts     int
}

func (c *callLog) emitSuccess(lo *logf.Logger, resp llm.CompletionResponse) {
	lo.Info(c.provider+".complete",
		"provider_family", c.family,
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
		"cache_markers", c.cacheMarkers,
		"stop_reason", string(resp.StopReason),
		"attempts", c.attempts,
		"outcome", "success",
	)
}

func (c *callLog) emitFailure(lo *logf.Logger, err error) {
	statusCode := 0
	message := ""
	requestID := ""
	var sdkErr *sdk.Error
	if errors.As(err, &sdkErr) {
		statusCode = sdkErr.StatusCode
		message = sdkErr.Message
		if sdkErr.Response != nil {
			requestID = sdkErr.Response.Header.Get("x-request-id")
		}
	}
	var httpErr anthropicHTTPError
	if errors.As(err, &httpErr) {
		statusCode = httpErr.StatusCode
		message = strings.TrimSpace(string(httpErr.Body))
	}

	lo.Error(c.provider+".complete",
		"provider_family", c.family,
		"trace_id", c.traceID,
		"purpose", c.purpose,
		"preset", c.preset,
		"model", c.model,
		"agent_id", c.agentID,
		"resolution_attempt_id", c.resAttemptID,
		"latency_ms", int(time.Since(c.startedAt).Milliseconds()),
		"cache_markers", c.cacheMarkers,
		"attempts", c.attempts,
		"outcome", "error",
		"error", err.Error(),
		"api_status_code", statusCode,
		"api_error_message", message,
		"api_request_id", requestID,
	)
}
