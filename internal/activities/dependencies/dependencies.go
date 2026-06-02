package dependencies

import (
	aiagents "github.com/abhinavxd/libredesk/internal/ai/agents"
	"github.com/abhinavxd/libredesk/internal/ai/embeddings"
	aiexecutions "github.com/abhinavxd/libredesk/internal/ai/executions"
	"github.com/abhinavxd/libredesk/internal/ai/llm"
	aicatalog "github.com/abhinavxd/libredesk/internal/ai/llm/catalog"
	aipreset "github.com/abhinavxd/libredesk/internal/ai/llm/preset"
	aioperations "github.com/abhinavxd/libredesk/internal/ai/operations"
	"github.com/abhinavxd/libredesk/internal/ai/reranker"
	airunner "github.com/abhinavxd/libredesk/internal/ai/runner"
	aitools "github.com/abhinavxd/libredesk/internal/ai/tools"
	aiusage "github.com/abhinavxd/libredesk/internal/ai/usage"
	businesshours "github.com/abhinavxd/libredesk/internal/business_hours"
	"github.com/abhinavxd/libredesk/internal/contract"
	"github.com/abhinavxd/libredesk/internal/conversation"
	"github.com/abhinavxd/libredesk/internal/conversation/priority"
	"github.com/abhinavxd/libredesk/internal/conversation/status"
	"github.com/abhinavxd/libredesk/internal/customer"
	"github.com/abhinavxd/libredesk/internal/detection"
	execrunner "github.com/abhinavxd/libredesk/internal/exec/runner"
	exectoken "github.com/abhinavxd/libredesk/internal/exec/token"
	"github.com/abhinavxd/libredesk/internal/function"
	"github.com/abhinavxd/libredesk/internal/knowledge"
	"github.com/abhinavxd/libredesk/internal/realtime"
	"github.com/abhinavxd/libredesk/internal/setting"
	"github.com/abhinavxd/libredesk/internal/sla"
	"github.com/abhinavxd/libredesk/internal/tag"
	"github.com/abhinavxd/libredesk/internal/team"
	"github.com/abhinavxd/libredesk/internal/user"
	usermodels "github.com/abhinavxd/libredesk/internal/user/models"
	"github.com/abhinavxd/libredesk/internal/verification"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/go-i18n"
	"github.com/redis/go-redis/v9"
	"github.com/zerodha/logf"
)

// Dependencies is the worker-wide dependency graph available to Temporal
// activities. Individual activity packages select the fields they need.
type Dependencies struct {
	DB    *sqlx.DB
	Redis *redis.Client
	I18n  *i18n.I18n
	Lo    *logf.Logger

	Loggers map[string]*logf.Logger

	AIModelCatalog *aicatalog.Catalog
	AIPresets      *aipreset.Registry
	LLMProvider    llm.LLMProvider

	EmbeddingProvider embeddings.Provider
	RerankProvider    reranker.Provider
	EmbeddingStore    *embeddings.Store
	EmbeddingSearcher *embeddings.Searcher
	QueryEmbedCache   *embeddings.QueryEmbeddingCache

	Knowledge    *knowledge.Manager
	Usage        *aiusage.Manager
	AIOperations *aioperations.Manager
	Settings     *setting.Manager

	BusinessHours *businesshours.Manager
	Teams         *team.Manager
	Users         *user.Manager
	SLA           *sla.Manager
	Statuses      *status.Manager
	Priorities    *priority.Manager
	Tags          *tag.Manager
	Conversation  *conversation.Manager

	Agents     *aiagents.Manager
	Tools      *aitools.Manager
	Executions *aiexecutions.Manager

	ExecToken  *exectoken.Manager
	ExecRunner *execrunner.Manager
	Functions  *function.Manager

	Customers    *customer.Manager
	Contracts    *contract.Manager
	Verification *verification.Manager

	Detection *detection.Manager
	Realtime  realtime.Publisher

	AgentRunner *airunner.Runner
	RunEvents   airunner.EventPublisher

	SystemInboxID int
	SystemUserID  int
	SystemUser    usermodels.User
}

func (d Dependencies) Logger(name string) *logf.Logger {
	if d.Loggers != nil {
		if lo := d.Loggers[name]; lo != nil {
			return lo
		}
	}
	return d.Lo
}
