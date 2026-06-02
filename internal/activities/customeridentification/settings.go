package customeridentification

import (
	"context"
	"encoding/json"

	"github.com/aziz-tb/merio/internal/setting"
	cimodels "github.com/aziz-tb/merio/internal/workflows/customeridentification"
)

func (a *Activities) LoadSettingsActivity(ctx context.Context, _ cimodels.LoadSettingsInput) (cimodels.Settings, error) {
	_ = ctx
	settings := defaultSettings()
	raw, err := a.Settings.GetByPrefix("customer_identification.")
	if err != nil {
		return settings, err
	}
	if len(raw) == 0 {
		return settings, nil
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return settings, err
	}
	unmarshal(values, "customer_identification.pattern_detection_enabled", &settings.PatternDetectionEnabled)
	unmarshal(values, "customer_identification.llm_extraction_enabled", &settings.LLMExtractionEnabled)
	unmarshal(values, "customer_identification.contract_number_regex", &settings.ContractNumberRegex)
	unmarshal(values, "customer_identification.customer_number_regex", &settings.CustomerNumberRegex)
	unmarshal(values, "customer_identification.excluded_email_domains", &settings.ExcludedEmailDomains)
	unmarshal(values, "customer_identification.search_parameters", &settings.SearchParameters)
	unmarshal(values, "customer_identification.verification_rules_async", &settings.VerificationRulesAsync)
	if rawTTL, ok := values["customer_identification.erp_cache_ttl_minutes"]; ok {
		settings.ERPCacheTTLMinutes = setting.IntJSON(rawTTL, settings.ERPCacheTTLMinutes)
	}
	return settings, nil
}

func defaultSettings() cimodels.Settings {
	return cimodels.Settings{
		PatternDetectionEnabled: true,
		LLMExtractionEnabled:    true,
		ExcludedEmailDomains:    []string{},
		SearchParameters:        map[string]cimodels.SearchParameter{},
		ERPCacheTTLMinutes:      60,
	}
}

func unmarshal(values map[string]json.RawMessage, key string, dest any) {
	raw, ok := values[key]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return
	}
	_ = json.Unmarshal(raw, dest)
}
