package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/aziz-tb/merio/internal/envelope"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

const customerIdentificationSettingsPrefix = "customer_identification."

var customerIdentificationSettingKinds = map[string]string{
	"pattern_detection_enabled": "bool",
	"contract_number_regex":     "regex",
	"customer_number_regex":     "regex",
	"llm_extraction_enabled":    "bool",
	"erp_cache_ttl_minutes":     "int_non_negative",
	"excluded_email_domains":    "array",
	"search_parameters":         "object",
	"verification_rules_async":  "object",
}

func handleGetCustomerIdentificationSettings(r *fastglue.Request) error {
	app := r.Context.(*App)
	raw, err := app.setting.GetByPrefix(customerIdentificationSettingsPrefix)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	var full map[string]json.RawMessage
	if err := json.Unmarshal(raw, &full); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}
	out := make(map[string]json.RawMessage, len(customerIdentificationSettingKinds))
	for key := range customerIdentificationSettingKinds {
		fullKey := customerIdentificationSettingsPrefix + key
		if value, ok := full[fullKey]; ok {
			out[key] = value
		}
	}
	return r.SendEnvelope(out)
}

func handleUpdateCustomerIdentificationSettings(r *fastglue.Request) error {
	app := r.Context.(*App)
	var req map[string]json.RawMessage
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}
	if len(req) == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "at least one setting is required", nil, envelope.InputError)
	}

	update := make(map[string]json.RawMessage, len(req))
	for key, raw := range req {
		shortKey := strings.TrimPrefix(key, customerIdentificationSettingsPrefix)
		kind, ok := customerIdentificationSettingKinds[shortKey]
		if !ok {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, fmt.Sprintf("unknown setting %q", key), nil, envelope.InputError)
		}
		if err := validateCustomerIdentificationSetting(shortKey, kind, raw); err != nil {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, envelope.InputError)
		}
		update[customerIdentificationSettingsPrefix+shortKey] = raw
	}

	if err := app.setting.Update(update); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}

func validateCustomerIdentificationSetting(key, kind string, raw json.RawMessage) error {
	if len(raw) == 0 || string(raw) == "null" {
		return fmt.Errorf("%s must not be null", key)
	}
	switch kind {
	case "bool":
		var v bool
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("%s must be a boolean", key)
		}
	case "regex":
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("%s must be a string", key)
		}
		if strings.TrimSpace(v) != "" {
			if _, err := regexp.Compile(v); err != nil {
				return fmt.Errorf("%s is not a valid regex", key)
			}
		}
	case "int_positive":
		var v int
		if err := json.Unmarshal(raw, &v); err != nil || v <= 0 {
			return fmt.Errorf("%s must be a positive integer", key)
		}
	case "int_non_negative":
		var v int
		if err := json.Unmarshal(raw, &v); err != nil || v < 0 {
			return fmt.Errorf("%s must be a non-negative integer", key)
		}
	case "array":
		var v []any
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("%s must be a JSON array", key)
		}
	case "object":
		var v map[string]any
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("%s must be a JSON object", key)
		}
	default:
		return fmt.Errorf("unknown setting kind for %s", key)
	}
	return nil
}
