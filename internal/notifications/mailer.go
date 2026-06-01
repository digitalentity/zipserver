package notifications

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/smtp"
	"net/url"
	"strings"

	"zipserver/internal/config"
)

var dialAndSend = func(addr string, host string, port int, localName string, encryption string, tlsConfig *tls.Config, auth smtp.Auth, from string, to []string, msg []byte) error {
	var conn net.Conn
	var err error

	if encryption == "ssl" || encryption == "tls" {
		conn, err = tls.Dial("tcp", addr, tlsConfig)
	} else {
		conn, err = net.Dial("tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("failed to dial SMTP server: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	if localName != "" {
		if err := client.Hello(localName); err != nil {
			return fmt.Errorf("failed SMTP Hello: %w", err)
		}
	}

	if encryption == "starttls" {
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("failed STARTTLS handshake: %w", err)
		}
	}

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("failed SMTP authentication: %w", err)
		}
	}

	if err := client.Mail(from); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}
	for _, rec := range to {
		if err := client.Rcpt(rec); err != nil {
			return fmt.Errorf("failed to add recipient %s: %w", rec, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to initiate data command: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("failed to write email body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}

	return client.Quit()
}

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

	fromEmail := from
	if parsed, err := mail.ParseAddress(from); err == nil {
		fromEmail = parsed.Address
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

	// Determine the encryption method
	encryption := strings.ToLower(strings.TrimSpace(m.cfg.SMTP.TLS))
	if encryption == "" {
		// Auto-detect based on port
		if m.cfg.SMTP.Port == 465 {
			encryption = "ssl"
		} else if m.cfg.SMTP.Port == 587 || m.cfg.SMTP.Port == 25 {
			encryption = "starttls"
		} else {
			encryption = "none"
		}
	}

	tlsConfig := &tls.Config{
		ServerName:         m.cfg.SMTP.Host,
		InsecureSkipVerify: m.cfg.SMTP.SkipVerify,
	}

	localName := m.cfg.SMTP.Hello
	if localName == "" {
		if u, err := url.Parse(m.cfg.BaseURL); err == nil && u.Hostname() != "" {
			localName = u.Hostname()
		} else {
			email := fromEmail
			if idx := strings.Index(email, "@"); idx != -1 {
				localName = email[idx+1:]
			}
		}
	}
	if localName == "" {
		localName = "localhost"
	}

	err := dialAndSend(addr, m.cfg.SMTP.Host, m.cfg.SMTP.Port, localName, encryption, tlsConfig, auth, fromEmail, to, []byte(message.String()))
	if err != nil {
		slog.Error("failed to send email", "to", to, "subject", subject, "error", err)
		return err
	}

	slog.Info("email sent successfully", "to", to, "subject", subject)
	return nil
}


