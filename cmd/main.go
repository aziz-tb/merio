package main

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	_ "time/tzdata"

	activitylog "github.com/aziz-tb/merio/internal/activity_log"
	aiLegacy "github.com/aziz-tb/merio/internal/ai/legacy"
	auth_ "github.com/aziz-tb/merio/internal/auth"
	"github.com/aziz-tb/merio/internal/authz"
	businesshours "github.com/aziz-tb/merio/internal/business_hours"
	"github.com/aziz-tb/merio/internal/colorlog"
	"github.com/aziz-tb/merio/internal/csat"
	customAttribute "github.com/aziz-tb/merio/internal/custom_attribute"
	"github.com/aziz-tb/merio/internal/macro"
	notifier "github.com/aziz-tb/merio/internal/notification"
	"github.com/aziz-tb/merio/internal/report"
	"github.com/aziz-tb/merio/internal/search"
	"github.com/aziz-tb/merio/internal/sla"
	umodels "github.com/aziz-tb/merio/internal/user/models"
	"github.com/aziz-tb/merio/internal/view"
	"github.com/redis/go-redis/v9"

	aiagents "github.com/aziz-tb/merio/internal/ai/agents"
	"github.com/aziz-tb/merio/internal/ai/embeddings"
	aiexecutions "github.com/aziz-tb/merio/internal/ai/executions"
	"github.com/aziz-tb/merio/internal/ai/llm"
	aicatalog "github.com/aziz-tb/merio/internal/ai/llm/catalog"
	aipreset "github.com/aziz-tb/merio/internal/ai/llm/preset"
	aioperations "github.com/aziz-tb/merio/internal/ai/operations"
	"github.com/aziz-tb/merio/internal/ai/reranker"
	"github.com/aziz-tb/merio/internal/ai/runner"
	aitools "github.com/aziz-tb/merio/internal/ai/tools"
	aibuiltin "github.com/aziz-tb/merio/internal/ai/tools/builtin"
	aiusage "github.com/aziz-tb/merio/internal/ai/usage"
	aiusagepartitions "github.com/aziz-tb/merio/internal/ai/usage/partitions"
	// aicapabilities is imported by alias so cmd/main.go can call
	// SetKBSearcher after the embeddings subsystem boots. The init()
	// blocks under capabilities/ still fire on import and register
	// kb_search / get_customer_data / get_conversation_history into
	// builtin.Default().
	aicapabilities "github.com/aziz-tb/merio/internal/ai/tools/builtin/capabilities"
	"github.com/aziz-tb/merio/internal/automation"
	"github.com/aziz-tb/merio/internal/contract"
	"github.com/aziz-tb/merio/internal/conversation"
	"github.com/aziz-tb/merio/internal/conversation/priority"
	"github.com/aziz-tb/merio/internal/conversation/status"
	"github.com/aziz-tb/merio/internal/customer"
	"github.com/aziz-tb/merio/internal/detection"
	execrunner "github.com/aziz-tb/merio/internal/exec/runner"
	exectoken "github.com/aziz-tb/merio/internal/exec/token"
	"github.com/aziz-tb/merio/internal/function"
	"github.com/aziz-tb/merio/internal/importer"
	"github.com/aziz-tb/merio/internal/inbox"
	"github.com/aziz-tb/merio/internal/knowledge"
	"github.com/aziz-tb/merio/internal/media"
	"github.com/aziz-tb/merio/internal/oidc"
	"github.com/aziz-tb/merio/internal/ratelimit"
	"github.com/aziz-tb/merio/internal/role"
	"github.com/aziz-tb/merio/internal/secret"
	"github.com/aziz-tb/merio/internal/setting"
	"github.com/aziz-tb/merio/internal/tag"
	"github.com/aziz-tb/merio/internal/team"
	"github.com/aziz-tb/merio/internal/template"
	"github.com/aziz-tb/merio/internal/temporal"
	"github.com/aziz-tb/merio/internal/user"
	"github.com/aziz-tb/merio/internal/webhook"
	"github.com/aziz-tb/merio/internal/worklog"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/go-i18n"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"github.com/zerodha/logf"
)

