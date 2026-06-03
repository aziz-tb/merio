package main

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"html/template"

	activitylog "github.com/aziz-tb/merio/internal/activity_log"
	aiagents "github.com/aziz-tb/merio/internal/ai/agents"
	"github.com/aziz-tb/merio/internal/ai/embeddings"
	voyageembed "github.com/aziz-tb/merio/internal/ai/embeddings/voyage"
	aiexecutions "github.com/aziz-tb/merio/internal/ai/executions"
	aiLegacy "github.com/aziz-tb/merio/internal/ai/legacy"
	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/aziz-tb/merio/internal/ai/llm/anthropic"
	aicatalog "github.com/aziz-tb/merio/internal/ai/llm/catalog"
	aipreset "github.com/aziz-tb/merio/internal/ai/llm/preset"
	aioperations "github.com/aziz-tb/merio/internal/ai/operations"
	"github.com/aziz-tb/merio/internal/ai/reranker"
	voyagerank "github.com/aziz-tb/merio/internal/ai/reranker/voyage"
	"github.com/aziz-tb/merio/internal/ai/runner"
	runnerevents "github.com/aziz-tb/merio/internal/ai/runner/events"
	aitools "github.com/aziz-tb/merio/internal/ai/tools"
	"github.com/aziz-tb/merio/internal/ai/tools/resolver"
	aiusage "github.com/aziz-tb/merio/internal/ai/usage"
	auth_ "github.com/aziz-tb/merio/internal/auth"
	"github.com/aziz-tb/merio/internal/authz"
	"github.com/aziz-tb/merio/internal/autoassigner"
	"github.com/aziz-tb/merio/internal/automation"
	businesshours "github.com/aziz-tb/merio/internal/business_hours"
	"github.com/aziz-tb/merio/internal/colorlog"
	"github.com/aziz-tb/merio/internal/contract"
	"github.com/aziz-tb/merio/internal/conversation"
	"github.com/aziz-tb/merio/internal/conversation/priority"
	"github.com/aziz-tb/merio/internal/conversation/status"
	"github.com/aziz-tb/merio/internal/csat"
	customAttribute "github.com/aziz-tb/merio/internal/custom_attribute"
	"github.com/aziz-tb/merio/internal/customer"
	"github.com/aziz-tb/merio/internal/detection"
	execrunner "github.com/aziz-tb/merio/internal/exec/runner"
	exectoken "github.com/aziz-tb/merio/internal/exec/token"
	"github.com/aziz-tb/merio/internal/function"
	"github.com/aziz-tb/merio/internal/importer"
	"github.com/aziz-tb/merio/internal/inbox"
	"github.com/aziz-tb/merio/internal/inbox/channel/email"
	"github.com/aziz-tb/merio/internal/inbox/channel/livechat"
	imodels "github.com/aziz-tb/merio/internal/inbox/models"
	"github.com/aziz-tb/merio/internal/knowledge"
	"github.com/aziz-tb/merio/internal/macro"
	"github.com/aziz-tb/merio/internal/media"
	fs "github.com/aziz-tb/merio/internal/media/stores/localfs"
	"github.com/aziz-tb/merio/internal/media/stores/s3"
	"github.com/aziz-tb/merio/internal/migrations"
	notifier "github.com/aziz-tb/merio/internal/notification"
	emailnotifier "github.com/aziz-tb/merio/internal/notification/providers/email"
	"github.com/aziz-tb/merio/internal/oidc"
	"github.com/aziz-tb/merio/internal/ratelimit"
	"github.com/aziz-tb/merio/internal/report"
	"github.com/aziz-tb/merio/internal/role"
	"github.com/aziz-tb/merio/internal/search"
	"github.com/aziz-tb/merio/internal/secret"
	"github.com/aziz-tb/merio/internal/setting"
	"github.com/aziz-tb/merio/internal/sla"
	"github.com/aziz-tb/merio/internal/tag"
	"github.com/aziz-tb/merio/internal/team"
	tmpl "github.com/aziz-tb/merio/internal/template"
	"github.com/aziz-tb/merio/internal/temporal"
	"github.com/aziz-tb/merio/internal/user"
	usermodels "github.com/aziz-tb/merio/internal/user/models"
	"github.com/aziz-tb/merio/internal/view"
	"github.com/aziz-tb/merio/internal/webhook"
	"github.com/aziz-tb/merio/internal/worklog"
	"github.com/aziz-tb/merio/internal/ws"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/go-i18n"
	kjson "github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	flag "github.com/spf13/pflag"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/logf"
)

// constants holds the app constants.
type constants struct {
	AppBaseURL                  string
	FaviconURL                  string
	LogoURL                     string
	SiteName                    string
	UploadProvider              string
	AllowedUploadFileExtensions []string
	MaxFileUploadSizeMB         int
}

// Config loads config from files and environment variables into koanf.
func initConfig(ko *koanf.Koanf) {
	for _, f := range ko.Strings("config") {
		log.Println("reading config file:", f)
		if err := ko.Load(file.Provider(f), toml.Parser()); err != nil {
			if os.IsNotExist(err) {
				log.Printf("WARNING: Config file not found. Continuing with defaults and environment variables.")
				continue
			}
			log.Fatalf("error loading config from file: %v.", err)
		}
	}
	// Load environment variables with `MERIO_` prefix.
	ko.Load(env.Provider(".", env.Opt{
		Prefix: "MERIO_",
		TransformFunc: func(key, val string) (string, any) {
			// Transform the key.
			key = strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(key, "MERIO_")), "__", ".")
			return key, val
		},
	}), nil)
	configRootURL = normalizeRootURL(ko.String("app.root_url"))
}

func normalizeRootURL(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), "/")
}

func applyConfigRuntimeOverrides() {
	if configRootURL != "" {
		ko.Set("app.root_url", configRootURL)
	}
}

// validateConfig logs warnings/fatals for invalid config values.
func validateConfig(ko *koanf.Koanf) {
	encKey := ko.MustString("app.encryption_key")

	if len(encKey) != 32 {
		log.Fatalf("encryption_key must be exactly 32 characters, got %d", len(encKey))
	}

	// Warn if using sample config value.
	if encKey == sampleEncKey {
		colorlog.Red("WARNING: You are using the sample encryption_key from config.sample.toml. Change it immediately. Generate a secure key with `openssl rand -hex 16`")
	}
}

// initFlags initializes the commandline flags.
func initFlags() {
	f := flag.NewFlagSet("config", flag.ContinueOnError)

	// Registering `--help` handler.
	f.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		f.PrintDefaults()
	}

	// Register the commandline flags and parse them.
	f.StringSlice("config", []string{"config.toml"},
		"path to one or more config files (will be merged in order)")
	f.Bool("version", false, "show current version of the build")
	f.Bool("install", false, "setup database")
	f.Bool("idempotent-install", false, "run idempotent installation, i.e., skip installion if schema is already installed useful for the first time setup")
	f.Bool("yes", false, "skip confirmation prompt")
	f.Bool("upgrade", false, "upgrade the database schema")
	f.Bool("set-system-user-password", false, "set password for the system user")
	f.String("static-dir", "", "path to a directory with custom static files and templates to override the defaults")

	// F12 RAG reindex CLI. --reindex-embeddings starts a Temporal
	// BulkReindexWorkflow; the binary exits after dispatch (the workflow
	// runs on the embeddings worker pool).
	f.Bool("reindex-embeddings", false, "trigger a Temporal bulk-reindex of embeddings and exit")
	f.String("reindex-content-type", "kb_chunk", "content type to reindex (default: kb_chunk)")
	f.Int("reindex-batch-size", 100, "articles per page during bulk reindex (default: 100)")
	f.Int("reindex-parallelism", 4, "concurrent child workflows in flight (default: 4)")

	if err := f.Parse(os.Args[1:]); err != nil {
		log.Fatalf("loading flags: %v", err)
	}

	if err := ko.Load(posflag.Provider(f, ".", ko), nil); err != nil {
		log.Fatalf("loading config: %v", err)
	}
}

