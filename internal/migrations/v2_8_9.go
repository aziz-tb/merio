package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_9 seeds the 13 customer_identification.* configuration rows in the
// settings table (design Section 3.7). Values are copied verbatim from
// the design:
//
//   - 5 scalar values (pattern_detection_enabled, contract_number_regex,
//     customer_number_regex, llm_extraction_enabled, erp_cache_ttl_minutes)
//   - excluded_email_domains: empty array (operator populates on install)
//   - search_parameters: the Enneo-derived default object keyed by
//     parameter name
//   - custom_criteria: empty array
//   - legitimation_rules_async: email channel seeded with the two default
//     groups (only async channel in v1)
//   - 4 executor rows, each with empty code + sane defaults (language
//     python311, 30s timeout, enabled, empty test_parameters)
//
// Operators populate regexes, excluded domains, and executor code via the
// admin UI after install. Idempotent via ON CONFLICT (key) DO NOTHING.
// Forward-only. Rollback: DELETE FROM settings WHERE key LIKE
// 'customer_identification.%';
func V2_8_9(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	if _, err := db.Exec(`
INSERT INTO settings ("key", value)
VALUES
    ('customer_identification.pattern_detection_enabled', 'true'::jsonb),
    ('customer_identification.contract_number_regex', '""'::jsonb),
    ('customer_identification.customer_number_regex', '""'::jsonb),
    ('customer_identification.llm_extraction_enabled', 'true'::jsonb),
    ('customer_identification.erp_cache_ttl_minutes', '60'::jsonb),
    ('customer_identification.excluded_email_domains', '[]'::jsonb),
    ('customer_identification.search_parameters', '{
        "contractId":   { "type": "string", "description": "Contract number — typically alphanumeric, may be prefixed with VN- or C-." },
        "customerId":   { "type": "string", "description": "Customer number — typically alphanumeric, may be prefixed with K- or C-." },
        "firstname":    { "type": "string", "description": "First name of the customer if mentioned." },
        "lastname":     { "type": "string", "description": "Last name of the customer if mentioned." },
        "company":      { "type": "string", "description": "Company name if the customer is a business." },
        "email":        { "type": "string", "description": "Email address mentioned in the message body (not necessarily the sender)." },
        "phone":        { "type": "string", "description": "Phone number mentioned in the message." },
        "meterNumber":  { "type": "string", "description": "Meter number — typically alphanumeric." },
        "address":      { "type": "string", "description": "Street address mentioned in the message." },
        "postalCode":   { "type": "string", "description": "Postal/ZIP code mentioned in the message." },
        "city":         { "type": "string", "description": "City mentioned in the message." }
    }'::jsonb),
    ('customer_identification.custom_criteria', '[]'::jsonb),
    ('customer_identification.legitimation_rules_async', '{
        "email": {
            "groups": [
                {
                    "label": "ID mentioned",
                    "min_count": 1,
                    "criteria": [
                        {"type": "standard", "name": "contract_id_mentioned"},
                        {"type": "standard", "name": "customer_id_mentioned"}
                    ]
                },
                {
                    "label": "Sender email matches",
                    "min_count": 1,
                    "criteria": [
                        {"type": "standard", "name": "sender_email_matches_erp"}
                    ]
                }
            ]
        }
    }'::jsonb),
    ('customer_identification.executor.search_customer_by_fields', '{
        "code": "",
        "language": "python311",
        "packages": "",
        "timeout_ms": 30000,
        "enabled": true,
        "test_parameters": []
    }'::jsonb),
    ('customer_identification.executor.search_contract_by_id', '{
        "code": "",
        "language": "python311",
        "packages": "",
        "timeout_ms": 30000,
        "enabled": true,
        "test_parameters": []
    }'::jsonb),
    ('customer_identification.executor.search_customer_by_id', '{
        "code": "",
        "language": "python311",
        "packages": "",
        "timeout_ms": 30000,
        "enabled": true,
        "test_parameters": []
    }'::jsonb),
    ('customer_identification.executor.search_customer_by_free_text', '{
        "code": "",
        "language": "python311",
        "packages": "",
        "timeout_ms": 30000,
        "enabled": true,
        "test_parameters": []
    }'::jsonb)
ON CONFLICT ("key") DO NOTHING;
	`); err != nil {
		return err
	}
	return nil
}
