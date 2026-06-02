package anthropic

import (
	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/aziz-tb/merio/internal/ai/llm/preset"
)

// maxCacheMarkers is the per-request cap Anthropic enforces. We dedupe
// and trim down to this many before placing markers.
const maxCacheMarkers = 4

// applyCacheMarkers places cache_control breakpoints on the SDK params
// according to the request's CachedBlockMarker positions. Returns the
// number of markers actually placed (≤ maxCacheMarkers; clamped by caching
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
	if !p.CachingEnabled || len(req.CachedBlocks) == 0 {
		return 0
	}

	// Translate marker positions from request indices to the locations
	// they should land on in the SDK params. Build a map: request-message
	// index → (which slice, slice index) in the SDK params.
	type target struct {
		slice string // "system" | "turn"
		idx   int    // index within that slice
	}
	indexMap := make(map[int]target, len(req.Messages))
	sysCursor, turnCursor := 0, 0
	for i, m := range req.Messages {
		if m.Role == llm.RoleSystem {
			indexMap[i] = target{slice: "system", idx: sysCursor + len(m.Content) - 1}
			sysCursor += len(m.Content)
			continue
		}
		indexMap[i] = target{slice: "turn", idx: turnCursor}
		turnCursor++
	}

	// Dedupe markers (multiple markers at the same index are wasted) and
	// cap at maxCacheMarkers. Keep the most recent (later) markers — they
	// cover larger spans, which is the higher-value cache.
	seen := make(map[int]struct{}, len(req.CachedBlocks))
	uniq := make([]int, 0, len(req.CachedBlocks))
	for _, mk := range req.CachedBlocks {
		if _, ok := indexMap[mk.AfterMessageIndex]; !ok {
			continue
		}
		if _, dup := seen[mk.AfterMessageIndex]; dup {
			continue
		}
		seen[mk.AfterMessageIndex] = struct{}{}
		uniq = append(uniq, mk.AfterMessageIndex)
	}
	if len(uniq) > maxCacheMarkers {
		uniq = uniq[len(uniq)-maxCacheMarkers:]
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
	for _, reqIdx := range uniq {
		t := indexMap[reqIdx]
		if t.slice == "system" {
			if t.idx >= 0 && t.idx < len(params.System) {
				params.System[t.idx].CacheControl = cc
				placed++
			}
			continue
		}
		// Turn message: place marker on its last content block.
		if t.idx < 0 || t.idx >= len(params.Messages) {
			continue
		}
		msg := &params.Messages[t.idx]
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
