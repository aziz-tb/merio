package openai

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	sdk "github.com/openai/openai-go"

	"github.com/aziz-tb/merio/internal/ai/llm"
)

func TestWrapProviderErrorPreservesSentinelAndSDKError(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/chat/completions", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	apiErr := &sdk.Error{
		StatusCode: http.StatusBadRequest,
		Message:    "Function tools with reasoning_effort are not supported",
		Request:    req,
		Response:   &http.Response{StatusCode: http.StatusBadRequest},
	}

	wrapped := wrapProviderError(llm.ErrInvalidRequest, apiErr)
	if !errors.Is(wrapped, llm.ErrInvalidRequest) {
		t.Fatalf("expected errors.Is ErrInvalidRequest, got %v", wrapped)
	}
	var got *sdk.Error
	if !errors.As(wrapped, &got) {
		t.Fatalf("expected errors.As sdk.Error, got %v", wrapped)
	}
	if got != apiErr {
		t.Fatalf("expected original sdk error to be preserved")
	}
	if !strings.Contains(wrapped.Error(), apiErr.Message) {
		t.Fatalf("expected provider message in wrapped error, got %q", wrapped.Error())
	}
}
