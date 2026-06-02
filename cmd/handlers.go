package main

import (
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aziz-tb/merio/internal/envelope"
	"github.com/aziz-tb/merio/internal/ws"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

const maxPageSize = 500

// initHandlers initializes the HTTP routes and handlers for the application.
func initHandlers(g *fastglue.Fastglue, hub *ws.Hub) {
	// Authentication.
	g.POST("/api/v1/auth/login", rateLimit(handleLogin, "auth"))
	g.GET("/logout", auth(handleLogout))
	g.GET("/api/v1/oidc/{id}/login", rateLimit(handleOIDCLogin, "auth"))
	g.GET("/api/v1/oidc/{id}/finish", rateLimit(handleOIDCCallback, "auth"))

	// i18n.
	g.GET("/api/v1/lang", handleGetAvailableLanguages)
	g.GET("/api/v1/lang/{lang}", handleGetI18nLang)

	// Public config for app initialization.
	g.GET("/api/v1/config", handleGetConfig)

	// Media - supports both authenticated access and signed URLs.
	g.GET("/uploads/{uuid}", authOrSignedURL(handleServeMedia))
	g.POST("/api/v1/media", auth(handleMediaUpload))

	// Settings.
	g.GET("/api/v1/settings/general", auth(handleGetGeneralSettings))
	g.PUT("/api/v1/settings/general", perm(handleUpdateGeneralSettings, "general_settings:manage"))
	g.GET("/api/v1/settings/notifications/email", perm(handleGetEmailNotificationSettings, "notification_settings:manage"))
	g.PUT("/api/v1/settings/notifications/email", perm(handleUpdateEmailNotificationSettings, "notification_settings:manage"))
	g.GET("/api/v1/settings/customer-identification", auth(handleGetCustomerIdentificationSettings))
	g.PUT("/api/v1/settings/customer-identification", perm(handleUpdateCustomerIdentificationSettings, "customers:manage"))

	// OpenID connect single sign-on.
	g.GET("/api/v1/oidc", perm(handleGetAllOIDC, "oidc:manage"))
	g.POST("/api/v1/oidc", perm(handleCreateOIDC, "oidc:manage"))
	g.GET("/api/v1/oidc/{id}", perm(handleGetOIDC, "oidc:manage"))
	g.PUT("/api/v1/oidc/{id}", perm(handleUpdateOIDC, "oidc:manage"))
	g.DELETE("/api/v1/oidc/{id}", perm(handleDeleteOIDC, "oidc:manage"))

	// Operator-managed secrets.
	g.GET("/api/v1/secrets", perm(handleListSecrets, "secrets:manage"))
	g.POST("/api/v1/secrets", perm(handleCreateSecret, "secrets:manage"))
	g.PUT("/api/v1/secrets/{name}", perm(handleUpdateSecret, "secrets:manage"))
	g.DELETE("/api/v1/secrets/{name}", perm(handleDeleteSecret, "secrets:manage"))

	// Conversations.
	g.GET("/api/v1/conversations/all", perm(handleGetAllConversations, "conversations:read_all"))
	g.GET("/api/v1/conversations/unassigned", perm(handleGetUnassignedConversations, "conversations:read_unassigned"))
	g.GET("/api/v1/conversations/assigned", perm(handleGetAssignedConversations, "conversations:read_assigned"))
	g.GET("/api/v1/conversations/mentioned", perm(handleGetMentionedConversations, "conversations:read"))
	g.GET("/api/v1/teams/{id}/conversations/unassigned", perm(handleGetTeamUnassignedConversations, "conversations:read_team_inbox"))
	g.GET("/api/v1/views/{id}/conversations", perm(handleGetViewConversations, "conversations:read"))
	g.GET("/api/v1/conversations/{uuid}", perm(handleGetConversation, "conversations:read"))
	g.GET("/api/v1/conversations/{uuid}/variables", perm(handleGetConversationVariables, "conversations:read"))
	g.POST("/api/v1/detection/evaluate", perm(handleEvaluateDetection, "conversations:read"))
	g.GET("/api/v1/conversations/{uuid}/participants", perm(handleGetConversationParticipants, "conversations:read"))
	g.PUT("/api/v1/conversations/{uuid}/assignee/user", perm(handleUpdateUserAssignee, "conversations:update_user_assignee"))
	g.PUT("/api/v1/conversations/{uuid}/assignee/team", perm(handleUpdateTeamAssignee, "conversations:update_team_assignee"))
	g.PUT("/api/v1/conversations/{uuid}/assignee/user/remove", perm(handleRemoveUserAssignee, "conversations:update_user_assignee"))
	g.PUT("/api/v1/conversations/{uuid}/assignee/team/remove", perm(handleRemoveTeamAssignee, "conversations:update_team_assignee"))
	g.PUT("/api/v1/conversations/{uuid}/priority", perm(handleUpdateConversationPriority, "conversations:update_priority"))
	g.PUT("/api/v1/conversations/{uuid}/status", perm(handleUpdateConversationStatus, "conversations:update_status"))
	g.PUT("/api/v1/conversations/{uuid}/last-seen", perm(handleUpdateConversationAssigneeLastSeen, "conversations:read"))
	g.PUT("/api/v1/conversations/{uuid}/mark-unread", perm(handleMarkConversationAsUnread, "conversations:read"))
	g.POST("/api/v1/conversations/{uuid}/tags", perm(handleUpdateConversationtags, "conversations:update_tags"))
	g.POST("/api/v1/conversations/{uuid}/tags/classify", perm(handleClassifyConversationTags, "conversations:update_tags"))
	g.POST("/api/v1/conversations/{uuid}/agents/validate", perm(handleValidateConversationAgents, "ai_agents:run"))
	g.GET("/api/v1/conversations/{uuid}/agents/validation/latest", perm(handleGetLatestAgentValidation, "conversations:read"))
	g.GET("/api/v1/conversations/{uuid}/ai-operations", perm(handleListConversationAIOperations, "conversations:read"))
	g.GET("/api/v1/conversations/{uuid}/customer", perm(handleGetConversationCustomer, "conversations:read"))
	g.GET("/api/v1/conversations/{uuid}/contract", perm(handleGetConversationContract, "conversations:read"))
	g.POST("/api/v1/conversations/{uuid}/customer/identify", perm(handleIdentifyConversationCustomer, "conversations:read"))
	g.POST("/api/v1/conversations/{uuid}/customer", perm(handleAssignConversationCustomer, "customers:assign"))
	g.DELETE("/api/v1/conversations/{uuid}/customer", perm(handleRemoveConversationCustomer, "customers:assign"))
	g.GET("/api/v1/conversations/{uuid}/page-visits", perm(handleGetContactPageVisits, "conversations:read"))
	g.GET("/api/v1/conversations/{cuuid}/messages/{uuid}", perm(handleGetMessage, "messages:read"))
	g.GET("/api/v1/conversations/{uuid}/messages", perm(handleGetMessages, "messages:read"))
	g.POST("/api/v1/conversations/{cuuid}/messages", perm(handleSendMessage, "messages:write"))
	g.POST("/api/v1/conversations/{cuuid}/messages/{uuid}/forward", perm(handleForwardMessage, "messages:write"))
	g.PUT("/api/v1/conversations/{cuuid}/messages/{uuid}/retry", perm(handleRetryMessage, "messages:write"))
	g.POST("/api/v1/conversations", perm(handleCreateConversation, "conversations:write"))
	g.PUT("/api/v1/conversations/{uuid}/custom-attributes", auth(handleUpdateConversationCustomAttributes))
	g.PUT("/api/v1/conversations/{uuid}/contacts/custom-attributes", auth(handleUpdateContactCustomAttributes))
	// Draft endpoints
	g.GET("/api/v1/drafts", auth(handleGetAllDrafts))
	g.POST("/api/v1/conversations/{uuid}/draft", auth(handleUpsertConversationDraft))
	g.DELETE("/api/v1/conversations/{uuid}/draft", auth(handleDeleteConversationDraft))

	// Search.
	g.GET("/api/v1/conversations/search", perm(handleSearchConversations, "conversations:read"))
	g.GET("/api/v1/messages/search", perm(handleSearchMessages, "messages:read"))
	g.GET("/api/v1/contacts/search", perm(handleSearchContacts, "contacts:read"))

	// Views.
	g.GET("/api/v1/views/me", perm(handleGetUserViews, "view:manage"))
	g.POST("/api/v1/views/me", perm(handleCreateUserView, "view:manage"))
	g.PUT("/api/v1/views/me/{id}", perm(handleUpdateUserView, "view:manage"))
	g.DELETE("/api/v1/views/me/{id}", perm(handleDeleteUserView, "view:manage"))

	g.GET("/api/v1/views/shared", auth(handleGetSharedViews))

	g.GET("/api/v1/shared-views", perm(handleGetAllSharedViews, "shared_views:manage"))
	g.GET("/api/v1/shared-views/{id}", perm(handleGetSharedView, "shared_views:manage"))
	g.POST("/api/v1/shared-views", perm(handleCreateSharedView, "shared_views:manage"))
	g.PUT("/api/v1/shared-views/{id}", perm(handleUpdateSharedView, "shared_views:manage"))
	g.DELETE("/api/v1/shared-views/{id}", perm(handleDeleteSharedView, "shared_views:manage"))

	// Status and priority.
	g.GET("/api/v1/statuses", auth(handleGetStatuses))
	g.POST("/api/v1/statuses", perm(handleCreateStatus, "status:manage"))
	g.PUT("/api/v1/statuses/{id}", perm(handleUpdateStatus, "status:manage"))
	g.DELETE("/api/v1/statuses/{id}", perm(handleDeleteStatus, "status:manage"))
	g.GET("/api/v1/priorities", auth(handleGetPriorities))

	// Tags.
	g.GET("/api/v1/tags", auth(handleGetTags))
	g.POST("/api/v1/tags", perm(handleCreateTag, "tags:manage"))
	g.PUT("/api/v1/tags/{id}", perm(handleUpdateTag, "tags:manage"))
	g.DELETE("/api/v1/tags/{id}", perm(handleDeleteTag, "tags:manage"))
	g.POST("/api/v1/tags/import", perm(handleImportTags, "tags:manage"))
	g.GET("/api/v1/tags/import/status", perm(handleGetTagImportStatus, "tags:manage"))
	g.GET("/api/v1/tags/{id}", perm(handleGetTag, "tags:manage"))
	g.POST("/api/v1/tags/{id}/test", perm(handleTestTagDetection, "tags:manage"))

	// Macros.
	g.GET("/api/v1/macros", auth(handleGetMacros))
	g.GET("/api/v1/macros/{id}", perm(handleGetMacro, "macros:manage"))
	g.POST("/api/v1/macros", perm(handleCreateMacro, "macros:manage"))
	g.PUT("/api/v1/macros/{id}", perm(handleUpdateMacro, "macros:manage"))
	g.DELETE("/api/v1/macros/{id}", perm(handleDeleteMacro, "macros:manage"))
	g.POST("/api/v1/conversations/{uuid}/macros/{id}/apply", auth(handleApplyMacro))

	// Knowledge base.
	g.GET("/api/v1/knowledge/folders", perm(handleListKnowledgeFolders, "knowledge:read"))
	g.GET("/api/v1/knowledge/folders/{id}", perm(handleGetKnowledgeFolder, "knowledge:read"))
	g.POST("/api/v1/knowledge/folders", perm(handleCreateKnowledgeFolder, "knowledge:write"))
	g.PUT("/api/v1/knowledge/folders/{id}", perm(handleUpdateKnowledgeFolder, "knowledge:write"))
	g.POST("/api/v1/knowledge/folders/{id}/move", perm(handleMoveKnowledgeFolder, "knowledge:write"))
	g.DELETE("/api/v1/knowledge/folders/{id}", perm(handleDeleteKnowledgeFolder, "knowledge:delete"))
	g.GET("/api/v1/knowledge/articles", perm(handleListKnowledgeArticles, "knowledge:read"))
	g.GET("/api/v1/knowledge/articles/{id}", perm(handleGetKnowledgeArticle, "knowledge:read"))
	g.POST("/api/v1/knowledge/articles", perm(handleCreateKnowledgeArticle, "knowledge:write"))
	g.PUT("/api/v1/knowledge/articles/{id}", perm(handleUpdateKnowledgeArticle, "knowledge:write"))
	g.POST("/api/v1/knowledge/articles/{id}/publish", perm(handlePublishKnowledgeArticle, "knowledge:publish"))
	g.POST("/api/v1/knowledge/articles/{id}/unpublish", perm(handleUnpublishKnowledgeArticle, "knowledge:publish"))
	g.POST("/api/v1/knowledge/articles/{id}/move", perm(handleMoveKnowledgeArticle, "knowledge:write"))
	g.DELETE("/api/v1/knowledge/articles/{id}", perm(handleDeleteKnowledgeArticle, "knowledge:delete"))

	// Script-side recursive tool dispatch. Only callable with a valid
	// script-execution token; the handler rejects session/API-key callers.
	// Inert in v1 (app.toolRegistry is nil → 404). Becomes live when the
	// AI Manager registers a tool registry.
	g.POST("/api/v1/tools/{name}/call", perm(handleToolCall, "tools:call"))

	// Agents.
	g.GET("/api/v1/agents/me", auth(handleGetCurrentAgent))
	g.PUT("/api/v1/agents/me", auth(handleUpdateCurrentAgent))
	g.GET("/api/v1/agents/me/teams", auth(handleGetCurrentAgentTeams))
	g.PUT("/api/v1/agents/me/availability", auth(handleUpdateAgentAvailability))
	g.DELETE("/api/v1/agents/me/avatar", auth(handleDeleteCurrentAgentAvatar))

	g.GET("/api/v1/agents/compact", auth(handleGetAgentsCompact))
	g.GET("/api/v1/agents", perm(handleGetAgents, "users:manage"))
	g.GET("/api/v1/agents/{id}", perm(handleGetAgent, "users:manage"))
	g.POST("/api/v1/agents", perm(handleCreateAgent, "users:manage"))
	g.PUT("/api/v1/agents/{id}", perm(handleUpdateAgent, "users:manage"))
	g.DELETE("/api/v1/agents/{id}", perm(handleDeleteAgent, "users:manage"))
	g.POST("/api/v1/agents/import", perm(handleImportAgents, "users:manage"))
	g.GET("/api/v1/agents/import/status", perm(handleGetAgentImportStatus, "users:manage"))
	g.POST("/api/v1/agents/{id}/api-key", perm(handleGenerateAPIKey, "users:manage"))
	g.DELETE("/api/v1/agents/{id}/api-key", perm(handleRevokeAPIKey, "users:manage"))
	g.POST("/api/v1/agents/reset-password", rateLimit(tryAuth(handleResetPassword), "auth"))
	g.POST("/api/v1/agents/set-password", rateLimit(tryAuth(handleSetPassword), "auth"))

	// Contacts.
	g.GET("/api/v1/contacts", perm(handleGetContacts, "contacts:read_all"))
	g.GET("/api/v1/contacts/{id}/conversations", perm(handleGetContactConversations, "conversations:read"))
	g.GET("/api/v1/contacts/{id}", perm(handleGetContact, "contacts:read"))
	g.PUT("/api/v1/contacts/{id}", perm(handleUpdateContact, "contacts:write"))
	g.PUT("/api/v1/contacts/{id}/linked-customer", perm(handleUpdateContactLinkedCustomer, "conversations:write"))
	g.PUT("/api/v1/contacts/{id}/block", perm(handleBlockContact, "contacts:block"))

	// Customer / contract identification resources.
	g.GET("/api/v1/customers/search", perm(handleSearchCustomers, "customers:search"))
	g.GET("/api/v1/customers/{customer_id}/linked-contacts", perm(handleGetCustomerLinkedContacts, "conversations:read"))
	g.GET("/api/v1/customers/{customer_id}", auth(handleGetCustomer))
	g.GET("/api/v1/contracts/{contract_id}", auth(handleGetContract))

	// Contact notes.
	g.GET("/api/v1/contacts/{id}/notes", perm(handleGetContactNotes, "contact_notes:read"))
	g.POST("/api/v1/contacts/{id}/notes", perm(handleCreateContactNote, "contact_notes:write"))
	g.DELETE("/api/v1/contacts/{id}/notes/{note_id}", perm(handleDeleteContactNote, "contact_notes:delete"))

	// Teams.
	g.GET("/api/v1/teams/compact", auth(handleGetTeamsCompact))
	g.GET("/api/v1/teams", perm(handleGetTeams, "teams:manage"))
	g.GET("/api/v1/teams/{id}", perm(handleGetTeam, "teams:manage"))
	g.POST("/api/v1/teams", perm(handleCreateTeam, "teams:manage"))
	g.PUT("/api/v1/teams/{id}", perm(handleUpdateTeam, "teams:manage"))
	g.DELETE("/api/v1/teams/{id}", perm(handleDeleteTeam, "teams:manage"))

	// Automations.
	g.GET("/api/v1/automations/rules", perm(handleGetAutomationRules, "automations:manage"))
	g.GET("/api/v1/automations/rules/{id}", perm(handleGetAutomationRule, "automations:manage"))
	g.POST("/api/v1/automations/rules", perm(handleCreateAutomationRule, "automations:manage"))
	g.PUT("/api/v1/automations/rules/{id}/toggle", perm(handleToggleAutomationRule, "automations:manage"))
	g.PUT("/api/v1/automations/rules/{id}", perm(handleUpdateAutomationRule, "automations:manage"))
	g.PUT("/api/v1/automations/rules/weights", perm(handleUpdateAutomationRuleWeights, "automations:manage"))
	g.PUT("/api/v1/automations/rules/execution-mode", perm(handleUpdateAutomationRuleExecutionMode, "automations:manage"))
	g.DELETE("/api/v1/automations/rules/{id}", perm(handleDeleteAutomationRule, "automations:manage"))

	// Inboxes.
	g.GET("/api/v1/inboxes", auth(handleGetInboxes))
	g.GET("/api/v1/inboxes/{id}", perm(handleGetInbox, "inboxes:manage"))
	g.POST("/api/v1/inboxes", perm(handleCreateInbox, "inboxes:manage"))
	g.PUT("/api/v1/inboxes/{id}/toggle", perm(handleToggleInbox, "inboxes:manage"))
	g.PUT("/api/v1/inboxes/{id}", perm(handleUpdateInbox, "inboxes:manage"))
	g.DELETE("/api/v1/inboxes/{id}", perm(handleDeleteInbox, "inboxes:manage"))

	// OAuth endpoints for email inboxes.
	g.POST("/api/v1/inboxes/oauth/{provider}/authorize", perm(handleOAuthAuthorize, "inboxes:manage"))
	g.GET("/api/v1/inboxes/oauth/{provider}/callback", perm(handleOAuthCallback, "inboxes:manage"))

	// Roles.
	g.GET("/api/v1/roles", auth(handleGetRoles))
	g.GET("/api/v1/roles/{id}", perm(handleGetRole, "roles:manage"))
	g.POST("/api/v1/roles", perm(handleCreateRole, "roles:manage"))
	g.PUT("/api/v1/roles/{id}", perm(handleUpdateRole, "roles:manage"))
	g.DELETE("/api/v1/roles/{id}", perm(handleDeleteRole, "roles:manage"))

	// Webhooks.
	g.GET("/api/v1/webhooks", perm(handleGetWebhooks, "webhooks:manage"))
	g.GET("/api/v1/webhooks/{id}", perm(handleGetWebhook, "webhooks:manage"))
	g.POST("/api/v1/webhooks", perm(handleCreateWebhook, "webhooks:manage"))
	g.PUT("/api/v1/webhooks/{id}", perm(handleUpdateWebhook, "webhooks:manage"))
	g.DELETE("/api/v1/webhooks/{id}", perm(handleDeleteWebhook, "webhooks:manage"))
	g.PUT("/api/v1/webhooks/{id}/toggle", perm(handleToggleWebhook, "webhooks:manage"))
	g.POST("/api/v1/webhooks/{id}/test", perm(handleTestWebhook, "webhooks:manage"))

	// Reports.
	g.GET("/api/v1/reports/overview/sla", perm(handleOverviewSLA, "reports:manage"))
	g.GET("/api/v1/reports/overview/counts", perm(handleOverviewCounts, "reports:manage"))
	g.GET("/api/v1/reports/overview/charts", perm(handleOverviewCharts, "reports:manage"))
	g.GET("/api/v1/reports/overview/csat", perm(handleOverviewCSAT, "reports:manage"))
	g.GET("/api/v1/reports/overview/messages", perm(handleOverviewMessageVolume, "reports:manage"))
	g.GET("/api/v1/reports/overview/tags", perm(handleOverviewTagDistribution, "reports:manage"))

	// Templates.
	g.GET("/api/v1/templates", perm(handleGetTemplates, "templates:manage"))
	g.GET("/api/v1/templates/{id}", perm(handleGetTemplate, "templates:manage"))
	g.POST("/api/v1/templates", perm(handleCreateTemplate, "templates:manage"))
	g.PUT("/api/v1/templates/{id}", perm(handleUpdateTemplate, "templates:manage"))
	g.DELETE("/api/v1/templates/{id}", perm(handleDeleteTemplate, "templates:manage"))

	// Business hours.
	g.GET("/api/v1/business-hours", auth(handleGetBusinessHours))
	g.GET("/api/v1/business-hours/{id}", perm(handleGetBusinessHour, "business_hours:manage"))
	g.POST("/api/v1/business-hours", perm(handleCreateBusinessHours, "business_hours:manage"))
	g.PUT("/api/v1/business-hours/{id}", perm(handleUpdateBusinessHours, "business_hours:manage"))
	g.DELETE("/api/v1/business-hours/{id}", perm(handleDeleteBusinessHour, "business_hours:manage"))

	// SLAs.
	g.GET("/api/v1/sla", auth(handleGetSLAs))
	g.GET("/api/v1/sla/{id}", perm(handleGetSLA, "sla:manage"))
	g.POST("/api/v1/sla", perm(handleCreateSLA, "sla:manage"))
	g.PUT("/api/v1/sla/{id}", perm(handleUpdateSLA, "sla:manage"))
	g.DELETE("/api/v1/sla/{id}", perm(handleDeleteSLA, "sla:manage"))

	// AI completions.
	g.GET("/api/v1/ai/prompts", auth(handleGetAIPrompts))
	g.POST("/api/v1/ai/completion", auth(handleAICompletion))
	g.PUT("/api/v1/ai/provider", perm(handleUpdateAIProvider, "ai:manage"))

	// AI agents (F9 — configuration data model).
	g.GET("/api/v1/ai/agents", perm(handleListAIAgents, "ai_agents:manage"))
	g.GET("/api/v1/ai/agents/{id}", perm(handleGetAIAgent, "ai_agents:manage"))
	g.POST("/api/v1/ai/agents", perm(handleCreateAIAgent, "ai_agents:manage"))
	g.PUT("/api/v1/ai/agents/{id}", perm(handleUpdateAIAgent, "ai_agents:manage"))
	g.DELETE("/api/v1/ai/agents/{id}", perm(handleDeleteAIAgent, "ai_agents:manage"))

	// AI tools (F9).
	g.GET("/api/v1/ai/tools", perm(handleListAITools, "ai_tools:manage"))
	g.GET("/api/v1/ai/tools/{id}", perm(handleGetAITool, "ai_tools:manage"))
	g.POST("/api/v1/ai/tools", perm(handleCreateAITool, "ai_tools:manage"))
	g.PUT("/api/v1/ai/tools/{id}", perm(handleUpdateAITool, "ai_tools:manage"))
	g.DELETE("/api/v1/ai/tools/{id}", perm(handleDeleteAITool, "ai_tools:manage"))
	// Manual tool invocation (F9 Chunk 6) — admin testing pre-agent-runtime.
	// After the F5 dispatcher unification, this endpoint accepts both
	// custom (Python) tools and internal capabilities; see handleInvokeAITool.
	g.POST("/api/v1/ai/tools/{slug}/invoke", perm(handleInvokeAITool, "ai_tools:invoke"))

	// Unified functions (Unified Executor Framework batch). The HTTP surface
	// is deliberately narrow — list/get/update + preview. Create and Delete
	// have no HTTP route in this batch (the ai_tools admin handlers call the
	// Go Manager methods directly; customer-identification rows are seeded by
	// migration and protected from deletion).
	g.GET("/api/v1/functions", perm(handleListFunctions, "functions:manage"))
	g.GET("/api/v1/functions/{name}", perm(handleGetFunction, "functions:manage"))
	g.PUT("/api/v1/functions/{name}", perm(handleUpdateFunction, "functions:manage"))
	g.POST("/api/v1/function/preview", perm(handlePreviewFunction, "functions:manage"))

	// AI internal capabilities — list-only (registered in code, not DB).
	// Gated on ai_agents:manage because the only consumer is the agent
	// edit form's tool picker.
	g.GET("/api/v1/ai/internal-capabilities", perm(handleListAIInternalCapabilities, "ai_agents:manage"))

	// AI presets — list of enabled reasoning presets from
	// config/ai_presets.json. Same gate; consumed by the agent edit
	// form's preset-overrides picker.
	g.GET("/api/v1/ai/presets", perm(handleListAIPresets, "ai_agents:manage"))

	// AI agent runner — manual run endpoint for testing pre-pipeline.
	// Gated on ai_agents:run, distinct from :manage so token-spending
	// authority can be granted independently.
	g.POST("/api/v1/ai/agents/{id}/run", perm(handleRunAIAgent, "ai_agents:run"))
	g.GET("/api/v1/ai/runs/{trace_id}/stream", perm(handleAgentRunStream, "ai_agents:run"))

	// History tab — paginated past runs for one agent. ai_agents:manage
	// gates editing the agent; viewing its run history is part of that
	// permission's surface.
	g.GET("/api/v1/ai/agents/{id}/runs", perm(handleListAgentRuns, "ai_agents:manage"))
	g.GET("/api/v1/ai/executions/{id}", perm(handleGetAgentExecution, "ai_agents:run"))

	// Composer tab — per-conversation persistence + send-reply action.
	// Same permission as the run endpoint (the Composer tab is just the
	// run endpoint with a persistence layer).
	g.GET("/api/v1/conversations/{uuid}/agent-run-state",
		perm(handleConversationAgentRunState, "ai_agents:run"))
	g.GET("/api/v1/conversations/{uuid}/agent-executions/latest",
		perm(handleListLatestAgentExecutions, "ai_agents:run"))
	g.POST("/api/v1/conversations/{uuid}/agent-executions/{execution_id}/send",
		perm(handleSendAgentExecution, "ai_agents:run"))

	// Custom attributes.
	g.GET("/api/v1/custom-attributes", auth(handleGetCustomAttributes))
	g.POST("/api/v1/custom-attributes", perm(handleCreateCustomAttribute, "custom_attributes:manage"))
	g.GET("/api/v1/custom-attributes/{id}", perm(handleGetCustomAttribute, "custom_attributes:manage"))
	g.PUT("/api/v1/custom-attributes/{id}", perm(handleUpdateCustomAttribute, "custom_attributes:manage"))
	g.DELETE("/api/v1/custom-attributes/{id}", perm(handleDeleteCustomAttribute, "custom_attributes:manage"))

	// Actvity logs.
	g.GET("/api/v1/activity-logs", perm(handleGetActivityLogs, "activity_logs:manage"))

	// CSAT.
	g.POST("/api/v1/csat/{uuid}/response", rateLimit(handleSubmitCSATResponse, "public"))

	// User notifications.
	g.GET("/api/v1/notifications", auth(handleGetUserNotifications))
	g.GET("/api/v1/notifications/stats", auth(handleGetUserNotificationStats))
	g.PUT("/api/v1/notifications/{id}/read", auth(handleMarkNotificationAsRead))
	g.PUT("/api/v1/notifications/read-all", auth(handleMarkAllNotificationsAsRead))
	g.DELETE("/api/v1/notifications/{id}", auth(handleDeleteNotification))
	g.DELETE("/api/v1/notifications", auth(handleDeleteAllNotifications))

	// WebSocket.
	g.GET("/ws", auth(func(r *fastglue.Request) error {
		return handleWS(r, hub)
	}))

	// Live chat widget websocket.
	g.GET("/widget/ws", rateLimit(handleWidgetWS, "widget"))

	// Widget APIs.
	g.GET("/api/v1/widget/chat/settings/launcher", rateLimit(validateWidgetInbox(handleGetChatLauncherSettings), "widget"))
	g.GET("/api/v1/widget/chat/settings", rateLimit(validateWidgetInbox(handleGetChatSettings), "widget"))
	g.POST("/api/v1/widget/chat/auth/exchange", rateLimit(validateWidgetInbox(handleAuthExchange), "widget"))
	g.GET("/api/v1/widget/chat/auth/me", rateLimit(widgetAuth(handleWidgetAuthMe), "widget"))
	g.POST("/api/v1/widget/chat/conversations/init", rateLimit(widgetAuth(handleChatInit), "widget"))
	g.GET("/api/v1/widget/chat/conversations", rateLimit(widgetAuth(handleGetConversations), "widget"))
	g.POST("/api/v1/widget/chat/conversations/{uuid}/update-last-seen", rateLimit(widgetAuth(handleChatUpdateLastSeen), "widget"))
	g.GET("/api/v1/widget/chat/conversations/{uuid}", rateLimit(widgetAuth(handleChatGetConversation), "widget"))
	g.POST("/api/v1/widget/chat/conversations/{uuid}/message", rateLimit(widgetAuth(handleChatSendMessage), "widget"))
	g.POST("/api/v1/widget/media/upload", rateLimit(widgetAuth(handleWidgetMediaUpload), "widget"))

	// Frontend pages.
	g.GET("/", notAuthPage(serveIndexPage))
	g.GET("/widget", validateWidgetInbox(serveWidgetIndexPage))
	g.GET("/inboxes/{all:*}", authPage(serveIndexPage))
	g.GET("/teams/{all:*}", authPage(serveIndexPage))
	g.GET("/views/{all:*}", authPage(serveIndexPage))
	g.GET("/admin/{all:*}", authPage(serveIndexPage))
	g.GET("/ai/{all:*}", authPage(serveIndexPage))
	g.GET("/contacts/{all:*}", authPage(serveIndexPage))
	g.GET("/reports/{all:*}", authPage(serveIndexPage))
	g.GET("/account/{all:*}", authPage(serveIndexPage))
	g.GET("/reset-password", notAuthPage(serveIndexPage))
	g.GET("/set-password", notAuthPage(serveIndexPage))

	// Assets and static files.
	// FIXME: Reduce the number of routes.
	g.GET("/widget.js", serveWidgetJS)
	g.GET("/assets/{all:*}", serveFrontendStaticFiles)
	g.GET("/widget/assets/{all:*}", serveWidgetStaticFiles)
	g.GET("/images/{all:*}", serveFrontendStaticFiles)
	g.GET("/static/public/{all:*}", serveStaticFiles)

	// Public pages.
	g.GET("/csat/{uuid}", rateLimit(handleShowCSAT, "public"))
	g.GET("/csat/{uuid}/widget", rateLimit(handleShowCSATWidget, "public"))
	g.POST("/csat/{uuid}", rateLimit(handleUpdateCSATResponse, "public"))

	// Health check.
	g.GET("/health", handleHealthCheck)
}

// serveIndexPage serves the main index page of the application.
func serveIndexPage(r *fastglue.Request) error {
	app := r.Context.(*App)

	// Prevent caching of the index page.
	r.RequestCtx.Response.Header.Add("Cache-Control", "no-store, no-cache, must-revalidate, post-check=0, pre-check=0")
	r.RequestCtx.Response.Header.Add("Pragma", "no-cache")
	r.RequestCtx.Response.Header.Add("Expires", "-1")

	// Serve the index.html file from the embedded filesystem.
	file, err := app.fs.Get(path.Join(frontendDir, "index.html"))
	if err != nil {
		return r.SendErrorEnvelope(http.StatusNotFound, app.i18n.T("validation.notFoundFile"), nil, envelope.NotFoundError)
	}
	r.RequestCtx.Response.Header.Set("Content-Type", "text/html")
	r.RequestCtx.SetBody(file.ReadBytes())

	// Set CSRF cookie if not already set.
	if err := app.auth.SetCSRFCookie(r); err != nil {
		app.lo.Error("error setting csrf cookie", "error", err)
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.T("globals.messages.somethingWentWrong"), nil))
	}
	return nil
}

