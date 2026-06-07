package main

import (
	"errors"
	"strings"

	amodels "github.com/aziz-tb/merio/internal/auth/models"
	"github.com/aziz-tb/merio/internal/envelope"
	"github.com/aziz-tb/merio/internal/secret"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

type secretCreateRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type secretUpdateRequest struct {
	Value string `json:"value"`
}

func handleListSecrets(r *fastglue.Request) error {
	app := r.Context.(*App)
	items, err := app.secret.List(r.RequestCtx)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}
	return r.SendEnvelope(items)
}

func handleCreateSecret(r *fastglue.Request) error {
	app := r.Context.(*App)
	var req secretCreateRequest
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), err.Error(), envelope.InputError)
	}
	auser := r.RequestCtx.UserValue("user").(amodels.User)
	if err := app.secret.Create(r.RequestCtx, strings.TrimSpace(req.Name), req.Value, auser.ID); err != nil {
		return sendSecretError(r, app, err)
	}
	return r.SendEnvelope(true)
}

func handleUpdateSecret(r *fastglue.Request) error {
	app := r.Context.(*App)
	name := strings.TrimSpace(r.RequestCtx.UserValue("name").(string))
	var req secretUpdateRequest
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), err.Error(), envelope.InputError)
	}
	if err := app.secret.Update(r.RequestCtx, name, req.Value); err != nil {
		return sendSecretError(r, app, err)
	}
	return r.SendEnvelope(true)
}

func handleDeleteSecret(r *fastglue.Request) error {
	app := r.Context.(*App)
	name := strings.TrimSpace(r.RequestCtx.UserValue("name").(string))
	if err := app.secret.Delete(r.RequestCtx, name); err != nil {
		return sendSecretError(r, app, err)
	}
	return r.SendEnvelope(true)
}

func sendSecretError(r *fastglue.Request, app *App, err error) error {
	status, msg, typ := secretErrorResponse(app, err)
	return r.SendErrorEnvelope(status, msg, nil, fastglue.ErrorType(typ))
}

func secretErrorResponse(app *App, err error) (int, string, string) {
	switch {
	case errors.Is(err, secret.ErrInvalidName):
		return fasthttp.StatusBadRequest, "secret name must match ^[A-Za-z][A-Za-z0-9_]{0,63}$", envelope.InputError
	case errors.Is(err, secret.ErrValueTooLarge):
		return fasthttp.StatusBadRequest, "secret value exceeds 5 KB", envelope.InputError
	case errors.Is(err, secret.ErrSecretExists):
		return fasthttp.StatusConflict, "secret already exists", envelope.InputError
	case errors.Is(err, secret.ErrSecretNotFound):
		return fasthttp.StatusNotFound, "secret not found", envelope.NotFoundError
	default:
		if app != nil && app.i18n != nil {
			return fasthttp.StatusInternalServerError, app.i18n.T("globals.messages.somethingWentWrong"), envelope.GeneralError
		}
		return fasthttp.StatusInternalServerError, "something went wrong", envelope.GeneralError
	}
}
