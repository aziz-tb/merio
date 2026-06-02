package main

import (
	"github.com/aziz-tb/merio/internal/envelope"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// aiPresetView is the trimmed shape returned to the admin UI's preset
// picker. The full Preset struct carries provider-key material and
// fine-grained knobs (top_p, stop_sequences, caching flags) the agent
// edit form has no business showing — those live in
// config/ai_presets.json, edited out-of-band. The form picks a slug;
// nothing else.
type aiPresetView struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Model       string `json:"model"`
}

// handleListAIPresets returns the enabled presets from the in-memory
// registry. Read-only, gated on ai_agents:manage (same gate as the
// internal-capabilities list — see cmd/ai_internal_capabilities.go for
// the consolidation note: keep separate handler files until this
// pattern hits 5+ similar GETs, then merge into cmd/ai_registries.go).
//
// The registry is loaded once at boot from config/ai_presets.json;
// edits require a restart, so the response is stable for the lifetime
// of the process — no caching headers needed beyond fastglue's default.
//
// 503 fires when the registry isn't wired (no Anthropic key configured,
// no presets file). The agent runner returns 503 in the same case, so
// this is consistent with the rest of the AI surface.
func handleListAIPresets(r *fastglue.Request) error {
	app := r.Context.(*App)
	if app.aiPresets == nil {
		return r.SendErrorEnvelope(fasthttp.StatusServiceUnavailable,
			"AI preset registry is not configured", nil, envelope.GeneralError)
	}
	presets := app.aiPresets.List()
	out := make([]aiPresetView, 0, len(presets))
	for _, p := range presets {
		if !p.Enabled {
			continue
		}
		out = append(out, aiPresetView{
			Slug:        p.Slug,
			Name:        p.Name,
			Description: p.Description,
			Model:       p.Model,
		})
	}
	return r.SendEnvelope(out)
}
