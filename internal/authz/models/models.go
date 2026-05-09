package models

const (
	// Conversation
	PermConversationsReadAll            = "conversations:read_all"
	PermConversationsReadUnassigned     = "conversations:read_unassigned"
	PermConversationsReadAssigned       = "conversations:read_assigned"
	PermConversationsReadTeamInbox      = "conversations:read_team_inbox"
	PermConversationsReadTeamAll        = "conversations:read_team_all"
	PermConversationsRead               = "conversations:read"
	PermConversationsUpdateUserAssignee = "conversations:update_user_assignee"
	PermConversationsUpdateTeamAssignee = "conversations:update_team_assignee"
	PermConversationsUpdatePriority     = "conversations:update_priority"
	PermConversationsUpdateStatus       = "conversations:update_status"
	PermConversationsUpdateTags         = "conversations:update_tags"
	PermConversationWrite               = "conversations:write"
	PermMessagesRead                    = "messages:read"
	PermMessagesWrite                   = "messages:write"
	PermMessagesWriteAsContact          = "messages:write_as_contact"

	// View
	PermViewManage        = "view:manage"
	PermSharedViewsManage = "shared_views:manage"

	// Status
	PermStatusManage = "status:manage"

	// Tags
	PermTagsManage = "tags:manage"

	// Macros
	PermMacrosManage = "macros:manage"

	// Users
	PermUsersManage = "users:manage"

	// Teams
	PermTeamsManage = "teams:manage"

	// Automations
	PermAutomationsManage = "automations:manage"

	// Inboxes
	PermInboxesManage = "inboxes:manage"

	// Roles
	PermRolesManage = "roles:manage"

	// Webhooks
	PermWebhooksManage = "webhooks:manage"

	// Context Links
	PermContextLinksManage = "context_links:manage"

	// Templates
	PermTemplatesManage = "templates:manage"

	// Reports
	PermReportsManage = "reports:manage"

	// Business Hours
	PermBusinessHoursManage = "business_hours:manage"

	// SLA
	PermSLAManage = "sla:manage"

	// General Settings
	PermGeneralSettingsManage = "general_settings:manage"

	// Notification Settings
	PermNotificationSettingsManage = "notification_settings:manage"

	// OpenID Connect SSO
	PermOIDCManage = "oidc:manage"

	// AI
	PermAIManage = "ai:manage"

	// Contacts
	PermContactsReadAll = "contacts:read_all"
	PermContactsRead    = "contacts:read"
	PermContactsWrite   = "contacts:write"
	PermContactsBlock   = "contacts:block"

	// Contact Notes
	PermContactNotesRead   = "contact_notes:read"
	PermContactNotesWrite  = "contact_notes:write"
	PermContactNotesDelete = "contact_notes:delete"

	// Custom attributes
	PermCustomAttributesManage = "custom_attributes:manage"

	// Activity log
	PermActivityLogsManage = "activity_logs:manage"

	// Worklog (B1)
	PermWorklogViewOwn  = "worklog:view_own"
	PermWorklogViewTeam = "worklog:view_team"
	PermWorklogViewAll  = "worklog:view_all"

	// Knowledge (B3)
	PermKnowledgeRead    = "knowledge:read"
	PermKnowledgeWrite   = "knowledge:write"
	PermKnowledgePublish = "knowledge:publish"
	PermKnowledgeDelete  = "knowledge:delete"

	// Tools (script execution — B5/exec-runner)
	PermToolsCall = "tools:call"

	// AI agents and tools (F9 — configuration data model)
	PermAIAgentsManage = "ai_agents:manage"
	PermAIToolsManage  = "ai_tools:manage"
	PermAIToolsInvoke  = "ai_tools:invoke"
)

var validPermissions = map[string]struct{}{
	PermConversationsReadAll:            {},
	PermConversationsReadUnassigned:     {},
	PermConversationsReadAssigned:       {},
	PermConversationsReadTeamInbox:      {},
	PermConversationsReadTeamAll:        {},
	PermConversationsRead:               {},
	PermConversationsUpdateUserAssignee: {},
	PermConversationsUpdateTeamAssignee: {},
	PermConversationsUpdatePriority:     {},
	PermConversationsUpdateStatus:       {},
	PermConversationsUpdateTags:         {},
	PermConversationWrite:               {},
	PermMessagesRead:                    {},
	PermMessagesWrite:                   {},
	PermMessagesWriteAsContact:          {},
	PermViewManage:                      {},
	PermSharedViewsManage:               {},
	PermStatusManage:                    {},
	PermTagsManage:                      {},
	PermMacrosManage:                    {},
	PermUsersManage:                     {},
	PermTeamsManage:                     {},
	PermAutomationsManage:               {},
	PermInboxesManage:                   {},
	PermRolesManage:                     {},
	PermTemplatesManage:                 {},
	PermReportsManage:                   {},
	PermBusinessHoursManage:             {},
	PermSLAManage:                       {},
	PermGeneralSettingsManage:           {},
	PermNotificationSettingsManage:      {},
	PermOIDCManage:                      {},
	PermAIManage:                        {},
	PermCustomAttributesManage:          {},
	PermContactsReadAll:                 {},
	PermContactsRead:                    {},
	PermContactsWrite:                   {},
	PermContactsBlock:                   {},
	PermContactNotesRead:                {},
	PermContactNotesWrite:               {},
	PermContactNotesDelete:              {},
	PermActivityLogsManage:              {},
	PermWebhooksManage:                  {},
	PermContextLinksManage:              {},
	PermWorklogViewOwn:                  {},
	PermWorklogViewTeam:                 {},
	PermWorklogViewAll:                  {},
	PermKnowledgeRead:                   {},
	PermKnowledgeWrite:                  {},
	PermKnowledgePublish:                {},
	PermKnowledgeDelete:                 {},
	PermToolsCall:                       {},
	PermAIAgentsManage:                  {},
	PermAIToolsManage:                   {},
	PermAIToolsInvoke:                   {},
}

// PermissionExists returns true if the permission exists else false
func PermissionExists(permission string) bool {
	_, exists := validPermissions[permission]
	return exists
}
