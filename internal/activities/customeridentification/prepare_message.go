package customeridentification

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	cmodels "github.com/abhinavxd/libredesk/internal/conversation/models"
	"github.com/abhinavxd/libredesk/internal/stringutil"
	cimodels "github.com/abhinavxd/libredesk/internal/workflows/customeridentification"
)

var emailRE = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)

type identificationMessageRow struct {
	ID             int    `db:"id"`
	ConversationID int    `db:"conversation_id"`
	Type           string `db:"type"`
	SenderType     string `db:"sender_type"`
	Private        bool   `db:"private"`
	Content        string `db:"content"`
	TextContent    string `db:"text_content"`
	ContentType    string `db:"content_type"`
	Subject        string `db:"subject"`
	SenderEmail    string `db:"sender_email"`
}

func (a *Activities) PrepareMessageActivity(ctx context.Context, in cimodels.PrepareMessageInput) (cimodels.PrepareMessageOutput, error) {
	var row identificationMessageRow
	if err := a.q.GetMessageForIdentification.GetContext(ctx, &row, in.MessageID, in.ConversationID); err != nil {
		if err == sql.ErrNoRows {
			return cimodels.PrepareMessageOutput{}, fmt.Errorf("message not found for conversation")
		}
		return cimodels.PrepareMessageOutput{}, err
	}
	if row.Type != cmodels.MessageIncoming || row.SenderType != cmodels.SenderTypeContact || row.Private {
		return cimodels.PrepareMessageOutput{}, fmt.Errorf("message is not an inbound public contact message")
	}

	body := strings.TrimSpace(row.TextContent)
	if body == "" {
		body = row.Content
		if row.ContentType == cmodels.ContentTypeHTML {
			body = stringutil.HTML2Text(body)
		}
	}
	body = normalizeWhitespace(body)
	prepared := cmodels.PreparedMessage{
		SenderEmail: strings.ToLower(strings.TrimSpace(row.SenderEmail)),
		Subject:     normalizeWhitespace(row.Subject),
		Body:        body,
		BodyEmails:  extractEmails(body, in.Settings.ExcludedEmailDomains),
	}
	return cimodels.PrepareMessageOutput{PreparedMessage: prepared}, nil
}

func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func extractEmails(body string, excludedDomains []string) []string {
	excluded := make(map[string]bool, len(excludedDomains))
	for _, d := range excludedDomains {
		d = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(d), "@"))
		if d != "" {
			excluded[d] = true
		}
	}
	seen := map[string]bool{}
	out := []string{}
	for _, m := range emailRE.FindAllString(body, -1) {
		email := strings.ToLower(strings.Trim(m, " \t\r\n.,;:()[]<>"))
		parts := strings.Split(email, "@")
		if len(parts) != 2 || excluded[parts[1]] || seen[email] {
			continue
		}
		seen[email] = true
		out = append(out, email)
	}
	return out
}
