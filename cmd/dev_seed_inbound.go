package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	conv "github.com/aziz-tb/merio/internal/conversation"
	cmodels "github.com/aziz-tb/merio/internal/conversation/models"
	"github.com/aziz-tb/merio/internal/envelope"
	"github.com/aziz-tb/merio/internal/inbox"
	"github.com/aziz-tb/merio/internal/stringutil"
	umodels "github.com/aziz-tb/merio/internal/user/models"
	"github.com/google/uuid"
	"github.com/volatiletech/null/v9"
)

func runDevSeedInboundCLI(ctx context.Context, args []string) {
	_ = ctx
	if err := devSeedInboundCmd(args); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

// devSeedInboundCmd creates a synthetic inbound email conversation for testing.
// It is intentionally gated behind MERIO_DEV=1 and bypasses IMAP while
// still using the production CreateConversation SQL path.
func devSeedInboundCmd(args []string) error {
	fs := flag.NewFlagSet("merio dev-seed-inbound", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), `Usage:
  MERIO_DEV=1 merio dev-seed-inbound \
    --from "lukas.weber@example.com" \
    --inbox-id 1 \
    --subject "Frage zu meinem Vertrag C-880011" \
    --body "Hallo, ich habe eine Frage zu C-880011 unter Kastanienallee 47, 10435 Berlin. Lukas Weber"

Options:
`)
		fs.PrintDefaults()
	}
	configPath := fs.String("config", "config.toml", "path to config file")
	from := fs.String("from", "", "sender email address")
	inboxID := fs.Int("inbox-id", 0, "email inbox id")
	subject := fs.String("subject", "", "email subject")
	body := fs.String("body", "", "email body")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if os.Getenv("MERIO_DEV") != "1" {
		return errors.New("dev-seed-inbound requires MERIO_DEV=1 to be set")
	}

	*from = strings.ToLower(strings.TrimSpace(*from))
	*subject = strings.TrimSpace(*subject)
	*body = strings.TrimSpace(*body)
	if *from == "" || *subject == "" || *body == "" || *inboxID <= 0 {
		fs.Usage()
		return errors.New("--from, --subject, --body, and --inbox-id are required")
	}
	if !stringutil.ValidEmail(*from) {
		return fmt.Errorf("--from must be a valid email address: %s", *from)
	}

	app, err := bootstrapDevSeedInboundApp(*configPath)
	if err != nil {
		return err
	}
	if err := requireEmailInbox(app.db, *inboxID); err != nil {
		return err
	}

	contact, created, err := findOrCreateDevSeedContact(app, *from)
	if err != nil {
		return err
	}
	if created {
		fmt.Printf("Created contact: id=%d email=%s linked_customer_id=%s\n", contact.ID, *from, printableNullString(contact.LinkedCustomerID))
	} else {
		fmt.Printf("Found existing contact: id=%d email=%s linked_customer_id=%s\n", contact.ID, *from, printableNullString(contact.LinkedCustomerID))
	}

	now := time.Now()
	convID, convUUID, err := app.conversation.CreateConversation(
		contact.ID,
		*inboxID,
		*body,
		now,
		*subject,
		false,
		nil,
		nil,
		0,
		0,
	)
	if err != nil {
		return fmt.Errorf("create conversation: %w", err)
	}
	fmt.Printf("Created conversation: id=%d uuid=%s\n", convID, convUUID)

	sourceID := fmt.Sprintf("dev-seed-%s@merio.local", uuid.NewString())
	meta, _ := json.Marshal(map[string]any{
		"dev_seed": true,
		"from":     *from,
	})
	msg := cmodels.Message{
		ConversationID:   convID,
		ConversationUUID: convUUID,
		Type:             cmodels.MessageIncoming,
		Status:           cmodels.MessageStatusReceived,
		SenderID:         contact.ID,
		SenderType:       cmodels.SenderTypeContact,
		Content:          *body,
		TextContent:      *body,
		ContentType:      cmodels.ContentTypeText,
		Private:          false,
		SourceID:         null.StringFrom(sourceID),
		Meta:             meta,
	}
	if err := app.conversation.InsertMessage(&msg); err != nil {
		return fmt.Errorf("insert inbound message: %w", err)
	}
	fmt.Printf("Inserted incoming message: id=%d uuid=%s source_id=%s\n", msg.ID, msg.UUID, sourceID)

	fmt.Printf("\nDone. Conversation URL: /conversations/%s\n", convUUID)
	fmt.Println("Conversation state (verify with SQL):")
	fmt.Printf("  SELECT customer_id, contract_id, verification_level FROM conversations WHERE uuid = '%s';\n", convUUID)
	return nil
}

