package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	content := `
port: "9090"
zip_dir: "./test_zips"
`
	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tmpfile.Name())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Port != "9090" {
		t.Errorf("expected port 9090, got %s", cfg.Port)
	}
	if cfg.ZipDir != "./test_zips" {
		t.Errorf("expected zip_dir ./test_zips, got %s", cfg.ZipDir)
	}
}

func TestLoadDefaults(t *testing.T) {
	content := "port: \"\""
	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	cfg, err := Load(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Port != "8080" {
		t.Errorf("expected default port 8080, got %s", cfg.Port)
	}
	if cfg.ZipDir != "./publish" {
		t.Errorf("expected default zip_dir ./publish, got %s", cfg.ZipDir)
	}
}

func TestLoadUploadConfig(t *testing.T) {
	content := `
upload:
  enabled: true
  token: "config-token"
`
	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	// Test config file values
	cfg, err := Load(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.Upload.Enabled {
		t.Error("expected upload.enabled to be true")
	}
	if cfg.Upload.Token != "config-token" {
		t.Errorf("expected upload.token config-token, got %s", cfg.Upload.Token)
	}

	// Test environment variable override
	os.Setenv("UPLOAD_TOKEN", "env-token")
	defer os.Unsetenv("UPLOAD_TOKEN")

	cfg, err = Load(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Upload.Token != "env-token" {
		t.Errorf("expected upload.token env-token, got %s", cfg.Upload.Token)
	}
}

func TestLoadNotificationsConfig(t *testing.T) {
	content := `
notifications:
  enabled: true
  base_url: "http://example.com"
  smtp:
    host: "smtp.mail.com"
    port: 25
    username: "user"
    password: "pwd"
    from: "noreply@mail.com"
  watchers:
    - "watcher@mail.com"
`
	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	cfg, err := Load(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.Notifications.Enabled {
		t.Error("expected notifications.enabled to be true")
	}
	if cfg.Notifications.BaseURL != "http://example.com" {
		t.Errorf("expected base_url http://example.com, got %s", cfg.Notifications.BaseURL)
	}
	if cfg.Notifications.SMTP.Host != "smtp.mail.com" || cfg.Notifications.SMTP.Port != 25 {
		t.Errorf("unexpected smtp config: %+v", cfg.Notifications.SMTP)
	}
	if len(cfg.Notifications.Watchers) != 1 || cfg.Notifications.Watchers[0] != "watcher@mail.com" {
		t.Errorf("unexpected watchers: %+v", cfg.Notifications.Watchers)
	}

	// Test env override
	os.Setenv("SMTP_PORT", "587")
	os.Setenv("NOTIFICATIONS_ENABLED", "false")
	defer func() {
		os.Unsetenv("SMTP_PORT")
		os.Unsetenv("NOTIFICATIONS_ENABLED")
	}()

	cfg, err = Load(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Notifications.SMTP.Port != 587 {
		t.Errorf("expected env overridden port 587, got %d", cfg.Notifications.SMTP.Port)
	}
	if cfg.Notifications.Enabled {
		t.Error("expected env overridden notifications.enabled to be false")
	}
}
