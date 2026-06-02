package main

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"

	amodels "github.com/aziz-tb/merio/internal/auth/models"
	"github.com/aziz-tb/merio/internal/customer"
	"github.com/aziz-tb/merio/internal/envelope"
	"github.com/aziz-tb/merio/internal/stringutil"
	"github.com/aziz-tb/merio/internal/user/models"
	"github.com/valyala/fasthttp"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/fastglue"
)

type createContactNoteReq struct {
	Note string `json:"note"`
}

type blockContactReq struct {
	Enabled bool `json:"enabled"`
}

type updateLinkedCustomerReq struct {
	CustomerID null.String `json:"customer_id"`
}

type updateContactFieldsReq struct {
	FirstName              *string `json:"first_name"`
	LastName               *string `json:"last_name"`
	PhoneNumber            *string `json:"phone_number"`
	PhoneNumberCountryCode *string `json:"phone_number_country_code"`
	Country                *string `json:"country"`
}

// handleGetContacts returns a list of contacts from the database.
func handleGetContacts(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		order   = string(r.RequestCtx.QueryArgs().Peek("order"))
		orderBy = string(r.RequestCtx.QueryArgs().Peek("order_by"))
		filters = string(r.RequestCtx.QueryArgs().Peek("filters"))
		total   = 0
	)
	page, pageSize := getPagination(r)
	contacts, err := app.user.GetContacts(page, pageSize, order, orderBy, filters)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if len(contacts) > 0 {
		total = contacts[0].Total
	}
	return r.SendEnvelope(envelope.PageResults{
		Results:    contacts,
		Total:      total,
		PerPage:    pageSize,
		TotalPages: (total + pageSize - 1) / pageSize,
		Page:       page,
	})
}

// handleGetTags returns a contact from the database.
func handleGetContact(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	if id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}
	c, err := app.user.GetContactOrVisitor(id, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(c)
}

