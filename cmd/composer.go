package main

import (
	"encoding/json"

	"github.com/aziz-tb/merio/internal/composer"
	composermodels "github.com/aziz-tb/merio/internal/composer/models"
	"github.com/aziz-tb/merio/internal/detection"
	"github.com/aziz-tb/merio/internal/envelope"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// composerConditionSources are the detection sources a Composer condition leaf
// (escalation rule or guidance condition) may reference. The stored verification
// level needs no dedicated source — it is already referenceable as
// ticket.verification_level (the Conversation marshals it into the ticket JSON).
var composerConditionSources = []string{
	detection.SourceTicket,
	detection.SourceCustomer,
	detection.SourceContract,
	detection.SourceTags,
}

// handleGetComposerConfig returns the single global Composer config.
func handleGetComposerConfig(r *fastglue.Request) error {
	app := r.Context.(*App)
	cfg, err := app.composer.GetConfig()
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(cfg)
}

// handleUpdateComposerConfig validates and upserts the single global Composer
// config (Pattern A: known-keys/type via strict decode → reference checks →
// persist).
func handleUpdateComposerConfig(r *fastglue.Request) error {
	app := r.Context.(*App)

	// Known-keys + type: strict decode rejects unknown fields at every level.
	cfg, err := composer.DecodeStrict(r.RequestCtx.PostBody())
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "invalid composer config: "+err.Error(), nil, envelope.InputError)
	}

	// Reference checks (gather valid sets from the managers, then validate pure).
	refs, gErr := app.composerValidationRefs(cfg)
	if gErr != nil {
		return sendErrorEnvelope(r, gErr)
	}
	if vErr := composer.ValidateConfig(cfg, refs); vErr != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, vErr.Error(), nil, envelope.InputError)
	}

	// Persist: re-marshal the validated typed config into the raw sections
	// (canonical form). Marshalling typed structs cannot fail.
	row, sErr := app.composer.Upsert(composermodels.ComposerConfig{
		Guidance:   mustJSON(cfg.Guidance),
		Automation: mustJSON(cfg.Automation),
		Escalation: mustJSON(cfg.Escalation),
	})
	if sErr != nil {
		return sendErrorEnvelope(r, sErr)
	}
	return r.SendEnvelope(row)
}

// composerValidationRefs gathers the reference sets the pure validator checks
// against: existing inbox ids, existing custom WRITE tool slugs referenced by
// the approval list, and the allowed condition sources.
func (app *App) composerValidationRefs(cfg composermodels.Config) (composer.ValidationRefs, error) {
	inboxes, err := app.inbox.GetAll()
	if err != nil {
		return composer.ValidationRefs{}, err
	}
	inboxIDs := make(map[int]bool, len(inboxes))
	for _, ib := range inboxes {
		inboxIDs[ib.ID] = true
	}

	writeSlugs := map[string]bool{}
	if slugs := cfg.Automation.ApprovalRequiredToolSlugs; len(slugs) > 0 {
		tools, tErr := app.aiTools.GetBySlugs(slugs)
		if tErr != nil {
			return composer.ValidationRefs{}, tErr
		}
		for _, t := range tools {
			if t.Category == "write" {
				writeSlugs[t.Slug] = true
			}
		}
	}

	return composer.ValidationRefs{
		WriteToolSlugs: writeSlugs,
		InboxIDs:       inboxIDs,
		AllowedSources: composerConditionSources,
	}, nil
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
