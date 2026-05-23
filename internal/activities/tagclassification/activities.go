package tagclassification

import (
	"fmt"

	"github.com/abhinavxd/libredesk/internal/ai/llm"
	aioperations "github.com/abhinavxd/libredesk/internal/ai/operations"
	"github.com/abhinavxd/libredesk/internal/ai/usage"
	"github.com/abhinavxd/libredesk/internal/conversation"
	"github.com/abhinavxd/libredesk/internal/realtime"
	"github.com/abhinavxd/libredesk/internal/tag"
	umodels "github.com/abhinavxd/libredesk/internal/user/models"
	"github.com/jmoiron/sqlx"
	"github.com/zerodha/logf"
)

// Activities holds dependencies for tag-classification activities.
type Activities struct {
	DB              *sqlx.DB
	Lo              *logf.Logger
	LLMProvider     llm.LLMProvider
	UsageMgr        *usage.Manager
	AIOperationsMgr *aioperations.Manager
	TagMgr          *tag.Manager
	ConversationMgr *conversation.Manager
	Realtime        realtime.Publisher
	SystemInboxID   int
	SystemUser      umodels.User
}

func New(a *Activities) (*Activities, error) {
	if a == nil {
		return nil, fmt.Errorf("tagclassification: Activities required")
	}
	if a.DB == nil {
		return nil, fmt.Errorf("tagclassification: DB required")
	}
	if a.Lo == nil {
		return nil, fmt.Errorf("tagclassification: Lo required")
	}
	if a.LLMProvider == nil {
		return nil, fmt.Errorf("tagclassification: LLMProvider required")
	}
	if a.UsageMgr == nil {
		return nil, fmt.Errorf("tagclassification: UsageMgr required")
	}
	if a.AIOperationsMgr == nil {
		return nil, fmt.Errorf("tagclassification: AIOperationsMgr required")
	}
	if a.TagMgr == nil {
		return nil, fmt.Errorf("tagclassification: TagMgr required")
	}
	if a.ConversationMgr == nil {
		return nil, fmt.Errorf("tagclassification: ConversationMgr required")
	}
	if a.Realtime == nil {
		a.Realtime = realtime.NoopPublisher{}
	}
	if a.SystemInboxID == 0 {
		return nil, fmt.Errorf("tagclassification: SystemInboxID required")
	}
	if a.SystemUser.ID == 0 {
		return nil, fmt.Errorf("tagclassification: SystemUser required")
	}
	return a, nil
}