func bootstrapDevSeedInboundApp(configPath string) (*App, error) {
	ko.Set("config", []string{configPath})
	initConfig(ko)
	validateConfig(ko)

	fs := initFS(getCustomStaticDir())
	db := initDB()
	installed, err := checkSchema(db)
	if err != nil {
		return nil, fmt.Errorf("check schema: %w", err)
	}
	if !installed {
		return nil, errors.New("database tables are missing. Run merio --install first")
	}

	i18n := initI18n(fs)
	userMgr := initUser(i18n, db)
	conversationMgr, err := conv.New(
		nil,
		i18n,
		nil,
		nil,
		nil,
		nil,
		userMgr,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		conv.Opts{
			DB:                       db,
			Lo:                       initLogger("dev_seed_inbound"),
			OutgoingMessageQueueSize: 1,
			IncomingMessageQueueSize: 1,
			SubjectRefFormat:         ko.String("conversation.subject_ref_format"),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("init conversation manager: %w", err)
	}
	return &App{db: db, i18n: i18n, user: userMgr, conversation: conversationMgr}, nil
}

func requireEmailInbox(db interface {
	Get(dest any, query string, args ...any) error
}, inboxID int) error {
	var channel string
	if err := db.Get(&channel, `SELECT channel FROM inboxes WHERE id = $1`, inboxID); err != nil {
		return fmt.Errorf("inbox %d not found: %w", inboxID, err)
	}
	if channel != inbox.ChannelEmail {
		return fmt.Errorf("inbox %d is %q, expected %q", inboxID, channel, inbox.ChannelEmail)
	}
	return nil
}

func findOrCreateDevSeedContact(app *App, email string) (umodels.User, bool, error) {
	existing, err := app.user.GetContactByEmail(email)
	if err == nil {
		contact, err := app.user.GetContactOrVisitor(existing.ID, "")
		return contact, false, err
	}
	var envErr envelope.Error
	if !errors.As(err, &envErr) || envErr.ErrorType != envelope.NotFoundError {
		return umodels.User{}, false, err
	}

	first, last := deriveContactName(email)
	contact := umodels.User{
		Type:      umodels.UserTypeContact,
		Email:     null.StringFrom(email),
		FirstName: first,
		LastName:  last,
	}
	if err := app.user.CreateContact(&contact); err != nil {
		return umodels.User{}, false, fmt.Errorf("create contact: %w", err)
	}
	full, err := app.user.GetContactOrVisitor(contact.ID, "")
	return full, true, err
}

func deriveContactName(email string) (string, string) {
	local := strings.Split(email, "@")[0]
	parts := strings.FieldsFunc(local, func(r rune) bool {
		return r == '.' || r == '_' || r == '-' || r == '+'
	})
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return "Test", "Contact"
	}
	first := titleASCII(parts[0])
	last := ""
	if len(parts) > 1 {
		last = titleASCII(parts[1])
	}
	return first, last
}

func titleASCII(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	runes := []rune(strings.ToLower(s))
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func printableNullString(s null.String) string {
	if !s.Valid || strings.TrimSpace(s.String) == "" {
		return "<null>"
	}
	return s.String
}