// serveWidgetIndexPage serves the widget index page of the application.
func serveWidgetIndexPage(r *fastglue.Request) error {
	app := r.Context.(*App)

	// Prevent caching of the index page.
	r.RequestCtx.Response.Header.Add("Cache-Control", "no-store, no-cache, must-revalidate, post-check=0, pre-check=0")
	r.RequestCtx.Response.Header.Add("Pragma", "no-cache")
	r.RequestCtx.Response.Header.Add("Expires", "-1")

	// CSP headers if trusted domains is set.
	if config, err := getWidgetConfig(r); err == nil && len(config.TrustedDomains) > 0 {
		csp := "frame-ancestors 'self' " + strings.Join(config.TrustedDomains, " ")
		r.RequestCtx.Response.Header.Set("Content-Security-Policy", csp)
	}

	// Serve the index.html file from the embedded filesystem.
	file, err := app.fs.Get(path.Join(widgetDir, "index.html"))
	if err != nil {
		return r.SendErrorEnvelope(http.StatusNotFound, app.i18n.T("validation.notFoundFile"), nil, envelope.NotFoundError)
	}
	r.RequestCtx.Response.Header.Set("Content-Type", "text/html")
	r.RequestCtx.SetBody(file.ReadBytes())

	return nil
}

// serveStaticFiles serves static assets from the filesystem.
func serveStaticFiles(r *fastglue.Request) error {
	app := r.Context.(*App)

	filePath := string(r.RequestCtx.Path())

	file, err := app.fs.Get(filePath)
	if err != nil {
		return r.SendErrorEnvelope(http.StatusNotFound, app.i18n.T("validation.notFoundFile"), nil, envelope.NotFoundError)
	}

	ext := filepath.Ext(filePath)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = http.DetectContentType(file.ReadBytes())
	}
	r.RequestCtx.Response.Header.Set("Content-Type", contentType)
	r.RequestCtx.SetBody(file.ReadBytes())
	return nil
}

