// Command temporal_worker runs the Temporal worker pool process. One
// binary, one process; inside the process, one worker per task queue
// (see internal/temporal/queues.go). Workers can be split into
// separate hosts later by deploying this binary multiple times with
// different --queues filters, but v1 runs all queues in one process.
//
// Build: go build -o temporal-worker ./cmd/temporal_worker
// Run:   ./temporal-worker --config /path/to/config.toml
//
// Graceful shutdown: SIGINT/SIGTERM cancels the root context; each
// worker drains in-flight activities before its goroutine returns;
// the process exits when all goroutines have returned or
// shutdown_grace elapses (whichever comes first).
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/v2"
	_ "github.com/lib/pq"
	flag "github.com/spf13/pflag"
	"github.com/zerodha/logf"

	actagentvalidation "github.com/abhinavxd/libredesk/internal/activities/agentvalidation"
	actairuns "github.com/abhinavxd/libredesk/internal/activities/airuns"
	actcustomeridentification "github.com/abhinavxd/libredesk/internal/activities/customeridentification"
	activitydeps "github.com/abhinavxd/libredesk/internal/activities/dependencies"
	actembeddings "github.com/abhinavxd/libredesk/internal/activities/embeddings"
	acttagclassification "github.com/abhinavxd/libredesk/internal/activities/tagclassification"
	aiagents "github.com/abhinavxd/libredesk/internal/ai/agents"
	"github.com/abhinavxd/libredesk/internal/ai/embeddings"
	voyageembed "github.com/abhinavxd/libredesk/internal/ai/embeddings/voyage"
	aiexecutions "github.com/abhinavxd/libredesk/internal/ai/executions"
	aillm "github.com/abhinavxd/libredesk/internal/ai/llm"
	anthropicllm "github.com/abhinavxd/libredesk/internal/ai/llm/anthropic"
	aicatalog "github.com/abhinavxd/libredesk/internal/ai/llm/catalog"
	aipreset "github.com/abhinavxd/libredesk/internal/ai/llm/preset"
	aioperations "github.com/abhinavxd/libredesk/internal/ai/operations"
	"github.com/abhinavxd/libredesk/internal/ai/reranker"
	voyagerank "github.com/abhinavxd/libredesk/internal/ai/reranker/voyage"
	airunner "github.com/abhinavxd/libredesk/internal/ai/runner"
	runnerevents "github.com/abhinavxd/libredesk/internal/ai/runner/events"
	aitools "github.com/abhinavxd/libredesk/internal/ai/tools"
	aicapabilities "github.com/abhinavxd/libredesk/internal/ai/tools/builtin/capabilities"
	"github.com/abhinavxd/libredesk/internal/ai/tools/resolver"
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
	"github.com/abhinavxd/libredesk/internal/migrations"
	"github.com/abhinavxd/libredesk/internal/realtime"
	"github.com/abhinavxd/libredesk/internal/setting"
	"github.com/abhinavxd/libredesk/internal/sla"
	"github.com/abhinavxd/libredesk/internal/tag"
	"github.com/abhinavxd/libredesk/internal/team"
	"github.com/abhinavxd/libredesk/internal/temporal"
	"github.com/abhinavxd/libredesk/internal/user"
	"github.com/abhinavxd/libredesk/internal/verification"
	wfagentvalidation "github.com/abhinavxd/libredesk/internal/workflows/agentvalidation"
	wfairuns "github.com/abhinavxd/libredesk/internal/workflows/airuns"
	wfcustomeridentification "github.com/abhinavxd/libredesk/internal/workflows/customeridentification"
	wfembeddings "github.com/abhinavxd/libredesk/internal/workflows/embeddings"
	wftagclassification "github.com/abhinavxd/libredesk/internal/workflows/tagclassification"

	"github.com/knadh/go-i18n"
	"github.com/knadh/stuffbin"
	"github.com/redis/go-redis/v9"
)

// Injected at build time via -ldflags. See Dockerfile.temporal-worker.
// When the binary is run outside a Docker build (e.g. `go run`), both
// strings are empty; that's intentional, the startup log handles the
// fallback.
var (
	buildString   string
	versionString string
)

