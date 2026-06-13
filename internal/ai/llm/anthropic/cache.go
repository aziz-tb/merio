// Package anthropic implements the llm provider for the direct Anthropic Messages API.
package anthropic

import (
	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/aziz-tb/merio/internal/ai/llm"
	anthropicshared "github.com/aziz-tb/merio/internal/ai/llm/anthropic/shared"
	"github.com/aziz-tb/merio/internal/ai/llm/preset"
)

// applyCacheMarkers places cache_control breakpoints on the SDK params per
// the request's CachedBlock positions. Returns the number of markers placed
// (≤ shared.MaxCacheMarkers; clamped by preset caching support and by
// alignment with real content blocks). Returns 0 without mutating params when
// the preset disables caching.
//
// A marker at request index N caches everything up to and including that
// message. Request indices span the original request.Messages (system + turn
// mixed); we map them to the SDK's combined position (system blocks first,
// then turn messages), so index 0 lands on the last system text block.
func applyCacheMarkers(params *sdk.MessageNewParams, req llm.CompletionRequest, p preset.Preset) int {
	targets := anthropicshared.CacheMarkerTargets(req, p.CachingEnabled)
	if len(targets) == 0 {
		return 0
	}

	// Use the SDK helper so Type is set explicitly: a zero-valued
	// CacheControlEphemeralParam is treated as omitted by the SDK's
	// reflection-based IsOmitted, silently dropping markers from the request.
	cc := sdk.NewCacheControlEphemeralParam()
	if p.CachingExtendedTTL {
		cc.TTL = sdk.CacheControlEphemeralTTLTTL1h
	}

	placed := 0
	for _, t := range targets {
		if t.Slice == anthropicshared.CacheSliceSystem {
			if t.Index >= 0 && t.Index < len(params.System) {
				params.System[t.Index].CacheControl = cc
				placed++
			}
			continue
		}
		// Turn message: place marker on its last content block.
		if t.Index < 0 || t.Index >= len(params.Messages) {
			continue
		}
		msg := &params.Messages[t.Index]
		if len(msg.Content) == 0 {
			continue
		}
		last := &msg.Content[len(msg.Content)-1]
		switch {
		case last.OfText != nil:
			last.OfText.CacheControl = cc
			placed++
		case last.OfToolResult != nil:
			last.OfToolResult.CacheControl = cc
			placed++
		case last.OfToolUse != nil:
			last.OfToolUse.CacheControl = cc
			placed++
		}
	}
	return placed
}
