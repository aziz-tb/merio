package llm

import (
	"context"
	"fmt"
)

// PresetProviderLookup returns the provider id configured for a preset slug.
// It intentionally exposes only the one field the router needs so this package
// does not import preset and close an llm <-> preset cycle.
type PresetProviderLookup func(slug string) (provider string, ok bool)

// Router implements LLMProvider by dispatching to an underlying provider based
// on the resolved preset's provider field.
type Router struct {
	providers         map[string]LLMProvider
	providerForPreset PresetProviderLookup
}

// RouterOpts wires dependencies into NewRouter.
type RouterOpts struct {
	Providers         map[string]LLMProvider
	ProviderForPreset PresetProviderLookup
}

// NewRouter builds a Router.
func NewRouter(opt RouterOpts) (*Router, error) {
	if len(opt.Providers) == 0 {
		return nil, fmt.Errorf("router: at least one provider is required")
	}
	if opt.ProviderForPreset == nil {
		return nil, fmt.Errorf("router: provider lookup is nil")
	}
	providers := make(map[string]LLMProvider, len(opt.Providers))
	for name, provider := range opt.Providers {
		if provider == nil {
			return nil, fmt.Errorf("router: provider %q is nil", name)
		}
		providers[name] = provider
	}
	return &Router{providers: providers, providerForPreset: opt.ProviderForPreset}, nil
}

// Complete dispatches the request to the provider named by the resolved preset.
func (r *Router) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	provider, err := r.providerFor(req.PresetSlug)
	if err != nil {
		return CompletionResponse{}, err
	}
	return provider.Complete(ctx, req)
}

// StreamComplete dispatches streaming calls in the same way as Complete.
func (r *Router) StreamComplete(ctx context.Context, req CompletionRequest, onChunk func(string)) (CompletionResponse, error) {
	provider, err := r.providerFor(req.PresetSlug)
	if err != nil {
		return CompletionResponse{}, err
	}
	return provider.StreamComplete(ctx, req, onChunk)
}

// Name reports the router's own identifier for logs and metrics.
func (r *Router) Name() string { return "router" }

func (r *Router) providerFor(slug string) (LLMProvider, error) {
	providerName, ok := r.providerForPreset(slug)
	if !ok {
		return nil, fmt.Errorf("router: %w (slug=%q)", ErrPresetNotFound, slug)
	}
	provider, ok := r.providers[providerName]
	if !ok {
		return nil, fmt.Errorf("router: no provider registered for %q (preset=%q)", providerName, slug)
	}
	return provider, nil
}