func main() {
	version := versionString
	if version == "" {
		version = "dev"
	}
	build := buildString
	if build == "" {
		build = "unknown"
	}
	log.Printf("temporal-worker starting: version=%s build=%s", version, build)

	ko := koanf.New(".")
	initFlags(ko)
	loadConfig(ko)

	lo := initLogger(ko, "temporal-worker")

	cfg := loadTemporalConfig(ko)
	if !cfg.Enabled {
		lo.Error("temporal.enabled is false in config — refusing to start worker binary")
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	tc, err := temporal.New(temporal.Opts{Config: cfg, Lo: lo})
	if err != nil {
		log.Fatalf("temporal: dial: %v", err)
	}
	if tc == nil {
		// Belt-and-braces — Enabled was true so this shouldn't happen.
		log.Fatalf("temporal: New returned nil unexpectedly")
	}
	defer tc.Close()

	// Optional --queues filter so a deployment can run a subset of
	// queues per host without code changes.
	queueFilter := parseQueueFilter(ko.String("queues"))
	queues := selectQueues(temporal.AllQueues, queueFilter)
	if len(queues) == 0 {
		log.Fatalf("temporal-worker: no queues selected — check --queues filter")
	}
	deps := buildDependencies(ko, queues, lo)

	// Bring up the dependencies the embedding workflows need: DB,
	// Voyage provider, embeddings store, knowledge manager, ai_usage
	// manager, system inbox id. Only constructed when the embeddings
	// queue is in the requested set — keeps the worker lightweight
	// when running narrow deployments.
	var embedActs *actembeddings.Activities
	if queueWanted(queues, temporal.QueueEmbeddings) {
		embedActs = buildEmbeddingActivities(deps)
	}
	var aiRunActs *actairuns.Activities
	if queueWanted(queues, temporal.QueueAIRuns) {
		aiRunActs = buildAIRunActivities(deps)
	}
	var tagClassActs *acttagclassification.Activities
	var agentValidationActs *actagentvalidation.Activities
	var customerIdentificationActs *actcustomeridentification.Activities
	if queueWanted(queues, temporal.QueuePipeline) {
		tagClassActs = buildTagClassificationActivities(deps)
		agentValidationActs = buildAgentValidationActivities(deps)
		customerIdentificationActs = buildCustomerIdentificationActivities(deps)
	}

	lo.Info("temporal worker pool starting", "queues", queues)

	var wg sync.WaitGroup
	for _, q := range queues {
		q := q
		workerCfg := cfg.Workers[q]
		w, err := temporal.NewWorker(tc, q, workerCfg, initLogger(ko, "temporal-worker-"+q))
		if err != nil {
			log.Fatalf("temporal: new worker %s: %v", q, err)
		}

		// queue exclusively.
		if q == temporal.QueueEmbeddings && embedActs != nil {
			w.RegisterWorkflow(wfembeddings.EmbedArticleWorkflow)
			w.RegisterWorkflow(wfembeddings.CleanupOldVectorsWorkflow)
			w.RegisterWorkflow(wfembeddings.BulkReindexWorkflow)
			w.RegisterActivity(embedActs.FetchSourceContent)
			w.RegisterActivity(embedActs.ChunkContent)
			w.RegisterActivity(embedActs.RedactText)
			w.RegisterActivity(embedActs.GenerateEmbeddings)
			w.RegisterActivity(embedActs.StoreEmbeddings)
			w.RegisterActivity(embedActs.SwapActiveVersion)
			w.RegisterActivity(embedActs.GetIndexState)
			w.RegisterActivity(embedActs.DeleteOldVectors)
			w.RegisterActivity(embedActs.DeleteArticleVectors)
			w.RegisterActivity(embedActs.ListPublishedArticles)
		}
		if q == temporal.QueueAIRuns && aiRunActs != nil {
			w.RegisterWorkflow(wfairuns.AgentRunWorkflow)
			w.RegisterActivity(aiRunActs.RunAgentActivity)
		}
		if q == temporal.QueuePipeline && tagClassActs != nil {
			w.RegisterWorkflow(wftagclassification.TagClassificationWorkflow)
			w.RegisterActivity(tagClassActs.ClassifyTagsActivity)
			w.RegisterActivity(tagClassActs.NotifyClassificationFailureActivity)
			if agentValidationActs != nil {
				w.RegisterWorkflow(wfagentvalidation.AgentValidationWorkflow)
				w.RegisterActivity(agentValidationActs.ValidateAgentsActivity)
			}
			if customerIdentificationActs != nil {
				w.RegisterWorkflow(wfcustomeridentification.CustomerIdentificationWorkflow)
				w.RegisterActivity(customerIdentificationActs.CreateOperationActivity)
				w.RegisterActivity(customerIdentificationActs.LoadSettingsActivity)
				w.RegisterActivity(customerIdentificationActs.ShouldRunActivity)
				w.RegisterActivity(customerIdentificationActs.PrepareMessageActivity)
				w.RegisterActivity(customerIdentificationActs.PatternDetectActivity)
				w.RegisterActivity(customerIdentificationActs.LLMExtractActivity)
				w.RegisterActivity(customerIdentificationActs.SearchCustomerByFieldsActivity)
				w.RegisterActivity(customerIdentificationActs.LoadContractAndCustomerActivity)
				w.RegisterActivity(customerIdentificationActs.EvaluateVerificationActivity)
				w.RegisterActivity(customerIdentificationActs.PersistResultActivity)
			}
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := w.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
				lo.Error("worker exited with error", "queue", q, "error", err)
			}
		}()
	}

	<-ctx.Done()
	lo.Info("shutdown signal received; draining workers", "grace", cfg.ShutdownGrace.String())

	// Wait for all workers to drain, bounded by ShutdownGrace.
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
		lo.Info("temporal workers drained cleanly")
	case <-time.After(cfg.ShutdownGrace):
		lo.Warn("shutdown grace elapsed; exiting with in-flight work",
			"grace_seconds", int(cfg.ShutdownGrace.Seconds()))
	}
}