// initConstants initializes the app constants.
func initConstants() *constants {
	return &constants{
		AppBaseURL:                  ko.String("app.root_url"),
		FaviconURL:                  ko.String("app.favicon_url"),
		LogoURL:                     ko.String("app.logo_url"),
		SiteName:                    ko.String("app.site_name"),
		UploadProvider:              ko.MustString("upload.provider"),
		AllowedUploadFileExtensions: ko.Strings("app.allowed_file_upload_extensions"),
		MaxFileUploadSizeMB:         ko.Int("app.max_file_upload_size"),
	}
}

// initFS initializes the stuffbin FileSystem. If staticDir is set, files from
// that directory are merged into the FS, overriding embedded defaults.
func initFS(staticDir string) stuffbin.FileSystem {
	// Custom static dir mirrors the structure of the built-in static/ directory.
	staticFiles := []string{
		"./:static",
	}

	// Get self executable path.
	path, err := os.Executable()
	if err != nil {
		log.Fatalf("error initializing FS: %v", err)
	}

	// Load embedded files in the executable.
	fs, err := stuffbin.UnStuff(path)

	if err != nil {
		if err == stuffbin.ErrNoID {
			// Running in local/dev mode, use the local filesystem.
			// Only include frontend dirs if they exist (frontend build is optional in dev).
			colorlog.Red("binary unstuff failed, using local filesystem for static files")
			files := []string{"i18n", "static"}
			for _, d := range []string{"frontend/dist/main", "frontend/dist/widget"} {
				if _, err := os.Stat(d); err == nil {
					files = append(files, d)
				}
			}
			fs, err = stuffbin.NewLocalFS("/", files...)
			if err != nil {
				log.Fatalf("error initializing local FS: %v", err)
			}
		} else {
			log.Fatalf("error initializing FS: %v", err)
		}
	}

	// Merge custom static files if a custom static dir is provided.
	if staticDir != "" {
		// Only include paths that exist in the custom dir.
		var sf []string
		for _, def := range staticFiles {
			src := strings.Split(def, ":")[0]
			if _, err := os.Stat(filepath.Join(staticDir, src)); err == nil {
				sf = append(sf, def)
			}
		}

		if len(sf) > 0 {
			files := joinFSPaths(staticDir, sf)
			fLocal, err := stuffbin.NewLocalFS("/", files...)
			if err != nil {
				log.Fatalf("error loading custom static files from '%s': %v", staticDir, err)
			}
			if err := fs.Merge(fLocal); err != nil {
				log.Fatalf("error merging custom static files from '%s': %v", staticDir, err)
			}
		}
	}

	return fs
}

// joinFSPaths joins a root directory with stuffbin path specs (local:virtual).
func joinFSPaths(root string, paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		f := strings.Split(p, ":")
		out = append(out, filepath.Join(root, f[0])+":"+f[1])
	}
	return out
}

// loadSettings loads settings from the DB into Koanf map.
func loadSettings(m *setting.Manager) {
	j, err := m.GetAllJSON()
	if err != nil {
		log.Fatalf("error parsing settings from DB: %v", err)
	}

	// Setting keys are dot separated, eg: app.favicon_url. Unflatten them into
	// nested maps {app: {favicon_url}}.
	var out map[string]interface{}

	if err := json.Unmarshal(j, &out); err != nil {
		log.Fatalf("error unmarshalling settings from DB: %v", err)
	}
	if err := ko.Load(confmap.Provider(out, "."), nil); err != nil {
		log.Fatalf("error parsing settings from DB: %v", err)
	}
	applyConfigRuntimeOverrides()
}

// initSettings inits setting manager.
func initSettings(db *sqlx.DB) *setting.Manager {
	s, err := setting.New(setting.Opts{
		DB:            db,
		Lo:            initLogger("settings"),
		EncryptionKey: ko.MustString("app.encryption_key"),
	})
	if err != nil {
		log.Fatalf("error initializing setting manager: %v", err)
	}
	return s
}

// initUser inits user manager.
func initUser(i18n *i18n.I18n, DB *sqlx.DB) *user.Manager {
	mgr, err := user.New(i18n, user.Opts{
		DB: DB,
		Lo: initLogger("user_manager"),
	})
	if err != nil {
		log.Fatalf("error initializing user manager: %v", err)
	}
	return mgr
}

