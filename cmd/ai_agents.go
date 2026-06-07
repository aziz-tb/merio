package main

import (
	"encoding/json"
	"strconv"
	"strings"

	agentmodels "github.com/aziz-tb/merio/internal/ai/agents/models"
	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/aziz-tb/merio/internal/ai/tools/builtin"
	"github.com/aziz-tb/merio/internal/envelope"
	"github.com/aziz-tb/merio/internal/stringutil"
	verificationmodels "github.com/aziz-tb/merio/internal/verification/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// handleListAIAgents lists ai agents with optional `enabled` filter and
// pagination (page, page_size).
func handleListAIAgents(r *fastglue.Request) error {
	var (
		app                  = r.Context.(*App)
		enabledQ             = r.RequestCtx.QueryArgs().Peek("enabled")
		page, pageSize       = getPagination(r)
		enabled        *bool = nil
	)
	if len(enabledQ) > 0 {
		v := strings.EqualFold(string(enabledQ), "true")
		enabled = &v
	}
	out, total, err := app.aiAgents.List(enabled, pageSize, (page-1)*pageSize)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(map[string]interface{}{
		"results":   out,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// handleGetAIAgent fetches one ai agent by ID.
func handleGetAIAgent(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		id, err = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	if err != nil || id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	out, fetchErr := app.aiAgents.Get(id)
	if fetchErr != nil {
		return sendErrorEnvelope(r, fetchErr)
	}
	return r.SendEnvelope(out)
}

// handleCreateAIAgent creates an ai agent. Runs the full validation pass
// (slug shape, prompt-required-when-enabled, kb_scope folder/article refs
// and tool refs exist, autonomy keys, tools[] shape, min_verification
// non-negative).
func handleCreateAIAgent(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		agent = agentmodels.Agent{}
	)
	if err := r.Decode(&agent, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}
	if err := validateAIAgent(app, &agent); err != nil {
		return sendErrorEnvelope(r, err)
	}
	out, err := app.aiAgents.Create(agent)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(out)
}

// handleUpdateAIAgent updates an ai agent. Same validation as Create.
func handleUpdateAIAgent(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		agent   = agentmodels.Agent{}
		id, err = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	if err != nil || id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	if err := r.Decode(&agent, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}
	// Reject slug mutations: slugs are permanent code references. Fetch
	// the existing row, compare body's slug if non-empty, force the
	// row's canonical slug onto the update payload either way.
	existing, fetchErr := app.aiAgents.Get(id)
	if fetchErr != nil {
		return sendErrorEnvelope(r, fetchErr)
	}
	if strings.TrimSpace(agent.Slug) != "" && agent.Slug != existing.Slug {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, stringutil.ErrSlugImmutableMessage, nil, envelope.InputError)
	}
	agent.Slug = existing.Slug
	if vErr := validateAIAgent(app, &agent); vErr != nil {
		return sendErrorEnvelope(r, vErr)
	}
	out, updErr := app.aiAgents.Update(id, agent)
	if updErr != nil {
		return sendErrorEnvelope(r, updErr)
	}
	return r.SendEnvelope(out)
}

// handleDeleteAIAgent hard-deletes an ai agent by ID.
func handleDeleteAIAgent(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		id, err = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	if err != nil || id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	if delErr := app.aiAgents.Delete(id); delErr != nil {
		return sendErrorEnvelope(r, delErr)
	}
	return r.SendEnvelope(true)
}

// validateAIAgent enforces the F9 validation rules locked for Chunk 3.
// Mutates the agent in place where appropriate (slug auto-generation
// when empty).
func validateAIAgent(app *App, a *agentmodels.Agent) error {
	// Name required.
	a.Name = strings.TrimSpace(a.Name)
	if a.Name == "" {
		return envelope.NewError(envelope.InputError, "name is required", nil)
	}

	// Slug: auto-generate from name if empty; otherwise validate shape.
	a.Slug = strings.TrimSpace(a.Slug)
	if a.Slug == "" {
		a.Slug = stringutil.Slugify(a.Name)
		if a.Slug == "" {
			return envelope.NewError(envelope.InputError, "could not derive slug from name; provide one explicitly", nil)
		}
	}
	if !stringutil.SlugRegex.MatchString(a.Slug) {
		return envelope.NewError(envelope.InputError, "slug must match ^[a-z0-9-]+$ with no leading/trailing/double dashes", nil)
	}

	// Prompt required when enabled.
	if a.Enabled && strings.TrimSpace(a.Prompt) == "" {
		return envelope.NewError(envelope.InputError, "prompt is required when the agent is enabled", nil)
	}

	// Confidence threshold range (DB CHECK enforces too; surface as 400 not 500).
	if a.ConfidenceThreshold < 0 || a.ConfidenceThreshold > 1 {
		return envelope.NewError(envelope.InputError, "confidence_threshold must be between 0 and 1", nil)
	}

	// Max iterations sanity (matches DB CHECK).
	if a.MaxIterations <= 0 || a.MaxIterations > 50 {
		return envelope.NewError(envelope.InputError, "max_iterations must be between 1 and 50", nil)
	}

	// Tools JSONB shape: array of {kind, id, config_override?}.
	//   kind="internal" → id is a string matching a registered builtin
	//                     capability (kb_search, get_customer_data, ...).
	//   kind="custom"   → id is an integer matching an ai_tools row.
	if len(a.Tools) > 0 && string(a.Tools) != "null" && string(a.Tools) != "[]" {
		var toolRefs []agentmodels.ToolRef
		if err := json.Unmarshal(a.Tools, &toolRefs); err != nil {
			return envelope.NewError(envelope.InputError,
				"tools must be an array of {kind, id, config_override?}", nil)
		}
		customIDs := make([]int, 0, len(toolRefs))
		for i, tr := range toolRefs {
			switch tr.Kind {
			case agentmodels.ToolRefKindCustom:
				id, ok := tr.CustomID()
				if !ok || id <= 0 {
					return envelope.NewError(envelope.InputError,
						"tools["+strconv.Itoa(i)+"].id must be a positive integer when kind=custom", nil)
				}
				customIDs = append(customIDs, id)
			case agentmodels.ToolRefKindInternal:
				cid, ok := tr.InternalID()
				if !ok || strings.TrimSpace(cid) == "" {
					return envelope.NewError(envelope.InputError,
						"tools["+strconv.Itoa(i)+"].id must be a non-empty string when kind=internal", nil)
				}
				if _, found := builtin.Default().Get(cid); !found {
					return envelope.NewError(envelope.InputError,
						"tools["+strconv.Itoa(i)+"].id=\""+cid+
							"\" is not a registered internal capability; valid: "+
							strings.Join(builtin.Default().IDs(), ", "),
						nil)
				}
			default:
				return envelope.NewError(envelope.InputError,
					"tools["+strconv.Itoa(i)+"].kind must be \"internal\" or \"custom\"", nil)
			}
		}
		// Verify every referenced custom tool exists.
		if len(customIDs) > 0 {
			found, err := app.aiTools.GetByIDs(customIDs)
			if err != nil {
				return err
			}
			seen := make(map[int]struct{}, len(found))
			for _, t := range found {
				seen[t.ID] = struct{}{}
			}
			for _, id := range customIDs {
				if _, ok := seen[id]; !ok {
					return envelope.NewError(envelope.InputError,
						"tools references nonexistent ai_tools.id="+strconv.Itoa(id), nil)
				}
			}
		}
	}

	// kb_scope: {"folder_ids":[…], "article_ids":[…]}. Each id must be a
	// positive integer and reference an existing kb_folders / kb_articles
	// row. Empty arrays are valid (agent has no KB access). The DB column
	// default sets the empty shape; an explicitly omitted field decodes
	// as nil here and we treat that identically.
	if len(a.KBScope) > 0 && string(a.KBScope) != "null" {
		var scope agentmodels.KBScope
		if err := json.Unmarshal(a.KBScope, &scope); err != nil {
			return envelope.NewError(envelope.InputError,
				"kb_scope must be an object of {\"folder_ids\":[…], \"article_ids\":[…]}", nil)
		}
		for _, fid := range scope.FolderIDs {
			if fid <= 0 {
				return envelope.NewError(envelope.InputError, "kb_scope.folder_ids entries must be positive integers", nil)
			}
			if _, err := app.knowledge.GetFolder(fid); err != nil {
				return envelope.NewError(envelope.InputError, "kb_scope.folder_ids references nonexistent kb_folders.id="+strconv.Itoa(fid), nil)
			}
		}
		for _, aid := range scope.ArticleIDs {
			if aid <= 0 {
				return envelope.NewError(envelope.InputError, "kb_scope.article_ids entries must be positive integers", nil)
			}
			if _, err := app.knowledge.GetArticle(aid); err != nil {
				return envelope.NewError(envelope.InputError, "kb_scope.article_ids references nonexistent kb_articles.id="+strconv.Itoa(aid), nil)
			}
		}
	}

	// min_verification: floor on the conversation verification level.
	if a.MinVerification < int(verificationmodels.VerificationLevelUnverified) ||
		a.MinVerification > int(verificationmodels.VerificationLevelManual) {
		return envelope.NewError(envelope.InputError, "min_verification must be between 0 and 3", nil)
	}

	// Autonomy: validate keys when non-empty (anything other than {} or null).
	if len(a.Autonomy) > 0 && string(a.Autonomy) != "{}" && string(a.Autonomy) != "null" {
		// Use a permissive struct decode, then a strict known-keys check
		// against the raw map so unknown keys are rejected.
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(a.Autonomy, &raw); err != nil {
			return envelope.NewError(envelope.InputError, "autonomy must be a JSON object", nil)
		}
		allowed := map[string]struct{}{
			"min_confidence_send":      {},
			"min_confidence_draft":     {},
			"escalate_on_disagreement": {},
		}
		for k := range raw {
			if _, ok := allowed[k]; !ok {
				return envelope.NewError(envelope.InputError, "autonomy contains unknown key: "+k, nil)
			}
		}
		// Type-check the values.
		var typed agentmodels.Autonomy
		if err := json.Unmarshal(a.Autonomy, &typed); err != nil {
			return envelope.NewError(envelope.InputError, "autonomy fields have wrong types (expected numbers/bool)", nil)
		}
	}

	// preset_overrides: must be a JSON object of purpose → preset slug.
	// Empty object {} means "use platform defaults from
	// config/ai_presets.json's purpose_mappings." Both shape (string→string,
	// non-empty values) and semantics (purpose is one of llm.KnownPurposes,
	// slug exists in the loaded preset registry) are checked here so a 400
	// fires at save time rather than the agent runner failing at run time.
	if len(a.PresetOverrides) > 0 && string(a.PresetOverrides) != "{}" && string(a.PresetOverrides) != "null" {
		var typed map[string]string
		if err := json.Unmarshal(a.PresetOverrides, &typed); err != nil {
			return envelope.NewError(envelope.InputError, "preset_overrides must be a JSON object of purpose → preset slug", nil)
		}
		for k, v := range typed {
			if strings.TrimSpace(k) == "" {
				return envelope.NewError(envelope.InputError, "preset_overrides keys must be non-empty purpose names", nil)
			}
			if strings.TrimSpace(v) == "" {
				return envelope.NewError(envelope.InputError, "preset_overrides[\""+k+"\"] must be a non-empty preset slug", nil)
			}
			if !llm.IsKnownPurpose(k) {
				known := make([]string, 0, len(llm.KnownPurposes))
				for _, p := range llm.KnownPurposes {
					known = append(known, string(p))
				}
				return envelope.NewError(envelope.InputError,
					"preset_overrides key \""+k+"\" is not a known purpose; expected one of: "+strings.Join(known, ", "),
					nil)
			}
			if app.aiPresets != nil && !app.aiPresets.Has(v) {
				return envelope.NewError(envelope.InputError,
					"preset_overrides[\""+k+"\"]=\""+v+"\" references an unknown preset slug; valid slugs: "+strings.Join(app.aiPresets.Slugs(), ", "),
					nil)
			}
		}
	}

	return nil
}