// initFlags wires command-line flags used by the worker binary.
// Mirrors the main app's flag style (knadh/koanf + spf13/pflag).
func initFlags(ko *koanf.Koanf) {
	f := flag.NewFlagSet("temporal-worker", flag.ContinueOnError)
	f.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		f.PrintDefaults()
	}
	f.StringSlice("config", []string{"config.toml"},
		"path to one or more config files (merged in order)")
	f.String("queues", "",
		"comma-separated subset of task queues to serve (empty = all). "+
			"Example: --queues=embeddings,default")

	if err := f.Parse(os.Args[1:]); err != nil {
		log.Fatalf("temporal-worker: parsing flags: %v", err)
	}
	if err := ko.Load(posflag.Provider(f, ".", ko), nil); err != nil {
		log.Fatalf("temporal-worker: loading flags: %v", err)
	}
}

// loadConfig loads TOML config + LIBREDESK_* env vars. Same shape as
// the main binary; the worker reads from the same config file so
// they always agree on Temporal address/namespace.
func loadConfig(ko *koanf.Koanf) {
	for _, p := range ko.Strings("config") {
		if err := ko.Load(file.Provider(p), toml.Parser()); err != nil {
			if !os.IsNotExist(err) {
				log.Fatalf("temporal-worker: loading %s: %v", p, err)
			}
		}
	}
	_ = ko.Load(env.Provider(".", env.Opt{
		Prefix: "LIBREDESK_",
		TransformFunc: func(key, val string) (string, any) {
			key = strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(key, "LIBREDESK_")), "__", ".")
			return key, val
		},
	}), nil)
}

// loadTemporalConfig pulls the [temporal] block (and its nested
// worker map) out of koanf. Defaults match what the main app uses.
func loadTemporalConfig(ko *koanf.Koanf) temporal.Config {
	cfg := temporal.Config{
		Enabled:       ko.Bool("temporal.enabled"),
		Address:       ko.String("temporal.address"),
		Namespace:     ko.String("temporal.namespace"),
		DialTimeout:   ko.Duration("temporal.dial_timeout"),
		ShutdownGrace: ko.Duration("temporal.shutdown_grace"),
		Workers:       map[string]temporal.WorkerConfig{},
	}
	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:7233"
	}
	if cfg.Namespace == "" {
		cfg.Namespace = "libredesk"
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 10 * time.Second
	}
	if cfg.ShutdownGrace == 0 {
		cfg.ShutdownGrace = 30 * time.Second
	}

	for _, q := range temporal.AllQueues {
		key := "temporal.workers." + q
		if !ko.Exists(key) {
			continue
		}
		cfg.Workers[q] = temporal.WorkerConfig{
			MaxConcurrentActivities: ko.Int(key + ".max_concurrent_activities"),
			MaxConcurrentWorkflows:  ko.Int(key + ".max_concurrent_workflows"),
		}
	}
	return cfg
}

