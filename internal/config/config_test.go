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
