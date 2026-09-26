// Package mailer sends transactional email. SMTP is used when configured;
// without configuration the LogMailer writes messages to the log, so local
// development never needs a real server (or pair it with Mailpit from
// docker-compose to read mail in a browser UI).
package mailer

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/smtp"
	"strings"
)

// Message is one outgoing email.
type Message struct {
	To      string
	Subject string
	// Body is an HTML fragment; Send wraps it in the base layout.
	Body template.HTML
}

// Sender sends transactional email; implementations must be safe for
// concurrent use.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// Config selects the SMTP transport.
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// Enabled reports whether SMTP is configured; otherwise the caller should
// fall back to LogMailer.
func (c Config) Enabled() bool { return c.Host != "" }

// SMTPSender posts email through net/smtp.
type SMTPSender struct {
	cfg    Config
	logger *slog.Logger
}

// NewSMTP builds the SMTP sender.
func NewSMTP(cfg Config, logger *slog.Logger) *SMTPSender {
	return &SMTPSender{cfg: cfg, logger: logger}
}

// Send delivers the message; TLS is used when the server advertises it.
func (s *SMTPSender) Send(_ context.Context, msg Message) error {
	body, err := render(msg)
	if err != nil {
		return fmt.Errorf("mailer render: %w", err)
	}
	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	header := strings.Join([]string{
		"From: " + s.cfg.From,
		"To: " + msg.To,
		"Subject: " + msg.Subject,
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=\"utf-8\"",
	}, "\r\n")
	return smtp.SendMail(addr, auth, s.cfg.From, []string{msg.To}, []byte(header+"\r\n\r\n"+body))
}

// LogMailer is the zero-infrastructure fallback: it logs the message
// instead of sending it, which keeps dev flows working without SMTP.
type LogMailer struct {
	logger *slog.Logger
}

// NewLogMailer builds the logging sender.
func NewLogMailer(logger *slog.Logger) *LogMailer { return &LogMailer{logger: logger} }

// Send writes the message to the log.
func (l *LogMailer) Send(_ context.Context, msg Message) error {
	l.logger.Info("email (log fallback)", "to", msg.To, "subject", msg.Subject)
	return nil
}

const baseLayout = `<!doctype html>
<html><body style="font-family:system-ui,sans-serif;max-width:32rem;margin:2rem auto;color:#111">
<h2 style="margin-bottom:.25rem">{{.AppName}}</h2>
<div style="font-size:15px;line-height:1.5">{{.Body}}</div>
<p style="margin-top:2rem;font-size:12px;color:#888">This is an automated message from {{.AppName}}.</p>
</body></html>`

func render(msg Message) (string, error) {
	tmpl, err := template.New("base").Parse(baseLayout)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	data := map[string]any{"AppName": "Go Template", "Body": msg.Body}
	if err := tmpl.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}

// Button renders a call-to-action paragraph callers embed into Body.
func Button(url, label string) template.HTML {
	return template.HTML(fmt.Sprintf(
		`<p><a href="%s" style="display:inline-block;padding:.6rem 1rem;background:#111;color:#fff;border-radius:.5rem;text-decoration:none">%s</a></p><p style="font-size:12px;color:#888">Or paste this link: %s</p>`,
		template.HTMLEscapeString(url), template.HTMLEscapeString(label), template.HTMLEscapeString(url),
	))
}
