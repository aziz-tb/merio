package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	sdk "github.com/openai/openai-go"

	"github.com/aziz-tb/merio/internal/ai/llm"
)

var retryDelays = []time.Duration{1 * time.Second, 3 * time.Second}

func classifyError(err error) (retryable bool, sentinel error) {
	if err == nil {
		return false, nil
	}
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

	return true, err
}

func withRetry(ctx context.Context, fn func(ctx context.Context) (*sdk.ChatCompletion, error)) (*sdk.ChatCompletion, error, int) {
	var (
		resp    *sdk.ChatCompletion
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
				return nil, wrapProviderError(sentinel, lastErr), attempt + 1
			}
			return nil, lastErr, attempt + 1
		}
		if attempt >= len(retryDelays) {
			if sentinel != nil && errors.Is(sentinel, llm.ErrRateLimited) {
				return nil, wrapProviderError(llm.ErrRateLimited, lastErr), attempt + 1
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

func wrapProviderError(sentinel, err error) error {
	if sentinel == nil {
		return err
	}
	if err == nil || errors.Is(err, sentinel) {
		return sentinel
	}
	var apiErr *sdk.Error
	if errors.As(err, &apiErr) && apiErr.Message != "" {
		return fmt.Errorf("%w: %s: %w", sentinel, apiErr.Message, err)
	}
	return fmt.Errorf("%w: %w", sentinel, err)
}
