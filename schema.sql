CREATE EXTENSION IF NOT EXISTS pg_trgm;

DROP TYPE IF EXISTS "channels" CASCADE; CREATE TYPE "channels" AS ENUM ('email', 'livechat');
DROP TYPE IF EXISTS "message_type" CASCADE; CREATE TYPE "message_type" AS ENUM ('incoming','outgoing','activity');
DROP TYPE IF EXISTS "message_sender_type" CASCADE; CREATE TYPE "message_sender_type" AS ENUM ('agent','contact');
DROP TYPE IF EXISTS "message_status" CASCADE; CREATE TYPE "message_status" AS ENUM ('received','sent','failed','pending');
DROP TYPE IF EXISTS "content_type" CASCADE; CREATE TYPE "content_type" AS ENUM ('text','html');
DROP TYPE IF EXISTS "conversation_assignment_type" CASCADE; CREATE TYPE "conversation_assignment_type" AS ENUM ('Round robin','Manual');
DROP TYPE IF EXISTS "template_type" CASCADE; CREATE TYPE "template_type" AS ENUM ('email_outgoing', 'email_notification');
-- Visitors are unauthenticated contacts.
DROP TYPE IF EXISTS "user_type" CASCADE; CREATE TYPE "user_type" AS ENUM ('agent', 'contact', 'visitor');
DROP TYPE IF EXISTS "automation_execution_mode" CASCADE; CREATE TYPE "automation_execution_mode" AS ENUM ('all', 'first_match');
DROP TYPE IF EXISTS "macro_visibility" CASCADE; CREATE TYPE "macro_visibility" AS ENUM ('all', 'team', 'user');
DROP TYPE IF EXISTS "view_visibility" CASCADE; CREATE TYPE "view_visibility" AS ENUM ('all', 'team', 'user');
DROP TYPE IF EXISTS "media_disposition" CASCADE; CREATE TYPE "media_disposition" AS ENUM ('inline', 'attachment');
DROP TYPE IF EXISTS "media_store" CASCADE; CREATE TYPE "media_store" AS ENUM ('s3', 'fs');
DROP TYPE IF EXISTS "user_availability_status" CASCADE; CREATE TYPE "user_availability_status" AS ENUM ('online', 'away', 'away_manual', 'offline', 'away_and_reassigning');
DROP TYPE IF EXISTS "applied_sla_status" CASCADE; CREATE TYPE "applied_sla_status" AS ENUM ('pending', 'breached', 'met', 'partially_met');
DROP TYPE IF EXISTS "sla_event_status" CASCADE; CREATE TYPE "sla_event_status" AS ENUM ('pending', 'breached', 'met');
DROP TYPE IF EXISTS "sla_metric" CASCADE; CREATE TYPE "sla_metric" AS ENUM ('first_response', 'resolution', 'next_response');
DROP TYPE IF EXISTS "sla_notification_type" CASCADE; CREATE TYPE "sla_notification_type" AS ENUM ('warning', 'breach');
DROP TYPE IF EXISTS "activity_log_type" CASCADE; CREATE TYPE "activity_log_type" AS ENUM ('agent_login', 'agent_logout', 'agent_away', 'agent_away_reassigned', 'agent_online', 'agent_password_set', 'agent_role_permissions_changed');
DROP TYPE IF EXISTS "macro_visible_when" CASCADE; CREATE TYPE "macro_visible_when" AS ENUM ('replying', 'starting_conversation', 'adding_private_note');
DROP TYPE IF EXISTS "user_notification_type" CASCADE; CREATE TYPE "user_notification_type" AS ENUM ('mention', 'assignment', 'sla_warning', 'sla_breach');
DROP TYPE IF EXISTS "conversation_status_category" CASCADE; CREATE TYPE "conversation_status_category" AS ENUM ('open', 'waiting', 'resolved');
DROP TYPE IF EXISTS "webhook_event" CASCADE; CREATE TYPE webhook_event AS ENUM (
	'conversation.created',
	'conversation.status_changed',
	'conversation.tags_changed',
	'conversation.assigned',
	'conversation.unassigned',
	'message.created',
	'message.updated'
);

-- Sequence to generate reference number for conversations.
DROP SEQUENCE IF EXISTS conversation_reference_number_sequence; CREATE SEQUENCE conversation_reference_number_sequence START 100;

-- Function to generate reference number for conversations with optional prefix.
CREATE OR REPLACE FUNCTION generate_reference_number(prefix TEXT)
RETURNS TEXT AS $$
BEGIN
    RETURN prefix || nextval('conversation_reference_number_sequence');
END;
$$ LANGUAGE plpgsql;

DROP TABLE IF EXISTS sla_policies CASCADE;
CREATE TABLE sla_policies (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	name TEXT NOT NULL,
	description TEXT NULL,
	first_response_time TEXT NOT NULL,
	resolution_time TEXT NOT NULL,
	next_response_time TEXT NULL,
	notifications JSONB DEFAULT '[]'::jsonb NOT NULL,
	CONSTRAINT constraint_sla_policies_on_name CHECK (length(name) <= 140),
	CONSTRAINT constraint_sla_policies_on_description CHECK (length(description) <= 300)
);

DROP TABLE IF EXISTS business_hours CASCADE;
CREATE TABLE business_hours (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
	name TEXT NOT NULL,
	description TEXT NULL,
	is_always_open BOOL DEFAULT false NOT NULL,
	hours JSONB NOT NULL,
	holidays JSONB DEFAULT '{}'::jsonb NOT NULL,
	CONSTRAINT constraint_business_hours_on_name CHECK (length(name) <= 140),
	CONSTRAINT constraint_business_hours_on_description CHECK (length(description) <= 300)
);

DROP TABLE IF EXISTS inboxes CASCADE;
CREATE TABLE inboxes (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	"uuid" UUID DEFAULT gen_random_uuid() NOT NULL UNIQUE,
	"name" TEXT NOT NULL,
	deleted_at TIMESTAMPTZ NULL,
	channel channels NOT NULL,
	enabled bool DEFAULT TRUE NOT NULL,
	csat_enabled bool DEFAULT false NOT NULL,
	prompt_tags_on_reply bool DEFAULT false NOT NULL,
	config jsonb DEFAULT '{}'::jsonb NOT NULL,
	"from" TEXT NULL,
	secret TEXT NULL,
	linked_email_inbox_id INT REFERENCES inboxes(id) ON DELETE SET NULL,
	CONSTRAINT constraint_inboxes_on_name CHECK (length("name") <= 140)
);

DROP TABLE IF EXISTS teams CASCADE;
CREATE TABLE teams (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	"name" TEXT NOT NULL,
	emoji TEXT NULL,
	conversation_assignment_type conversation_assignment_type NOT NULL,
	max_auto_assigned_conversations INT DEFAULT 0 NOT NULL,

	-- Set to NULL when business hours or SLA policy is deleted.
	business_hours_id INT REFERENCES business_hours(id) ON DELETE SET NULL ON UPDATE CASCADE NULL,
	sla_policy_id INT REFERENCES sla_policies(id) ON DELETE SET NULL ON UPDATE CASCADE NULL,

	timezone TEXT NULL,
	CONSTRAINT constraint_teams_on_emoji CHECK (length(emoji) <= 50),
	CONSTRAINT constraint_teams_on_name CHECK (length("name") <= 140),
	CONSTRAINT constraint_teams_on_timezone CHECK (length(timezone) <= 140),
	CONSTRAINT constraint_teams_on_name_unique UNIQUE ("name")
);

DROP TABLE IF EXISTS roles CASCADE;
CREATE TABLE roles (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    permissions TEXT[] DEFAULT '{}'::TEXT[] NOT NULL,
    "name" TEXT UNIQUE NOT NULL,
    description TEXT NULL,
	CONSTRAINT constraint_roles_on_name CHECK (length("name") <= 50),
	CONSTRAINT constraint_roles_on_description CHECK (length(description) <= 300)
);

DROP TABLE IF EXISTS users CASCADE;
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    type user_type NOT NULL,
    deleted_at TIMESTAMPTZ NULL,
    enabled BOOL DEFAULT TRUE NOT NULL,
    is_system BOOL DEFAULT FALSE NOT NULL,
    email TEXT NULL,
    first_name TEXT NOT NULL,
    last_name TEXT NULL,
	phone_number_country_code TEXT NULL,
    phone_number TEXT NULL,
    country TEXT NULL,
    "password" VARCHAR(150) NULL,
    avatar_url TEXT NULL,
	custom_attributes JSONB DEFAULT '{}'::jsonb NOT NULL,
	external_user_id TEXT NULL,
    reset_password_token TEXT NULL,
    reset_password_token_expiry TIMESTAMPTZ NULL,
	availability_status user_availability_status DEFAULT 'offline' NOT NULL,
	last_active_at TIMESTAMPTZ NULL,
	last_login_at TIMESTAMPTZ NULL,
	-- API key authentication fields
	api_key TEXT NULL,
	api_secret TEXT NULL,
	api_key_last_used_at TIMESTAMPTZ NULL,
	linked_customer_id TEXT NULL,
	linked_customer_id_at TIMESTAMPTZ NULL,
    CONSTRAINT constraint_users_on_country CHECK (LENGTH(country) <= 140),
    CONSTRAINT constraint_users_on_phone_number CHECK (LENGTH(phone_number) <= 20),
	CONSTRAINT constraint_users_on_phone_number_country_code CHECK (LENGTH(phone_number_country_code) <= 10),
    CONSTRAINT constraint_users_on_email_length CHECK (LENGTH(email) <= 320),
    CONSTRAINT constraint_users_on_first_name CHECK (LENGTH(first_name) <= 140),
    CONSTRAINT constraint_users_on_last_name CHECK (LENGTH(last_name) <= 140)
);
CREATE INDEX index_tgrm_users_on_email ON users USING GIN (email gin_trgm_ops);
CREATE INDEX index_users_on_api_key ON users(api_key);
CREATE UNIQUE INDEX index_unique_users_on_email_when_type_is_agent
	ON users(email)
	WHERE type = 'agent' AND deleted_at IS NULL;
CREATE UNIQUE INDEX index_unique_users_on_ext_id_when_type_is_contact 
	ON users (external_user_id) 
	WHERE type = 'contact' AND deleted_at IS NULL AND external_user_id IS NOT NULL;
CREATE UNIQUE INDEX index_unique_users_on_email_when_no_ext_id_contact
	ON users (email)
	WHERE type = 'contact' AND deleted_at IS NULL AND external_user_id IS NULL;

DROP TABLE IF EXISTS secrets CASCADE;
CREATE TABLE secrets (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    encrypted_value BYTEA NOT NULL,
    nonce BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_accessed_at TIMESTAMPTZ,
    created_by_user_id INT REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT secrets_name_format CHECK (name ~ '^[A-Za-z][A-Za-z0-9_]{0,63}$')
);
CREATE INDEX index_secrets_on_name ON secrets(name);