// initLogger mirrors the main app's logger setup so worker logs go to
// the same place in the same format.
func initLogger(ko *koanf.Koanf, src string) *logf.Logger {
	lvl, envName := ko.String("app.log_level"), ko.String("app.env")
	if lvl == "" {
		lvl = "info"
	}
	color := envName == "dev"
	lo := logf.New(logf.Opts{
		Level:                logLevel(lvl),
		EnableColor:          color,
		EnableCaller:         true,
		CallerSkipFrameCount: 3,
		DefaultFields:        []any{"sc", src},
	})
	return &lo
}

func logLevel(lvl string) logf.Level {
	switch lvl {
	case "debug":
		return logf.DebugLevel
	case "warn":
		return logf.WarnLevel
	case "error":
		return logf.ErrorLevel
	case "fatal":
		return logf.FatalLevel
	default:
		return logf.InfoLevel
	}
}

// parseQueueFilter splits the --queues flag and returns the set of
// queue names requested. Empty string returns nil (meaning "all").
func parseQueueFilter(s string) map[string]bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	out := map[string]bool{}
	for _, q := range strings.Split(s, ",") {
		q = strings.TrimSpace(q)
		if q != "" {
			out[q] = true
		}
	}
	return out
}

// selectQueues filters all queues against the requested set. Returns
// all queues when filter is nil.
func selectQueues(all []string, filter map[string]bool) []string {
	if filter == nil {
		return all
	}
	out := make([]string, 0, len(filter))
	for _, q := range all {
		if filter[q] {
			out = append(out, q)
		}
	}
	return out
}

// queueWanted reports whether `q` appears in `selected`.
func queueWanted(selected []string, q string) bool {
	for _, s := range selected {
		if s == q {
			return true
		}
	}
	return false
}

