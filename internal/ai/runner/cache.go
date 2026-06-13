package runner

import "github.com/aziz-tb/merio/internal/ai/llm"

// maxCacheMarkers is Anthropic's per-request cap on cache_control
// breakpoints. The manager keeps its own count so logs reflect the
// pre-truncation intent.
const maxCacheMarkers = 4

// cacheMarkerManager tracks cumulative cache-marker positions across a
// run. The system prompt is always cached (marker 0); a fresh marker is
// added after each completed tool round so later turns hit growing
// cached spans. At the cap, the OLDEST marker is dropped — newer markers
// cover larger, higher-value spans (system + N tool rounds).
type cacheMarkerManager struct {
	markers []llm.CachedBlockMarker
	cap     int
}

// newCacheMarkerManager seeds one marker on the system-prompt boundary,
// at index (systemCount - 1). With no system messages, no initial marker.
func newCacheMarkerManager(systemCount int) *cacheMarkerManager {
	if systemCount <= 0 {
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

// AddMarkerAfter records a fresh marker at messageIndex (called after a
// turn's tool_use + tool_result pair is appended). A marker at or before
// the most recent one is dropped; at the cap, the oldest is dropped.
func (m *cacheMarkerManager) AddMarkerAfter(messageIndex int) {
	if m == nil {
		return
	}
	if len(m.markers) > 0 && m.markers[len(m.markers)-1].AfterMessageIndex >= messageIndex {
		return
	}
	m.markers = append(m.markers, llm.CachedBlockMarker{AfterMessageIndex: messageIndex})
	if len(m.markers) > m.cap {
		// Drop oldest into a fresh slice; reslicing would keep the
		// dropped element's backing array alive via GC.
		out := make([]llm.CachedBlockMarker, m.cap)
		copy(out, m.markers[len(m.markers)-m.cap:])
		m.markers = out
	}
}

// Count returns the current marker count, used for diagnostics.
func (m *cacheMarkerManager) Count() int {
	if m == nil {
		return 0
	}
	return len(m.markers)
}