DROP TABLE IF EXISTS user_roles CASCADE;
CREATE TABLE user_roles (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),

	-- Cascade deletes when user or role is deleted, as they are not useful without each other.
	user_id INT REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	role_id INT REFERENCES roles(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,

	CONSTRAINT constraint_user_roles_on_user_id_and_role_id_unique UNIQUE (user_id, role_id)
);
CREATE INDEX index_user_roles_on_user_id ON user_roles(user_id);

DROP TABLE IF EXISTS conversation_statuses CASCADE;
CREATE TABLE conversation_statuses (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	"name" TEXT NOT NULL UNIQUE,
	category conversation_status_category NOT NULL DEFAULT 'open',
	color TEXT NOT NULL DEFAULT 'blue',
	icon TEXT NOT NULL DEFAULT 'CircleDot'
);

DROP TABLE IF EXISTS conversation_priorities CASCADE;
CREATE TABLE conversation_priorities (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	"name" TEXT NOT NULL UNIQUE
);

DROP TABLE IF EXISTS conversations CASCADE;
CREATE TABLE conversations (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    "uuid" UUID DEFAULT gen_random_uuid() NOT NULL UNIQUE,
	reference_number TEXT DEFAULT generate_reference_number('') NOT NULL UNIQUE,

	-- Cascade deletes when contact is deleted.
    contact_id BIGINT REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,

	-- Set to NULL when assigned user or team is deleted.
    assigned_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL ON UPDATE CASCADE,
    assigned_team_id INT REFERENCES teams(id) ON DELETE SET NULL ON UPDATE CASCADE,

	-- Set to NULL when SLA policy is deleted.
	sla_policy_id INT REFERENCES sla_policies(id) ON DELETE SET NULL ON UPDATE CASCADE,

    -- Cascade deletes when inbox is deleted.
	inbox_id INT REFERENCES inboxes(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,

	-- Restrict delete.
	status_id INT REFERENCES conversation_statuses(id) ON DELETE RESTRICT ON UPDATE CASCADE NOT NULL,
    priority_id INT REFERENCES conversation_priorities(id) ON DELETE RESTRICT ON UPDATE CASCADE,

	meta JSONB DEFAULT '{}'::jsonb NOT NULL,
	custom_attributes JSONB DEFAULT '{}'::jsonb NOT NULL,
	contact_last_seen_at TIMESTAMPTZ DEFAULT NOW(),
    first_reply_at TIMESTAMPTZ NULL,
    last_reply_at TIMESTAMPTZ NULL,
    closed_at TIMESTAMPTZ NULL,
    resolved_at TIMESTAMPTZ NULL,

	"subject" TEXT NULL,
	waiting_since TIMESTAMPTZ NULL,
	last_message_at TIMESTAMPTZ NULL,
	last_message TEXT NULL,
	last_message_sender message_sender_type NULL,
	last_message_sender_id BIGINT REFERENCES users(id) ON DELETE SET NULL ON UPDATE CASCADE,
	last_interaction TEXT NULL,
	last_interaction_sender message_sender_type NULL,
	last_interaction_sender_id BIGINT REFERENCES users(id) ON DELETE SET NULL ON UPDATE CASCADE,
	last_interaction_at TIMESTAMPTZ NULL,
	next_sla_deadline_at TIMESTAMPTZ NULL,
	snoozed_until TIMESTAMPTZ NULL,
	last_continuity_email_sent_at TIMESTAMPTZ NULL
);
CREATE INDEX index_conversations_on_assigned_user_id ON conversations (assigned_user_id);
CREATE INDEX index_conversations_on_assigned_team_id ON conversations (assigned_team_id);
CREATE INDEX index_conversations_on_snoozed_until ON conversations (snoozed_until);
CREATE INDEX index_conversations_on_contact_id ON conversations (contact_id);
CREATE INDEX index_conversations_on_inbox_id ON conversations (inbox_id);
CREATE INDEX index_conversations_on_status_id ON conversations (status_id);
CREATE INDEX index_conversations_on_priority_id ON conversations (priority_id);
CREATE INDEX index_conversations_on_created_at ON conversations (created_at);
CREATE INDEX index_conversations_on_last_message_at ON conversations (last_message_at);
CREATE INDEX index_conversations_on_last_interaction_at ON conversations (last_interaction_at);
CREATE INDEX index_conversations_on_next_sla_deadline_at ON conversations (next_sla_deadline_at);
CREATE INDEX index_conversations_on_waiting_since ON conversations (waiting_since);
CREATE INDEX index_conversations_on_last_continuity_email_sent_at ON conversations (last_continuity_email_sent_at);

DROP TABLE IF EXISTS conversation_messages CASCADE;
CREATE TABLE conversation_messages (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    "uuid" UUID DEFAULT gen_random_uuid() NOT NULL UNIQUE,
    "type" message_type NOT NULL,
    status message_status NOT NULL,
    private BOOL DEFAULT FALSE NOT NULL,
    conversation_id BIGINT REFERENCES conversations(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
    content_type content_type NULL,
    "content" TEXT NULL,
	text_content TEXT NULL,
    source_id TEXT NULL,
    send_inbox_id INT REFERENCES inboxes(id) ON DELETE RESTRICT ON UPDATE CASCADE NULL,
 	sender_id BIGINT REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
    sender_type message_sender_type NOT NULL,
    meta JSONB DEFAULT '{}'::JSONB NULL
);
CREATE INDEX index_trgm_conversation_messages_on_text_content ON conversation_messages USING GIN (text_content gin_trgm_ops);
CREATE INDEX index_conversation_messages_on_conversation_id ON conversation_messages (conversation_id);
CREATE INDEX index_conversation_messages_on_created_at ON conversation_messages (created_at);
CREATE UNIQUE INDEX index_unique_conversation_messages_on_source_id ON conversation_messages (source_id) WHERE source_id IS NOT NULL;
CREATE INDEX index_conversation_messages_on_send_inbox_id ON conversation_messages (send_inbox_id) WHERE send_inbox_id IS NOT NULL;
CREATE INDEX index_conversation_messages_on_status ON conversation_messages (status);
CREATE INDEX index_conversation_messages_on_conversation_id_and_created_at ON conversation_messages (conversation_id, created_at);

DROP TABLE IF EXISTS automation_rules CASCADE;
CREATE TABLE automation_rules (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    "name" TEXT NOT NULL,
    description TEXT NULL,
    "type" VARCHAR NOT NULL,
    rules JSONB NULL,
    events TEXT[] DEFAULT '{}'::TEXT[] NOT NULL,
    enabled BOOL DEFAULT TRUE NOT NULL,
	weight INT DEFAULT 0 NOT NULL,
	execution_mode automation_execution_mode DEFAULT 'all' NOT NULL,
    CONSTRAINT constraint_automation_rules_on_name CHECK (length("name") <= 140),
    CONSTRAINT constraint_automation_rules_on_description CHECK (length(description) <= 300)
);
CREATE INDEX index_automation_rules_on_enabled_and_weight ON automation_rules(enabled, weight);
CREATE INDEX index_automation_rules_on_type_and_weight ON automation_rules(type, weight);

DROP TABLE IF EXISTS conversation_drafts CASCADE;
CREATE TABLE conversation_drafts (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    conversation_id BIGINT REFERENCES conversations(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
    user_id BIGINT REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
    content TEXT NOT NULL,
	meta JSONB DEFAULT '{}'::jsonb NOT NULL
);
CREATE UNIQUE INDEX index_uniq_conversation_drafts_on_conversation_id_and_user_id ON conversation_drafts (conversation_id, user_id);

DROP TABLE IF EXISTS macros CASCADE;
CREATE TABLE macros (
   id SERIAL PRIMARY KEY,
   created_at TIMESTAMPTZ DEFAULT NOW(),
   updated_at TIMESTAMPTZ DEFAULT NOW(),
   name TEXT NOT NULL,
   actions JSONB DEFAULT '{}'::jsonb NOT NULL,
   visibility macro_visibility NOT NULL,
   visible_when macro_visible_when[] NOT NULL DEFAULT ARRAY['replying', 'starting_conversation', 'adding_private_note']::macro_visible_when[],
   message_content TEXT NOT NULL,
   -- Cascade deletes when user is deleted.
   user_id BIGINT REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE,
   team_id BIGINT REFERENCES teams(id) ON DELETE CASCADE ON UPDATE CASCADE,
   usage_count INT DEFAULT 0 NOT NULL,
   CONSTRAINT name_length CHECK (length(name) <= 140),
   CONSTRAINT message_content_length CHECK (length(message_content) <= 5000)
);

DROP TABLE IF EXISTS conversation_participants CASCADE;
CREATE TABLE conversation_participants (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	-- Cascade deletes when user or conversation is deleted.
	user_id BIGINT REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	conversation_id BIGINT REFERENCES conversations(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL
);
CREATE UNIQUE INDEX index_unique_conversation_participants_on_conversation_id_and_user_id ON conversation_participants (conversation_id, user_id);

DROP TABLE IF EXISTS conversation_mentions CASCADE;
CREATE TABLE conversation_mentions (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	conversation_id BIGINT REFERENCES conversations(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	message_id BIGINT REFERENCES conversation_messages(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	mentioned_user_id BIGINT REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE,
	mentioned_team_id INT REFERENCES teams(id) ON DELETE CASCADE ON UPDATE CASCADE,
	mentioned_by_user_id BIGINT REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	CONSTRAINT constraint_mention_target CHECK (
		(mentioned_user_id IS NOT NULL AND mentioned_team_id IS NULL) OR
		(mentioned_user_id IS NULL AND mentioned_team_id IS NOT NULL)
	)
);
CREATE INDEX index_conversation_mentions_on_mentioned_user_id ON conversation_mentions(mentioned_user_id);
CREATE INDEX index_conversation_mentions_on_mentioned_team_id ON conversation_mentions(mentioned_team_id);
CREATE INDEX index_conversation_mentions_on_conversation_id ON conversation_mentions(conversation_id);

DROP TABLE IF EXISTS conversation_last_seen CASCADE;
CREATE TABLE conversation_last_seen (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	user_id BIGINT REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	conversation_id BIGINT REFERENCES conversations(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	last_seen_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);
CREATE UNIQUE INDEX index_unique_conversation_last_seen ON conversation_last_seen (conversation_id, user_id);

DROP TABLE IF EXISTS media CASCADE;
CREATE TABLE media (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	"uuid" uuid DEFAULT gen_random_uuid() NOT NULL UNIQUE,
	store "media_store" NOT NULL,
	filename TEXT NOT NULL,
	content_type TEXT NOT NULL,
	content_id TEXT NULL,
	model_id INT NULL,
	model_type TEXT NULL,
	disposition media_disposition NULL,
	"size" INT NULL,
	meta jsonb DEFAULT '{}'::jsonb NOT NULL,
	CONSTRAINT constraint_media_on_filename CHECK (length(filename) <= 1000),
	CONSTRAINT constraint_media_on_content_id CHECK (length(content_id) <= 300)
);
CREATE INDEX index_media_on_model_type_and_model_id ON media(model_type, model_id);
CREATE INDEX index_media_on_content_id ON media(content_id);

DROP TABLE IF EXISTS oidc CASCADE;
CREATE TABLE oidc (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	"name" TEXT NULL,
	provider_url TEXT NOT NULL,
	client_id TEXT NOT NULL,
	client_secret TEXT NOT NULL,
	enabled bool DEFAULT TRUE NOT NULL,
	provider VARCHAR NULL,
	logo_url TEXT NOT NULL DEFAULT '',
	CONSTRAINT constraint_oidc_on_name CHECK (length("name") <= 140)
);

DROP TABLE IF EXISTS settings CASCADE;
CREATE TABLE settings (
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	"key" TEXT NOT NULL UNIQUE,
	value jsonb DEFAULT '{}'::jsonb NOT NULL,
	CONSTRAINT settings_key_key UNIQUE ("key")
);
CREATE INDEX index_settings_on_key ON settings USING btree ("key");

DROP TABLE IF EXISTS tags CASCADE;
CREATE TABLE tags (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	"name" TEXT NOT NULL UNIQUE,
	parent_id INT NULL REFERENCES tags(id) ON DELETE SET NULL,
	type TEXT NOT NULL DEFAULT 'skill',
	color TEXT NOT NULL DEFAULT 'grey',
	visibility TEXT NOT NULL DEFAULT 'public',
	description TEXT NOT NULL DEFAULT '',
	detection JSONB NOT NULL DEFAULT '{"mode":"manual_only"}'::jsonb,
	sla_policy_id INT NULL REFERENCES sla_policies(id) ON DELETE SET NULL,
	priority_id INT NULL REFERENCES conversation_priorities(id) ON DELETE SET NULL,
	updated_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT constraint_tags_on_name CHECK (length("name") <= 140),
	CONSTRAINT constraint_tags_on_type CHECK (type IN ('skill', 'topic', 'contact', 'other')),
	CONSTRAINT constraint_tags_on_visibility CHECK (visibility IN ('public', 'private', 'disabled')),
	CONSTRAINT constraint_tags_on_color CHECK (color IN ('grey', 'green', 'red', 'blue', 'yellow', 'purple', 'orange', 'teal'))
);
CREATE INDEX index_tags_on_parent_id ON tags(parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX index_tags_on_visibility ON tags(visibility);
CREATE INDEX index_tags_on_detection_mode ON tags((detection->>'mode'));
CREATE INDEX index_tags_on_updated_by ON tags(updated_by) WHERE updated_by IS NOT NULL;

DROP TABLE IF EXISTS team_members CASCADE;
CREATE TABLE team_members (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	-- Cascade deletes when team or user is deleted.
	team_id BIGINT REFERENCES teams(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	user_id BIGINT REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	emoji TEXT NULL,
	CONSTRAINT constraint_team_members_on_emoji CHECK (length(emoji) <= 1)
);
CREATE UNIQUE INDEX index_unique_team_members_on_team_id_and_user_id ON team_members (team_id, user_id);
CREATE INDEX index_team_members_on_user_id ON team_members (user_id);

DROP TABLE IF EXISTS templates CASCADE;
CREATE TABLE templates (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	type template_type NOT NULL,
	body TEXT NOT NULL,
	is_default bool DEFAULT false NOT NULL,
	"name" TEXT NOT NULL,
	subject TEXT NULL,
	is_builtin bool DEFAULT false NOT NULL,
	CONSTRAINT constraint_templates_on_name CHECK (length("name") <= 140),
	CONSTRAINT constraint_templates_on_subject CHECK (length(subject) <= 1000)
);
CREATE UNIQUE INDEX index_unique_templates_on_is_default_when_is_default_is_true ON templates USING btree (is_default)
WHERE (is_default = true);

DROP TABLE IF EXISTS conversation_tags CASCADE;
CREATE TABLE conversation_tags (
	-- Cascade deletes when tag or conversation is deleted.
	tag_id INT REFERENCES tags(id) ON DELETE CASCADE ON UPDATE CASCADE,
	conversation_id BIGINT REFERENCES conversations(id) ON DELETE CASCADE ON UPDATE CASCADE,
	applied_by TEXT NOT NULL DEFAULT 'manual',
	-- ai_operation_id links an AI-applied tag back to the ai_operations
	-- row that applied it (NULL for manual/rule tags). Provenance only —
	-- nothing reads it for logic; the FK SET NULLs if the op is purged.
	ai_operation_id BIGINT NULL,
	-- Natural key is sufficient; the surrogate BIGSERIAL id was dropped
	-- in Batch 9 (discovery confirmed nothing read it).
	PRIMARY KEY (conversation_id, tag_id),
	CONSTRAINT constraint_conversation_tags_on_applied_by CHECK (applied_by IN ('manual', 'ai', 'rule'))
);
CREATE INDEX index_conversation_tags_on_applied_by ON conversation_tags(applied_by);

-- ai_operations — unified per-logical-AI-operation outcome audit (Batch 9,
-- replacing tag_classifications). Houses tag classification today and
-- future AI feature outcomes (agent validation, OCR, summarization, etc.).
-- Lightweight: operation-specific data lives in payload JSONB; the
-- underlying LLM call(s) are joined from ai_usage via trace_id. Heavyweight
-- agent runs stay in agent_executions and are UNIONed at query time.
-- operation_type has no CHECK enum — values are managed in code
-- (internal/ai/operations/models) so new ops need no migration.
DROP TABLE IF EXISTS ai_operations CASCADE;
CREATE TABLE ai_operations (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	operation_type TEXT NOT NULL,
	-- Nullable: not every future op is conversation-scoped.
	conversation_id BIGINT REFERENCES conversations(id) ON DELETE CASCADE,
	triggered_by TEXT NOT NULL,
	triggered_by_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
	-- Join key to ai_usage. NOT NULL even for skipped ops (callers use a
	-- deterministic trace pattern, e.g. "tag-classify:<conv-uuid>").
	trace_id TEXT NOT NULL,
	status TEXT NOT NULL,
	error_message TEXT,
	-- Operation-specific outcome data; shape per operation_type documented
	-- in internal/aioperations/payloads/. Empty/partial for failed/skipped.
	payload JSONB NOT NULL DEFAULT '{}'::jsonb,
	CONSTRAINT check_ai_operations_triggered_by CHECK (triggered_by IN ('auto', 'manual')),
	CONSTRAINT check_ai_operations_status CHECK (status IN ('running', 'success', 'failed', 'skipped'))
);
CREATE INDEX index_ai_operations_on_conversation
	ON ai_operations(conversation_id, created_at DESC)
	WHERE conversation_id IS NOT NULL;
CREATE INDEX index_ai_operations_on_type_created
	ON ai_operations(operation_type, created_at DESC);
CREATE INDEX index_ai_operations_on_trace
	ON ai_operations(trace_id);

ALTER TABLE conversation_tags
	ADD CONSTRAINT fk_conversation_tags_ai_operation_id
	FOREIGN KEY (ai_operation_id) REFERENCES ai_operations(id) ON DELETE SET NULL;

DROP TABLE IF EXISTS csat_responses CASCADE;
CREATE TABLE csat_responses (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
	uuid UUID DEFAULT gen_random_uuid() NOT NULL UNIQUE,

	-- Cascade deletes when conversation is deleted.
    conversation_id BIGINT REFERENCES conversations(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,

    rating INT DEFAULT 0 NOT NULL,
    feedback TEXT NULL,
    meta JSONB DEFAULT '{}' NOT NULL,
    response_timestamp TIMESTAMPTZ NULL,
    CONSTRAINT constraint_csat_responses_on_rating CHECK (rating >= 0 AND rating <= 5),
    CONSTRAINT constraint_csat_responses_on_feedback CHECK (length(feedback) <= 1000)
);
CREATE INDEX index_csat_responses_on_uuid ON csat_responses(uuid);
CREATE INDEX index_csat_responses_on_conversation_id ON csat_responses(conversation_id);

DROP TABLE IF EXISTS views CASCADE;
CREATE TABLE views (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    name TEXT NOT NULL,
    filters JSONB NOT NULL,
    visibility view_visibility NOT NULL DEFAULT 'user',
    -- Delete user views when user / team is deleted.
    user_id BIGINT REFERENCES users (id) ON DELETE CASCADE ON UPDATE CASCADE,
    team_id BIGINT REFERENCES teams(id) ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT constraint_views_on_name CHECK (length(name) <= 140),
    CONSTRAINT constraint_views_visibility_user CHECK (visibility != 'user' OR user_id IS NOT NULL),
    CONSTRAINT constraint_views_visibility_team CHECK (visibility != 'team' OR team_id IS NOT NULL)
);
CREATE INDEX index_views_on_user_id ON views(user_id);
CREATE INDEX index_views_on_visibility ON views(visibility);
CREATE INDEX index_views_on_team_id ON views(team_id);

DROP TABLE IF EXISTS applied_slas CASCADE;
CREATE TABLE applied_slas (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),

	status applied_sla_status DEFAULT 'pending' NOT NULL,

	-- Cascade deletes when conversation or SLA policy is deleted.
	conversation_id BIGINT REFERENCES conversations(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	sla_policy_id INT REFERENCES sla_policies(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,

	first_response_deadline_at TIMESTAMPTZ NULL,
	resolution_deadline_at TIMESTAMPTZ NULL,
	first_response_breached_at TIMESTAMPTZ NULL,
	resolution_breached_at TIMESTAMPTZ NULL,
	first_response_met_at TIMESTAMPTZ NULL,
	resolution_met_at TIMESTAMPTZ NULL
);
CREATE INDEX index_applied_slas_on_conversation_id ON applied_slas(conversation_id);
CREATE INDEX index_applied_slas_on_status ON applied_slas(status);
CREATE UNIQUE INDEX index_applied_slas_unique_pending_per_conv ON applied_slas(conversation_id) WHERE status = 'pending';

DROP TABLE IF EXISTS sla_events CASCADE;
CREATE TABLE sla_events (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	status sla_event_status DEFAULT 'pending' NOT NULL,
	applied_sla_id BIGINT REFERENCES applied_slas(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	sla_policy_id INT REFERENCES sla_policies(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	type sla_metric NOT NULL,
	deadline_at TIMESTAMPTZ NOT NULL,
	met_at TIMESTAMPTZ,
	breached_at TIMESTAMPTZ
);
CREATE INDEX index_sla_events_on_applied_sla_id ON sla_events(applied_sla_id);
CREATE INDEX index_sla_events_on_status ON sla_events(status);

DROP TABLE IF EXISTS scheduled_sla_notifications CASCADE;
CREATE TABLE scheduled_sla_notifications (
  id BIGSERIAL PRIMARY KEY,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  applied_sla_id BIGINT NOT NULL REFERENCES applied_slas(id) ON DELETE CASCADE,
  sla_event_id BIGINT REFERENCES sla_events(id) ON DELETE CASCADE,
  metric sla_metric NOT NULL,
  notification_type sla_notification_type NOT NULL,
  recipients TEXT[] NOT NULL,
  send_at TIMESTAMPTZ NOT NULL,
  processed_at TIMESTAMPTZ
);
CREATE INDEX index_scheduled_sla_notifications_on_send_at ON scheduled_sla_notifications(send_at);
CREATE INDEX index_scheduled_sla_notifications_on_processed_at ON scheduled_sla_notifications(processed_at);

DROP TABLE IF EXISTS ai_prompts CASCADE;
CREATE TABLE ai_prompts (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	title TEXT NOT NULL,
    key TEXT NOT NULL UNIQUE,
    content TEXT NOT NULL,
	CONSTRAINT constraint_prompts_on_title CHECK (length(title) <= 140),
    CONSTRAINT constraint_prompts_on_key CHECK (length(key) <= 140)
);
CREATE INDEX index_ai_prompts_on_key ON ai_prompts USING btree (key);

DROP TABLE IF EXISTS custom_attribute_definitions CASCADE;
CREATE TABLE custom_attribute_definitions (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	"name" TEXT NOT NULL,
	description TEXT NOT NULL,
	applies_to TEXT NOT NULL,
	key TEXT NOT NULL,
	values TEXT[] DEFAULT '{}'::TEXT[] NOT NULL,
	data_type TEXT NOT NULL,
	regex TEXT NULL,
	regex_hint TEXT NULL,
	CONSTRAINT constraint_custom_attribute_definitions_on_name CHECK (length("name") <= 140),
	CONSTRAINT constraint_custom_attribute_definitions_on_description CHECK (length(description) <= 300),
	CONSTRAINT constraint_custom_attribute_definitions_on_key CHECK (length(key) <= 140),
	CONSTRAINT constraint_custom_attribute_definitions_on_applies_to CHECK (length(applies_to) <= 50),
	CONSTRAINT constraint_custom_attribute_definitions_on_data_type CHECK (length(data_type) <= 100),
	CONSTRAINT constraint_custom_attribute_definitions_on_regex CHECK (length(regex) <= 1000),
	CONSTRAINT constraint_custom_attribute_definitions_on_regex_hint CHECK (length(regex_hint) <= 1000),
	CONSTRAINT constraint_custom_attribute_definitions_key_applies_to_unique UNIQUE (key, applies_to)
);

DROP TABLE IF EXISTS contact_notes CASCADE;
CREATE TABLE contact_notes (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	contact_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE,
	note TEXT NOT NULL,
	user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX index_contact_notes_on_contact_id_created_at ON contact_notes (contact_id, created_at);

DROP TABLE IF EXISTS activity_logs CASCADE;
CREATE TABLE activity_logs (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	activity_type activity_log_type NOT NULL,
	activity_description TEXT NOT NULL,
	actor_id INT REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	target_model_type TEXT NOT NULL,
	target_model_id BIGINT NOT NULL,
	ip INET
);
CREATE INDEX IF NOT EXISTS index_activity_logs_on_actor_id ON activity_logs (actor_id);
CREATE INDEX IF NOT EXISTS index_activity_logs_on_activity_type ON activity_logs (activity_type);
CREATE INDEX IF NOT EXISTS index_activity_logs_on_created_at ON activity_logs (created_at);

DROP TABLE IF EXISTS webhooks CASCADE;
CREATE TABLE webhooks (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	name TEXT NOT NULL,
	url TEXT NOT NULL,
	events webhook_event[] NOT NULL DEFAULT '{}',
	secret TEXT DEFAULT '',
	is_active BOOLEAN DEFAULT true,
	CONSTRAINT constraint_webhooks_on_name CHECK (length(name) <= 255),
	CONSTRAINT constraint_webhooks_on_url CHECK (length(url) <= 2048),
	CONSTRAINT constraint_webhooks_on_secret CHECK (length(secret) <= 255),
	CONSTRAINT constraint_webhooks_on_events_not_empty CHECK (array_length(events, 1) > 0)
);

DROP TABLE IF EXISTS user_notifications CASCADE;
CREATE TABLE user_notifications (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	user_id BIGINT REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	notification_type user_notification_type NOT NULL,
	title TEXT NOT NULL,
	body TEXT NULL,
	is_read BOOLEAN DEFAULT FALSE NOT NULL,
	conversation_id BIGINT REFERENCES conversations(id) ON DELETE CASCADE ON UPDATE CASCADE,
	message_id BIGINT REFERENCES conversation_messages(id) ON DELETE CASCADE ON UPDATE CASCADE,
	actor_id BIGINT REFERENCES users(id) ON DELETE SET NULL ON UPDATE CASCADE,
	meta JSONB DEFAULT '{}'::jsonb NOT NULL,
	CONSTRAINT constraint_user_notifications_on_title CHECK (length(title) <= 500),
	CONSTRAINT constraint_user_notifications_on_body CHECK (length(body) <= 2000)
);
CREATE INDEX index_user_notifications_on_user_id ON user_notifications(user_id);
CREATE INDEX index_user_notifications_on_user_id_is_read ON user_notifications(user_id, is_read);
CREATE INDEX index_user_notifications_on_created_at ON user_notifications(created_at);
CREATE INDEX index_user_notifications_on_conversation_id ON user_notifications(conversation_id);

DROP TABLE IF EXISTS worklogs CASCADE;
CREATE TABLE worklogs (
	id BIGSERIAL PRIMARY KEY,
	conversation_id BIGINT REFERENCES conversations(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	user_id BIGINT REFERENCES users(id) ON DELETE SET NULL ON UPDATE CASCADE NULL,
	started_at TIMESTAMPTZ NOT NULL,
	ended_at TIMESTAMPTZ NULL,
	duration_seconds INT NULL,
	action_type VARCHAR(32) NOT NULL,
	automation_level VARCHAR(2) NOT NULL DEFAULT 'L0',
	context JSONB NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	CONSTRAINT constraint_worklogs_on_action_type
		CHECK (action_type IN ('write', 'status', 'note', 'forward')),
	CONSTRAINT constraint_worklogs_on_automation_level
		CHECK (automation_level IN ('L0','L1','L2','L3','L4','L5')),
	CONSTRAINT constraint_worklogs_on_duration_non_negative
		CHECK (duration_seconds IS NULL OR duration_seconds >= 0),
	CONSTRAINT constraint_worklogs_on_ended_at_after_started_at
		CHECK (ended_at IS NULL OR ended_at >= started_at)
);
CREATE INDEX index_worklogs_on_conversation_id ON worklogs (conversation_id);
CREATE INDEX index_worklogs_on_user_id ON worklogs (user_id);
CREATE INDEX index_worklogs_on_started_at ON worklogs (started_at);
CREATE INDEX index_worklogs_on_action_type ON worklogs (action_type);

DROP TABLE IF EXISTS kb_articles CASCADE;
DROP TABLE IF EXISTS kb_folders CASCADE;
CREATE TABLE kb_folders (
	id BIGSERIAL PRIMARY KEY,
	parent_id BIGINT NULL REFERENCES kb_folders(id) ON DELETE RESTRICT ON UPDATE CASCADE,
	name VARCHAR(255) NOT NULL,
	slug VARCHAR(255) NOT NULL,
	sort_order INT NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX kb_folders_parent_slug_uniq
	ON kb_folders (parent_id, slug)
	WHERE parent_id IS NOT NULL;
CREATE UNIQUE INDEX kb_folders_root_slug_uniq
	ON kb_folders (slug)
	WHERE parent_id IS NULL;

CREATE TABLE kb_articles (
	id BIGSERIAL PRIMARY KEY,
	folder_id BIGINT NOT NULL REFERENCES kb_folders(id) ON DELETE RESTRICT ON UPDATE CASCADE,
	slug VARCHAR(255) NOT NULL,
	title VARCHAR(500) NOT NULL,
	locale VARCHAR(8) NOT NULL DEFAULT 'en',
	classification VARCHAR(32) NOT NULL,
	status VARCHAR(32) NOT NULL DEFAULT 'draft',
	-- is_ai_groundable: legacy flag retained for data compatibility. New
	-- indexing logic ignores it (F12 fork-5 resolution); agent kb_scope
	-- is the authoritative gate for what each agent can see.
	is_ai_groundable BOOLEAN NOT NULL DEFAULT FALSE,
	body_html TEXT NOT NULL DEFAULT '',
	body_tiptap_json JSONB NOT NULL DEFAULT '{}'::jsonb,
	body_markdown TEXT NOT NULL DEFAULT '',
	-- revision is bumped in Go on every UpdateArticle / PublishArticle.
	-- Workflow IDs `embed-article:<id>:v<revision>` derive idempotency from this.
	revision INT NOT NULL DEFAULT 1,
	-- body_tsvector is a locale-aware generated column powering the
	-- keyword half of hybrid search. Stemming dictionary is selected
	-- per row based on the article's locale.
	body_tsvector tsvector GENERATED ALWAYS AS (
		CASE locale
			WHEN 'de' THEN to_tsvector('german',
				coalesce(title, '') || ' ' || coalesce(body_markdown, ''))
			WHEN 'en' THEN to_tsvector('english',
				coalesce(title, '') || ' ' || coalesce(body_markdown, ''))
			ELSE to_tsvector('simple',
				coalesce(title, '') || ' ' || coalesce(body_markdown, ''))
		END
	) STORED,
	author_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
	published_at TIMESTAMPTZ NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	CONSTRAINT constraint_kb_articles_on_classification
		CHECK (classification IN ('faq', 'how_to', 'policy', 'news', 'other')),
	CONSTRAINT constraint_kb_articles_on_status
		CHECK (status IN ('draft', 'published')),
	CONSTRAINT kb_articles_folder_slug_locale_uniq
		UNIQUE (folder_id, slug, locale)
);
CREATE INDEX index_kb_articles_on_status_classification_locale
	ON kb_articles (status, classification, locale);
CREATE INDEX index_kb_articles_on_author_user_id
	ON kb_articles (author_user_id);
CREATE INDEX index_kb_articles_on_body_tsvector
	ON kb_articles USING GIN (body_tsvector);

-- F12 RAG foundation: pgvector extension + universal embeddings table +
-- per-article active-version pointer.
CREATE EXTENSION IF NOT EXISTS vector;

DROP TABLE IF EXISTS articles_index_state CASCADE;
DROP TABLE IF EXISTS embeddings CASCADE;

CREATE TABLE embeddings (
	id              BIGSERIAL PRIMARY KEY,
	content_type    TEXT NOT NULL,
	source_id       BIGINT NOT NULL,
	source_version  INT NOT NULL DEFAULT 1,
	chunk_index     INT NOT NULL DEFAULT 0,
	chunk_text      TEXT NOT NULL,
	content_hash    TEXT NOT NULL,
	embedding_model TEXT NOT NULL,
	embedding       halfvec(1024) NOT NULL,
	source_path     TEXT NOT NULL DEFAULT '',
	created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	CONSTRAINT constraint_embeddings_on_content_type
		CHECK (content_type IN ('kb_chunk', 'ticket', 'customer_note')),
	CONSTRAINT constraint_embeddings_on_source_version
		CHECK (source_version > 0),
	CONSTRAINT constraint_embeddings_on_chunk_index
		CHECK (chunk_index >= 0)
);
CREATE UNIQUE INDEX index_embeddings_on_content_source_version_chunk_model
	ON embeddings (content_type, source_id, source_version, chunk_index, embedding_model);
CREATE INDEX index_embeddings_on_vector
	ON embeddings USING hnsw (embedding halfvec_cosine_ops)
	WITH (m = 16, ef_construction = 64);
CREATE INDEX index_embeddings_on_content_source_version
	ON embeddings (content_type, source_id, source_version);
CREATE INDEX index_embeddings_on_model_created
	ON embeddings (embedding_model, created_at DESC);

CREATE TABLE articles_index_state (
	article_id       BIGINT PRIMARY KEY
		REFERENCES kb_articles(id) ON DELETE CASCADE,
	active_version   INT NOT NULL DEFAULT 1,
	active_model     TEXT NOT NULL,
	last_indexed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	is_stale         BOOLEAN NOT NULL DEFAULT FALSE,
	CONSTRAINT constraint_articles_index_state_on_active_version
		CHECK (active_version > 0)
);
CREATE INDEX index_articles_index_state_on_stale
	ON articles_index_state (is_stale, last_indexed_at)
	WHERE is_stale = TRUE;

DROP TABLE IF EXISTS ai_tools CASCADE;
DROP TABLE IF EXISTS ai_agents CASCADE;
DROP TABLE IF EXISTS functions CASCADE;
CREATE TABLE ai_agents (
	id                   SERIAL PRIMARY KEY,
	created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	name                 TEXT NOT NULL,
	slug                 TEXT NOT NULL,
	description          TEXT NULL,
	prompt               TEXT NOT NULL DEFAULT '',
	detection            JSONB NOT NULL DEFAULT '{"mode":"manual_only","rules":{"op":"AND","children":[]},"ai":{"prompt":""}}'::jsonb,
	tools                JSONB NOT NULL DEFAULT '[]'::jsonb,
	kb_scope             JSONB NOT NULL DEFAULT '{"folder_ids":[],"article_ids":[]}'::jsonb,
	preset_overrides     JSONB NOT NULL DEFAULT '{}'::jsonb,
	max_iterations       INT NOT NULL DEFAULT 5,
	extended_thinking_enabled BOOL NOT NULL DEFAULT TRUE,
	min_verification     INT NOT NULL DEFAULT 0,
	enabled              BOOL NOT NULL DEFAULT TRUE,
	CONSTRAINT constraint_ai_agents_on_name              CHECK (length(name) <= 140),
	CONSTRAINT constraint_ai_agents_on_slug              CHECK (length(slug) <= 140),
	CONSTRAINT constraint_ai_agents_on_description       CHECK (description IS NULL OR length(description) <= 1000),
	CONSTRAINT constraint_ai_agents_on_iterations        CHECK (max_iterations > 0 AND max_iterations <= 50),
	CONSTRAINT constraint_ai_agents_on_min_verification  CHECK (min_verification >= 0 AND min_verification <= 3),
	CONSTRAINT constraint_ai_agents_on_name_unique       UNIQUE (name),
	CONSTRAINT constraint_ai_agents_on_slug_unique       UNIQUE (slug)
);
CREATE INDEX index_ai_agents_on_enabled ON ai_agents(enabled) WHERE enabled = TRUE;

-- functions — the single, unified home for every named, sandbox-runnable
-- code blob (custom AI tools, customer-identification executors, future
-- domains). Created before ai_tools, which links to it via function_id.
CREATE TABLE functions (
    id              BIGSERIAL PRIMARY KEY,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    name            TEXT NOT NULL,
    domain          TEXT NOT NULL,

    code            TEXT NOT NULL DEFAULT '',
    language        TEXT NOT NULL DEFAULT 'python',
    packages        TEXT NOT NULL DEFAULT '',
    timeout_ms      INT  NOT NULL DEFAULT 30000,

    parameters      JSONB NOT NULL DEFAULT '[]'::jsonb,

    updated_by_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,

    CONSTRAINT constraint_functions_on_name_unique
        UNIQUE (name),
    CONSTRAINT check_functions_language
        CHECK (language IN ('python', 'javascript')),
    CONSTRAINT check_functions_timeout
        CHECK (timeout_ms > 0 AND timeout_ms <= 120000),
    CONSTRAINT check_functions_domain
        CHECK (domain IN ('ai_tool', 'customer_identification'))
);
CREATE INDEX index_functions_on_domain ON functions (domain);

-- Seed the 4 customer-identification executors as functions rows with
-- starter code and preview parameters. Operators replace TODOs with their
-- ERP calls. A function is callable once it exists with non-empty code.
INSERT INTO functions (name, domain, code, language, packages, timeout_ms, parameters)
VALUES
    ('search_customer_by_fields', 'customer_identification', $code$
import sdk
import requests

# Inputs come from the AI's parameter extraction (configured in
# the "AI-based identification" admin section). Any subset may be
# present; absent fields are None.
contract_id = sdk.input.get("contractId")
customer_id = sdk.input.get("customerId")
firstname = sdk.input.get("firstname")
lastname = sdk.input.get("lastname")
email = sdk.input.get("email")
phone = sdk.input.get("phone")
zip_code = sdk.input.get("zip")
city = sdk.input.get("city")
company = sdk.input.get("company")
ticket_id = sdk.input.get("ticketId")

# Reject calls with no useful inputs.
if not any([contract_id, customer_id, firstname, lastname, email, phone, company]):
    sdk.failure("no identifying attributes provided", code="MISSING_INPUT")
    raise SystemExit

# TODO: replace with your ERP endpoint + auth.
# Example shape - adjust to your API.
try:
    resp = requests.get(
        "https://your-erp.example.com/api/customers/search",
        params={
            "contractId": contract_id,
            "customerId": customer_id,
            "firstName": firstname,
            "lastName": lastname,
            "email": email,
        },
        timeout=10,
    )
except requests.RequestException as e:
    sdk.error(f"ERP unreachable: {e}", retryable=True)
    raise SystemExit

if resp.status_code == 404:
    sdk.failure("no customer matched the provided attributes", code="NOT_FOUND")
    raise SystemExit
if not resp.ok:
    sdk.error(f"ERP returned {resp.status_code}", retryable=resp.status_code >= 500)
    raise SystemExit

# Return AT MOST ONE match. If multiple candidates exist, return the
# most likely one. Empty {} is also a valid "no match" outcome via failure().
match = resp.json()

# Expected output shape:
#
#   {
#     "contractId": "<external contract id>",   # required
#     "customerId": "<external customer id>",   # required
#
#     # Optional override of the verification flow:
#     #   "customerVerification": 2,
#     #   "customerVerificationMessage": "<reason if not 2>",
#     # When set, the engine uses these values directly and skips
#     # the rule evaluation. Use sparingly.
#
#     # Optional preview shown to operators in the sidebar:
#     #   "agentPreview": [{"label": "...", "value": "...", "tooltip": null, "url": null}],
#   }

sdk.success(data=match)
$code$, 'python', '', 30000, '[
      {"id":"11111111-1111-4111-8111-000000000001","key":"contractId","value":""},
      {"id":"11111111-1111-4111-8111-000000000002","key":"customerId","value":""},
      {"id":"11111111-1111-4111-8111-000000000003","key":"firstname","value":""},
      {"id":"11111111-1111-4111-8111-000000000004","key":"lastname","value":""},
      {"id":"11111111-1111-4111-8111-000000000005","key":"email","value":""},
      {"id":"11111111-1111-4111-8111-000000000006","key":"phone","value":""},
      {"id":"11111111-1111-4111-8111-000000000007","key":"zip","value":""},
      {"id":"11111111-1111-4111-8111-000000000008","key":"city","value":""},
      {"id":"11111111-1111-4111-8111-000000000009","key":"company","value":""},
      {"id":"11111111-1111-4111-8111-000000000010","key":"ticketId","value":""}
    ]'::jsonb),
    ('search_contract_by_id', 'customer_identification', $code$
import sdk
import requests

contract_id = sdk.input.get("contractId")
if not contract_id:
    sdk.failure("contractId is required", code="MISSING_INPUT")
    raise SystemExit

# TODO: fetch the contract from your ERP by ID.
try:
    resp = requests.get(
        f"https://your-erp.example.com/api/contracts/{contract_id}",
        timeout=10,
    )
except requests.RequestException as e:
    sdk.error(f"ERP unreachable: {e}", retryable=True)
    raise SystemExit

if resp.status_code == 404:
    sdk.failure(f"contract {contract_id} not found", code="NO_CONTRACT")
    raise SystemExit
if not resp.ok:
    sdk.error(f"ERP returned {resp.status_code}", retryable=resp.status_code >= 500)
    raise SystemExit

contract = resp.json()

# Expected output shape - the full contract record:
#
#   {
#     "id": "<external contract id>",            # required (string or int)
#     "customerId": "<external customer id>",    # required (FK to the customer)
#     "business": false,                         # required (true if commercial)
#     "lastname": "<surname>",                   # required if business=false
#     "company": "<company name>",               # required if business=true
#     "firstname": "<given name>",               # optional
#     "email": "<contact email>",                # optional
#     "phone": "<contact phone>",                # optional
#     "deliveryAddress": "<full delivery address>",
#     "billingAddress": "<full billing address>",
#     "zip": "<delivery zip>",
#     "agentPreview": [{"label": "...", "value": "...", "tooltip": null, "url": null}],
#     "erpUrls": [{"label": "...", "url": "...", "icon": null}],
#     # Any additional fields are preserved verbatim and available
#     # to custom criteria via dot-notation (e.g., extra_data.malo).
#   }

sdk.success(data=contract)
$code$, 'python', '', 30000, '[
      {"id":"22222222-2222-4222-8222-000000000001","key":"contractId","value":""}
    ]'::jsonb),
    ('search_customer_by_id', 'customer_identification', $code$
import sdk
import requests

customer_id = sdk.input.get("customerId")
if not customer_id:
    sdk.failure("customerId is required", code="MISSING_INPUT")
    raise SystemExit

# TODO: fetch the customer from your ERP by ID.
try:
    resp = requests.get(
        f"https://your-erp.example.com/api/customers/{customer_id}",
        timeout=10,
    )
except requests.RequestException as e:
    sdk.error(f"ERP unreachable: {e}", retryable=True)
    raise SystemExit

if resp.status_code == 404:
    sdk.failure(f"customer {customer_id} not found", code="NO_CUSTOMER")
    raise SystemExit
if not resp.ok:
    sdk.error(f"ERP returned {resp.status_code}", retryable=resp.status_code >= 500)
    raise SystemExit

customer = resp.json()

# Expected output shape - the full customer record:
#
#   {
#     "id": "<external customer id>",            # required
#     "business": false,                         # required (true if commercial)
#     "lastname": "<surname>",                   # required if business=false
#     "company": "<company name>",               # required if business=true
#     "contractIds": ["<id>", "<id>"],           # required; at least one
#     "firstname": "<given name>",               # optional
#     "email": "<contact email>",                # optional
#     "phone": "<contact phone>",                # optional
#     "deliveryAddress": "<full delivery address>",
#     "billingAddress": "<full billing address>",
#   }

sdk.success(data=customer)
$code$, 'python', '', 30000, '[
      {"id":"33333333-3333-4333-8333-000000000001","key":"customerId","value":""}
    ]'::jsonb),
    ('search_customer_by_free_text', 'customer_identification', $code$
import sdk
import requests

q = sdk.input.get("q", "")
offset = int(sdk.input.get("offset", 0))
limit = int(sdk.input.get("limit", 10))

if not q.strip():
    sdk.failure("search query is empty", code="MISSING_INPUT")
    raise SystemExit

# TODO: free-text search across customers using q.
# Used by the agent search bar at the top of the screen.
try:
    resp = requests.get(
        "https://your-erp.example.com/api/customers/search",
        params={"q": q, "offset": offset, "limit": limit},
        timeout=10,
    )
except requests.RequestException as e:
    sdk.error(f"ERP unreachable: {e}", retryable=True)
    raise SystemExit

if not resp.ok:
    sdk.error(f"ERP returned {resp.status_code}", retryable=resp.status_code >= 500)
    raise SystemExit

# Expected output shape - array of partial customer records.
# merio will fetch full details for each via search_customer_by_id,
# so this only needs to return the IDs:
#
#   [ { "id": "<external customer id>" }, ... ]

results = resp.json()
sdk.success(data=results)
$code$, 'python', '', 30000, '[
      {"id":"44444444-4444-4444-8444-000000000001","key":"q","value":""},
      {"id":"44444444-4444-4444-8444-000000000002","key":"offset","value":"0"},
      {"id":"44444444-4444-4444-8444-000000000003","key":"limit","value":"10"}
    ]'::jsonb)
ON CONFLICT (name) DO NOTHING;

CREATE TABLE ai_tools (
	id                SERIAL PRIMARY KEY,
	created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	name              TEXT NOT NULL,
	slug              TEXT NOT NULL,
	description       TEXT NULL,
	input_parameters  JSONB NOT NULL DEFAULT '[]'::jsonb,
	output_schema     JSONB NOT NULL DEFAULT '{}'::jsonb,
	category          TEXT NOT NULL DEFAULT 'write',
	timeout_ms        INT NOT NULL DEFAULT 30000,
	async             BOOL NOT NULL DEFAULT FALSE,
	enabled           BOOL NOT NULL DEFAULT TRUE,
	function_id       BIGINT NOT NULL REFERENCES functions(id) ON DELETE RESTRICT,
	CONSTRAINT constraint_ai_tools_on_name        CHECK (length(name) <= 140),
	CONSTRAINT constraint_ai_tools_on_slug        CHECK (length(slug) <= 140),
	CONSTRAINT constraint_ai_tools_on_description CHECK (description IS NULL OR length(description) <= 1000),
	CONSTRAINT constraint_ai_tools_on_category    CHECK (category IN ('read', 'write')),
	CONSTRAINT constraint_ai_tools_on_timeout     CHECK (timeout_ms > 0 AND timeout_ms <= 120000),
	CONSTRAINT constraint_ai_tools_on_name_unique UNIQUE (name),
	CONSTRAINT constraint_ai_tools_on_slug_unique UNIQUE (slug)
);
CREATE INDEX index_ai_tools_on_enabled ON ai_tools(enabled) WHERE enabled = TRUE;

-- Seed data — one stub agent (disabled). The F9 kb_search_universal
-- ai_tools row was removed in the F5 schema cleanup; kb_search now lives
-- in code under internal/ai/tools/builtin/.
INSERT INTO ai_agents (
	name, slug, description, prompt, detection, tools, kb_scope,
	preset_overrides, max_iterations, min_verification, enabled
) VALUES (
	'Test Agent',
	'test-agent',
	'Stub agent — disabled. Edit prompt and enable when ready.',
	'',
	'{"mode":"manual_only","rules":{"op":"AND","children":[]},"ai":{"prompt":""}}'::jsonb,
	'[]'::jsonb,
	'{"folder_ids":[],"article_ids":[]}'::jsonb,
	'{}'::jsonb,
	5,
	0,
	FALSE
) ON CONFLICT (slug) DO NOTHING;

DROP TABLE IF EXISTS function_executions CASCADE;
CREATE TABLE function_executions (
	execution_id     TEXT PRIMARY KEY,
	tool_name        TEXT,
	invoker_user_id  BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
	conversation_id  BIGINT REFERENCES conversations(id) ON DELETE SET NULL,
	tool_depth       INT NOT NULL DEFAULT 0,
	language         TEXT NOT NULL,
	started_at       TIMESTAMPTZ NOT NULL,
	finished_at      TIMESTAMPTZ,
	duration_ms      INT NOT NULL DEFAULT 0,
	kind             TEXT NOT NULL CHECK (kind IN ('success','failure','error','noop')),
	result_code      TEXT,
	message          TEXT,
	data             JSONB,
	warm_pool_hit    BOOLEAN NOT NULL DEFAULT FALSE,
	trace_id         TEXT,
	created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_function_executions_tool_name
	ON function_executions(tool_name) WHERE tool_name IS NOT NULL;
CREATE INDEX idx_function_executions_conversation_id
	ON function_executions(conversation_id) WHERE conversation_id IS NOT NULL;
CREATE INDEX idx_function_executions_started_at
	ON function_executions(started_at DESC);
CREATE INDEX idx_function_executions_invoker_user_id
	ON function_executions(invoker_user_id);

-- Customer / contract identification (Wave 3 Batch 10). customers and
-- contracts are caches of ERP-resolved records; last_synced_at drives the
-- TTL. The conversations identification columns are added here (after both
-- tables exist) to mirror the current baseline state exactly.
CREATE TABLE customers (
    id              BIGSERIAL PRIMARY KEY,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_synced_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    customer_id     TEXT NOT NULL,
    business        BOOLEAN NOT NULL,

    firstname       TEXT NULL,
    lastname        TEXT NULL,
    company         TEXT NULL,
    email           TEXT NULL,
    phone           TEXT NULL,
    zip             TEXT NULL,
    delivery_address TEXT NULL,
    billing_address TEXT NULL,

    contract_ids    JSON NOT NULL DEFAULT '[]'::json,
    extra_data      JSON NOT NULL DEFAULT '{}'::json,

    CONSTRAINT constraint_customers_on_customer_id_unique
        UNIQUE (customer_id),
    CONSTRAINT constraint_customers_on_business_consistency
        CHECK (
            (business = TRUE  AND company IS NOT NULL) OR
            (business = FALSE AND lastname IS NOT NULL)
        )
);
CREATE INDEX index_customers_on_email ON customers (email) WHERE email IS NOT NULL;
CREATE INDEX index_customers_on_phone ON customers (phone) WHERE phone IS NOT NULL;
CREATE INDEX index_customers_on_zip ON customers (zip) WHERE zip IS NOT NULL;
CREATE INDEX index_customers_on_last_synced_at ON customers (last_synced_at);

ALTER TABLE users
    ADD CONSTRAINT users_linked_customer_id_fkey
    FOREIGN KEY (linked_customer_id)
    REFERENCES customers(customer_id)
    ON UPDATE CASCADE
    ON DELETE SET NULL;
CREATE INDEX index_users_on_linked_customer_id
    ON users(linked_customer_id)
    WHERE linked_customer_id IS NOT NULL;

CREATE TABLE contracts (
    id              BIGSERIAL PRIMARY KEY,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_synced_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    contract_id     TEXT NOT NULL,
    customer_id     TEXT NOT NULL REFERENCES customers(customer_id) ON UPDATE CASCADE ON DELETE CASCADE,
    business        BOOLEAN NOT NULL,

    firstname       TEXT NULL,
    lastname        TEXT NULL,
    company         TEXT NULL,
    email           TEXT NULL,
    phone           TEXT NULL,
    zip             TEXT NULL,
    delivery_address TEXT NULL,
    billing_address TEXT NULL,

    agent_preview   JSONB NOT NULL DEFAULT '[]'::jsonb,
    erp_urls        JSONB NOT NULL DEFAULT '[]'::jsonb,
    extra_data      JSON NOT NULL DEFAULT '{}'::json,

    CONSTRAINT constraint_contracts_on_contract_id_unique
        UNIQUE (contract_id),
    CONSTRAINT constraint_contracts_on_business_consistency
        CHECK (
            (business = TRUE  AND company IS NOT NULL) OR
            (business = FALSE AND lastname IS NOT NULL)
        )
);
CREATE INDEX index_contracts_on_customer_id ON contracts (customer_id);
CREATE INDEX index_contracts_on_email ON contracts (email) WHERE email IS NOT NULL;
CREATE INDEX index_contracts_on_phone ON contracts (phone) WHERE phone IS NOT NULL;
CREATE INDEX index_contracts_on_zip ON contracts (zip) WHERE zip IS NOT NULL;
CREATE INDEX index_contracts_on_last_synced_at ON contracts (last_synced_at);

ALTER TABLE conversations
    ADD COLUMN customer_id          TEXT     NULL REFERENCES customers(customer_id) ON UPDATE CASCADE ON DELETE SET NULL,
    ADD COLUMN contract_id          TEXT     NULL REFERENCES contracts(contract_id) ON UPDATE CASCADE ON DELETE SET NULL,
    ADD COLUMN verification_level   SMALLINT NOT NULL DEFAULT 0,
    ADD COLUMN verification_message TEXT     NULL;

CREATE INDEX index_conversations_on_customer_id
    ON conversations (customer_id) WHERE customer_id IS NOT NULL;
CREATE INDEX index_conversations_on_contract_id
    ON conversations (contract_id) WHERE contract_id IS NOT NULL;

-- agent_executions — immutable record of every agent run on a
-- conversation. agent_execution_outcomes — at-most-one commitment per
-- run (sent / drafted / escalated / discarded).
DROP TABLE IF EXISTS agent_execution_outcomes CASCADE;
DROP TABLE IF EXISTS agent_executions CASCADE;
CREATE TABLE agent_executions (
	id                    BIGSERIAL PRIMARY KEY,
	conversation_id       BIGINT NOT NULL
	                      REFERENCES conversations(id) ON DELETE CASCADE,
	agent_id              INT NOT NULL
	                      REFERENCES ai_agents(id) ON DELETE RESTRICT,
	source                TEXT NOT NULL,
	resolution_attempt_id INT NULL,
	trace_id              TEXT NOT NULL DEFAULT '',
	format_version        TEXT NOT NULL DEFAULT '2',
	status                TEXT NOT NULL,
	draft                 TEXT,
	trace                 JSONB NOT NULL DEFAULT '[]'::jsonb,
	usage                 JSONB NOT NULL DEFAULT '{}'::jsonb,
	duration_ms           INT NOT NULL DEFAULT 0,
	run_by_user_id        BIGINT NULL
	                      REFERENCES users(id) ON DELETE SET NULL,
	run_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	agent_updated_at      TIMESTAMPTZ NOT NULL,
	created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	CONSTRAINT constraint_agent_executions_on_source
	    CHECK (source IN ('composer', 'pipeline', 'subagent'))
);
CREATE INDEX index_agent_executions_on_conv_agent_run_at
	ON agent_executions (conversation_id, agent_id, run_at DESC);
CREATE INDEX index_agent_executions_on_source_run_at
	ON agent_executions (source, run_at DESC);

CREATE TABLE agent_execution_outcomes (
	id                   BIGSERIAL PRIMARY KEY,
	agent_execution_id   BIGINT NOT NULL UNIQUE
	                     REFERENCES agent_executions(id) ON DELETE CASCADE,
	outcome              TEXT NOT NULL,
	outcome_reason       TEXT NULL,
	message_id           BIGINT NULL
	                     REFERENCES conversation_messages(id) ON DELETE SET NULL,
	committed_by_user_id BIGINT NULL
	                     REFERENCES users(id) ON DELETE SET NULL,
	committed_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	CONSTRAINT constraint_agent_execution_outcomes_on_outcome
	    CHECK (outcome IN ('sent', 'drafted', 'escalated', 'discarded'))
);
CREATE INDEX index_agent_execution_outcomes_on_outcome_committed_at
	ON agent_execution_outcomes (outcome, committed_at DESC);

-- Composer config — the single GLOBAL configuration for the Composer (the
-- runtime that calls sub-agents and decides answer/escalate). Holds situational
-- behavior layered on the platform-owned Composer persona: guidance items,
-- automation switches, escalation rules + guidance. One row only (id=1 CHECK);
-- per-inbox differences are inbox scope on individual items (Model A), not
-- separate config objects. Sections are JSONB, decoded to typed structs at the
-- handler/runtime boundary (internal/composer).
CREATE TABLE composer_config (
	id          INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
	guidance    JSONB NOT NULL DEFAULT '[]'::jsonb,
	automation  JSONB NOT NULL DEFAULT '{"auto_send":false,"approval_required_tool_slugs":[]}'::jsonb,
	escalation  JSONB NOT NULL DEFAULT '{"rules":[],"guidance":[]}'::jsonb,
	created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO composer_config (id) VALUES (1) ON CONFLICT (id) DO NOTHING;

-- Agent write approvals — durable record of a proposed write tool call awaiting
-- human approval (run-ends-at-write). References use FK ids (agent_id, tool_id);
-- the gate + LLM tool call stay slug-driven. gated_run_trace_id is the run this
-- approval gates; parent_trace_id is the explicit composer run a sub-agent gate
-- belongs to (NULL when the composer itself gated). The atomic status
-- transition (pending → resolved) is the idempotency lock so the write executes
-- at most once, in the resuming run. See internal/approvals.
CREATE TABLE agent_write_approvals (
	id                   BIGSERIAL PRIMARY KEY,
	conversation_id      BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
	gated_run_trace_id   TEXT NOT NULL,
	parent_trace_id      TEXT NULL,
	agent_id             INT NULL REFERENCES ai_agents(id) ON DELETE SET NULL,
	tool_id              INT NOT NULL REFERENCES ai_tools(id) ON DELETE CASCADE,
	tool_use_id          TEXT NOT NULL,
	proposed_inputs      JSONB NOT NULL DEFAULT '{}'::jsonb,
	status               TEXT NOT NULL DEFAULT 'pending',
	approved_inputs      JSONB NULL,
	rejection_reason     TEXT NULL,
	reviewed_by_user_id  BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
	resolved_at          TIMESTAMPTZ NULL,
	created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	CONSTRAINT constraint_agent_write_approvals_on_status
	    CHECK (status IN ('pending', 'approved', 'edited', 'rejected'))
);
CREATE INDEX index_agent_write_approvals_on_conversation
	ON agent_write_approvals (conversation_id, status);
CREATE UNIQUE INDEX index_agent_write_approvals_one_pending_per_run
	ON agent_write_approvals (gated_run_trace_id) WHERE status = 'pending';

-- System (Internal) inbox — sentinel FK target for ai_usage rows that
-- have no real inbox context (rewrite manager calls, AgentTestPanel
-- synthetic runs, future internal jobs). Channel is 'email' because
-- the enum has no 'system' value; enabled=false keeps it out of inbox
-- pickers. The UUID is stable and resolved at app startup; see
-- internal/migrations/v0_1_0.go::SystemInboxUUID.
INSERT INTO inboxes (uuid, name, channel, enabled, config, "from")
VALUES ('00000000-0000-0000-0000-000000000001', 'System (Internal)', 'email', FALSE, '{}'::jsonb, '')
ON CONFLICT (uuid) DO NOTHING;

-- ai_usage — one row per LLM provider call. Partitioned by month on
-- created_at; the application's partition manager keeps the runway
-- extended. Indexes declared on the parent; PG14+ propagates to every
-- child partition automatically.
DROP TABLE IF EXISTS ai_usage CASCADE;
CREATE TABLE ai_usage (
	id                      BIGSERIAL NOT NULL,
	schema_version          SMALLINT NOT NULL DEFAULT 1,
	created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	trace_id                TEXT NOT NULL,
	call_sequence_in_trace  INT NOT NULL,
	inbox_id                INT NOT NULL,
	conversation_id         BIGINT NULL,
	agent_id                INT NULL,
	agent_execution_id      BIGINT NULL,
	resolution_attempt_id   INT NULL,
	user_id                 BIGINT NULL,
	provider                TEXT NOT NULL,
	model                   TEXT NOT NULL,
	preset_slug             TEXT NOT NULL,
	purpose                 TEXT NOT NULL,
	stop_reason             TEXT NOT NULL DEFAULT '',
	input_tokens            INT NOT NULL DEFAULT 0,
	output_tokens           INT NOT NULL DEFAULT 0,
	cached_input_tokens     INT NOT NULL DEFAULT 0,
	cache_markers_used      INT NOT NULL DEFAULT 0,
	input_cost_usd          NUMERIC(12, 8) NOT NULL DEFAULT 0,
	output_cost_usd         NUMERIC(12, 8) NOT NULL DEFAULT 0,
	cached_input_cost_usd   NUMERIC(12, 8) NOT NULL DEFAULT 0,
	total_cost_usd          NUMERIC(12, 8) NOT NULL DEFAULT 0,
	latency_ms              INT NOT NULL DEFAULT 0,
	provider_latency_ms     INT NULL,
	retry_count             INT NOT NULL DEFAULT 0,
	outcome                 TEXT NOT NULL,
	error_class             TEXT NULL,
	error_message           TEXT NULL,
	provider_request_id     TEXT NULL,
	provider_metadata       JSONB NULL,
	request_payload         JSONB NULL,
	response_payload        JSONB NULL,
	structured_output       JSONB NULL,
	CONSTRAINT constraint_ai_usage_on_call_sequence CHECK (call_sequence_in_trace > 0),
	CONSTRAINT constraint_ai_usage_on_input_tokens  CHECK (input_tokens >= 0),
	CONSTRAINT constraint_ai_usage_on_output_tokens CHECK (output_tokens >= 0),
	CONSTRAINT constraint_ai_usage_on_cached_tokens CHECK (cached_input_tokens >= 0),
	CONSTRAINT constraint_ai_usage_on_latency_ms    CHECK (latency_ms >= 0),
	CONSTRAINT constraint_ai_usage_on_retry_count   CHECK (retry_count >= 0),
	CONSTRAINT constraint_ai_usage_on_total_cost    CHECK (total_cost_usd >= 0),
	CONSTRAINT constraint_ai_usage_on_outcome CHECK (
	    outcome IN ('success', 'timeout', 'auth_error', 'rate_limited',
	                'circuit_open', 'invalid_request', 'other')
	)
) PARTITION BY RANGE (created_at);

ALTER TABLE ai_usage ADD CONSTRAINT fk_ai_usage_on_inbox
	FOREIGN KEY (inbox_id) REFERENCES inboxes(id) ON DELETE RESTRICT;
ALTER TABLE ai_usage ADD CONSTRAINT fk_ai_usage_on_conversation
	FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE SET NULL;
ALTER TABLE ai_usage ADD CONSTRAINT fk_ai_usage_on_agent
	FOREIGN KEY (agent_id) REFERENCES ai_agents(id) ON DELETE SET NULL;
ALTER TABLE ai_usage ADD CONSTRAINT fk_ai_usage_on_agent_execution
	FOREIGN KEY (agent_execution_id) REFERENCES agent_executions(id) ON DELETE SET NULL;
ALTER TABLE ai_usage ADD CONSTRAINT fk_ai_usage_on_user
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX index_ai_usage_on_trace_seq
	ON ai_usage (trace_id, call_sequence_in_trace, created_at);
CREATE INDEX index_ai_usage_on_created_at
	ON ai_usage (created_at DESC);
CREATE INDEX index_ai_usage_on_inbox_created
	ON ai_usage (inbox_id, created_at DESC);
CREATE INDEX index_ai_usage_on_agent_created
	ON ai_usage (agent_id, created_at DESC) WHERE agent_id IS NOT NULL;
CREATE INDEX index_ai_usage_on_agent_execution
	ON ai_usage (agent_execution_id) WHERE agent_execution_id IS NOT NULL;
CREATE INDEX index_ai_usage_on_trace
	ON ai_usage (trace_id);
CREATE INDEX index_ai_usage_on_purpose_model_created
	ON ai_usage (purpose, model, created_at DESC);
CREATE INDEX index_ai_usage_on_errors
	ON ai_usage (outcome, created_at DESC) WHERE outcome != 'success';
CREATE INDEX index_ai_usage_on_provider_model
	ON ai_usage (provider, model, created_at DESC);
-- Cross-feature "all AI calls on conversation X" lookups (Batch 9).
CREATE INDEX index_ai_usage_on_conversation_id_created_at
	ON ai_usage (conversation_id, created_at DESC)
	WHERE conversation_id IS NOT NULL;

-- Default AI prompts
INSERT INTO ai_prompts ("key", "content", title)
VALUES
('make_friendly', 'Modify the text to make it more friendly and approachable.', 'Make Friendly'),
('make_concise', 'Simplify the text to make it more concise and to the point.', 'Make Concise'),
('add_empathy', 'Add empathy to the text while retaining the original meaning.', 'Add Empathy'),
('adjust_positive_tone', 'Adjust the tone of the text to make it sound more positive and reassuring.', 'Adjust Positive Tone'),
('make_professional', 'Rephrase the text to make it sound more formal and professional and to the point.', 'Make Professional');

-- Default settings
INSERT INTO settings ("key", value)
VALUES
    ('app.lang', '"en"'::jsonb),
    ('app.root_url', '"http://localhost:9000"'::jsonb),
    ('app.logo_url', '"http://localhost:9000/logo.png"'::jsonb),
    ('app.site_name', '"merio"'::jsonb),
    ('app.favicon_url', '"http://localhost:9000/favicon.ico"'::jsonb),
    ('app.max_file_upload_size', '20'::jsonb),
    ('app.allowed_file_upload_extensions', '["*"]'::jsonb),
	('app.timezone', '"Asia/Kolkata"'::jsonb),
	('app.business_hours_id', '""'::jsonb),
    ('notification.email.username', '"admin@yourcompany.com"'::jsonb),
    ('notification.email.host', '""'::jsonb),
    ('notification.email.port', '587'::jsonb),
    ('notification.email.password', '""'::jsonb),
    ('notification.email.max_conns', '5'::jsonb),
    ('notification.email.idle_timeout', '"25s"'::jsonb),
    ('notification.email.wait_timeout', '"60s"'::jsonb),
    ('notification.email.auth_protocol', '"plain"'::jsonb),
	('notification.email.tls_type', '"starttls"'::jsonb),
	('notification.email.tls_skip_verify', 'false'::jsonb),
	('notification.email.hello_hostname', '""'::jsonb),
    ('notification.email.email_address', '"admin@yourcompany.com"'::jsonb),
    ('notification.email.max_msg_retries', '3'::jsonb),
    ('notification.email.enabled', 'false'::jsonb),
    ('migrations', '["v0.1.0"]'::jsonb);

-- Customer / contract identification settings (Wave 3 Batch 10). 8 rows
-- under the customer_identification.* namespace. Operators populate
-- regexes, excluded domains, and rules via the admin UI on install.
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
    ('customer_identification.verification_rules_async', '{
        "email": {
            "groups": [
                {
                    "label": "ID mentioned",
                    "min_count": 1,
                    "criteria": [
                        {"type": "standard", "name": "contract_id_mentioned"},
                        {"type": "standard", "name": "customer_id_mentioned"}
                    ],
                    "custom_criteria": []
                },
                {
                    "label": "Sender email matches",
                    "min_count": 1,
                    "criteria": [
                        {"type": "standard", "name": "sender_email_matches_erp"}
                    ],
                    "custom_criteria": []
                }
            ]
        }
    }'::jsonb);

-- Default conversation priorities
INSERT INTO conversation_priorities (name) VALUES
('Low'),
('Medium'),
('High'),
('Urgent');

-- Default conversation statuses
INSERT INTO conversation_statuses (name, category, color, icon) VALUES
('Open', 'open', 'blue', 'CircleDot'),
('Snoozed', 'waiting', 'yellow', 'Moon'),
('Resolved', 'resolved', 'green', 'CircleCheck'),
('Closed', 'resolved', 'grey', 'CircleSlash');

-- Default roles
INSERT INTO
	roles ("name", description, permissions)
VALUES
	(
		'Agent',
		'Role for all agents with limited access to conversations.',
		'{conversations:read_all,conversations:read_unassigned,conversations:read_assigned,conversations:read_team_inbox,conversations:read_team_all,conversations:read,conversations:update_user_assignee,conversations:update_team_assignee,conversations:update_priority,conversations:update_status,conversations:update_tags,messages:read,messages:write,view:manage,worklog:view_own,knowledge:read,customers:assign,customers:search}'
	);

INSERT INTO
	roles ("name", description, permissions)
VALUES
	(
		'Admin',
		'Role for users who have complete access to everything.',
		'{webhooks:manage,activity_logs:manage,custom_attributes:manage,contacts:read_all,contacts:read,contacts:write,contacts:block,contact_notes:read,contact_notes:write,contact_notes:delete,conversations:write,ai:manage,general_settings:manage,notification_settings:manage,oidc:manage,conversations:read_all,conversations:read_unassigned,conversations:read_assigned,conversations:read_team_inbox,conversations:read_team_all,conversations:read,conversations:update_user_assignee,conversations:update_team_assignee,conversations:update_priority,conversations:update_status,conversations:update_tags,messages:read,messages:write,view:manage,shared_views:manage,status:manage,tags:manage,tags:view_private,macros:manage,users:manage,teams:manage,automations:manage,inboxes:manage,roles:manage,reports:manage,templates:manage,business_hours:manage,sla:manage,worklog:view_own,worklog:view_team,worklog:view_all,knowledge:read,knowledge:write,knowledge:publish,knowledge:delete,tools:call,ai_agents:manage,ai_agents:run,ai_tools:manage,ai_tools:invoke,customers:assign,customers:search,customers:manage,functions:manage,secrets:manage}'
	);


-- System Agent user — script-execution callbacks act as this user when
-- a script opts into system identity. Identified by stable reserved email.
INSERT INTO users (type, enabled, is_system, email, first_name, last_name, password)
SELECT 'agent', TRUE, TRUE, 'system@merio.local', 'System Agent', '', NULL
WHERE NOT EXISTS (
	SELECT 1 FROM users
	WHERE email = 'system@merio.local'
	  AND type = 'agent' AND deleted_at IS NULL
);

INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id
FROM   users u, roles r
WHERE  u.email = 'system@merio.local'
  AND  u.type = 'agent' AND u.deleted_at IS NULL
  AND  r.name = 'Admin';


-- Email notification templates
INSERT INTO templates
("type", body, is_default, "name", subject, is_builtin)
VALUES('email_notification'::template_type, '
<p>A new conversation has been assigned to you:</p>

<div>
    Reference number: {{ .Conversation.ReferenceNumber }} <br>
    Subject: {{ .Conversation.Subject }}
</div>

<p>
    <a href="{{ RootURL }}/inboxes/assigned/conversation/{{ .Conversation.UUID }}">View Conversation</a>
</p>

<div>
    Best regards,<br>
    Merio
</div>

', false, 'Conversation assigned', 'New conversation assigned to you', true);

INSERT INTO templates
("type", body, is_default, "name", subject, is_builtin)
VALUES (
  'email_notification'::template_type,
  '

<p>This is a notification that the SLA for conversation {{ .Conversation.ReferenceNumber }} is approaching the SLA deadline for {{ .SLA.Metric }}.</p>

<p>
  Details:<br>
  - Conversation reference number: {{ .Conversation.ReferenceNumber }}<br>
  - Metric: {{ .SLA.Metric }}<br>
  - Due in: {{ .SLA.DueIn }}
</p>

<p>
    <a href="{{ RootURL }}/inboxes/assigned/conversation/{{ .Conversation.UUID }}">View Conversation</a>
</p>


<p>
  Best regards,<br>
  Merio
</p>

',
  false,
  'SLA breach warning',
  'SLA Alert: Conversation {{ .Conversation.ReferenceNumber }} is approaching SLA deadline for {{ .SLA.Metric }}',
  true
);

INSERT INTO templates
("type", body, is_default, "name", subject, is_builtin)
VALUES (
  'email_notification'::template_type,
  '
<p>This is an urgent alert that the SLA for conversation {{ .Conversation.ReferenceNumber }} has been breached for {{ .SLA.Metric }}. Please take immediate action.</p>

<p>
  Details:<br>
  - Conversation reference number: {{ .Conversation.ReferenceNumber }}<br>
  - Metric: {{ .SLA.Metric }}<br>
  - Overdue by: {{ .SLA.OverdueBy }}
</p>

<p>
    <a href="{{ RootURL }}/inboxes/assigned/conversation/{{ .Conversation.UUID }}">View Conversation</a>
</p>


<p>
  Best regards,<br>
  Merio
</p>

',
  false,
  'SLA breached',
  'Urgent: SLA Breach for Conversation {{ .Conversation.ReferenceNumber }} for {{ .SLA.Metric }}',
  true
);

INSERT INTO templates
("type", body, is_default, "name", subject, is_builtin)
VALUES (
  'email_notification'::template_type,
  '
<p>{{ .MentionedBy.FullName }} mentioned you in a private note on conversation #{{ .Conversation.ReferenceNumber }}.</p>

<blockquote style="background-color: #f5f5f5; padding: 12px; margin: 16px 0; border-left: 4px solid #ddd;">
{{ .Message.Content }}
</blockquote>

<p>
<a href="{{ RootURL }}/inboxes/mentioned/conversation/{{ .Conversation.UUID }}?scrollTo={{ .Message.UUID }}">View Conversation</a>
</p>

<p>
Best regards,<br>
merio
</p>
',
  false,
  'Mentioned in conversation',
  '{{ .MentionedBy.FullName }} mentioned you in conversation #{{ .Conversation.ReferenceNumber }}',
  true
);

INSERT INTO templates
("type", body, is_default, "name", subject, is_builtin)
VALUES (
  'email_notification'::template_type,
  '
<p style="margin: 0 0 4px; font-size: 15px; color: #374151; text-align: center; line-height: 1.5;">
  Your conversation <strong style="color: #111827;">#{{ .Conversation.ReferenceNumber }}</strong> has been resolved.
</p>
<p style="margin: 0 0 28px; font-size: 13px; color: #9ca3af; text-align: center;">
  We would love to hear how it went.
</p>
<p style="margin: 0 0 20px; font-size: 14px; font-weight: 600; color: #374151; text-align: center;">
  How would you rate your experience?
</p>
<!-- Variable CSATUUID is also available -->
<div style="text-align: center; margin: 0 auto; max-width: 400px; font-size: 0;">
  <div style="display: inline-block; width: 72px; text-align: center; vertical-align: top; padding: 4px 0;">
    <a href="{{ .CSATLink }}?rating=1" style="text-decoration: none; display: block;">
      <span style="font-size: 34px; display: block; line-height: 1.4;">&#128546;</span>
      <span style="font-size: 10px; display: block; font-weight: 600; color: #b0b5bd; text-transform: uppercase; letter-spacing: 0.05em; margin-top: 4px;">Poor</span>
    </a>
  </div>
  <div style="display: inline-block; width: 72px; text-align: center; vertical-align: top; padding: 4px 0;">
    <a href="{{ .CSATLink }}?rating=2" style="text-decoration: none; display: block;">
      <span style="font-size: 34px; display: block; line-height: 1.4;">&#128533;</span>
      <span style="font-size: 10px; display: block; font-weight: 600; color: #b0b5bd; text-transform: uppercase; letter-spacing: 0.05em; margin-top: 4px;">Fair</span>
    </a>
  </div>
  <div style="display: inline-block; width: 72px; text-align: center; vertical-align: top; padding: 4px 0;">
    <a href="{{ .CSATLink }}?rating=3" style="text-decoration: none; display: block;">
      <span style="font-size: 34px; display: block; line-height: 1.4;">&#128522;</span>
      <span style="font-size: 10px; display: block; font-weight: 600; color: #b0b5bd; text-transform: uppercase; letter-spacing: 0.05em; margin-top: 4px;">Good</span>
    </a>
  </div>
  <div style="display: inline-block; width: 72px; text-align: center; vertical-align: top; padding: 4px 0;">
    <a href="{{ .CSATLink }}?rating=4" style="text-decoration: none; display: block;">
      <span style="font-size: 34px; display: block; line-height: 1.4;">&#128515;</span>
      <span style="font-size: 10px; display: block; font-weight: 600; color: #b0b5bd; text-transform: uppercase; letter-spacing: 0.05em; margin-top: 4px;">Great</span>
    </a>
  </div>
  <div style="display: inline-block; width: 72px; text-align: center; vertical-align: top; padding: 4px 0;">
    <a href="{{ .CSATLink }}?rating=5" style="text-decoration: none; display: block;">
      <span style="font-size: 34px; display: block; line-height: 1.4;">&#129321;</span>
      <span style="font-size: 10px; display: block; font-weight: 600; color: #b0b5bd; text-transform: uppercase; letter-spacing: 0.05em; margin-top: 4px;">Excellent</span>
    </a>
  </div>
</div>
',
  false,
  'CSAT request',
  '',
  true
);