// serveFrontendStaticFiles serves static assets from the embedded filesystem.
func serveFrontendStaticFiles(r *fastglue.Request) error {
	app := r.Context.(*App)

	// Get the requested file path.
	filePath := string(r.RequestCtx.Path())

	// Fetch and serve the file from the embedded filesystem.
	finalPath := filepath.Join(frontendDir, filePath)
	file, err := app.fs.Get(finalPath)
	if err != nil {
		return r.SendErrorEnvelope(http.StatusNotFound, app.i18n.T("validation.notFoundFile"), nil, envelope.NotFoundError)
	}

	// Set the appropriate Content-Type based on the file extension.
	ext := filepath.Ext(filePath)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = http.DetectContentType(file.ReadBytes())
	}
	r.RequestCtx.Response.Header.Set("Content-Type", contentType)
	r.RequestCtx.SetBody(file.ReadBytes())
	return nil
}

// serveWidgetStaticFiles serves widget static assets from the embedded filesystem.
func serveWidgetStaticFiles(r *fastglue.Request) error {
	app := r.Context.(*App)

	filePath := string(r.RequestCtx.Path())
	finalPath := filepath.Join(widgetDir, strings.TrimPrefix(filePath, "/widget"))

	file, err := app.fs.Get(finalPath)
	if err != nil {
		return r.SendErrorEnvelope(http.StatusNotFound, app.i18n.T("validation.notFoundFile"), nil, envelope.NotFoundError)
	}

	// Set the appropriate Content-Type based on the file extension.
	ext := filepath.Ext(filePath)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = http.DetectContentType(file.ReadBytes())
	}
	r.RequestCtx.Response.Header.Set("Content-Type", contentType)
	r.RequestCtx.SetBody(file.ReadBytes())
	return nil
}

