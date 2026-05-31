package notifications

import (
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"

	"zipserver/internal/config"
)

var sendMail = smtp.SendMail

type Mailer struct {
	cfg config.NotificationsConfig
}

func NewMailer(cfg config.NotificationsConfig) *Mailer {
	return &Mailer{cfg: cfg}
}

func (m *Mailer) SendEmail(to []string, subject, body string) error {
	if m.cfg.SMTP.Host == "" {
		slog.Debug("SMTP host not configured, skipping email")
		return nil
	}

	addr := fmt.Sprintf("%s:%d", m.cfg.SMTP.Host, m.cfg.SMTP.Port)
	var auth smtp.Auth
	if m.cfg.SMTP.Username != "" {
		auth = smtp.PlainAuth("", m.cfg.SMTP.Username, m.cfg.SMTP.Password, m.cfg.SMTP.Host)
	}

	from := m.cfg.SMTP.From
	if from == "" {
		from = m.cfg.SMTP.Username
	}

	header := make(map[string]string)
	header["From"] = from
	header["To"] = strings.Join(to, ", ")
	header["Subject"] = subject
	header["MIME-Version"] = "1.0"
	header["Content-Type"] = `text/html; charset="utf-8"`

	var message strings.Builder
	for k, v := range header {
		message.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	message.WriteString("\r\n")
	message.WriteString(body)

	err := sendMail(addr, auth, from, to, []byte(message.String()))
	if err != nil {
		slog.Error("failed to send email", "to", to, "subject", subject, "error", err)
		return err
	}

	slog.Info("email sent successfully", "to", to, "subject", subject)
	return nil
}
