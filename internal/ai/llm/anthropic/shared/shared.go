// Package shared holds Anthropic Messages API policy helpers shared by the
// direct Anthropic SDK provider and Azure Foundry's direct HTTP Anthropic path.
package shared

import (
	"encoding/json"
	"fmt"

	"github.com/aziz-tb/merio/internal/ai/llm"
)

const (
	// MaxCacheMarkers is the per-request cache_control breakpoint cap
	// enforced by Anthropic-compatible Messages APIs.
	MaxCacheMarkers = 4

	// ThinkingTopPMin and ThinkingTopPMax bound the only top_p range
	// Anthropic accepts when extended thinking is enabled.
	ThinkingTopPMin = 0.95
	ThinkingTopPMax = 1.0

	// ThinkingTypeAdaptive is the thinking mode both direct Anthropic and
	// Foundry Anthropic use for models marked SupportsThinking.
	ThinkingTypeAdaptive = "adaptive"

	CacheSliceSystem = "system"
	CacheSliceTurn   = "turn"
)

// CacheMarkerTarget is where a request cache marker should land after system
// messages are hoisted away from the user/assistant turn list.
type CacheMarkerTarget struct {
	Slice string
	Index int
}

// CacheMarkerTargets maps request-level CachedBlockMarker values to provider
// request locations. It dedupes markers and keeps the most recent ones when the
// request exceeds MaxCacheMarkers.
func CacheMarkerTargets(req llm.CompletionRequest, cachingEnabled bool) []CacheMarkerTarget {
	if !cachingEnabled || len(req.CachedBlocks) == 0 {
		return nil
	}

	indexMap := make(map[int]CacheMarkerTarget, len(req.Messages))
	sysCursor, turnCursor := 0, 0
	for i, m := range req.Messages {
		if m.Role == llm.RoleSystem {
			indexMap[i] = CacheMarkerTarget{Slice: CacheSliceSystem, Index: sysCursor + len(m.Content) - 1}
			sysCursor += len(m.Content)
			continue
		}
		indexMap[i] = CacheMarkerTarget{Slice: CacheSliceTurn, Index: turnCursor}
		turnCursor++
	}

	seen := make(map[int]struct{}, len(req.CachedBlocks))
	uniq := make([]int, 0, len(req.CachedBlocks))
	for _, marker := range req.CachedBlocks {
		if _, ok := indexMap[marker.AfterMessageIndex]; !ok {
			continue
		}
		if _, dup := seen[marker.AfterMessageIndex]; dup {
			continue
		}
		seen[marker.AfterMessageIndex] = struct{}{}
		uniq = append(uniq, marker.AfterMessageIndex)
	}
	if len(uniq) > MaxCacheMarkers {
		uniq = uniq[len(uniq)-MaxCacheMarkers:]
	}

	out := make([]CacheMarkerTarget, 0, len(uniq))
	for _, reqIdx := range uniq {
		out = append(out, indexMap[reqIdx])
	}
	return out
}

// TopPAllowedForThinking reports whether a top_p value can be sent alongside
// extended thinking.
func TopPAllowedForThinking(v float64) bool {
	return v >= ThinkingTopPMin && v <= ThinkingTopPMax
}

// ResponseBlock is the JSON shape of Anthropic Messages API content blocks.
// Foundry returns this directly; the SDK provider adapts SDK unions into this
// shape before using ResponseBlocksToContentBlocks.
type ResponseBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Signature string          `json:"signature,omitempty"`
	Data      string          `json:"data,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
}

// ResponseBlocksToContentBlocks translates Anthropic response content into the
// platform content-block shape. Unknown blocks are surfaced as placeholder text
// instead of being silently dropped.
func ResponseBlocksToContentBlocks(content []ResponseBlock, unknownPrefix string) []llm.ContentBlock {
	if unknownPrefix == "" {
		unknownPrefix = "anthropic"
	}
	out := make([]llm.ContentBlock, 0, len(content))
	for _, c := range content {
		switch c.Type {
		case "text":
			out = append(out, llm.ContentBlock{Type: llm.ContentBlockTypeText, Text: c.Text})
		case "thinking":
			out = append(out, llm.ContentBlock{
				Type:      llm.ContentBlockTypeThinking,
				Text:      c.Thinking,
				Signature: c.Signature,
			})
		case "redacted_thinking":
			out = append(out, llm.ContentBlock{
				Type: llm.ContentBlockTypeRedactedThinking,
				Data: c.Data,
			})
		case "tool_use":
			input := map[string]any{}
			if len(c.Input) > 0 {
				_ = json.Unmarshal(c.Input, &input)
			}
			out = append(out, llm.ContentBlock{
				Type:      llm.ContentBlockTypeToolUse,
				ToolUseID: c.ID,
				ToolName:  c.Name,
				ToolInput: input,
			})
		default:
			out = append(out, llm.ContentBlock{
				Type: llm.ContentBlockTypeText,
				Text: fmt.Sprintf("[%s: %s block elided]", unknownPrefix, c.Type),
			})
		}
	}
	return out
}
