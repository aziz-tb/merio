package runner

import "github.com/abhinavxd/libredesk/internal/ai/llm"

// maxCacheMarkers is Anthropic's per-request cap on cache_control
// breakpoints. The provider also enforces this; the runner's manager
// keeps its own count so logs / Trace.CacheMetrics.Markers reflects the
// pre-truncation intent.
const maxCacheMarkers = 4

// cacheMarkerManager tracks the cumulative cache-marker positions
// across an agent run.
//
// Cumulative semantics: the system prompt is always cached (marker 0).
// As the loop progresses, the just-completed tool_use + tool_result
// turn becomes "stable" — Anthropic can hit the cache for everything
// up to and including those messages on the next request. We add a
// fresh marker after each tool round so subsequent turns see growing
// cached spans.
//
// Cap: 4 markers total. When a new marker would exceed the cap, drop
// the OLDEST one. Reasoning: older markers cover smaller cached spans
// (the system prompt alone); newer markers cover larger spans (system
// + N tool rounds). Discarding older markers preserves the high-value
// caches and recycles the budget.
type cacheMarkerManager struct {
	markers []llm.CachedBlockMarker
	cap     int
}

// newCacheMarkerManager seeds the manager with one marker on the
// system prompt boundary (assumed to be the last system message in
// the initial messages slice). Callers pass the system-message count;
// the marker lands on index (systemCount - 1).
func newCacheMarkerManager(systemCount int) *cacheMarkerManager {
	if systemCount <= 0 {
		// No system messages → no initial marker.
		return &cacheMarkerManager{cap: maxCacheMarkers}
	}
	return &cacheMarkerManager{
		markers: []llm.CachedBlockMarker{{AfterMessageIndex: systemCount - 1}},
		cap:     maxCacheMarkers,
	}
}

// Markers returns the current marker list. Provider implementations
// take this as input to llm.CompletionRequest.CachedBlocks.
func (m *cacheMarkerManager) Markers() []llm.CachedBlockMarker {
	if m == nil {
		return nil
	}
	out := make([]llm.CachedBlockMarker, len(m.markers))
	copy(out, m.markers)
	return out
}

// AddMarkerAfter records a fresh marker at the given message index.
// The runner calls this after appending a turn's tool_use + tool_result
// pair to messages so the next iteration's request gets cumulative
// caching.
//
// Idempotency: a marker at the same index as the most recent existing
// marker is silently dropped (the prefix is already cached).
//
// Cap enforcement: when len(markers) > cap, drop the oldest.
func (m *cacheMarkerManager) AddMarkerAfter(messageIndex int) {
	if m == nil {
		return
	}
	if len(m.markers) > 0 && m.markers[len(m.markers)-1].AfterMessageIndex >= messageIndex {
		return
	}
	m.markers = append(m.markers, llm.CachedBlockMarker{AfterMessageIndex: messageIndex})
	if len(m.markers) > m.cap {
		// Drop oldest. Use a new slice rather than reslicing the
		// existing one to avoid a hidden reference to the dropped
		// element (Go GC keeps the original-array memory alive while
		// any reslice references it).
		out := make([]llm.CachedBlockMarker, m.cap)
		copy(out, m.markers[len(m.markers)-m.cap:])
		m.markers = out
	}
}

// Count returns the current marker count. Surfaced into RunMetrics
// indirectly via TurnTrace.Usage.CachedTokens, but ops sometimes wants
// the raw marker count for diagnostics.
func (m *cacheMarkerManager) Count() int {
	if m == nil {
		return 0
	}
	return len(m.markers)
}
