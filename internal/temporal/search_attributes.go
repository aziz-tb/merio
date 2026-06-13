package temporal

// Search-attribute names attached at workflow start (see CONVENTIONS.md).
// They must be registered per namespace; registerSearchAttributes does this
// at boot.
//
// Adding one: append a constant here AND add it to registerSearchAttributes.
const (
	// SATraceID correlates a Temporal execution with rows in ai_usage and
	// script_executions for that ticket. Always populated.
	SATraceID = "trace_id"

	// SAInboxID enables per-inbox filtering. Populated for workflows triggered
	// from a conversation context; zero for system / cron workflows.
	SAInboxID = "inbox_id"

	// SAWorkflowKind is the high-level category. Values: "embedding",
	// "pipeline", "ai-run", "sla-timer", "hello".
	SAWorkflowKind = "workflow_kind"

	// SAEntityType + SAEntityID identify the thing being processed.
	// Values for SAEntityType: "kb_article", "ticket",
	// "conversation", "agent_execution", "sla", etc.
	SAEntityType = "entity_type"
	SAEntityID   = "entity_id"

	// SAAgentID scopes ai-run workflows by the agent being executed.
	SAAgentID = "agent_id"
)
