package llm

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeRouterProvider struct {
	name string
	got  CompletionRequest
}

func (f *fakeRouterProvider) Complete(_ context.Context, req CompletionRequest) (CompletionResponse, error) {
	f.got = req
	return CompletionResponse{Provider: f.name, PresetUsed: req.PresetSlug}, nil
}

func (f *fakeRouterProvider) StreamComplete(_ context.Context, req CompletionRequest, _ func(string)) (CompletionResponse, error) {
	f.got = req
	return CompletionResponse{Provider: f.name, PresetUsed: req.PresetSlug}, nil
}

func (f *fakeRouterProvider) Name() string { return f.name }

func TestRouterDispatchesByPresetProvider(t *testing.T) {
	openaiProvider := &fakeRouterProvider{name: "openai"}
	anthropicProvider := &fakeRouterProvider{name: "anthropic"}
	r, err := NewRouter(RouterOpts{
		Providers: map[string]LLMProvider{
			"anthropic": anthropicProvider,
			"openai":    openaiProvider,
		},
		ProviderForPreset: staticPresetProvider(map[string]string{"test-preset": "openai"}),
	})
	require.NoError(t, err)

	resp, err := r.Complete(context.Background(), CompletionRequest{PresetSlug: "test-preset"})
	require.NoError(t, err)
	require.Equal(t, "openai", resp.Provider)
	require.Equal(t, "test-preset", openaiProvider.got.PresetSlug)
	require.Empty(t, anthropicProvider.got.PresetSlug)
}

func TestRouterUnknownProvider(t *testing.T) {
	r, err := NewRouter(RouterOpts{
		Providers:         map[string]LLMProvider{"anthropic": &fakeRouterProvider{name: "anthropic"}},
		ProviderForPreset: staticPresetProvider(map[string]string{"test-preset": "azure"}),
	})
	require.NoError(t, err)

	_, err = r.Complete(context.Background(), CompletionRequest{PresetSlug: "test-preset"})
	require.ErrorContains(t, err, `no provider registered for "azure"`)
}

func TestRouterMissingPreset(t *testing.T) {
	r, err := NewRouter(RouterOpts{
		Providers:         map[string]LLMProvider{"anthropic": &fakeRouterProvider{name: "anthropic"}},
		ProviderForPreset: staticPresetProvider(map[string]string{"test-preset": "anthropic"}),
	})
	require.NoError(t, err)

	_, err = r.Complete(context.Background(), CompletionRequest{PresetSlug: "missing"})
	require.True(t, errors.Is(err, ErrPresetNotFound))
}

func staticPresetProvider(providers map[string]string) PresetProviderLookup {
	return func(slug string) (string, bool) {
		provider, ok := providers[slug]
		return provider, ok
	}
}
