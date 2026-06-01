package notifications

import (
	"crypto/tls"
	"net/smtp"
	"strings"
	"testing"

	"zipserver/internal/config"
)

func TestSendEmail(t *testing.T) {
	tests := []struct {
		name          string
		fromConfig    string
		baseURL       string
		smtpHello     string
		expectedFrom  string
		expectedHello string
	}{
		{
			name:          "Plain email address",
			fromConfig:    "noreply@example.com",
			expectedFrom:  "noreply@example.com",
			expectedHello: "example.com",
		},
		{
			name:          "RFC 5322 Formatted from with name",
			fromConfig:    "ZipServer<noreply@example.com>",
			expectedFrom:  "noreply@example.com",
			expectedHello: "example.com",
		},
		{
			name:          "RFC 5322 Formatted from with spaces",
			fromConfig:    "ZipServer <noreply@example.com>",
			expectedFrom:  "noreply@example.com",
			expectedHello: "example.com",
		},
		{
			name:          "Fallback using baseURL",
			fromConfig:    "noreply@example.com",
			baseURL:       "https://air.inavflight.com/",
			expectedFrom:  "noreply@example.com",
			expectedHello: "air.inavflight.com",
		},
		{
			name:          "Explicit smtp hello name overrides all",
			fromConfig:    "noreply@example.com",
			baseURL:       "https://air.inavflight.com/",
			smtpHello:     "custom.local",
			expectedFrom:  "noreply@example.com",
			expectedHello: "custom.local",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NotificationsConfig{
				BaseURL: tt.baseURL,
				SMTP: config.SMTPConfig{
					Host:     "localhost",
					Port:     25,
					Username: "user",
					Password: "pwd",
					From:     tt.fromConfig,
					Hello:    tt.smtpHello,
				},
			}

			var calledAddr string
			var calledFrom string
			var calledTo []string
			var calledMsg []byte
			var calledHello string

			// Mock dialAndSend
			oldDialAndSend := dialAndSend
			dialAndSend = func(addr string, host string, port int, localName string, encryption string, tlsConfig *tls.Config, a smtp.Auth, from string, to []string, msg []byte) error {
				calledAddr = addr
				calledFrom = from
				calledTo = to
				calledMsg = msg
				calledHello = localName
				return nil
			}
			defer func() {
				dialAndSend = oldDialAndSend
			}()

			mailer := NewMailer(cfg)
			err := mailer.SendEmail([]string{"test@example.com"}, "Test Subject", "Test Body")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if calledAddr != "localhost:25" {
				t.Errorf("expected localhost:25, got %s", calledAddr)
			}
			if calledFrom != tt.expectedFrom {
				t.Errorf("expected %s, got %s", tt.expectedFrom, calledFrom)
			}
			if len(calledTo) != 1 || calledTo[0] != "test@example.com" {
				t.Errorf("expected [test@example.com], got %v", calledTo)
			}
			if calledHello != tt.expectedHello {
				t.Errorf("expected hello %s, got %s", tt.expectedHello, calledHello)
			}

			msgStr := string(calledMsg)
			if !strings.Contains(msgStr, "Subject: Test Subject") {
				t.Errorf("expected message to contain Subject: Test Subject, got %s", msgStr)
			}
			if !strings.Contains(msgStr, "Test Body") {
				t.Errorf("expected message to contain Test Body, got %s", msgStr)
			}
			if !strings.Contains(msgStr, "From: "+tt.fromConfig) {
				t.Errorf("expected header From: %s, got %s", tt.fromConfig, msgStr)
			}
		})
	}
}