func buildDependencies(ko *koanf.Koanf, queues []string, lo *logf.Logger) activitydeps.Dependencies {
	deps := activitydeps.Dependencies{
		DB:      openDB(ko),
		Redis:   openRedis(ko),
		I18n:    loadI18n(),
		Lo:      lo,
		Loggers: map[string]*logf.Logger{},
	}
	logger := func(name string) *logf.Logger {
		l := initLogger(ko, name)
		deps.Loggers[name] = l
		return l
	}

	needsEmbeddings := queueWanted(queues, temporal.QueueEmbeddings)
	needsAIRuns := queueWanted(queues, temporal.QueueAIRuns)
	needsPipeline := queueWanted(queues, temporal.QueuePipeline)
	needsAI := needsEmbeddings || needsAIRuns || needsPipeline
	needsLLM := needsAIRuns || needsPipeline
	needsExec := needsAIRuns || needsPipeline
	needsConversationGraph := needsAIRuns || needsPipeline
	needsEmbeddingSearch := needsEmbeddings || needsAIRuns

	deps.SystemInboxID = resolveSystemInboxID(deps.DB)
	deps.SystemUserID = resolveSystemUserID(deps.DB)

	if needsAI {
		var err error
		deps.AIModelCatalog, err = loadAIModelCatalog(ko)
		if err != nil {
			log.Fatalf("temporal-worker: load AI model catalog: %v", err)
		}
		deps.AIPresets, err = loadAIPresetRegistry(ko, deps.AIModelCatalog)
		if err != nil {
			log.Fatalf("temporal-worker: load AI preset registry: %v", err)
		}
		deps.Usage, err = aiusage.New(aiusage.Opts{
			DB:      deps.DB,
			Catalog: deps.AIModelCatalog,
			Lo:      logger("ai-usage"),
		})
		if err != nil {
			log.Fatalf("temporal-worker: ai_usage manager: %v", err)
		}
	}
	if needsLLM {
		deps.LLMProvider = buildAnthropicProvider(ko, deps.DB, deps.AIPresets, deps.AIModelCatalog)
		if deps.LLMProvider == nil {
			log.Fatalf("temporal-worker: Anthropic provider not configured — selected queues require it")
		}
	}
	if needsExec {
		var err error
		deps.ExecToken, err = exectoken.New(exectoken.Opts{
			Redis:  deps.Redis,
			Logger: logger("exec-token"),
		})
		if err != nil {
			log.Fatalf("temporal-worker: exec token manager: %v", err)
		}
		deps.ExecRunner, err = execrunner.New(execrunner.Opts{
			Token:           deps.ExecToken,
			DB:              deps.DB,
			Logger:          logger("exec-runner"),
			SandboxURL:      ko.MustString("exec.sandbox_url"),
			SandboxToken:    ko.MustString("exec.sandbox_token"),
			PlatformBaseURL: ko.MustString("exec.platform_base_url"),
			SystemUserID:    deps.SystemUserID,
		})
		if err != nil {
			log.Fatalf("temporal-worker: exec runner: %v", err)
		}
		deps.Functions, err = function.New(function.Opts{
			DB:     deps.DB,
			Runner: deps.ExecRunner,
			Lo:     logger("function"),
			I18n:   deps.I18n,
		})
		if err != nil {
			log.Fatalf("temporal-worker: function manager: %v", err)
		}
	}
	if needsEmbeddingSearch {
		var err error
		deps.Knowledge, err = knowledge.New(knowledge.Opts{DB: deps.DB, Lo: logger("knowledge"), I18n: deps.I18n})
		if err != nil {
			log.Fatalf("temporal-worker: knowledge manager: %v", err)
		}
		deps.EmbeddingProvider = buildEmbeddingProvider(deps.AIPresets, ko)
		if needsEmbeddings && deps.EmbeddingProvider == nil {
			log.Fatalf("temporal-worker: Voyage API key not set (provider_keys.voyage) — required for the embeddings queue")
		}
		deps.RerankProvider = buildRerankProvider(deps.AIPresets, ko)
		deps.EmbeddingStore, err = embeddings.NewStore(embeddings.StoreOpts{
			DB: deps.DB,
			Lo: logger("embeddings-store"),
		})
		if err != nil {
			log.Fatalf("temporal-worker: embeddings store: %v", err)
		}
		deps.QueryEmbedCache = buildQueryEmbeddingCache(ko, deps.Redis)
		deps.EmbeddingSearcher, err = buildSearcher(deps.EmbeddingStore, deps.EmbeddingProvider, deps.RerankProvider, deps.QueryEmbedCache, ko)
		if err != nil {
			log.Fatalf("temporal-worker: embeddings searcher: %v", err)
		}
		if needsAIRuns {
			aicapabilities.SetKBSearcher(deps.EmbeddingSearcher)
		}
	}
	if needsConversationGraph {
		var err error
		deps.Settings, err = setting.New(setting.Opts{
			DB:            deps.DB,
			Lo:            logger("setting"),
			EncryptionKey: ko.MustString("app.encryption_key"),
		})
		if err != nil {
			log.Fatalf("temporal-worker: setting manager: %v", err)
		}
		deps.BusinessHours, err = businesshours.New(businesshours.Opts{
			DB:   deps.DB,
			Lo:   logger("business-hours"),
			I18n: deps.I18n,
		})
		if err != nil {
			log.Fatalf("temporal-worker: business-hours manager: %v", err)
		}
		deps.Teams, err = team.New(team.Opts{DB: deps.DB, Lo: logger("team"), I18n: deps.I18n})
		if err != nil {
			log.Fatalf("temporal-worker: team manager: %v", err)
		}
		deps.Users, err = user.New(deps.I18n, user.Opts{DB: deps.DB, Lo: logger("user")})
		if err != nil {
			log.Fatalf("temporal-worker: user manager: %v", err)
		}
		deps.SLA, err = sla.New(
			sla.Opts{DB: deps.DB, Lo: logger("sla"), I18n: deps.I18n},
			deps.Teams,
			deps.Settings,
			deps.BusinessHours,
			nil,
			deps.Users,
			nil,
		)
		if err != nil {
			log.Fatalf("temporal-worker: sla manager: %v", err)
		}
		deps.Statuses, err = status.New(status.Opts{DB: deps.DB, Lo: logger("status"), I18n: deps.I18n})
		if err != nil {
			log.Fatalf("temporal-worker: status manager: %v", err)
		}
		deps.Priorities, err = priority.New(priority.Opts{DB: deps.DB, Lo: logger("priority"), I18n: deps.I18n})
		if err != nil {
			log.Fatalf("temporal-worker: priority manager: %v", err)
		}
		deps.Conversation, err = conversation.New(
			nil,
			deps.I18n,
			deps.SLA,
			deps.Statuses,
			deps.Priorities,
			nil,
			deps.Users,
			deps.Teams,
			nil,
			deps.Settings,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			conversation.Opts{
				DB:               deps.DB,
				Lo:               logger("conversation"),
				SubjectRefFormat: "#{ref}",
			},
		)
		if err != nil {
			log.Fatalf("temporal-worker: conversation manager: %v", err)
		}
	}
	if needsAIRuns || needsPipeline {
		var err error
		deps.Customers, err = customer.New(customer.Opts{
			DB:       deps.DB,
			Lo:       logger("customer"),
			I18n:     deps.I18n,
			Settings: deps.Settings,
			Function: deps.Functions,
		})
		if err != nil {
			log.Fatalf("temporal-worker: customer manager: %v", err)
		}
		deps.Contracts, err = contract.New(contract.Opts{
			DB:       deps.DB,
			Lo:       logger("contract"),
			I18n:     deps.I18n,
			Settings: deps.Settings,
			Function: deps.Functions,
			Customer: deps.Customers,
		})
		if err != nil {
			log.Fatalf("temporal-worker: contract manager: %v", err)
		}
	}
	if needsAIRuns || needsPipeline {
		var err error
		deps.Agents, err = aiagents.New(aiagents.Opts{DB: deps.DB, Lo: logger("ai-agents"), I18n: deps.I18n})
		if err != nil {
			log.Fatalf("temporal-worker: ai agents manager: %v", err)
		}
	}
	if needsAIRuns {
		var err error
		deps.Tools, err = aitools.New(aitools.Opts{DB: deps.DB, Lo: logger("ai-tools"), I18n: deps.I18n})
		if err != nil {
			log.Fatalf("temporal-worker: ai tools manager: %v", err)
		}
		deps.Executions, err = aiexecutions.New(aiexecutions.Opts{DB: deps.DB, Lo: logger("ai-executions"), I18n: deps.I18n})
		if err != nil {
			log.Fatalf("temporal-worker: ai executions manager: %v", err)
		}
		dispatcher := aitools.NewDispatcher(deps.Tools, deps.ExecRunner, deps.Functions)
		deps.AgentRunner, err = airunner.New(airunner.Opts{
			Provider:     deps.LLMProvider,
			Agents:       deps.Agents,
			Resolver:     resolver.New(deps.Tools, nil),
			Dispatcher:   dispatcher,
			PresetReg:    deps.AIPresets,
			Lo:           logger("agent-runner"),
			Knowledge:    deps.Knowledge,
			Conversation: deps.Conversation,
			User:         deps.Users,
			Customer:     deps.Customers,
			Contract:     deps.Contracts,
			Usage:        &workerRunnerUsageAdapter{m: deps.Usage},
		})
		if err != nil {
			log.Fatalf("temporal-worker: agent runner: %v", err)
		}
		deps.RunEvents, err = runnerevents.NewRedisStreamPublisher(deps.Redis, logger("ai-run-events"), ko.Duration("agent_run.event_ttl"))
		if err != nil {
			log.Fatalf("temporal-worker: ai run event publisher: %v", err)
		}
	}
	if needsPipeline {
		var err error
		deps.AIOperations, err = aioperations.New(aioperations.Opts{DB: deps.DB, Lo: logger("ai-operations")})
		if err != nil {
			log.Fatalf("temporal-worker: ai_operations manager: %v", err)
		}
		deps.Tags, err = tag.New(tag.Opts{DB: deps.DB, Lo: logger("tag"), I18n: deps.I18n})
		if err != nil {
			log.Fatalf("temporal-worker: tag manager: %v", err)
		}
		deps.Realtime, err = realtime.NewRedisPublisher(deps.Redis)
		if err != nil {
			log.Fatalf("temporal-worker: realtime publisher: %v", err)
		}
		deps.SystemUser, err = deps.Users.GetSystemUser()
		if err != nil {
			log.Fatalf("temporal-worker: resolve system user: %v", err)
		}
		deps.Detection = detection.New(detection.Opts{
			Lo:            logger("detection"),
			LLMProvider:   deps.LLMProvider,
			UsageMgr:      deps.Usage,
			SystemInboxID: deps.SystemInboxID,
		})
		deps.Verification = verification.New(verification.Opts{Lo: logger("verification")})
	}

	return deps
}

