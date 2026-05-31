package notifications

import (
	"net/smtp"
	"strings"
	"testing"

	"zipserver/internal/config"
)

func TestSendEmail(t *testing.T) {
	cfg := config.NotificationsConfig{
		SMTP: config.SMTPConfig{
			Host:     "localhost",
			Port:     25,
			Username: "user",
			Password: "pwd",
			From:     "noreply@example.com",
		},
	}

	var calledAddr string
	var calledFrom string
	var calledTo []string
	var calledMsg []byte

	// Mock sendMail
	sendMail = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		calledAddr = addr
		calledFrom = from
		calledTo = to
		calledMsg = msg
		return nil
	}
	defer func() {
		sendMail = smtp.SendMail
	}()

	mailer := NewMailer(cfg)
	err := mailer.SendEmail([]string{"test@example.com"}, "Test Subject", "Test Body")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if calledAddr != "localhost:25" {
		t.Errorf("expected localhost:25, got %s", calledAddr)
	}
	if calledFrom != "noreply@example.com" {
		t.Errorf("expected noreply@example.com, got %s", calledFrom)
	}
	if len(calledTo) != 1 || calledTo[0] != "test@example.com" {
		t.Errorf("expected [test@example.com], got %v", calledTo)
	}

	msgStr := string(calledMsg)
	if !strings.Contains(msgStr, "Subject: Test Subject") {
		t.Errorf("expected message to contain Subject: Test Subject, got %s", msgStr)
	}
	if !strings.Contains(msgStr, "Test Body") {
		t.Errorf("expected message to contain Test Body, got %s", msgStr)
	}
}
