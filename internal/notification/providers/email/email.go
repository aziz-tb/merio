// Package email is the SMTP-backed notifier.Notifier registered under notifier.ProviderEmail;
// it sends through one of the configured SMTP pools, chosen at random when several exist.
package email

import (
	"math/rand"
	"net/textproto"

	"github.com/aziz-tb/merio/internal/attachment"
	"github.com/aziz-tb/merio/internal/inbox/channel/email"
	"github.com/aziz-tb/merio/internal/inbox/models"
	notifier "github.com/aziz-tb/merio/internal/notification"
	"github.com/knadh/smtppool"
	"github.com/zerodha/logf"
)

// Email is the SMTP-backed notifier.Notifier; it holds no per-send state, so a single instance
// is safe for concurrent use.
type Email struct {
	lo        *logf.Logger
	from      string
	smtpPools []*smtppool.Pool
}

// Opts contains options for creating a new Email sender.
type Opts struct {
	Lo        *logf.Logger
	FromEmail string
}

// New builds an Email sender with one SMTP pool per config entry; it errors if any pool fails
// to initialize.
func New(smtpConfig []models.SMTPConfig, opts Opts) (*Email, error) {
	pools, err := email.NewSmtpPool(smtpConfig, nil)
	if err != nil {
		return nil, err
	}
	return &Email{
		lo:        opts.Lo,
		smtpPools: pools,
		from:      opts.FromEmail,
	}, nil
}

// Send renders msg and delivers it over one randomly chosen SMTP pool. msg.ContentType "plain"
// sets the text body; anything else is HTML (with msg.AltContent as the text fallback).
func (e *Email) Send(msg notifier.Message) error {
	emailMessage := e.prepareEmail(msg.Subject, msg.Content, msg.RecipientEmails, msg)
	return e.send(emailMessage)
}

// Name returns notifier.ProviderEmail, the key this provider is registered under.
func (e *Email) Name() string {
	return notifier.ProviderEmail
}

// send sends an email message.
func (e *Email) send(em smtppool.Email) error {
	srv := e.selectSmtpPool()
	return srv.Send(em)
}

// selectSmtpPool selects a random SMTP pool if multiple are available.
func (e *Email) selectSmtpPool() *smtppool.Pool {
	if len(e.smtpPools) > 1 {
		return e.smtpPools[rand.Intn(len(e.smtpPools))]
	}
	return e.smtpPools[0]
}

// prepareEmail prepares the email message with attachments and headers.
func (e *Email) prepareEmail(subject, content string, recipients []string, msg notifier.Message) smtppool.Email {
	var files []smtppool.Attachment
	if len(msg.Attachments) > 0 {
		files = e.prepareAttachments(msg.Attachments)
	}

	em := smtppool.Email{
		From:        e.from,
		To:          recipients,
		Subject:     subject,
		Attachments: files,
		Headers:     textproto.MIMEHeader{},
	}

	// Set content based on provided type
	switch msg.ContentType {
	case "plain":
		em.Text = []byte(content)
	default:
		em.HTML = []byte(content)
		if len(msg.AltContent) > 0 {
			em.Text = []byte(msg.AltContent)
		}
	}

	// Set any additional headers
	for headerKey, headerValue := range msg.Headers {
		em.Headers[headerKey] = headerValue
	}

	return em
}

// prepareAttachments prepares email attachments.
func (e *Email) prepareAttachments(attachments []attachment.Attachment) []smtppool.Attachment {
	files := make([]smtppool.Attachment, len(attachments))
	for i, f := range attachments {
		files[i] = smtppool.Attachment{
			Filename: f.Name,
			Header:   f.Header,
			Content:  append([]byte(nil), f.Content...),
		}
	}
	return files
}
