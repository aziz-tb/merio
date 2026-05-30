package main

import cmodels "github.com/abhinavxd/libredesk/internal/conversation/models"

func invalidateLinkedConversationCaches(app *App, conv *cmodels.Conversation, reason string) {
	if app == nil || conv == nil {
		return
	}
	if conv.ContractID.Valid && app.contract != nil {
		if err := app.contract.InvalidateCache(conv.ContractID.String); err != nil {
			app.lo.Warn("conversation cache invalidation failed",
				"reason", reason,
				"conversation_id", conv.ID,
				"contract_id", conv.ContractID.String,
				"error", err)
		}
	}
	if conv.CustomerID.Valid && app.customer != nil {
		if err := app.customer.InvalidateCache(conv.CustomerID.String); err != nil {
			app.lo.Warn("conversation cache invalidation failed",
				"reason", reason,
				"conversation_id", conv.ID,
				"customer_id", conv.CustomerID.String,
				"error", err)
		}
	}
}
