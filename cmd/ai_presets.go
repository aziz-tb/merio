package main

import (
	"github.com/aziz-tb/merio/internal/envelope"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// aiPresetView is the trimmed shape returned to the admin UI's preset picker.
// API keys are resolved via secret.Resolver (env vars or the secrets table);
// the full Preset struct also carries tuning knobs the agent edit form has no
// business showing. The form needs enough metadata to filter by purpose,
// provider, and enabled state without exposing raw tuning details.
type aiPresetView struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Model       string   `json:"model"`
	Provider    string   `json:"provider"`
	Enabled     bool     `json:"enabled"`
	Purposes    []string `json:"purposes"`
	Visibility  []string `json:"visibility"`
}

// handleListAIPresets returns presets from the in-memory registry. Read-only,
// gated on ai_agents:manage (same gate as the internal-capabilities list — see
// cmd/ai_internal_capabilities.go for the consolidation note: keep separate
// handler files until this pattern hits 5+ similar GETs, then merge into
// cmd/ai_registries.go).
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
		purposes := app.aiPresets.PurposesForPreset(p.Slug)
		purposeNames := make([]string, 0, len(purposes))
		for _, purpose := range purposes {
			purposeNames = append(purposeNames, string(purpose))
		}
		visibility := p.Visibility
		if visibility == nil {
			visibility = []string{}
		}
		out = append(out, aiPresetView{
			Slug:        p.Slug,
			Name:        p.Name,
			Description: p.Description,
			Model:       p.Model,
			Provider:    p.Provider,
			Enabled:     p.Enabled,
			Purposes:    purposeNames,
			Visibility:  visibility,
		})
	}
	return r.SendEnvelope(out)
}
