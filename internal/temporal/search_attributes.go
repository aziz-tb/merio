package temporal

// Search-attribute names. Every workflow attaches these at start time
// (see CONVENTIONS.md). They must be registered with the Temporal
// server once per namespace; the worker binary verifies registration
// at boot and registers any missing ones via the operator service.
//
// Adding a new attribute: append a constant here AND extend
// EnsureSearchAttributes to register it.
const (
	// SATraceID correlates a Temporal execution with rows in ai_usage
	// and script_executions for that ticket. Always populated.
	SATraceID = "trace_id"

	// SAInboxID — per-inbox filtering in the Temporal UI. Populated
	// for workflows triggered from a conversation context; zero for
	// system / cron workflows.
	SAInboxID = "inbox_id"

	// SAWorkflowKind — high-level category. Values: "embedding",
	// "pipeline", "ai-run", "sla-timer", "hello". Used for
	// dashboard slice-and-dice.
	SAWorkflowKind = "workflow_kind"

	// SAEntityType + SAEntityID identify the thing being processed.
	// Values for SAEntityType: "kb_article", "ticket",
	// "conversation", "agent_execution", "sla", etc.
	SAEntityType = "entity_type"
	SAEntityID   = "entity_id"

	// SAAgentID scopes ai-run workflows by the agent being executed.
	// Stored as a KEYWORD string like every other custom attribute.
	SAAgentID = "agent_id"
)
