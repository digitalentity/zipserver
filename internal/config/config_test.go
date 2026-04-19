package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	content := `
port: "9090"
zip_dir: "./test_zips"
storage_type: "gcs"
gcs:
  bucket: "test-bucket"
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
	if cfg.StorageType != "gcs" {
		t.Errorf("expected storage_type gcs, got %s", cfg.StorageType)
	}
	if cfg.GCS.Bucket != "test-bucket" {
		t.Errorf("expected bucket test-bucket, got %s", cfg.GCS.Bucket)
	}
	// Test default cache_dir
	if cfg.CacheDir != "./cache" {
		t.Errorf("expected default cache_dir ./cache, got %s", cfg.CacheDir)
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
