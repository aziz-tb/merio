package anthropic

import (
	"context"
	"errors"
	"net/http"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/aziz-tb/merio/internal/ai/llm"
)

// retryDelays defines the exponential backoff schedule used for transient
// errors. Two retries (three attempts total: 0, 1s, 3s). v1 keeps it
// simple; if production data shows tail-latency issues we'll move to
// jittered backoff.
var retryDelays = []time.Duration{1 * time.Second, 3 * time.Second}

// classifyError sorts an SDK error into one of:
//   - retryable (transient): caller retries up to len(retryDelays) times.
//   - terminal: caller stops immediately.
//
// Retryable: network errors (ctx cancellation due to upstream timeout),
// 5xx server errors, 429 rate limited.
// Terminal: 4xx other than 429 (auth, bad request).
//
// Returns the canonical sentinel for terminal errors so callers can
// errors.Is them rather than parsing strings.
func classifyError(err error) (retryable bool, sentinel error) {
	if err == nil {
		return false, nil
	}
	// Context errors propagate as-is — caller observes them through ctx.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false, err
	}

	var apiErr *sdk.Error
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden:
			return false, llm.ErrAuth
		case apiErr.StatusCode == http.StatusTooManyRequests:
			return true, llm.ErrRateLimited
		case apiErr.StatusCode >= 500 && apiErr.StatusCode < 600:
			return true, err
		case apiErr.StatusCode >= 400 && apiErr.StatusCode < 500:
			return false, llm.ErrInvalidRequest
		}
		return false, err
	}

	// Non-API errors (DNS, dial, TLS, EOF, etc.) — transient by default.
	return true, err
}

// withRetry calls fn up to len(retryDelays)+1 times with backoff between
// attempts on retryable errors. Stops early if ctx is canceled or the
// error is terminal.
//
// Final error: if the last attempt's error was rate-limit-classified
// (ErrRateLimited), wraps it for caller distinction; otherwise returns
// the underlying error.
func withRetry(ctx context.Context, fn func(ctx context.Context) (*sdk.Message, error)) (*sdk.Message, error, int) {
	var (
		msg     *sdk.Message
		lastErr error
		attempt int
	)
	for attempt = 0; ; attempt++ {
		msg, lastErr = fn(ctx)
		if lastErr == nil {
			return msg, nil, attempt + 1
		}
		retryable, sentinel := classifyError(lastErr)
		if !retryable {
			if sentinel != nil {
				return nil, sentinel, attempt + 1
			}
			return nil, lastErr, attempt + 1
		}
		if attempt >= len(retryDelays) {
			// Out of retries. Surface ErrRateLimited specifically when the
			// last failure was a 429.
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