var (
	ko          = koanf.New(".")
	ctx         = context.Background()
	appName     = "merio"
	frontendDir = "frontend/dist/main"
	widgetDir   = "frontend/dist/widget"

	// Injected at build time.
	buildString   string
	versionString string
)

const (
	sampleEncKey = "your-32-char-random-string-here!"
)

// App is the global app context which is passed and injected in the http handlers.
type App struct {
	ctx        context.Context
	db         *sqlx.DB
	fs         stuffbin.FileSystem
	consts     atomic.Value
	auth       *auth_.Auth
	authz      *authz.Enforcer
	i18n       *i18n.I18n
	lo         *logf.Logger
	oidc       *oidc.Manager
	media      *media.Manager
	setting    *setting.Manager
	role       *role.Manager
	user       *user.Manager
	team       *team.Manager
	status     *status.Manager
	priority   *priority.Manager
	tag        *tag.Manager
	inbox      *inbox.Manager
	tmpl       *template.Manager
	macro      *macro.Manager
	knowledge  *knowledge.Manager
	execToken  *exectoken.Manager
	execRunner *execrunner.Manager
	// toolRegistry is nil in v1 — handleToolCall returns 404 until the AI
	// Manager lands and provides a registry implementation.
	toolRegistry     toolRegistry
	worklog          *worklog.Manager
	detectionMgr     *detection.Manager
	conversation     *conversation.Manager
	automation       *automation.Engine
	businessHours    *businesshours.Manager
	sla              *sla.Manager
	csat             *csat.Manager
	view             *view.Manager
	ai               *aiLegacy.Manager
	aiAgents         *aiagents.Manager
	aiTools          *aitools.Manager
	aiToolDispatcher *aitools.Dispatcher
	function         *function.Manager
	secret           *secret.Manager
	customer         *customer.Manager
	contract         *contract.Manager
	aiExecutions     *aiexecutions.Manager
	aiOperations     *aioperations.Manager
	aiUsage          *aiusage.Manager
	// systemInboxID is the id of the seeded "System (Internal)" inbox
	// (see migration v2_7_7 / SystemInboxUUID). Resolved once at boot
	// by UUID; every ai_usage row that has no real conversation
	// context FKs to this id. Zero if seeding failed — boot is
	// expected to fail before that path.
	systemInboxID int
	// AI runtime: model catalog (loaded from config/anthropic_models.json)
	// and preset registry (loaded from config/ai_presets.json). Both are
	// read-only after startup; reload requires app restart in v1.
	aiCatalog *aicatalog.Catalog
	aiPresets *aipreset.Registry
	// LLM provider — nil when no Anthropic API key is configured.
	// Run endpoint returns 503 in that case.
	aiProvider llm.LLMProvider
	// Agent runner — nil when aiProvider is nil. The handler checks.
	agentRunner *runner.Runner
	// aiRunEvents publishes live progress for manual agent runs.
	aiRunEvents      runner.EventPublisher
	search           *search.Manager
	activityLog      *activitylog.Manager
	notifier         *notifier.Service
	userNotification *notifier.UserNotificationManager
	customAttribute  *customAttribute.Manager
	report           *report.Manager
	webhook          *webhook.Manager
	rateLimit        *ratelimit.Limiter
	redis            *redis.Client
	importer         *importer.Importer
	// temporal is the workflow client used by handlers and managers
	// to start async workflows. Nil when Temporal is disabled or the
	// dial failed at startup; callers MUST check before use and fall
	// back to sync or refuse loudly (see internal/temporal/CONVENTIONS.md).
	temporal *temporal.Client
	// F12 hybrid-search engine — used directly by the kb_search
	// capability via aicapabilities.SetKBSearcher. Stored on App for
	// future direct uses (e.g., admin "preview search" endpoints).
	// Nil when Voyage key is unset.
	searcher *embeddings.Searcher
	// embeddingsStore is the synchronous DB surface used by handlers
	// (Unpublish / Delete) that need to drop vectors without spinning
	// up a workflow. Always populated (no provider dependency).
	embeddingsStore *embeddings.Store
	// reranker is held so cmd/main.go can advertise its readiness in
	// the boot log; the Searcher already holds its own reference.
	reranker reranker.Provider

	// Global state that stores data on an available app update.
	update *AppUpdate
	// Flag to indicate if app restart is required for settings to take effect.
	restartRequired bool
	sync.Mutex
}

