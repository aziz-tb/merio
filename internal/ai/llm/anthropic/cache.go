package anthropic

import (
	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/aziz-tb/merio/internal/ai/llm"
	anthropicshared "github.com/aziz-tb/merio/internal/ai/llm/anthropic/shared"
	"github.com/aziz-tb/merio/internal/ai/llm/preset"
)

// applyCacheMarkers places cache_control breakpoints on the SDK params
// according to the request's CachedBlockMarker positions. Returns the
// number of markers actually placed (≤ shared.MaxCacheMarkers; clamped by caching
// support on the preset and by the markers' alignment with real content
// blocks).
//
// Marker semantics (mirroring the runtime contract documented in the F7
// plan): an AfterMessageIndex of N means "everything up to and including
// the message at request index N is stable and worth caching." We index
// into the original request.Messages (which mixes system + turn messages);
// after reqToSDKParams hoisted system messages out, we map the request
// index to the SDK's combined position (system blocks first, then turn
// messages). Index 0 typically points at the system prompt — its marker
// lands on the last system text block.
//
// When the preset disables caching, we return 0 without mutating params
// (the call goes through with no cache_control headers).
func applyCacheMarkers(params *sdk.MessageNewParams, req llm.CompletionRequest, p preset.Preset) int {
	targets := anthropicshared.CacheMarkerTargets(req, p.CachingEnabled)
	if len(targets) == 0 {
		return 0
	}

	// Construct via the SDK's helper so the Type field is set explicitly.
	// A zero-valued CacheControlEphemeralParam is treated as omitted by
	// the SDK's reflection-based IsOmitted, which would silently drop our
	// markers from the wire request.
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
