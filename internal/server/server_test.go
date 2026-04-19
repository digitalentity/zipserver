package server

import (
	"context"
	"testing"
	"time"

	"zipserver/internal/storage"
)

type mockStorage struct {
	storage.Storage
	versions []storage.VersionInfo
}

func (m *mockStorage) ListVersions(ctx context.Context, book string) ([]storage.VersionInfo, error) {
	return m.versions, nil
}

func (m *mockStorage) OpenZip(ctx context.Context, book, version string) (storage.ZipFileContent, error) {
	return nil, nil // Not needed for this test
}

func TestGetLatestVersion(t *testing.T) {
	now := time.Now()
	m := &mockStorage{
		versions: []storage.VersionInfo{
			{Name: "v1", Time: now.Add(-2 * time.Hour)},
			{Name: "v3", Time: now},
			{Name: "v2", Time: now.Add(-1 * time.Hour)},
		},
	}
	s := &Server{storage: m}

	latest, err := s.getLatestVersion(context.Background(), "testbook")
	if err != nil {
		t.Fatalf("getLatestVersion failed: %v", err)
	}
	if latest != "v3" {
		t.Errorf("expected v3, got %s", latest)
	}
}