// handleUpdateContact updates a contact in the database.
func handleUpdateContact(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	if id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	contact, err := app.user.GetContactOrVisitor(id, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	if strings.Contains(strings.ToLower(string(r.RequestCtx.Request.Header.ContentType())), "application/json") {
		var req updateContactFieldsReq
		if err := r.Decode(&req, "json"); err != nil {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
		}

		contactToUpdate := models.User{
			FirstName:              contact.FirstName,
			LastName:               contact.LastName,
			Email:                  contact.Email,
			AvatarURL:              contact.AvatarURL,
			PhoneNumber:            contact.PhoneNumber,
			PhoneNumberCountryCode: contact.PhoneNumberCountryCode,
			Country:                contact.Country,
		}
		if req.FirstName != nil {
			contactToUpdate.FirstName = strings.TrimSpace(*req.FirstName)
		}
		if req.LastName != nil {
			contactToUpdate.LastName = strings.TrimSpace(*req.LastName)
		}
		if req.PhoneNumber != nil {
			phoneNumber := strings.TrimSpace(*req.PhoneNumber)
			contactToUpdate.PhoneNumber = null.NewString(phoneNumber, phoneNumber != "")
		}
		if req.PhoneNumberCountryCode != nil {
			countryCode := strings.TrimSpace(*req.PhoneNumberCountryCode)
			contactToUpdate.PhoneNumberCountryCode = null.NewString(countryCode, countryCode != "")
		}
		if req.Country != nil {
			country := strings.TrimSpace(*req.Country)
			contactToUpdate.Country = null.NewString(country, country != "")
		}

		if err := app.user.UpdateContact(id, contactToUpdate); err != nil {
			return sendErrorEnvelope(r, err)
		}

		contact, err = app.user.GetContactOrVisitor(id, "")
		if err != nil {
			return sendErrorEnvelope(r, err)
		}
		return r.SendEnvelope(contact)
	}

	form, err := r.RequestCtx.MultipartForm()
	if err != nil {
		app.lo.Error("error parsing form data", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.T("errors.parsingRequest"), nil, envelope.GeneralError)
	}

	// Parse form data
	firstName := ""
	if v, ok := form.Value["first_name"]; ok && len(v) > 0 {
		firstName = string(v[0])
	}
	lastName := ""
	if v, ok := form.Value["last_name"]; ok && len(v) > 0 {
		lastName = string(v[0])
	}
	email := ""
	if v, ok := form.Value["email"]; ok && len(v) > 0 {
		email = strings.TrimSpace(string(v[0]))
	}
	phoneNumber := ""
	if v, ok := form.Value["phone_number"]; ok && len(v) > 0 {
		phoneNumber = string(v[0])
	}
	phoneNumberCountryCode := ""
	if v, ok := form.Value["phone_number_country_code"]; ok && len(v) > 0 {
		phoneNumberCountryCode = string(v[0])
	}
	country := ""
	if v, ok := form.Value["country"]; ok && len(v) > 0 {
		country = string(v[0])
	}
	avatarURL := ""
	if v, ok := form.Value["avatar_url"]; ok && len(v) > 0 {
		avatarURL = string(v[0])
	}

	// Set nulls to empty strings.
	if avatarURL == "null" {
		avatarURL = ""
	}
	if phoneNumberCountryCode == "null" {
		phoneNumberCountryCode = ""
	}
	if phoneNumber == "null" {
		phoneNumber = ""
	}
	if country == "null" {
		country = ""
	}

	// Validate mandatory fields.
	if email == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.Ts("globals.messages.empty", "name", "email"), nil, envelope.InputError)
	}
	if !stringutil.ValidEmail(email) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("validation.invalidEmail"), nil, envelope.InputError)
	}
	if firstName == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.Ts("globals.messages.empty", "name", "first_name"), nil, envelope.InputError)
	}

	contactToUpdate := models.User{
		FirstName:              firstName,
		LastName:               lastName,
		Email:                  null.StringFrom(email),
		AvatarURL:              null.NewString(avatarURL, avatarURL != ""),
		PhoneNumber:            null.NewString(phoneNumber, phoneNumber != ""),
		PhoneNumberCountryCode: null.NewString(phoneNumberCountryCode, phoneNumberCountryCode != ""),
		Country:                null.NewString(country, country != ""),
	}

	if err := app.user.UpdateContact(id, contactToUpdate); err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Delete avatar?
	if avatarURL == "" && contact.AvatarURL.Valid {
		fileName := filepath.Base(contact.AvatarURL.String)
		app.media.Delete(fileName)
		contact.AvatarURL.Valid = false
		contact.AvatarURL.String = ""
	}

	// Upload avatar?
	files, ok := form.File["files"]
	if ok && len(files) > 0 {
		if err := uploadUserAvatar(r, contact, files); err != nil {
			return sendErrorEnvelope(r, err)
		}
	}

	// Refetch contact and return it
	contact, err = app.user.GetContactOrVisitor(id, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(contact)
}

func handleUpdateContactLinkedCustomer(r *fastglue.Request) error {
	app := r.Context.(*App)
	id, _ := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	var req updateLinkedCustomerReq
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}

	if _, err := app.user.GetContactOrVisitor(id, ""); err != nil {
		return sendErrorEnvelope(r, err)
	}

	if req.CustomerID.Valid {
		req.CustomerID.String = strings.TrimSpace(req.CustomerID.String)
		if req.CustomerID.String == "" {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "customer_id must not be empty", nil, envelope.InputError)
		}
		if _, err := app.customer.GetByCustomerID(req.CustomerID.String); err != nil {
			if errors.Is(err, customer.ErrCustomerNotFound) {
				return r.SendErrorEnvelope(fasthttp.StatusUnprocessableEntity, "customer_id does not exist", nil, envelope.DataError)
			}
			return sendCustomerContractError(r, app, err)
		}
	}

	if err := app.user.UpdateContactLinkedCustomer(id, req.CustomerID); err != nil {
		return sendErrorEnvelope(r, err)
	}

	contact, err := app.user.GetContactOrVisitor(id, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(contact)
}

