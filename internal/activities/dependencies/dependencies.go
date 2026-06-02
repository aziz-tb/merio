package dependencies

import (
	aiagents "github.com/aziz-tb/merio/internal/ai/agents"
	"github.com/aziz-tb/merio/internal/ai/embeddings"
	aiexecutions "github.com/aziz-tb/merio/internal/ai/executions"
	"github.com/aziz-tb/merio/internal/ai/llm"
	aicatalog "github.com/aziz-tb/merio/internal/ai/llm/catalog"
	aipreset "github.com/aziz-tb/merio/internal/ai/llm/preset"
	aioperations "github.com/aziz-tb/merio/internal/ai/operations"
	"github.com/aziz-tb/merio/internal/ai/reranker"
	airunner "github.com/aziz-tb/merio/internal/ai/runner"
	aitools "github.com/aziz-tb/merio/internal/ai/tools"
	aiusage "github.com/aziz-tb/merio/internal/ai/usage"
	businesshours "github.com/aziz-tb/merio/internal/business_hours"
	"github.com/aziz-tb/merio/internal/contract"
	"github.com/aziz-tb/merio/internal/conversation"
	"github.com/aziz-tb/merio/internal/conversation/priority"
	"github.com/aziz-tb/merio/internal/conversation/status"
	"github.com/aziz-tb/merio/internal/customer"
	"github.com/aziz-tb/merio/internal/detection"
	execrunner "github.com/aziz-tb/merio/internal/exec/runner"
	exectoken "github.com/aziz-tb/merio/internal/exec/token"
	"github.com/aziz-tb/merio/internal/function"
	"github.com/aziz-tb/merio/internal/knowledge"
	"github.com/aziz-tb/merio/internal/realtime"
	"github.com/aziz-tb/merio/internal/setting"
	"github.com/aziz-tb/merio/internal/sla"
	"github.com/aziz-tb/merio/internal/tag"
	"github.com/aziz-tb/merio/internal/team"
	"github.com/aziz-tb/merio/internal/user"
	usermodels "github.com/aziz-tb/merio/internal/user/models"
	"github.com/aziz-tb/merio/internal/verification"
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