// initWorklog inits the B1 worklog manager.
func initWorklog(db *sqlx.DB, i18n *i18n.I18n) *worklog.Manager {
	wl, err := worklog.New(worklog.Opts{
		DB:   db,
		Lo:   initLogger("worklog_manager"),
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing worklog manager: %v", err)
	}
	return wl
}

// initConversations inits conversation manager.
func initConversations(
	i18n *i18n.I18n,
	sla *sla.Manager,
	status *status.Manager,
	priority *priority.Manager,
	hub *ws.Hub,
	db *sqlx.DB,
	inboxStore *inbox.Manager,
	userStore *user.Manager,
	teamStore *team.Manager,
	mediaStore *media.Manager,
	settings *setting.Manager,
	csat *csat.Manager,
	automationEngine *automation.Engine,
	template *tmpl.Manager,
	webhook *webhook.Manager,
	worklog *worklog.Manager,
	dispatcher *notifier.Dispatcher,
	temporalClient *temporal.Client,
) *conversation.Manager {
	continuityConfig := &conversation.ContinuityConfig{}
	if ko.Exists("conversation.continuity_scan_interval") {
		continuityConfig.BatchCheckInterval = ko.MustDuration("conversation.continuity_scan_interval")
	}

	c, err := conversation.New(hub, i18n, sla, status, priority, inboxStore, userStore, teamStore, mediaStore, settings, csat, automationEngine, template, webhook, worklog, dispatcher, conversation.Opts{
		DB:                       db,
		Lo:                       initLogger("conversation_manager"),
		OutgoingMessageQueueSize: ko.MustInt("message.outgoing_queue_size"),
		IncomingMessageQueueSize: ko.MustInt("message.incoming_queue_size"),
		ContinuityConfig:         continuityConfig,
		SubjectRefFormat:         ko.String("conversation.subject_ref_format"),
		TemporalClient:           temporalClient,
	})
	if err != nil {
		log.Fatalf("error initializing conversation manager: %v", err)
	}
	return c
}

// initTag inits tag manager.
func initTag(db *sqlx.DB, i18n *i18n.I18n) *tag.Manager {
	var lo = initLogger("tag_manager")
	mgr, err := tag.New(tag.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing tags: %v", err)
	}
	return mgr
}

// initViews inits view manager.
func initView(db *sqlx.DB, i18n *i18n.I18n) *view.Manager {
	var lo = initLogger("view_manager")
	m, err := view.New(view.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing view manager: %v", err)
	}
	return m
}

// initMacro inits macro manager.
func initMacro(db *sqlx.DB, i18n *i18n.I18n) *macro.Manager {
	var lo = initLogger("macro")
	m, err := macro.New(macro.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing macro manager: %v", err)
	}
	return m
}

// initKnowledge inits knowledge base manager.
func initKnowledge(db *sqlx.DB, i18n *i18n.I18n) *knowledge.Manager {
	var lo = initLogger("knowledge")
	m, err := knowledge.New(knowledge.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing knowledge manager: %v", err)
	}
	return m
}

// initExecToken constructs the script-execution token manager. Reuses
// the shared Redis client.
func initExecToken(rdb *redis.Client) *exectoken.Manager {
	lo := initLogger("exec-token")
	mgr, err := exectoken.New(exectoken.Opts{
		Redis:  rdb,
		Logger: lo,
	})
	if err != nil {
		log.Fatalf("error initializing exec-token manager: %v", err)
	}
	return mgr
}

// initSystemUserID looks up the System Agent user's ID. The user is
// created by the v2.5.0 migration (and seeded in schema.sql for fresh
// installs). Boots fatally if the user is missing — that indicates the
// migration didn't run.
//
// The system user is identified by a stable email, not by ID, so this
// works across deployments.
func initSystemUserID(db *sqlx.DB) int {
	var id int
	err := db.QueryRow(
		`SELECT id FROM users WHERE email = $1 AND type = 'agent' AND deleted_at IS NULL`,
		usermodels.SystemUserEmail,
	).Scan(&id)
	if err != nil {
		log.Fatalf("System Agent user not found (email=%s) — did the v2.5.0 migration run? %v",
			usermodels.SystemUserEmail, err)
	}
	return id
}

// initExecRunner constructs the script-execution dispatcher. Fatally
// errors if the [exec] config block is missing or incomplete; that's
// deliberate, the platform should not boot in a half-configured state.
func initExecRunner(db *sqlx.DB, tokMgr *exectoken.Manager, systemUserID int, secretMgr *secret.Manager) *execrunner.Manager {
	lo := initLogger("exec-runner")
	mgr, err := execrunner.New(execrunner.Opts{
		Token:           tokMgr,
		DB:              db,
		Logger:          lo,
		Secrets:         secretMgr,
		SandboxURL:      ko.MustString("exec.sandbox_url"),
		SandboxToken:    ko.MustString("exec.sandbox_token"),
		PlatformBaseURL: ko.MustString("exec.platform_base_url"),
		SystemUserID:    systemUserID,
	})
	if err != nil {
		log.Fatalf("error initializing exec-runner: %v", err)
	}
	return mgr
}

func initSecretsKEK() []byte {
	kek, err := secret.LoadKEK()
	if err != nil {
		log.Fatalf("error loading secrets KEK from %s: %v", secret.KEKEnvVar, err)
	}
	colorlog.Green("Secrets KEK loaded successfully (length=%d)", len(kek))
	return kek
}

func initSecret(db *sqlx.DB, i18n *i18n.I18n, kek []byte) *secret.Manager {
	mgr, err := secret.New(secret.Opts{
		DB:   db,
		Lo:   initLogger("secret_manager"),
		I18n: i18n,
		KEK:  kek,
	})
	if err != nil {
		log.Fatalf("error initializing secret manager: %v", err)
	}
	return mgr
}

// initBusinessHours inits business hours manager.
func initBusinessHours(db *sqlx.DB, i18n *i18n.I18n) *businesshours.Manager {
	var lo = initLogger("business-hours")
	m, err := businesshours.New(businesshours.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing business hours manager: %v", err)
	}
	return m
}

// initSLA inits SLA manager.
func initSLA(db *sqlx.DB, teamManager *team.Manager, settings *setting.Manager, businessHours *businesshours.Manager, template *tmpl.Manager, userManager *user.Manager, i18n *i18n.I18n, dispatcher *notifier.Dispatcher) *sla.Manager {
	var lo = initLogger("sla")
	m, err := sla.New(sla.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	}, teamManager, settings, businessHours, template, userManager, dispatcher)
	if err != nil {
		log.Fatalf("error initializing SLA manager: %v", err)
	}
	return m
}

// initCSAT inits CSAT manager.
func initCSAT(db *sqlx.DB, i18n *i18n.I18n) *csat.Manager {
	var lo = initLogger("csat")
	m, err := csat.New(csat.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing CSAT manager: %v", err)
	}
	return m
}

// initWS inits websocket hub.
func initWS(user *user.Manager) *ws.Hub {
	return ws.NewHub(user)
}

