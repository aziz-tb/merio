package main

import (
	"github.com/aziz-tb/merio/internal/envelope"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// aiPresetView is the trimmed Preset shape for the admin preset picker: enough
// to filter by purpose/provider/enabled, without the tuning knobs or resolved
// API keys (resolved elsewhere via secret.Resolver).
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

// handleListAIPresets lists presets from the in-memory registry (read-only, gated on
// ai_agents:manage). The registry loads once at boot, so edits need a restart; returns 503 when
// it isn't wired (no provider key or presets file).
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