// buildEmbeddingActivities adapts the worker-wide dependency graph to the
// embedding activities package.
func buildEmbeddingActivities(deps activitydeps.Dependencies) *actembeddings.Activities {
	acts, err := actembeddings.NewActivities(deps)
	if err != nil {
		log.Fatalf("temporal-worker: embedding activities: %v", err)
	}
	return acts
}

func buildAIRunActivities(deps activitydeps.Dependencies) *actairuns.Activities {
	acts, err := actairuns.NewActivities(deps)
	if err != nil {
		log.Fatalf("temporal-worker: ai run activities: %v", err)
	}
	return acts
}

func buildTagClassificationActivities(deps activitydeps.Dependencies) *acttagclassification.Activities {
	acts, err := acttagclassification.NewActivities(deps)
	if err != nil {
		log.Fatalf("temporal-worker: tag classification activities: %v", err)
	}
	return acts
}

func buildAgentValidationActivities(deps activitydeps.Dependencies) *actagentvalidation.Activities {
	acts, err := actagentvalidation.NewActivities(deps)
	if err != nil {
		log.Fatalf("temporal-worker: agent validation activities: %v", err)
	}
	return acts
}

func buildCustomerIdentificationActivities(deps activitydeps.Dependencies) *actcustomeridentification.Activities {
	acts, err := actcustomeridentification.NewActivities(deps)
	if err != nil {
		log.Fatalf("temporal-worker: customer identification activities: %v", err)
	}
	return acts
}

