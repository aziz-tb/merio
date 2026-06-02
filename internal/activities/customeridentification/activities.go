package customeridentification

import (
	"context"
	"embed"
	"errors"
	"fmt"

	activitydeps "github.com/aziz-tb/merio/internal/activities/dependencies"
	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/aziz-tb/merio/internal/ai/usage"
	"github.com/aziz-tb/merio/internal/contract"
	"github.com/aziz-tb/merio/internal/customer"
	"github.com/aziz-tb/merio/internal/dbutil"
	execmodels "github.com/aziz-tb/merio/internal/exec/runner/models"
	"github.com/aziz-tb/merio/internal/function"
	"github.com/aziz-tb/merio/internal/realtime"
	"github.com/aziz-tb/merio/internal/verification"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/types"
	"github.com/zerodha/logf"
)

//go:embed queries.sql
var efs embed.FS

type settingStore interface {
	GetByPrefix(prefix string) (types.JSONText, error)
}

type functionRunner interface {
	Run(ctx context.Context, req function.RunRequest) (*execmodels.Result, error)
}

type Activities struct {
	DB              *sqlx.DB
	Lo              *logf.Logger
	Settings        settingStore
	Function        functionRunner
	LLMProvider     llm.LLMProvider
	UsageMgr        *usage.Manager
	CustomerMgr     *customer.Manager
	ContractMgr     *contract.Manager
	VerificationMgr *verification.Manager
	Realtime        realtime.Publisher
	SystemInboxID   int
	q               queries
}

type queries struct {
	GetMessageForIdentification        *sqlx.Stmt `query:"get-message-for-identification"`
	FindFreshCustomerByEmail           *sqlx.Stmt `query:"find-fresh-customer-by-email"`
	FindFreshContractByEmail           *sqlx.Stmt `query:"find-fresh-contract-by-email"`
	GetConversationIdentificationLinks string     `query:"get-conversation-identification-links"`
	UpdateConversationIdentification   string     `query:"update-conversation-identification"`
	UpdateContactLinkedCustomer        string     `query:"update-contact-linked-customer"`
	InsertAIOperation                  string     `query:"insert-ai-operation"`
	UpdateAIOperation                  string     `query:"update-ai-operation"`
}

func New(a *Activities) (*Activities, error) {
	if a == nil {
		return nil, errors.New("customeridentification: Activities required")
	}
	if a.DB == nil {
		return nil, errors.New("customeridentification: DB required")
	}
	if a.Lo == nil {
		return nil, errors.New("customeridentification: Lo required")
	}
	if a.Settings == nil {
		return nil, errors.New("customeridentification: Settings required")
	}
	if a.Function == nil {
		return nil, errors.New("customeridentification: Function required")
	}
	if a.LLMProvider == nil {
		return nil, errors.New("customeridentification: LLMProvider required")
	}
	if a.UsageMgr == nil {
		return nil, errors.New("customeridentification: UsageMgr required")
	}
	if a.CustomerMgr == nil {
		return nil, errors.New("customeridentification: CustomerMgr required")
	}
	if a.ContractMgr == nil {
		return nil, errors.New("customeridentification: ContractMgr required")
	}
	if a.VerificationMgr == nil {
		a.VerificationMgr = verification.New(verification.Opts{Lo: a.Lo})
	}
	if a.Realtime == nil {
		a.Realtime = realtime.NoopPublisher{}
	}
	if a.SystemInboxID == 0 {
		return nil, errors.New("customeridentification: SystemInboxID required")
	}
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, a.DB, efs); err != nil {
		return nil, fmt.Errorf("customeridentification: prepare queries: %w", err)
	}
	a.q = q
	return a, nil
}

func NewActivities(deps activitydeps.Dependencies) (*Activities, error) {
	return New(&Activities{
		DB:              deps.DB,
		Lo:              deps.Logger("customer-identification"),
		Settings:        deps.Settings,
		Function:        deps.Functions,
		LLMProvider:     deps.LLMProvider,
		UsageMgr:        deps.Usage,
		CustomerMgr:     deps.Customers,
		ContractMgr:     deps.Contracts,
		VerificationMgr: deps.Verification,
		Realtime:        deps.Realtime,
		SystemInboxID:   deps.SystemInboxID,
	})
}

func intPtrToInt(p *int64) int {
	if p == nil {
		return 0
	}
	return int(*p)
}