func main() {
	// Set up signal handler.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if len(os.Args) > 1 && os.Args[1] == "secrets" {
		runSecretsCLI(ctx, os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "dev-seed-inbound" {
		runDevSeedInboundCLI(ctx, os.Args[2:])
		return
	}

	// Load command line flags into Koanf.
	initFlags()

	if ko.Bool("version") {
		fmt.Println(buildString)
		os.Exit(0)
	}

	// Build string injected at build time.
	colorlog.Green("Build: %s", buildString)

	// version=<...> token mirrors the temporal-worker startup line so
	// `make deploy-verify` can grep both services the same way.
	appVersion := versionString
	if appVersion == "" {
		appVersion = "dev"
	}
	colorlog.Green("merio starting: version=%s build=%s", appVersion, buildString)

	// Load the config files into Koanf.
	initConfig(ko)

	// Validate custom static directory if provided.
	if dir := getCustomStaticDir(); dir != "" {
		if _, err := os.Stat(dir); err != nil {
			log.Fatalf("--static-dir path not accessible: %s: %v", dir, err)
		}
		colorlog.Green("Using custom static directory: %s", dir)
	}

	// Init stuffbin fs with optional custom static dir overlay.
	fs := initFS(getCustomStaticDir())

	db := initDB()

	if ko.Bool("install") {
		install(ctx, db, fs, ko.Bool("idempotent-install"), !ko.Bool("yes"))
		os.Exit(0)
	}

	if ko.Bool("set-system-user-password") {
		setSystemUserPass(ctx, db)
		os.Exit(0)
	}

	// Check if schema is installed.
	installed, err := checkSchema(db)
	if err != nil {
		log.Fatalf("error checking db schema: %v", err)
	}
	if !installed {
		log.Println("database tables are missing. Use the `--install` flag to set up the database schema.")
		os.Exit(0)
	}

	if ko.Bool("upgrade") {
		upgrade(db, fs, !ko.Bool("yes"))
		os.Exit(0)
	}

	checkPendingUpgrade(db)

	// Load app settings from DB into the Koanf instance.
	settings := initSettings(db)
	loadSettings(settings)

	validateConfig(ko)

	// Fallback for config typo. Logs a warning but continues to work with the incorrect key.
	// Uses 'message.message_outgoing_scan_interval' (correct key) as default key, falls back to the common typo.
	msgOutgoingScanIntervalKey := "message.message_outgoing_scan_interval"
	if ko.String(msgOutgoingScanIntervalKey) == "" {
		if ko.String("message.message_outoing_scan_interval") != "" {
			colorlog.Red("WARNING: typo in config key 'message.message_outoing_scan_interval' detected. Use 'message.message_outgoing_scan_interval' instead in your config.toml file. Support for this incorrect key will be removed in a future release.")
			msgOutgoingScanIntervalKey = "message.message_outoing_scan_interval"
		}
	}

	var (
		autoAssignInterval          = ko.MustDuration("autoassigner.autoassign_interval")
		unsnoozeInterval            = ko.MustDuration("conversation.unsnooze_interval")
		draftRetentionDuration      = cmp.Or(ko.Duration("conversation.draft_retention_duration"), 360*time.Hour)
		automationWorkers           = ko.MustInt("automation.worker_count")
		messageOutgoingQWorkers     = ko.MustDuration("message.outgoing_queue_workers")
		messageIncomingQWorkers     = ko.MustDuration("message.incoming_queue_workers")
		messageOutgoingScanInterval = ko.MustDuration(msgOutgoingScanIntervalKey)
		slaEvaluationInterval       = ko.MustDuration("sla.evaluation_interval")
		lo                          = initLogger(appName)
		rdb                         = initRedis()
		execTokenMgr                = initExecToken(rdb)
		systemUserID                = initSystemUserID(db)
		constants                   = initConstants()
		i18n                        = initI18n(fs)
		secretsKEK                  = initSecretsKEK()
		secretMgr                   = initSecret(db, i18n, secretsKEK)
		execRunnerMgr               = initExecRunner(db, execTokenMgr, systemUserID, secretMgr)
		csat                        = initCSAT(db, i18n)
		oidc                        = initOIDC(db, settings, i18n)
		status                      = initStatus(db, i18n)
		priority                    = initPriority(db, i18n)
		auth                        = initAuth(oidc, rdb, i18n)
		template                    = initTemplate(db, fs, constants, i18n)
		media                       = initMedia(db, i18n, settings)
		inbox                       = initInbox(db, i18n)
		team                        = initTeam(db, i18n)
		businessHours               = initBusinessHours(db, i18n)
		webhook                     = initWebhook(db, i18n)
		user                        = initUser(i18n, db)
		wsHub                       = initWS(user)
		notifier                    = initNotifier()
		userNotification            = initUserNotification(db, i18n)
		notifDispatcher             = initNotifDispatcher(userNotification, notifier, wsHub, ko.Bool("notification.email.enabled"))
		sla                         = initSLA(db, team, settings, businessHours, template, user, i18n, notifDispatcher)
		worklogMgr                  = initWorklog(db, i18n)
		rateLimiter                 = initRateLimit(rdb)
		// Catalog and preset registry are loaded here so the registry's
		// model-existence validation has the catalog available. Both fail
		// fast on bad config — see initAIModelCatalog / initAIPresetRegistry.
		aiCatalogInst     = initAIModelCatalog()
		aiPresetsInst     = initAIPresetRegistry(aiCatalogInst)
		aiProviderInst    = initAnthropicProvider(db, aiPresetsInst, aiCatalogInst)
		aiUsageInst       = initAIUsage(db, aiCatalogInst)
		aiOperationsInst  = initAIOperations(db)
		systemInboxIDInst = initSystemInboxID(db)
		detectionMgrInst  = initDetectionManager(aiProviderInst, aiUsageInst, systemInboxIDInst)
		temporalClient    = initTemporal()
		functionMgr       = initFunction(db, execRunnerMgr, i18n)
		customerMgr       = initCustomer(db, i18n, settings, functionMgr)
		contractMgr       = initContract(db, i18n, settings, functionMgr, customerMgr)
		automation        = initAutomationEngine(db, i18n, detectionMgrInst, customerMgr, contractMgr)
		conversation      = initConversations(i18n, sla, status, priority, wsHub, db, inbox, user, team, media, settings, csat, automation, template, webhook, worklogMgr, notifDispatcher, temporalClient)
		autoassigner      = initAutoAssigner(team, user, conversation)
		// F12: hybrid-search subsystem. Each piece is independently
		// nil-safe — if Voyage key is absent or Redis is down, kb_search
		// surfaces a clear error at first invocation rather than
		// crashing the boot.
		embedProviderInst   = initEmbeddingProvider(aiPresetsInst)
		rerankProviderInst  = initRerankProvider(aiPresetsInst)
		embeddingsStoreInst = initEmbeddingsStore(db)
		queryEmbedCacheInst = initQueryEmbeddingCache(rdb)
		searcherInst        = initSearcher(embeddingsStoreInst, embedProviderInst, rerankProviderInst, queryEmbedCacheInst)
	)

	// Log the registered built-in AI capabilities. The init() blocks in
	// internal/ai/tools/builtin/capabilities/* fire on import (blank
	// import in this file's import block); this line confirms the
	// registry is populated before the agent runtime reads from it.
	colorlog.Green("AI built-in capabilities: %v", aibuiltin.Default().IDs())

	wsHub.SetConversationStore(conversation)
	automation.SetConversationStore(conversation)

	// Periodic re-broadcast of conversation viewer presence (option-a reconciliation).
	wsHub.StartViewerReconciler(ctx)

	// Start inboxes.
	startInboxes(ctx, inbox, conversation, user, conversation.SignAvatarURL)

	go automation.Run(ctx, automationWorkers)
	go autoassigner.Run(ctx, autoAssignInterval)
	go conversation.Run(ctx, messageIncomingQWorkers, messageOutgoingQWorkers, messageOutgoingScanInterval)
	go conversation.RunUnsnoozer(ctx, unsnoozeInterval)
	go conversation.RunContinuity(ctx)
	go webhook.Run(ctx)
	go notifier.Run(ctx)
	go sla.Run(ctx, slaEvaluationInterval)
	go sla.SendNotifications(ctx)
	go media.DeleteUnlinkedMedia(ctx)
	go user.MonitorUserAvailability(ctx, onUsersOffline(conversation))
	go conversation.RunDraftCleaner(ctx, draftRetentionDuration)
	go userNotification.RunNotificationCleaner(ctx)

	var app = &App{
		ctx:             ctx,
		db:              db,
		lo:              lo,
		fs:              fs,
		sla:             sla,
		oidc:            oidc,
		i18n:            i18n,
		auth:            auth,
		media:           media,
		setting:         settings,
		inbox:           inbox,
		user:            user,
		team:            team,
		csat:            csat,
		status:          status,
		priority:        priority,
		tmpl:            template,
		notifier:        notifier,
		consts:          atomic.Value{},
		worklog:         worklogMgr,
		detectionMgr:    detectionMgrInst,
		conversation:    conversation,
		automation:      automation,
		businessHours:   businessHours,
		activityLog:     initActivityLog(db, i18n),
		customAttribute: initCustomAttribute(db, i18n),
		authz:           initAuthz(i18n),
		view:            initView(db, i18n),
		report:          initReport(db, i18n),
		search:          initSearch(db, i18n),
		role:            initRole(db, i18n),
		tag:             initTag(db, i18n),
		macro:           initMacro(db, i18n),
		knowledge:       initKnowledge(db, i18n),
		execToken:       execTokenMgr,
		execRunner:      execRunnerMgr,
		ai:              initAI(db, i18n),
		aiAgents:        initAIAgents(db, i18n),
		aiTools:         initAITools(db, i18n),
		function:        functionMgr,
		secret:          secretMgr,
		customer:        customerMgr,
		contract:        contractMgr,
		aiExecutions:    initAIExecutions(db, i18n),
		aiOperations:    aiOperationsInst,
		aiUsage:         aiUsageInst,
		systemInboxID:   systemInboxIDInst,
		aiCatalog:       aiCatalogInst,
		aiPresets:       aiPresetsInst,
		aiProvider:      aiProviderInst,
		// Dispatcher bridges ai_tools to the F6 runner for manual
		// invocation. Wired here (after the AI managers and execRunner
		// are constructed) so handlers can reach it via app.aiToolDispatcher.
		aiToolDispatcher: nil, // assigned below — see line just after this struct literal
		importer:         initImporter(i18n),
		webhook:          webhook,
		rateLimit:        rateLimiter,
		redis:            rdb,
		userNotification: userNotification,
		// Temporal client: nil when disabled or dial failed. The main
		// API binary stays up either way; workflow-starting handlers
		// branch on app.temporal == nil.
		temporal: temporalClient,
		// Assigned below once the shared Redis-backed publisher is built.
		aiRunEvents: nil,
		// F12 hybrid-search engine. Nil when Voyage key unset; kb_search
		// surfaces that as a clear error at invocation time.
		searcher:        searcherInst,
		embeddingsStore: embeddingsStoreInst,
		reranker:        rerankProviderInst,
	}
	// Wire the searcher into the kb_search capability. The capability
	// is registered at package init() with nil; this assignment makes
	// hybrid search live for every agent that lists kb_search in tools.
	aicapabilities.SetKBSearcher(searcherInst)

	// --reindex-embeddings: dispatch the bulk-reindex Temporal workflow
	// and exit. Runs after all subsystems are wired so the Temporal
	// client + searcher are available, but before the HTTP server
	// starts.
	if ko.Bool("reindex-embeddings") {
		runReindexEmbeddings(app.temporal)
	}
	app.consts.Store(constants)
	app.aiRunEvents = initAgentRunEventPublisher(rdb)
	startRealtimeBridge(ctx, rdb, wsHub, initLogger("realtime"))
	// Wire the AI tool dispatcher now that App is built — it needs both
	// the tools manager and the execRunner.
	app.aiToolDispatcher = aitools.NewDispatcher(app.aiTools, app.execRunner, app.function)
	// Wire the agent runner — depends on the dispatcher above. Returns
	// nil when the Anthropic provider isn't configured; the run endpoint
	// returns 503 in that case.
	app.agentRunner = initAgentRunner(
		app.aiProvider,
		app.aiAgents,
		app.aiTools,
		app.aiToolDispatcher,
		app.aiPresets,
		app.knowledge,
		app.conversation,
		app.user,
		app.customer,
		app.contract,
		app.aiUsage,
		app.aiRunEvents,
	)

	// Keep the ai_usage monthly partition runway extended. Boot-time
	// EnsureRunway is a no-op when the migration's pre-created
	// partitions are still in the future; the daily goroutine takes
	// over as months roll. See internal/ai/usage/partitions.
	usagePartitions := aiusagepartitions.New(aiusagepartitions.Opts{
		DB:      db,
		Table:   "ai_usage",
		Premake: 4,
		Lo:      initLogger("ai-usage-partitions"),
	})
	if err := usagePartitions.EnsureRunway(context.Background()); err != nil {
		log.Fatalf("error ensuring ai_usage partition runway: %v", err)
	}
	go usagePartitions.RunDaily(context.Background())

	g := fastglue.NewGlue()
	g.SetContext(app)
	initHandlers(g, wsHub)

	s := &fasthttp.Server{
		Name:                 appName,
		ReadTimeout:          ko.MustDuration("app.server.read_timeout"),
		WriteTimeout:         ko.MustDuration("app.server.write_timeout"),
		MaxRequestBodySize:   ko.MustInt("app.server.max_body_size"),
		MaxKeepaliveDuration: ko.MustDuration("app.server.keepalive_timeout"),
		ReadBufferSize:       ko.Int("app.server.read_buffer_size"),
	}

	go func() {
		colorlog.Green("Server started at %s", ko.String("app.server.address"))
		if ko.String("server.socket") != "" {
			colorlog.Green("Unix socket created at %s", ko.String("server.socket"))
		}
		if err := g.ListenAndServe(ko.String("app.server.address"), ko.String("server.socket"), s); err != nil {
			log.Fatalf("error starting server: %v", err)
		}
	}()

	// Start the app update checker.
	if ko.Bool("app.check_updates") {
		go checkUpdates(versionString, time.Hour*1, app)
	}

	// Wait for shutdown signal.
	<-ctx.Done()
	colorlog.Red("Shutting down HTTP server...")
	s.Shutdown()
	colorlog.Red("Shutting down inboxes...")
	inbox.Close()
	colorlog.Red("Shutting down automation...")
	automation.Close()
	colorlog.Red("Shutting down autoassigner...")
	autoassigner.Close()
	colorlog.Red("Shutting down notifier...")
	notifier.Close()
	colorlog.Red("Shutting down webhook...")
	webhook.Close()
	colorlog.Red("Shutting down conversation...")
	conversation.Close()
	colorlog.Red("Shutting down SLA...")
	sla.Close()
	colorlog.Red("Shutting down importer...")
	app.importer.Close()
	if app.temporal != nil {
		colorlog.Red("Shutting down Temporal client...")
		app.temporal.Close()
	}
	colorlog.Red("Shutting down database...")
	db.Close()
	colorlog.Red("Shutting down redis...")
	rdb.Close()
	colorlog.Green("Shutdown complete.")
}

// onUsersOffline returns a callback for MonitorUserAvailability that broadcasts
// offline status to the appropriate clients based on user type.
func onUsersOffline(conv *conversation.Manager) func([]umodels.OfflineUser) {
	return func(users []umodels.OfflineUser) {
		for _, u := range users {
			switch u.Type {
			case umodels.UserTypeAgent:
				conv.BroadcastAgentStatusToWidget(u.ID, umodels.Offline)
			case umodels.UserTypeContact, umodels.UserTypeVisitor:
				conv.BroadcastContactUpdate(u.ID, map[string]any{"availability_status": umodels.Offline})
			}
		}
	}
}