// openDB connects to Postgres using the same [db] block the main app
// reads. Fatal on connection failure — the worker can't usefully run
// without DB access.
func openDB(ko *koanf.Koanf) *sqlx.DB {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s %s",
		ko.MustString("db.host"),
		ko.MustInt("db.port"),
		ko.MustString("db.user"),
		ko.MustString("db.password"),
		ko.MustString("db.database"),
		ko.String("db.ssl_mode"),
		ko.String("db.params"),
	)
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		log.Fatalf("temporal-worker: connect to DB: %v", err)
	}
	return db
}

func openRedis(ko *koanf.Koanf) *redis.Client {
	redisURL := ko.String("redis.url")
	if redisURL != "" {
		options, err := redis.ParseURL(redisURL)
		if err != nil {
			log.Fatalf("temporal-worker: parse redis url: %v", err)
		}
		return redis.NewClient(options)
	}
	return redis.NewClient(&redis.Options{
		Addr:     ko.MustString("redis.address"),
		Username: ko.String("redis.user"),
		Password: ko.String("redis.password"),
		DB:       ko.Int("redis.db"),
	})
}

func loadAIModelCatalog(ko *koanf.Koanf) (*aicatalog.Catalog, error) {
	path := ko.String("ai.models_catalog_path")
	if path == "" {
		path = "config/anthropic_models.json"
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return aicatalog.Load(path)
}

func loadAIPresetRegistry(ko *koanf.Koanf, cat *aicatalog.Catalog) (*aipreset.Registry, error) {
	path := ko.String("ai.presets_path")
	if path == "" {
		path = "config/ai_presets.json"
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return aipreset.Load(path, cat)
}

func buildAnthropicProvider(ko *koanf.Koanf, db *sqlx.DB, reg *aipreset.Registry, cat *aicatalog.Catalog) aillm.LLMProvider {
	apiKey := reg.ProviderKey("anthropic")
	if apiKey == "" {
		encKey := ko.MustString("app.encryption_key")
		k, err := anthropicllm.LookupAPIKey(db, encKey)
		if err != nil {
			log.Fatalf("temporal-worker: read anthropic api key: %v", err)
		}
		apiKey = k
	}
	if apiKey == "" {
		return nil
	}
	p, err := anthropicllm.New(anthropicllm.Opts{
		APIKey:   apiKey,
		Catalog:  cat,
		Registry: reg,
		Lo:       initLogger(ko, "anthropic"),
	})
	if err != nil {
		log.Fatalf("temporal-worker: anthropic provider: %v", err)
	}
	return p
}

func buildEmbeddingProvider(reg *aipreset.Registry, ko *koanf.Koanf) embeddings.Provider {
	apiKey := reg.ProviderKey("voyage")
	if apiKey == "" {
		return nil
	}
	p, err := voyageembed.New(voyageembed.Opts{
		APIKey:       apiKey,
		Concurrency:  ko.Int("embeddings.concurrency"),
		TotalTimeout: ko.Duration("embeddings.total_timeout"),
		Lo:           initLogger(ko, "voyage-embeddings"),
	})
	if err != nil {
		log.Fatalf("temporal-worker: voyage embeddings provider: %v", err)
	}
	return p
}

func buildRerankProvider(reg *aipreset.Registry, ko *koanf.Koanf) reranker.Provider {
	apiKey := reg.ProviderKey("voyage")
	if apiKey == "" {
		return nil
	}
	p, err := voyagerank.New(voyagerank.Opts{
		APIKey: apiKey,
		Lo:     initLogger(ko, "voyage-reranker"),
	})
	if err != nil {
		log.Fatalf("temporal-worker: voyage reranker: %v", err)
	}
	return p
}

func buildQueryEmbeddingCache(ko *koanf.Koanf, rdb *redis.Client) *embeddings.QueryEmbeddingCache {
	if !ko.Bool("embeddings.cache.enabled") {
		return nil
	}
	ttl := ko.Duration("embeddings.cache.ttl")
	if ttl == 0 {
		ttl = time.Hour
	}
	c, err := embeddings.NewQueryEmbeddingCache(rdb, ttl, initLogger(ko, "embeddings-cache"))
	if err != nil {
		log.Fatalf("temporal-worker: embeddings cache: %v", err)
	}
	return c
}

func buildSearcher(
	store *embeddings.Store,
	embed embeddings.Provider,
	rank reranker.Provider,
	cache *embeddings.QueryEmbeddingCache,
	ko *koanf.Koanf,
) (*embeddings.Searcher, error) {
	return embeddings.NewConfiguredSearcher(ko, embeddings.SearcherOpts{
		Store:    store,
		Embed:    embed,
		Reranker: rank,
		Cache:    cache,
		Lo:       initLogger(ko, "embeddings-searcher"),
	})
}

func resolveSystemUserID(db *sqlx.DB) int {
	const systemEmail = "system@libredesk.local"
	var id int
	if err := db.QueryRow(
		`SELECT id FROM users WHERE email = $1 AND type = 'agent' AND deleted_at IS NULL`,
		systemEmail,
	).Scan(&id); err != nil {
		log.Fatalf("temporal-worker: resolve system user: %v", err)
	}
	return id
}

func resolveSystemInboxID(db *sqlx.DB) int {
	var id int
	if err := db.QueryRow(`SELECT id FROM inboxes WHERE uuid = $1`, migrations.SystemInboxUUID).Scan(&id); err != nil {
		log.Fatalf("temporal-worker: resolve system inbox: %v", err)
	}
	return id
}

type workerRunnerUsageAdapter struct {
	m *aiusage.Manager
}

func (a *workerRunnerUsageAdapter) Record(ctx context.Context, in airunner.UsageRow) error {
	if a == nil || a.m == nil {
		return nil
	}
	return a.m.Record(ctx, aiusage.Row{
		CreatedAt:           time.Now().UTC(),
		TraceID:             in.TraceID,
		CallSequenceInTrace: in.CallSequenceInTrace,
		InboxID:             in.InboxID,
		ConversationID:      in.ConversationID,
		AgentID:             in.AgentID,
		UserID:              in.UserID,
		Provider:            in.Provider,
		Model:               in.Model,
		PresetSlug:          in.PresetSlug,
		Purpose:             in.Purpose,
		StopReason:          in.StopReason,
		InputTokens:         in.InputTokens,
		OutputTokens:        in.OutputTokens,
		CachedInputTokens:   in.CachedInputTokens,
		CacheMarkersUsed:    in.CacheMarkersUsed,
		LatencyMs:           in.LatencyMs,
		RetryCount:          in.RetryCount,
		Outcome:             in.Outcome,
		ProviderRequestID:   in.ProviderRequestID,
		ProviderMetadata:    in.ProviderMetadata,
		RequestPayload:      in.RequestPayload,
		ResponsePayload:     in.ResponsePayload,
	})
}

// loadI18n loads a minimal i18n with the default English catalog. The
// worker doesn't render user-facing messages, but knowledge.Manager
// requires a non-nil I18n at construction.
func loadI18n() *i18n.I18n {
	// Try to read the bundled JSON file from the local filesystem so
	// the worker doesn't need stuffbin-embedded assets.
	candidates := []string{"i18n/en.json", "./i18n/en.json"}
	for _, p := range candidates {
		if raw, err := os.ReadFile(p); err == nil {
			if inst, err := i18n.New(raw); err == nil {
				return inst
			}
		}
	}
	// Fallback to an empty catalog — keys round-trip as themselves,
	// good enough for the worker's path (no user-facing rendering).
	inst, _ := i18n.New([]byte(`{}`))
	return inst
}

// stuffbinUnused keeps the stuffbin import linked if loadI18n later
// switches to embedded assets; for now we read from the local FS.
var _ = stuffbin.NewLocalFS