// getCustomStaticDir returns the custom static directory path from CLI flag or config.
func getCustomStaticDir() string {
	dir := ko.String("static-dir")
	if dir == "" {
		dir = ko.String("app.static_dir")
	}
	if dir == "" {
		return ""
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	return abs
}

// initTemplates inits template manager.
func initTemplate(db *sqlx.DB, fs stuffbin.FileSystem, consts *constants, i18n *i18n.I18n) *tmpl.Manager {
	var (
		lo      = initLogger("template")
		funcMap = getTmplFuncs(consts, i18n)
	)
	tpls, err := stuffbin.ParseTemplatesGlob(funcMap, fs, "/static/email-templates/*.html")
	if err != nil {
		log.Fatalf("error parsing e-mail templates: %v", err)
	}
	webTpls, err := stuffbin.ParseTemplatesGlob(funcMap, fs, "/static/public/web-templates/*.html")
	if err != nil {
		log.Fatalf("error parsing web templates: %v", err)
	}

	m, err := tmpl.New(lo, db, webTpls, tpls, funcMap, i18n)
	if err != nil {
		log.Fatalf("error initializing template manager: %v", err)
	}
	return m
}

// getTmplFuncs returns the template functions.
func getTmplFuncs(consts *constants, i18n *i18n.I18n) template.FuncMap {
	return template.FuncMap{
		"RootURL": func() string {
			return consts.AppBaseURL
		},
		"FaviconURL": func() string {
			return consts.FaviconURL
		},
		"Date": func(layout string) string {
			if layout == "" {
				layout = time.ANSIC
			}
			return time.Now().Format(layout)
		},
		"LogoURL": func() string {
			return consts.LogoURL
		},
		"SiteName": func() string {
			return consts.SiteName
		},
		"L": func() interface{} {
			return i18n
		},
	}
}

// reloadSettings reloads the settings from the database into the Koanf instance.
func reloadSettings(app *App) error {
	app.lo.Info("reloading settings")
	j, err := app.setting.GetAllJSON()
	if err != nil {
		app.lo.Error("error parsing settings from DB", "error", err)
		return err
	}
	var out map[string]interface{}
	if err := json.Unmarshal(j, &out); err != nil {
		app.lo.Error("error unmarshalling settings from DB", "error", err)
		return err
	}
	app.Lock()
	err = ko.Load(confmap.Provider(out, "."), nil)
	applyConfigRuntimeOverrides()
	app.Unlock()
	if err != nil {
		app.lo.Error("error loading settings into koanf", "error", err)
		return err
	}
	newConsts := initConstants()
	app.consts.Store(newConsts)
	return nil
}

// reloadTemplates reloads the templates from the filesystem.
func reloadTemplates(app *App) error {
	app.lo.Info("reloading templates")
	funcMap := getTmplFuncs(app.consts.Load().(*constants), app.i18n)
	tpls, err := stuffbin.ParseTemplatesGlob(funcMap, app.fs, "/static/email-templates/*.html")
	if err != nil {
		app.lo.Error("error parsing email templates", "error", err)
		return err
	}
	webTpls, err := stuffbin.ParseTemplatesGlob(funcMap, app.fs, "/static/public/web-templates/*.html")
	if err != nil {
		app.lo.Error("error parsing web templates", "error", err)
		return err
	}

	return app.tmpl.Reload(webTpls, tpls, funcMap)
}

// initTeam inits team manager.
func initTeam(db *sqlx.DB, i18n *i18n.I18n) *team.Manager {
	var lo = initLogger("team-manager")
	mgr, err := team.New(team.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing team manager: %v", err)
	}
	return mgr
}

// initMedia inits media manager.
func initMedia(db *sqlx.DB, i18n *i18n.I18n, settings *setting.Manager) *media.Manager {
	var (
		store media.Store
		err   error
		lo    = initLogger("media")
	)
	switch s := ko.MustString("upload.provider"); s {
	case "s3":
		store, err = s3.New(s3.Opt{
			URL:        ko.String("upload.s3.url"),
			PublicURL:  ko.String("upload.s3.public_url"),
			AccessKey:  ko.String("upload.s3.access_key"),
			SecretKey:  ko.String("upload.s3.secret_key"),
			Region:     ko.String("upload.s3.region"),
			Bucket:     ko.String("upload.s3.bucket"),
			BucketPath: ko.String("upload.s3.bucket_path"),
			// All files are private by default.
			BucketType: "private",
			Expiry:     ko.Duration("upload.s3.expiry"),
		})
		if err != nil {
			log.Fatalf("error initializing s3 media store: %v", err)
		}
	case "fs":
		// Default expiry to 1h if not set.
		fsExpiry := ko.Duration("upload.fs.expiry")
		if fsExpiry == 0 {
			fsExpiry = 1 * time.Hour
		}
		store, err = fs.New(fs.Opts{
			UploadURI:  "/uploads",
			UploadPath: filepath.Clean(ko.String("upload.fs.upload_path")),
			RootURL: func() string {
				rootURL, err := settings.GetAppRootURL()
				if err != nil {
					// Fallback to config if settings fetch fails
					return ko.String("app.root_url")
				}
				return rootURL
			},
			SigningKey: ko.MustString("app.encryption_key"),
			Expiry:     fsExpiry,
		})
		if err != nil {
			log.Fatalf("error initializing fs media store: %v", err)
		}
	default:
		log.Fatalf("unknown media store: %s", s)
	}

	media, err := media.New(media.Opts{
		Store: store,
		Lo:    lo,
		DB:    db,
		I18n:  i18n,
	})
	if err != nil {
		log.Fatalf("error initializing media: %v", err)
	}
	return media
}

// initInbox initializes the inbox manager without registering inboxes.
func initInbox(db *sqlx.DB, i18n *i18n.I18n) *inbox.Manager {
	var lo = initLogger("inbox-manager")
	mgr, err := inbox.New(lo, db, i18n, ko.MustString("app.encryption_key"))
	if err != nil {
		log.Fatalf("error initializing inbox manager: %v", err)
	}
	return mgr
}

// initAIAgents initializes the ai_agents CRUD manager (F9).
func initAIAgents(db *sqlx.DB, i18n *i18n.I18n) *aiagents.Manager {
	var lo = initLogger("ai-agents")
	m, err := aiagents.New(aiagents.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing ai agents manager: %v", err)
	}
	return m
}

// initAITools initializes the ai_tools CRUD manager (F9).
func initAITools(db *sqlx.DB, i18n *i18n.I18n) *aitools.Manager {
	var lo = initLogger("ai-tools")
	m, err := aitools.New(aitools.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing ai tools manager: %v", err)
	}
	return m
}

// initFunction initializes the unified functions manager — the single home
// for every named, sandbox-runnable code blob (custom AI tools, customer-
// identification executors, future domains). It runs functions through the
// shared F6 exec runner.
func initFunction(db *sqlx.DB, runner *execrunner.Manager, i18n *i18n.I18n) *function.Manager {
	var lo = initLogger("function")
	m, err := function.New(function.Opts{
		DB:     db,
		Runner: runner,
		Lo:     lo,
		I18n:   i18n,
	})
	if err != nil {
		log.Fatalf("error initializing function manager: %v", err)
	}
	return m
}

func initCustomer(db *sqlx.DB, i18n *i18n.I18n, settings *setting.Manager, functions *function.Manager) *customer.Manager {
	m, err := customer.New(customer.Opts{
		DB:       db,
		Lo:       initLogger("customer"),
		I18n:     i18n,
		Settings: settings,
		Function: functions,
	})
	if err != nil {
		log.Fatalf("error initializing customer manager: %v", err)
	}
	return m
}

func initContract(db *sqlx.DB, i18n *i18n.I18n, settings *setting.Manager, functions *function.Manager, customers *customer.Manager) *contract.Manager {
	m, err := contract.New(contract.Opts{
		DB:       db,
		Lo:       initLogger("contract"),
		I18n:     i18n,
		Settings: settings,
		Function: functions,
		Customer: customers,
	})
	if err != nil {
		log.Fatalf("error initializing contract manager: %v", err)
	}
	return m
}

// initAIExecutions wires the persistence manager for agent_executions +
// agent_execution_outcomes (the Composer tab's storage layer, migration
// v2_7_6).
func initAIExecutions(db *sqlx.DB, i18n *i18n.I18n) *aiexecutions.Manager {
	var lo = initLogger("ai-executions")
	m, err := aiexecutions.New(aiexecutions.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing ai executions manager: %v", err)
	}
	return m
}

// initAIOperations wires the unified per-logical-AI-operation audit manager.
func initAIOperations(db *sqlx.DB) *aioperations.Manager {
	m, err := aioperations.New(aioperations.Opts{
		DB: db,
		Lo: initLogger("ai-operations"),
	})
	if err != nil {
		log.Fatalf("error initializing ai operations manager: %v", err)
	}
	return m
}

// initAIUsage wires the ai_usage Manager — one row per LLM provider
// call. Depends on the model catalog (cost calculation reads
// per-million-token rates from there); fails loudly if the catalog
// hasn't been loaded yet.
func initAIUsage(db *sqlx.DB, cat *aicatalog.Catalog) *aiusage.Manager {
	var lo = initLogger("ai-usage")
	m, err := aiusage.New(aiusage.Opts{
		DB:      db,
		Catalog: cat,
		Lo:      lo,
	})
	if err != nil {
		log.Fatalf("error initializing ai usage manager: %v", err)
	}
	return m
}

// initSystemInboxID resolves the seeded "System (Internal)" inbox row
// by its stable UUID (see migrations.SystemInboxUUID). Every ai_usage
// row that has no real conversation context FKs to this id. Fails
// loudly when the seed row is missing — that indicates the v2.7.7
// migration didn't run, and a missing FK target means every system-
// scoped LLM call would error at insert time.
func initSystemInboxID(db *sqlx.DB) int {
	var id int
	err := db.QueryRow(
		`SELECT id FROM inboxes WHERE uuid = $1`,
		migrations.SystemInboxUUID,
	).Scan(&id)
	if err != nil {
		log.Fatalf("error resolving system inbox by uuid %s: %v (run --upgrade?)",
			migrations.SystemInboxUUID, err)
	}
	if id <= 0 {
		log.Fatalf("system inbox uuid %s resolved to zero id", migrations.SystemInboxUUID)
	}
	return id
}

// initAIModelCatalog loads config/anthropic_models.json into an in-memory
// catalog used to validate preset model references at startup. Path is
// configurable via `ai.models_catalog_path` (defaults to
// config/anthropic_models.json relative to the working directory).
//
// Missing or malformed catalog is fatal: a typo in a model id should
// crash startup, not silently fall back to a default.
func initAIModelCatalog() *aicatalog.Catalog {
	path := ko.String("ai.models_catalog_path")
	if path == "" {
		path = "config/anthropic_models.json"
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	cat, err := aicatalog.Load(path)
	if err != nil {
		log.Fatalf("error loading AI model catalog: %v", err)
	}
	colorlog.Green("AI model catalog: %d models loaded from %s", len(cat.IDs()), path)
	return cat
}

// initAIPresetRegistry loads config/ai_presets.json into the in-memory
// preset registry, validating every preset's model against the catalog
// and every purpose mapping against llm.KnownPurposes. Path is
// configurable via `ai.presets_path`. Validation failure is fatal — the
// alternative (silently starting with broken config) is worse than a
// crash on launch.
func initAIPresetRegistry(cat *aicatalog.Catalog) *aipreset.Registry {
	path := ko.String("ai.presets_path")
	if path == "" {
		path = "config/ai_presets.json"
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	reg, err := aipreset.Load(path, cat)
	if err != nil {
		log.Fatalf("error loading AI preset registry: %v", err)
	}
	colorlog.Green("AI preset registry: %d presets loaded from %s", len(reg.Slugs()), path)
	return reg
}

// initAnthropicProvider builds the production llm.LLMProvider against
// Anthropic. Resolution order for the API key:
//
//  1. config/ai_presets.json → provider_keys.anthropic (platform key,
//     set by engineering at deploy time)
//  2. ai_providers row WHERE provider='anthropic' (encrypted, fallback
//     path for when a per-tenant secrets UI lands)
//
// Returns nil + warns when neither is set. The agent runner is nil in
// that case; the run endpoint returns 503 at run time.
func initAnthropicProvider(db *sqlx.DB, registry *aipreset.Registry, cat *aicatalog.Catalog) llm.LLMProvider {
	apiKey := registry.ProviderKey("anthropic")
	source := "config/ai_presets.json"
	if apiKey == "" {
		// Fall back to the DB-stored key.
		encKey := ko.MustString("app.encryption_key")
		k, err := anthropic.LookupAPIKey(db, encKey)
		if err != nil {
			log.Fatalf("error reading Anthropic api key from ai_providers: %v", err)
		}
		if k != "" {
			apiKey = k
			source = "ai_providers row"
		}
	}
	if apiKey == "" {
		colorlog.Red("Anthropic API key not set — set provider_keys.anthropic in config/ai_presets.json (or the ai_providers row) to enable the agent runner")
		return nil
	}
	lo := initLogger("anthropic")
	p, err := anthropic.New(anthropic.Opts{
		APIKey:   apiKey,
		Catalog:  cat,
		Registry: registry,
		Lo:       lo,
	})
	if err != nil {
		log.Fatalf("error initializing Anthropic provider: %v", err)
	}
	colorlog.Green("Anthropic provider: ready (key from %s)", source)
	return p
}

// initAgentRunner wires up the agent runner. Returns nil when the
// Anthropic provider isn't configured (agent runs error at run time
// rather than at boot — keeps the rest of the app available).
func initAgentRunner(
	provider llm.LLMProvider,
	agents *aiagents.Manager,
	tools *aitools.Manager,
	dispatcher *aitools.Dispatcher,
	presetReg *aipreset.Registry,
	knowledgeMgr *knowledge.Manager,
	conversationMgr *conversation.Manager,
	userMgr *user.Manager,
	customerMgr *customer.Manager,
	contractMgr *contract.Manager,
	usageMgr *aiusage.Manager,
	eventPub runner.EventPublisher,
) *runner.Runner {
	if provider == nil {
		return nil
	}
	lo := initLogger("agent-runner")
	r, err := runner.New(runner.Opts{
		Provider:     provider,
		Agents:       agents,
		Resolver:     resolver.New(tools, nil),
		Dispatcher:   dispatcher,
		PresetReg:    presetReg,
		Lo:           lo,
		Events:       eventPub,
		Knowledge:    knowledgeMgr,
		Conversation: conversationMgr,
		User:         userMgr,
		Customer:     customerMgr,
		Contract:     contractMgr,
		Usage:        &runnerUsageAdapter{m: usageMgr},
	})
	if err != nil {
		log.Fatalf("error initializing agent runner: %v", err)
	}
	return r
}

func initAgentRunEventPublisher(rdb *redis.Client) runner.EventPublisher {
	if rdb == nil {
		return nil
	}
	pub, err := runnerevents.NewRedisStreamPublisher(rdb, initLogger("ai-run-events"), ko.Duration("agent_run.event_ttl"))
	if err != nil {
		log.Fatalf("error initializing ai run event publisher: %v", err)
	}
	return pub
}

// runnerUsageAdapter bridges runner.UsageRecorder (which uses
// runner.UsageRow to avoid importing internal/ai/usage from inside the
// runner package) and the real usage.Manager.Record. The adapter
// copies fields 1:1; CreatedAt is stamped here so the
// (trace_id, call_sequence, created_at) idempotency invariant holds
// even on retry of the parent HTTP request.
type runnerUsageAdapter struct {
	m *aiusage.Manager
}

func (a *runnerUsageAdapter) Record(ctx context.Context, in runner.UsageRow) error {
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

func initDetectionManager(provider llm.LLMProvider, usageMgr *aiusage.Manager, systemInboxID int) *detection.Manager {
	lo := initLogger("detection")
	return detection.New(detection.Opts{
		Lo:            lo,
		LLMProvider:   provider,
		UsageMgr:      usageMgr,
		SystemInboxID: systemInboxID,
	})
}

// initAutomationEngine initializes the automation engine.
func initAutomationEngine(db *sqlx.DB, i18n *i18n.I18n, detectionMgr *detection.Manager, customerMgr *customer.Manager, contractMgr *contract.Manager) *automation.Engine {
	var lo = initLogger("automation_engine")
	engine, err := automation.New(automation.Opts{
		DB:           db,
		Lo:           lo,
		I18n:         i18n,
		DetectionMgr: detectionMgr,
		Customer:     customerMgr,
		Contract:     contractMgr,
	})
	if err != nil {
		log.Fatalf("error initializing automation engine: %v", err)
	}
	return engine
}

// initAutoAssigner initializes the auto assigner.
func initAutoAssigner(teamManager *team.Manager, userManager *user.Manager, conversationManager *conversation.Manager) *autoassigner.Engine {
	systemUser, err := userManager.GetSystemUser()
	if err != nil {
		log.Fatalf("error fetching system user: %v", err)
	}
	e, err := autoassigner.New(teamManager, conversationManager, systemUser, initLogger("autoassigner"))
	if err != nil {
		log.Fatalf("error initializing auto assigner: %v", err)
	}
	return e
}

// initNotifier initializes the notifier service with available providers.
func initNotifier() *notifier.Service {
	smtpCfg := imodels.SMTPConfig{}
	if err := ko.UnmarshalWithConf("notification.email", &smtpCfg, koanf.UnmarshalConf{Tag: "json"}); err != nil {
		log.Fatalf("error unmarshalling email notification provider config: %v", err)
	}

	emailNotifier, err := emailnotifier.New([]imodels.SMTPConfig{smtpCfg}, emailnotifier.Opts{
		Lo:        initLogger("email-notifier"),
		FromEmail: ko.String("notification.email.email_address"),
	})
	if err != nil {
		log.Fatalf("error initializing email notifier: %v", err)
	}

	notifierProviders := map[string]notifier.Notifier{
		emailNotifier.Name(): emailNotifier,
	}

	return notifier.NewService(notifierProviders, ko.MustInt("notification.concurrency"), ko.MustInt("notification.queue_size"), initLogger("notifier"))
}

// initEmailInbox loads inbox config from DB and initializes the email inbox.
func initEmailInbox(inboxRecord imodels.Inbox, msgStore inbox.MessageStore, usrStore inbox.UserStore, mgr *inbox.Manager) (inbox.Inbox, error) {
	var config imodels.Config

	// Load JSON data into Koanf.
	if err := ko.Load(rawbytes.Provider([]byte(inboxRecord.Config)), kjson.Parser()); err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	if err := ko.UnmarshalWithConf("", &config, koanf.UnmarshalConf{Tag: "json"}); err != nil {
		return nil, fmt.Errorf("unmarshalling `%s` %s config: %w", inboxRecord.Channel, inboxRecord.Name, err)
	}

	if len(config.SMTP) == 0 {
		log.Printf("WARNING: Zero SMTP servers configured for `%s` inbox: Name: `%s`", inboxRecord.Channel, inboxRecord.Name)
	}

	if len(config.IMAP) == 0 {
		log.Printf("WARNING: Zero IMAP clients configured for `%s` inbox: Name: `%s`", inboxRecord.Channel, inboxRecord.Name)
	}

	config.From = inboxRecord.From

	if len(config.From) == 0 {
		log.Printf("WARNING: No `from` email address set for `%s` inbox: Name: `%s`", inboxRecord.Channel, inboxRecord.Name)
	}

	// Callback to persist refreshed tokens in DB.
	tokenRefreshCallback := func(inboxID int, updatedConfig imodels.Config) error {
		// Marshal updated config to JSON
		updatedConfigJSON, err := json.Marshal(updatedConfig)
		if err != nil {
			log.Printf("ERROR: Failed to marshal updated config during token refresh for inbox %d: %v", inboxID, err)
			return err
		}

		// Persist updated config to DB
		if err := mgr.UpdateConfig(inboxID, updatedConfigJSON); err != nil {
			log.Printf("ERROR: Failed to persist refreshed tokens during operation for inbox %d: %v", inboxID, err)
			return err
		}

		log.Printf("INFO: Successfully persisted refreshed tokens during operation for inbox: %d", inboxID)
		return nil
	}

	inbox, err := email.New(msgStore, usrStore, email.Opts{
		ID:                   inboxRecord.ID,
		Config:               config,
		Lo:                   initLogger("email_inbox"),
		TokenRefreshCallback: tokenRefreshCallback,
	})

	if err != nil {
		return nil, fmt.Errorf("initializing `%s` inbox: `%s` error : %w", inboxRecord.Channel, inboxRecord.Name, err)
	}

	log.Printf("`%s` inbox successfully initialized", inboxRecord.Name)

	return inbox, nil
}

// initLiveChatInbox initializes the live chat inbox.
func initLiveChatInbox(inboxRecord imodels.Inbox, msgStore inbox.MessageStore, usrStore inbox.UserStore, signAvatarURL func(*null.String)) (inbox.Inbox, error) {
	var config livechat.Config

	// Load JSON data into Koanf.
	if err := ko.Load(rawbytes.Provider([]byte(inboxRecord.Config)), kjson.Parser()); err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	if err := ko.UnmarshalWithConf("", &config, koanf.UnmarshalConf{Tag: "json"}); err != nil {
		return nil, fmt.Errorf("unmarshalling `%s` %s config: %w", inboxRecord.Channel, inboxRecord.Name, err)
	}

	inbox, err := livechat.New(msgStore, usrStore, livechat.Opts{
		ID:            inboxRecord.ID,
		Config:        config,
		Lo:            initLogger("livechat_inbox"),
		SignAvatarURL: signAvatarURL,
	})

	if err != nil {
		return nil, fmt.Errorf("initializing `%s` inbox: `%s` error : %w", inboxRecord.Channel, inboxRecord.Name, err)
	}

	log.Printf("`%s` inbox successfully initialized", inboxRecord.Name)

	return inbox, nil
}

// makeInboxInitializer creates an inbox initializer function.
func makeInboxInitializer(mgr *inbox.Manager, signAvatarURL func(*null.String)) func(imodels.Inbox, inbox.MessageStore, inbox.UserStore) (inbox.Inbox, error) {
	return func(inboxR imodels.Inbox, msgStore inbox.MessageStore, usrStore inbox.UserStore) (inbox.Inbox, error) {
		switch inboxR.Channel {
		case inbox.ChannelEmail:
			return initEmailInbox(inboxR, msgStore, usrStore, mgr)
		case inbox.ChannelLiveChat:
			return initLiveChatInbox(inboxR, msgStore, usrStore, signAvatarURL)
		default:
			return nil, fmt.Errorf("unknown inbox channel: %s", inboxR.Channel)
		}
	}
}

// reloadInbox reloads a single inbox by ID using the signal-aware context.
func reloadInbox(app *App, id int) error {
	app.lo.Info("reloading inbox", "id", id)
	return app.inbox.ReloadInbox(app.ctx, id, makeInboxInitializer(app.inbox, app.conversation.SignAvatarURL))
}

// startInboxes registers the active inboxes and starts receiver for each.
func startInboxes(ctx context.Context, mgr *inbox.Manager, msgStore inbox.MessageStore, usrStore inbox.UserStore, signAvatarURL func(*null.String)) {
	mgr.SetMessageStore(msgStore)
	mgr.SetUserStore(usrStore)

	if err := mgr.InitInboxes(makeInboxInitializer(mgr, signAvatarURL)); err != nil {
		log.Fatalf("error initializing inboxes: %v", err)
	}

	if err := mgr.Start(ctx); err != nil {
		log.Fatalf("error starting inboxes: %v", err)
	}
}

// initAuthz initializes authorization enforcer.
func initAuthz(i18n *i18n.I18n) *authz.Enforcer {
	enforcer, err := authz.NewEnforcer(initLogger("authz"), i18n)
	if err != nil {
		log.Fatalf("error initializing authz: %v", err)
	}
	return enforcer
}

// initAuth initializes the authentication manager.
func initAuth(o *oidc.Manager, rd *redis.Client, i18n *i18n.I18n) *auth_.Auth {
	lo := initLogger("auth")

	providers, err := buildProviders(o)
	if err != nil {
		log.Fatalf("error initializing auth: %v", err)
	}

	secure := !ko.Bool("app.server.disable_secure_cookies")
	sessionLifetime := ko.Duration("app.server.session_lifetime")
	auth, err := auth_.New(auth_.Config{Providers: providers, SecureCookies: secure, SessionLifetime: sessionLifetime}, i18n, rd, lo)
	if err != nil {
		log.Fatalf("error initializing auth: %v", err)
	}

	return auth
}

// reloadAuth reloads the auth providers.
func reloadAuth(app *App) error {
	app.lo.Info("reloading auth manager")
	providers, err := buildProviders(app.oidc)
	if err != nil {
		log.Fatalf("error reloading auth: %v", err)
	}
	if err := app.auth.Reload(auth_.Config{Providers: providers}); err != nil {
		app.lo.Error("error reloading auth", "error", err)
		return err
	}
	return nil
}

// buildProviders creates a list of auth providers from the OIDC manager.
func buildProviders(o *oidc.Manager) ([]auth_.Provider, error) {
	oidcConfigs, err := o.GetAll()
	if err != nil {
		return nil, err
	}

	providers := make([]auth_.Provider, 0, len(oidcConfigs))
	for _, config := range oidcConfigs {
		if !config.Enabled {
			continue
		}
		providers = append(providers, auth_.Provider{
			ID:           config.ID,
			Provider:     config.Provider,
			ProviderURL:  config.ProviderURL,
			RedirectURL:  config.RedirectURI,
			ClientID:     config.ClientID,
			ClientSecret: config.ClientSecret,
		})
	}
	return providers, nil
}

// initOIDC initializes open id connect config manager.
func initOIDC(db *sqlx.DB, settings *setting.Manager, i18n *i18n.I18n) *oidc.Manager {
	lo := initLogger("oidc")
	o, err := oidc.New(oidc.Opts{
		DB:            db,
		Lo:            lo,
		I18n:          i18n,
		EncryptionKey: ko.MustString("app.encryption_key"),
	}, settings)
	if err != nil {
		log.Fatalf("error initializing oidc: %v", err)
	}
	return o
}

// initI18n inits i18n.
func initI18n(fs stuffbin.FileSystem) *i18n.I18n {
	fileName := cmp.Or(ko.String("app.lang"), defLang)
	log.Printf("loading i18n language file: %s", fileName)
	file, err := fs.Get("i18n/" + fileName + ".json")
	if err != nil {
		log.Fatalf("error reading i18n language file `%s` : %v", fileName, err)
	}
	i18n, err := i18n.New(file.ReadBytes())
	if err != nil {
		log.Fatalf("error initializing i18n: %v", err)
	}
	return i18n
}

// initRedis inits redis DB.
func initRedis() *redis.Client {
	// Load options from redis URL if set.
	redisURL := ko.String("redis.url")
	if redisURL != "" {
		options, err := redis.ParseURL(redisURL)
		if err != nil {
			log.Fatalf("error parsing redis url: %v", err)
		}
		return redis.NewClient(options)
	}
	// Load from individual config options.
	return redis.NewClient(&redis.Options{
		Addr:     ko.MustString("redis.address"),
		Username: ko.String("redis.user"),
		Password: ko.String("redis.password"),
		DB:       ko.Int("redis.db"),
	})
}

// initRedis inits postgres DB.
func initDB() *sqlx.DB {
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
		log.Fatalf("error connecting to DB: %v", err)
	}

	db.SetMaxOpenConns(ko.MustInt("db.max_open"))
	db.SetMaxIdleConns(ko.MustInt("db.max_idle"))
	db.SetConnMaxLifetime(ko.MustDuration("db.max_lifetime"))

	return db
}

// initRedis inits role manager.
func initRole(db *sqlx.DB, i18n *i18n.I18n) *role.Manager {
	var lo = initLogger("role_manager")
	r, err := role.New(role.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing role manager: %v", err)
	}
	return r
}

// initStatus inits conversation status manager.
func initStatus(db *sqlx.DB, i18n *i18n.I18n) *status.Manager {
	manager, err := status.New(status.Opts{
		DB:   db,
		Lo:   initLogger("status-manager"),
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing status manager: %v", err)
	}
	return manager
}

// initPriority inits conversation priority manager.
func initPriority(db *sqlx.DB, i18n *i18n.I18n) *priority.Manager {
	manager, err := priority.New(priority.Opts{
		DB:   db,
		Lo:   initLogger("priority-manager"),
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing priority manager: %v", err)
	}
	return manager
}

// initAI inits AI manager.
func initAI(db *sqlx.DB, i18n *i18n.I18n) *aiLegacy.Manager {
	lo := initLogger("ai")
	m, err := aiLegacy.New(aiLegacy.Opts{
		DB:            db,
		Lo:            lo,
		I18n:          i18n,
		EncryptionKey: ko.MustString("app.encryption_key"),
	})
	if err != nil {
		log.Fatalf("error initializing AI manager: %v", err)
	}
	return m
}

// initSearch inits search manager.
func initSearch(db *sqlx.DB, i18n *i18n.I18n) *search.Manager {
	lo := initLogger("search")
	m, err := search.New(search.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing search manager: %v", err)
	}
	return m
}

// initCustomAttribute inits custom attribute manager.
func initCustomAttribute(db *sqlx.DB, i18n *i18n.I18n) *customAttribute.Manager {
	lo := initLogger("custom-attribute")
	m, err := customAttribute.New(customAttribute.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing custom attribute manager: %v", err)
	}
	return m
}

// initActivityLog inits activity log manager.
func initActivityLog(db *sqlx.DB, i18n *i18n.I18n) *activitylog.Manager {
	lo := initLogger("activity-log")
	m, err := activitylog.New(activitylog.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing activity log manager: %v", err)
	}
	return m
}

// initReport inits report manager.
func initReport(db *sqlx.DB, i18n *i18n.I18n) *report.Manager {
	lo := initLogger("report")
	m, err := report.New(report.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing report manager: %v", err)
	}
	return m
}

// initWebhook inits webhook manager.
func initWebhook(db *sqlx.DB, i18n *i18n.I18n) *webhook.Manager {
	var lo = initLogger("webhook")
	m, err := webhook.New(webhook.Opts{
		DB:            db,
		Lo:            lo,
		I18n:          i18n,
		Workers:       ko.MustInt("webhook.workers"),
		QueueSize:     ko.MustInt("webhook.queue_size"),
		Timeout:       ko.MustDuration("webhook.timeout"),
		EncryptionKey: ko.MustString("app.encryption_key"),
		AllowedHosts:  ko.Strings("webhook.allowed_hosts"),
	})
	if err != nil {
		log.Fatalf("error initializing webhook manager: %v", err)
	}
	return m
}

// initUserNotification inits user notification manager.
func initUserNotification(db *sqlx.DB, i18n *i18n.I18n) *notifier.UserNotificationManager {
	var lo = initLogger("user-notification")
	m, err := notifier.NewUserNotificationManager(notifier.UserNotificationOpts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing user notification manager: %v", err)
	}
	return m
}

// initImporter inits the importer manager.
func initImporter(i18n *i18n.I18n) *importer.Importer {
	return importer.New(importer.Opts{
		Lo:   initLogger("importer"),
		I18n: i18n,
	})
}

// initNotifDispatcher initializes the notification dispatcher.
func initNotifDispatcher(userNotification *notifier.UserNotificationManager, outbound *notifier.Service, wsHub *ws.Hub, emailEnabled bool) *notifier.Dispatcher {
	return notifier.NewDispatcher(notifier.DispatcherOpts{
		InApp:        userNotification,
		Outbound:     outbound,
		WSHub:        wsHub,
		EmailEnabled: emailEnabled,
		Lo:           initLogger("notification-dispatcher"),
	})
}

// initLogger initializes a logf logger.
func initLogger(src string) *logf.Logger {
	lvl, env := ko.MustString("app.log_level"), ko.MustString("app.env")
	lo := logf.New(logf.Opts{
		Level:                getLogLevel(lvl),
		EnableColor:          getColor(env),
		EnableCaller:         true,
		CallerSkipFrameCount: 3,
		DefaultFields:        []any{"sc", src},
	})
	return &lo
}

func getColor(env string) bool {
	color := false
	if env == "dev" {
		color = true
	}
	return color
}

func getLogLevel(lvl string) logf.Level {
	switch lvl {
	case "info":
		return logf.InfoLevel
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

// initEmbeddingProvider builds the Voyage embeddings.Provider. Returns
// nil when no Voyage API key is configured — the main app stays up,
// but kb_search returns "RAG subsystem not initialized" until the key
// is set. Wire the key in config/ai_presets.json provider_keys.voyage.
func initEmbeddingProvider(reg *aipreset.Registry) embeddings.Provider {
	apiKey := reg.ProviderKey("voyage")
	if apiKey == "" {
		colorlog.Red("Voyage API key not set (provider_keys.voyage in config/ai_presets.json) — kb_search hybrid search disabled")
		return nil
	}
	lo := initLogger("voyage-embeddings")
	p, err := voyageembed.New(voyageembed.Opts{
		APIKey: apiKey,
		Lo:     lo,
	})
	if err != nil {
		log.Fatalf("error initializing Voyage embeddings provider: %v", err)
	}
	colorlog.Green("Voyage embeddings provider: ready (model=%s)", p.Model())
	return p
}

// initRerankProvider builds the Voyage reranker.Provider. Shares the
// same API key as the embedding provider. Returns nil when the key is
// absent (kb_search degrades to RRF-order fallback).
func initRerankProvider(reg *aipreset.Registry) reranker.Provider {
	apiKey := reg.ProviderKey("voyage")
	if apiKey == "" {
		return nil
	}
	lo := initLogger("voyage-reranker")
	p, err := voyagerank.New(voyagerank.Opts{
		APIKey: apiKey,
		Lo:     lo,
	})
	if err != nil {
		log.Fatalf("error initializing Voyage reranker: %v", err)
	}
	colorlog.Green("Voyage reranker: ready (model=%s)", p.Model())
	return p
}

// initEmbeddingsStore prepares the embedded SQL statements for the
// hybrid-search + indexing storage layer.
func initEmbeddingsStore(db *sqlx.DB) *embeddings.Store {
	lo := initLogger("embeddings-store")
	s, err := embeddings.NewStore(embeddings.StoreOpts{DB: db, Lo: lo})
	if err != nil {
		log.Fatalf("error initializing embeddings store: %v", err)
	}
	return s
}

// initQueryEmbeddingCache wires the Redis-backed query embedding cache.
// TTL is configurable via [embeddings.cache].ttl_seconds (default 1h).
// Setting [embeddings.cache].enabled = false returns nil — Searcher
// falls through to always-embed.
func initQueryEmbeddingCache(rdb *redis.Client) *embeddings.QueryEmbeddingCache {
	if !ko.Bool("embeddings.cache.enabled") {
		return nil
	}
	ttl := ko.Duration("embeddings.cache.ttl")
	if ttl == 0 {
		ttl = time.Hour
	}
	lo := initLogger("embeddings-cache")
	c, err := embeddings.NewQueryEmbeddingCache(rdb, ttl, lo)
	if err != nil {
		log.Fatalf("error initializing query embedding cache: %v", err)
	}
	return c
}

// initSearcher composes the hybrid search engine. Returns nil if the
// embedding provider isn't configured — kb_search will report the
// disabled state via SetKBSearcher receiving nil.
func initSearcher(store *embeddings.Store, embed embeddings.Provider, rank reranker.Provider, cache *embeddings.QueryEmbeddingCache) *embeddings.Searcher {
	s, err := embeddings.NewConfiguredSearcher(ko, embeddings.SearcherOpts{
		Store:    store,
		Embed:    embed,
		Reranker: rank,
		Cache:    cache,
		Lo:       initLogger("embeddings-searcher"),
	})
	if err != nil {
		log.Fatalf("error initializing embeddings searcher: %v", err)
	}
	return s
}

// initTemporal builds the Temporal client used by handlers and
// managers to start workflows. Returns nil when [temporal].enabled
// is false (graceful-degradation per CONVENTIONS.md) — callers
// branch on app.temporal == nil before starting a workflow.
//
// Dial failures are NOT fatal: Temporal outages shouldn't take down
// the inbox. The error is logged and the client stays nil so
// workflow-starting paths fall back to their documented behaviour
// (sync, or refuse-and-log).
func initTemporal() *temporal.Client {
	cfg := temporal.Config{
		Enabled:       ko.Bool("temporal.enabled"),
		Address:       ko.String("temporal.address"),
		Namespace:     ko.String("temporal.namespace"),
		DialTimeout:   ko.Duration("temporal.dial_timeout"),
		ShutdownGrace: ko.Duration("temporal.shutdown_grace"),
	}
	if !cfg.Enabled {
		colorlog.Red("Temporal disabled (temporal.enabled=false); workflow-starting paths will fall back to sync or refuse-and-log")
		return nil
	}
	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:7233"
	}
	if cfg.Namespace == "" {
		cfg.Namespace = "merio"
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 10 * time.Second
	}
	if cfg.ShutdownGrace == 0 {
		cfg.ShutdownGrace = 30 * time.Second
	}

	lo := initLogger("temporal")
	c, err := temporal.New(temporal.Opts{Config: cfg, Lo: lo})
	if err != nil {
		// Dial failures are warn-and-continue: the main API binary
		// stays available, the worker binary fails fast separately.
		colorlog.Red("Temporal dial failed (%v) — workflow-starting paths will fall back; check the temporal service", err)
		return nil
	}
	return c
}

// initRateLimit initializes the rate limiter with default rules.
// Defaults are used unless overridden in config.toml under [rate_limit.<name>].
func initRateLimit(redisClient *redis.Client) *ratelimit.Limiter {
	limiter := ratelimit.New(redisClient)

	defaults := []struct {
		Name string
		RPM  int
	}{
		{"widget", 100},
		{"auth", 30},
		{"public", 100},
	}

	for _, d := range defaults {
		enabled := true
		rpm := d.RPM

		cfgKey := "rate_limit." + d.Name
		if ko.Exists(cfgKey) {
			var cfg struct {
				Enabled           bool `toml:"enabled"`
				RequestsPerMinute int  `toml:"requests_per_minute"`
			}
			ko.UnmarshalWithConf(cfgKey, &cfg, koanf.UnmarshalConf{Tag: "toml"})
			enabled = cfg.Enabled
			if cfg.RequestsPerMinute > 0 {
				rpm = cfg.RequestsPerMinute
			}
		}

		if !enabled {
			log.Printf("WARNING: rate limit rule '%s' is disabled", d.Name)
		}

		limiter.AddRule(ratelimit.Rule{
			Name:              d.Name,
			Enabled:           enabled,
			RequestsPerMinute: rpm,
		})
	}

	return limiter
}