// handleGetContactConversations returns paginated conversation history for a contact.
func handleGetContactConversations(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	if id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	page, pageSize := getPagination(r)
	if len(r.RequestCtx.QueryArgs().Peek("page_size")) == 0 {
		pageSize = 20
	}
	excludeUUID := string(r.RequestCtx.QueryArgs().Peek("exclude_uuid"))

	items, total, err := app.conversation.GetConversationsByContact(r.RequestCtx, id, page, pageSize, excludeUUID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	return r.SendEnvelope(map[string]any{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// handleGetCustomerLinkedContacts returns contacts that share a linked customer.
func handleGetCustomerLinkedContacts(r *fastglue.Request) error {
	app := r.Context.(*App)
	customerID := strings.TrimSpace(r.RequestCtx.UserValue("customer_id").(string))
	if customerID == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	excludeContactID, _ := strconv.Atoi(string(r.RequestCtx.QueryArgs().Peek("exclude_contact_id")))
	contacts, err := app.user.GetContactsLinkedToCustomer(customerID, excludeContactID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(contacts)
}

// handleGetContactNotes returns all notes for a contact.
func handleGetContactNotes(r *fastglue.Request) error {
	var (
		app          = r.Context.(*App)
		contactID, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	if contactID <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}
	notes, err := app.user.GetNotes(contactID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(notes)
}

// handleCreateContactNote creates a note for a contact.
func handleCreateContactNote(r *fastglue.Request) error {
	var (
		app          = r.Context.(*App)
		contactID, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
		auser        = r.RequestCtx.UserValue("user").(amodels.User)
		req          = createContactNoteReq{}
	)
	if err := r.Decode(&req, "json"); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.T("errors.parsingRequest"), nil))
	}
	if len(req.Note) == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.Ts("globals.messages.empty", "name", "note"), nil, envelope.InputError)
	}
	n, err := app.user.CreateNote(contactID, auser.ID, req.Note)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	n, err = app.user.GetNote(n.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(n)
}

// handleDeleteContactNote deletes a note for a contact.
func handleDeleteContactNote(r *fastglue.Request) error {
	var (
		app          = r.Context.(*App)
		contactID, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
		noteID, _    = strconv.Atoi(r.RequestCtx.UserValue("note_id").(string))
		auser        = r.RequestCtx.UserValue("user").(amodels.User)
	)
	if contactID <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}
	if noteID <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	agent, err := app.user.GetAgent(auser.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Allow deletion of only own notes and not those created by others, but also allow `Admin` to delete any note.
	if !agent.HasAdminRole() {
		note, err := app.user.GetNote(noteID)
		if err != nil {
			return sendErrorEnvelope(r, err)
		}
		if note.UserID != auser.ID {
			return r.SendErrorEnvelope(fasthttp.StatusForbidden, app.i18n.T("errors.canOnlyDeleteOwnNote"), nil, envelope.InputError)
		}
	}

	app.lo.Info("deleting contact note", "note_id", noteID, "contact_id", contactID, "actor_id", auser.ID)

	if err := app.user.DeleteNote(noteID, contactID); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}

// handleBlockContact blocks a contact.
func handleBlockContact(r *fastglue.Request) error {
	var (
		app          = r.Context.(*App)
		contactID, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
		auser        = r.RequestCtx.UserValue("user").(amodels.User)
		req          = blockContactReq{}
	)

	if contactID <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	if err := r.Decode(&req, "json"); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.T("errors.parsingRequest"), nil))
	}

	app.lo.Info("setting contact block status", "contact_id", contactID, "enabled", req.Enabled, "actor_id", auser.ID)

	contact, err := app.user.GetContactOrVisitor(contactID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	if err := app.user.ToggleEnabled(contactID, contact.Type, req.Enabled); err != nil {
		return sendErrorEnvelope(r, err)
	}

	contact, err = app.user.GetContactOrVisitor(contactID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(contact)
}
