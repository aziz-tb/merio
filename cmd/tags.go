package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	amodels "github.com/abhinavxd/libredesk/internal/auth/models"
	authzmodels "github.com/abhinavxd/libredesk/internal/authz/models"
	"github.com/abhinavxd/libredesk/internal/envelope"
	tmodels "github.com/abhinavxd/libredesk/internal/tag/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// handleGetTags returns tags visible to the current user.
func handleGetTags(r *fastglue.Request) error {
	app := r.Context.(*App)
	auser := r.RequestCtx.UserValue("user").(amodels.User)
	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	tags, err := app.tag.GetAll(slices.Contains(user.Permissions, authzmodels.PermTagsViewPrivate))
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(tags)
}

func handleGetTag(r *fastglue.Request) error {
	app := r.Context.(*App)
	id, err := parseTagID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "invalid tag id", nil, envelope.InputError)
	}
	tag, err := app.tag.Get(id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(tag)
}

// handleCreateTag creates a new tag in the database.
func handleCreateTag(r *fastglue.Request) error {
	app := r.Context.(*App)
	tag := tmodels.Tag{}
	if err := r.Decode(&tag, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), err.Error(), envelope.InputError)
	}
	if tag.Name == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.Ts("globals.messages.empty", "name", "`name`"), nil, envelope.InputError)
	}

	auser := r.RequestCtx.UserValue("user").(amodels.User)
	createdTag, err := app.tag.Create(tag, auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(createdTag)
}

// handleDeleteTag deletes a tag from the database.
func handleDeleteTag(r *fastglue.Request) error {
	app := r.Context.(*App)
	id, err := parseTagID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	if err = app.tag.Delete(id); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}

// handleUpdateTag updates an existing tag in the database.
func handleUpdateTag(r *fastglue.Request) error {
	app := r.Context.(*App)
	id, err := parseTagID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	tag := tmodels.Tag{}
	if err := r.Decode(&tag, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), err.Error(), envelope.InputError)
	}
	if tag.Name == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.Ts("globals.messages.empty", "name", "`name`"), nil, envelope.InputError)
	}

	auser := r.RequestCtx.UserValue("user").(amodels.User)
	updatedTag, err := app.tag.Update(id, tag, auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(updatedTag)
}

// handleTestTagDetection runs a tag's detection against a real conversation.
func handleTestTagDetection(r *fastglue.Request) error {
	app := r.Context.(*App)
	id, err := parseTagID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "invalid tag id", nil, envelope.InputError)
	}
	if app.detectionMgr == nil {
		return r.SendErrorEnvelope(fasthttp.StatusServiceUnavailable, "detection manager unavailable", nil, envelope.GeneralError)
	}

	var body struct {
		ConversationUUID string `json:"conversation_uuid"`
	}
	if err := json.Unmarshal(r.RequestCtx.PostBody(), &body); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}
	if body.ConversationUUID == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "conversation identifier required", nil, envelope.InputError)
	}

	tag, err := app.tag.Get(id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	auser := r.RequestCtx.UserValue("user").(amodels.User)
	user, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	conv, err := enforceConversationIdentifierAccess(app, body.ConversationUUID, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	result, err := app.detectionMgr.Evaluate(r.RequestCtx, tag.Detection, buildDetectionEvaluationContext(r.RequestCtx, app, *conv, conv.TagSlugs(), ""))
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, err.Error(), nil, envelope.GeneralError)
	}
	return r.SendEnvelope(result)
}

func parseTagID(r *fastglue.Request) (int, error) {
	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if err != nil || id <= 0 {
		if err == nil {
			err = fmt.Errorf("invalid tag id")
		}
		return 0, err
	}
	return id, nil
}
