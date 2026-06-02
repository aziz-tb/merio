-- Demo seed data for merio / Rabot Energy support scenarios.
--
-- Apply:
--   docker exec -i merio_db psql -U merio -d merio < scripts/seed-demo.sql
--
-- Notes:
-- - Inboxes are intentionally not seeded. Demo conversations attach to the
--   first existing enabled inbox. If none exists, conversation rows are skipped.
-- - All removable demo rows use deterministic names, slugs, emails, IDs, or
--   JSON markers so scripts/seed-demo-reset.sql can remove only this seed data.

BEGIN;

SET search_path = public;

-- ---------------------------------------------------------------------------
-- Human support users and teams
-- ---------------------------------------------------------------------------

WITH seed_agents(email, first_name, last_name, role_name) AS (
  VALUES
    ('aziz@merio.io', 'Aziz', 'Taieb', 'Admin'),
    ('klara.hoffmann@merio.io', 'Klara', 'Hoffmann', 'Agent'),
    ('jonas.keller@merio.io', 'Jonas', 'Keller', 'Agent'),
    ('miriam.stein@merio.io', 'Miriam', 'Stein', 'Agent'),
    ('leona.wagner@merio.io', 'Leona', 'Wagner', 'Agent')
),
inserted_agents AS (
  INSERT INTO users (
    type, enabled, email, first_name, last_name, country, custom_attributes,
    availability_status, last_active_at
  )
  SELECT
    'agent'::user_type,
    TRUE,
    email,
    first_name,
    last_name,
    'DE',
    jsonb_build_object('seed', 'merio-demo-rabot'),
    'online'::user_availability_status,
    NOW() - INTERVAL '7 minutes'
  FROM seed_agents s
  WHERE NOT EXISTS (
    SELECT 1 FROM users u
    WHERE u.email = s.email AND u.type = 'agent' AND u.deleted_at IS NULL
  )
  RETURNING id, email
)
INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id
FROM users u
JOIN seed_agents s ON s.email = u.email
JOIN roles r ON r.name = s.role_name
WHERE u.type = 'agent'
  AND u.deleted_at IS NULL
ON CONFLICT (user_id, role_id) DO NOTHING;

INSERT INTO teams (
  name, emoji, conversation_assignment_type, max_auto_assigned_conversations,
  timezone
)
VALUES
  ('Demo Energy Support', 'E', 'Manual', 0, 'Europe/Berlin'),
  ('Demo Second Level', 'S', 'Manual', 0, 'Europe/Berlin')
ON CONFLICT (name) DO UPDATE
SET updated_at = NOW(),
    emoji = EXCLUDED.emoji,
    timezone = EXCLUDED.timezone;

