// Package models defines the authorization permission keys recognized by the
// authz enforcer. Each value is a "resource:action" string persisted on roles
// and checked when a request is authorized.
package models

const (
	// PermConversationsReadAll grants all conversations; it is the superset that
	// short-circuits the rest.
	PermConversationsReadAll = "conversations:read_all"
	// PermConversationsReadUnassigned additionally grants conversations not assigned to anyone.
	PermConversationsReadUnassigned = "conversations:read_unassigned"
	// PermConversationsReadAssigned additionally grants conversations assigned to the user.
	PermConversationsReadAssigned = "conversations:read_assigned"
	// PermConversationsReadTeamInbox additionally grants unassigned conversations in the
	// user's team inboxes (subsumed by ReadTeamAll).
	PermConversationsReadTeamInbox = "conversations:read_team_inbox"
	// PermConversationsReadTeamAll additionally grants all conversations in the user's teams.
	PermConversationsReadTeamAll = "conversations:read_team_all"
	// PermConversationsRead grants reading an individual conversation and team views.
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

	PermViewManage        = "view:manage"
	PermSharedViewsManage = "shared_views:manage"

	PermStatusManage = "status:manage"

	PermTagsManage      = "tags:manage"
	PermTagsViewPrivate = "tags:view_private"

	PermMacrosManage = "macros:manage"

	PermUsersManage = "users:manage"

	PermTeamsManage = "teams:manage"

	PermAutomationsManage = "automations:manage"

	PermInboxesManage = "inboxes:manage"

	PermRolesManage = "roles:manage"

	PermWebhooksManage = "webhooks:manage"

	PermTemplatesManage = "templates:manage"

	PermReportsManage = "reports:manage"

	PermBusinessHoursManage = "business_hours:manage"

	PermSLAManage = "sla:manage"

	PermGeneralSettingsManage = "general_settings:manage"

	PermNotificationSettingsManage = "notification_settings:manage"

	PermOIDCManage = "oidc:manage"

	PermAIManage = "ai:manage"

	PermContactsReadAll = "contacts:read_all"
	PermContactsRead    = "contacts:read"
	PermContactsWrite   = "contacts:write"
	PermContactsBlock   = "contacts:block"

	PermContactNotesRead   = "contact_notes:read"
	PermContactNotesWrite  = "contact_notes:write"
	PermContactNotesDelete = "contact_notes:delete"

	PermCustomAttributesManage = "custom_attributes:manage"

	PermActivityLogsManage = "activity_logs:manage"

	PermWorklogViewOwn  = "worklog:view_own"
	PermWorklogViewTeam = "worklog:view_team"
	PermWorklogViewAll  = "worklog:view_all"

	PermKnowledgeRead    = "knowledge:read"
	PermKnowledgeWrite   = "knowledge:write"
	PermKnowledgePublish = "knowledge:publish"
	PermKnowledgeDelete  = "knowledge:delete"

	// PermToolsCall authorizes invoking tools (script execution).
	PermToolsCall = "tools:call"

	PermAIAgentsManage = "ai_agents:manage"
	// PermAIAgentsRun gates the agent runner's manual-run endpoint.
	PermAIAgentsRun   = "ai_agents:run"
	PermAIToolsManage = "ai_tools:manage"
	PermAIToolsInvoke = "ai_tools:invoke"

	PermCustomersAssign = "customers:assign"
	PermCustomersSearch = "customers:search"
	PermCustomersManage = "customers:manage"
	PermFunctionsManage = "functions:manage"
	PermSecretsManage   = "secrets:manage"
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
	PermTagsViewPrivate:                 {},
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
	PermWorklogViewOwn:                  {},
	PermWorklogViewTeam:                 {},
	PermWorklogViewAll:                  {},
	PermKnowledgeRead:                   {},
	PermKnowledgeWrite:                  {},
	PermKnowledgePublish:                {},
	PermKnowledgeDelete:                 {},
	PermToolsCall:                       {},
	PermAIAgentsManage:                  {},
	PermAIAgentsRun:                     {},
	PermAIToolsManage:                   {},
	PermAIToolsInvoke:                   {},
	PermCustomersAssign:                 {},
	PermCustomersSearch:                 {},
	PermCustomersManage:                 {},
	PermFunctionsManage:                 {},
	PermSecretsManage:                   {},
}

// PermissionExists reports whether permission is a recognized permission key.
func PermissionExists(permission string) bool {
	_, exists := validPermissions[permission]
	return exists
}