// serveWidgetJS serves the widget JavaScript file.
func serveWidgetJS(r *fastglue.Request) error {
	app := r.Context.(*App)

	r.RequestCtx.Response.Header.Set("Content-Type", "application/javascript")
	r.RequestCtx.Response.Header.Set("Cache-Control", "no-cache")

	file, err := app.fs.Get("static/widget.js")
	if err != nil {
		return r.SendErrorEnvelope(http.StatusNotFound, app.i18n.T("validation.notFoundFile"), nil, envelope.NotFoundError)
	}

	r.RequestCtx.SetBody(file.ReadBytes())
	return nil
}

// getPagination extracts page and page_size from query params with defaults.
func getPagination(r *fastglue.Request) (page, pageSize int) {
	page, _ = strconv.Atoi(string(r.RequestCtx.QueryArgs().Peek("page")))
	pageSize, _ = strconv.Atoi(string(r.RequestCtx.QueryArgs().Peek("page_size")))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 30
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize
}

// sendErrorEnvelope sends a standardized error response to the client.
func sendErrorEnvelope(r *fastglue.Request, err error) error {
	e, ok := err.(envelope.Error)
	if !ok {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError,
			"Error interface conversion failed", nil, fastglue.ErrorType(envelope.GeneralError))
	}
	return r.SendErrorEnvelope(e.Code, e.Error(), e.Data, fastglue.ErrorType(e.ErrorType))
}

// handleHealthCheck handles the health check endpoint.
func handleHealthCheck(r *fastglue.Request) error {
	return r.SendEnvelope(true)
}