INSERT INTO team_members (team_id, user_id, emoji)
SELECT t.id, u.id, 'E'
FROM teams t
JOIN users u ON u.email IN (
  'klara.hoffmann@merio.io',
  'jonas.keller@merio.io',
  'miriam.stein@merio.io',
  'leona.wagner@merio.io'
)
WHERE t.name = 'Demo Energy Support'
ON CONFLICT (team_id, user_id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- ERP cache: customers and contracts
-- ---------------------------------------------------------------------------

WITH seed_customers(
  customer_id, business, firstname, lastname, company, email, phone, zip,
  delivery_address, billing_address, contract_ids, extra_data
) AS (
  VALUES
    ('K-100023', FALSE, 'Lukas', 'Weber', NULL, 'lukas.weber@example.com', '+49 170 1234567', '10435', 'Lukas Weber, Kastanienallee 47, 10435 Berlin', NULL, '["C-880011","C-880042"]'::json, '{"seed":"merio-demo-rabot","customer_since":"2023-12-18","segment":"Prosumer","vat_id":null}'::json),
    ('K-100087', TRUE, NULL, NULL, 'Bäckerei Schneider GmbH', 'kontakt@baeckerei-schneider.example.com', '+49 89 9876543', '80331', 'Bäckerei Schneider GmbH, Sendlinger Str. 12, 80331 München', 'Bäckerei Schneider GmbH, Sendlinger Str. 12, 80331 München', '["C-880117","C-880118"]'::json, '{"seed":"merio-demo-rabot","customer_since":"2022-11-10","industry":"Gastronomy","vat_id":"DE214589673"}'::json),
    ('K-100156', FALSE, 'Sophie', 'Becker', NULL, 'sophie.becker@example.com', '+49 151 5556789', '20099', 'Sophie Becker, Lange Reihe 88, 20099 Hamburg', NULL, '["C-880204","C-880205"]'::json, '{"seed":"merio-demo-rabot","customer_since":"2024-06-02","segment":"EV","vat_id":null}'::json),
    ('K-100241', TRUE, NULL, NULL, 'Alpine Metallbau GmbH', 'energie@alpine-metallbau.example.com', '+49 711 4920180', '70173', 'Alpine Metallbau GmbH, Königstr. 22, 70173 Stuttgart', 'Alpine Metallbau GmbH, Königstr. 22, 70173 Stuttgart', '["C-880301","C-880302"]'::json, '{"seed":"merio-demo-rabot","customer_since":"2023-02-14","industry":"Manufacturing","vat_id":"DE327491856"}'::json),
    ('K-100318', TRUE, NULL, NULL, 'Heller & Partner Steuerberatungsgesellschaft mbH', 'office@heller-partner.example.com', '+49 69 2038400', '60311', 'Heller & Partner, Neue Mainzer Str. 14, 60311 Frankfurt am Main', 'Heller & Partner, Neue Mainzer Str. 14, 60311 Frankfurt am Main', '["C-880411","C-880412"]'::json, '{"seed":"merio-demo-rabot","customer_since":"2023-08-29","industry":"Professional Services","vat_id":"DE192738465"}'::json),
    ('K-100404', TRUE, NULL, NULL, 'Nordlicht Pflege AG', 'verwaltung@nordlicht-pflege.example.com', '+49 40 8899012', '22767', 'Nordlicht Pflege AG, Große Elbstr. 65, 22767 Hamburg', 'Nordlicht Pflege AG, Große Elbstr. 65, 22767 Hamburg', '["C-880501","C-880502"]'::json, '{"seed":"merio-demo-rabot","customer_since":"2022-10-03","industry":"Healthcare","vat_id":"DE284910357"}'::json),
    ('K-100512', TRUE, NULL, NULL, 'Grüner Markt Freiburg eG', 'strom@gruener-markt.example.com', '+49 761 342190', '79098', 'Grüner Markt Freiburg eG, Kaiser-Joseph-Str. 201, 79098 Freiburg', 'Grüner Markt Freiburg eG, Kaiser-Joseph-Str. 201, 79098 Freiburg', '["C-880601","C-880602"]'::json, '{"seed":"merio-demo-rabot","customer_since":"2024-01-18","industry":"Retail","vat_id":"DE301948572"}'::json),
    ('K-100609', TRUE, NULL, NULL, 'Autohaus RheinMain GmbH', 'fuhrpark@rheinmain-autohaus.example.com', '+49 6131 771940', '55116', 'Autohaus RheinMain GmbH, Rheinallee 29, 55116 Mainz', 'Autohaus RheinMain GmbH, Rheinallee 29, 55116 Mainz', '["C-880701","C-880702"]'::json, '{"seed":"merio-demo-rabot","customer_since":"2022-06-21","industry":"Automotive","vat_id":"DE193847562"}'::json),
    ('K-100711', TRUE, NULL, NULL, 'Bergmann Logistik SE', 'energy@bergmann-logistik.example.com', '+49 341 509870', '04109', 'Bergmann Logistik SE, Petersstr. 35, 04109 Leipzig', 'Bergmann Logistik SE, Petersstr. 35, 04109 Leipzig', '["C-880801","C-880802"]'::json, '{"seed":"merio-demo-rabot","customer_since":"2023-05-05","industry":"Logistics","vat_id":"DE268415793"}'::json),
    ('K-100845', TRUE, NULL, NULL, 'Café Morgenrot GbR', 'hallo@cafe-morgenrot.example.com', '+49 30 4908732', '10999', 'Café Morgenrot GbR, Oranienstr. 31, 10999 Berlin', 'Café Morgenrot GbR, Oranienstr. 31, 10999 Berlin', '["C-880901","C-880902"]'::json, '{"seed":"merio-demo-rabot","customer_since":"2024-03-11","industry":"Hospitality","vat_id":"DE213487965"}'::json),
    ('K-100902', TRUE, NULL, NULL, 'Sonnental Immobilien GmbH', 'service@sonnental-immobilien.example.com', '+49 89 221980', '80333', 'Sonnental Immobilien GmbH, Brienner Str. 18, 80333 München', 'Sonnental Immobilien GmbH, Brienner Str. 18, 80333 München', '["C-881001","C-881002"]'::json, '{"seed":"merio-demo-rabot","customer_since":"2022-12-07","industry":"Real Estate","vat_id":"DE309284716"}'::json),
    ('K-101044', TRUE, NULL, NULL, 'Helvetic Labs AG', 'operations@helvetic-labs.example.com', '+41 44 555 1820', '8001', 'Helvetic Labs AG, Bahnhofstrasse 24, 8001 Zürich', 'Helvetic Labs AG, Bahnhofstrasse 24, 8001 Zürich', '["C-881101","C-881102"]'::json, '{"seed":"merio-demo-rabot","customer_since":"2023-09-19","industry":"Technology","vat_id":"CHE-284.915.307"}'::json),
    ('K-101105', TRUE, NULL, NULL, 'Wiener Werkraum GmbH', 'energie@wiener-werkraum.example.com', '+43 1 555 9182', '1010', 'Wiener Werkraum GmbH, Schottenring 11, 1010 Wien', 'Wiener Werkraum GmbH, Schottenring 11, 1010 Wien', '["C-881201","C-881202"]'::json, '{"seed":"merio-demo-rabot","customer_since":"2024-04-04","industry":"Design","vat_id":"ATU73918426"}'::json),
    ('K-101208', TRUE, NULL, NULL, 'Kühlhaus Bremen GmbH', 'abrechnung@kuehlhaus-bremen.example.com', '+49 421 778801', '28195', 'Kühlhaus Bremen GmbH, Langenstr. 44, 28195 Bremen', 'Kühlhaus Bremen GmbH, Langenstr. 44, 28195 Bremen', '["C-881301","C-881302"]'::json, '{"seed":"merio-demo-rabot","customer_since":"2023-11-02","industry":"Cold Storage","vat_id":"DE291746530"}'::json),
    ('K-101330', FALSE, 'Mira', 'Özdemir', NULL, 'mira.oezdemir@example.com', '+49 152 4400123', '50667', 'Mira Özdemir, Ehrenstr. 8, 50667 Köln', NULL, '["C-881401","C-881402"]'::json, '{"seed":"merio-demo-rabot","customer_since":"2024-07-22","segment":"EV","vat_id":null}'::json),
    ('K-101402', FALSE, 'Jan', 'Krüger', NULL, 'jan.krueger@example.com', '+49 176 9988123', '30159', 'Jan Krüger, Georgstr. 39, 30159 Hannover', NULL, '["C-881501","C-881502"]'::json, '{"seed":"merio-demo-rabot","customer_since":"2023-03-09","segment":"HeatPumpOwner","vat_id":null}'::json),
    ('K-101509', TRUE, NULL, NULL, 'Hofgut Linden GbR', 'verwaltung@hofgut-linden.example.com', '+49 621 903118', '68159', 'Hofgut Linden GbR, Fressgasse 17, 68159 Mannheim', 'Hofgut Linden GbR, Fressgasse 17, 68159 Mannheim', '["C-881601","C-881602"]'::json, '{"seed":"merio-demo-rabot","customer_since":"2022-08-16","industry":"Agriculture","vat_id":"DE189472630"}'::json),
    ('K-101612', TRUE, NULL, NULL, 'Praxis Dr. Adler', 'praxis@dr-adler.example.com', '+49 211 480812', '40213', 'Praxis Dr. Adler, Königsallee 58, 40213 Düsseldorf', 'Praxis Dr. Adler, Königsallee 58, 40213 Düsseldorf', '["C-881701","C-881702"]'::json, '{"seed":"merio-demo-rabot","customer_since":"2023-06-26","industry":"Healthcare","vat_id":"DE318762945"}'::json)
)
INSERT INTO customers (
  customer_id, business, firstname, lastname, company, email, phone, zip,
  delivery_address, billing_address, contract_ids, extra_data, last_synced_at
)
SELECT
  customer_id, business, firstname, lastname, company, email, phone, zip,
  delivery_address, billing_address, contract_ids, extra_data, NOW()
FROM seed_customers
ON CONFLICT (customer_id) DO UPDATE
SET updated_at = NOW(),
    last_synced_at = NOW(),
    business = EXCLUDED.business,
    firstname = EXCLUDED.firstname,
    lastname = EXCLUDED.lastname,
    company = EXCLUDED.company,
    email = EXCLUDED.email,
    phone = EXCLUDED.phone,
    zip = EXCLUDED.zip,
    delivery_address = EXCLUDED.delivery_address,
    billing_address = EXCLUDED.billing_address,
    contract_ids = EXCLUDED.contract_ids,
    extra_data = EXCLUDED.extra_data;

INSERT INTO users (
  type, enabled, email, first_name, last_name, country, external_user_id,
  linked_customer_id, custom_attributes
)
SELECT
  'contact'::user_type,
  TRUE,
  c.email,
  COALESCE(c.firstname, c.company),
  c.lastname,
  CASE WHEN c.zip IN ('8001') THEN 'CH' WHEN c.zip IN ('1010') THEN 'AT' ELSE 'DE' END,
  c.customer_id,
  c.customer_id,
  jsonb_build_object('seed', 'merio-demo-rabot')
FROM customers c
WHERE c.extra_data->>'seed' = 'merio-demo-rabot'
  AND NOT EXISTS (
    SELECT 1 FROM users u
    WHERE u.external_user_id = c.customer_id
      AND u.type = 'contact'
      AND u.deleted_at IS NULL
  );

