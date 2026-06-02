package agentvalidation

import (
	"fmt"

	activitydeps "github.com/abhinavxd/libredesk/internal/activities/dependencies"
	aiagents "github.com/abhinavxd/libredesk/internal/ai/agents"
	aioperations "github.com/abhinavxd/libredesk/internal/ai/operations"
	"github.com/abhinavxd/libredesk/internal/contract"
	"github.com/abhinavxd/libredesk/internal/conversation"
	"github.com/abhinavxd/libredesk/internal/customer"
	"github.com/abhinavxd/libredesk/internal/detection"
	"github.com/abhinavxd/libredesk/internal/realtime"
	"github.com/jmoiron/sqlx"
	"github.com/zerodha/logf"
)

type Activities struct {
	DB              *sqlx.DB
	Lo              *logf.Logger
	AgentsMgr       *aiagents.Manager
	ConversationMgr *conversation.Manager
	CustomerMgr     *customer.Manager
	ContractMgr     *contract.Manager
	DetectionMgr    *detection.Manager
	AIOperationsMgr *aioperations.Manager
	Realtime        realtime.Publisher
}

func New(a *Activities) (*Activities, error) {
	if a == nil {
		return nil, fmt.Errorf("agentvalidation: Activities required")
	}
	if a.DB == nil {
		return nil, fmt.Errorf("agentvalidation: DB required")
	}
	if a.Lo == nil {
		return nil, fmt.Errorf("agentvalidation: Lo required")
	}
	if a.AgentsMgr == nil {
		return nil, fmt.Errorf("agentvalidation: AgentsMgr required")
	}
	if a.ConversationMgr == nil {
		return nil, fmt.Errorf("agentvalidation: ConversationMgr required")
	}
	if a.DetectionMgr == nil {
		return nil, fmt.Errorf("agentvalidation: DetectionMgr required")
	}
	if a.AIOperationsMgr == nil {
		return nil, fmt.Errorf("agentvalidation: AIOperationsMgr required")
	}
	if a.Realtime == nil {
		a.Realtime = realtime.NoopPublisher{}
	}
	return a, nil
}

func NewActivities(deps activitydeps.Dependencies) (*Activities, error) {
	return New(&Activities{
		DB:              deps.DB,
		Lo:              deps.Logger("agent-validation"),
		AgentsMgr:       deps.Agents,
		ConversationMgr: deps.Conversation,
		CustomerMgr:     deps.Customers,
		ContractMgr:     deps.Contracts,
		DetectionMgr:    deps.Detection,
		AIOperationsMgr: deps.AIOperations,
		Realtime:        deps.Realtime,
	})
}
