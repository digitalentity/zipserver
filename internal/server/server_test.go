package server

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"zipserver/internal/storage"
)

type mockStorage struct {
	storage.Storage
	versions []storage.VersionInfo
	zipData  []byte
}

func (m *mockStorage) ListVersions(ctx context.Context, book string) ([]storage.VersionInfo, error) {
	return m.versions, nil
}

func (m *mockStorage) OpenZip(ctx context.Context, book, version string) (storage.ZipFileContent, error) {
	return &mockZipContent{ReaderAt: bytes.NewReader(m.zipData), size: int64(len(m.zipData))}, nil
}

type mockZipContent struct {
	io.ReaderAt
	size int64
}

func (m *mockZipContent) Size() int64 { return m.size }
func (m *mockZipContent) Close() error { return nil }

func TestServeFromZipIntegration(t *testing.T) {
	// Create a real ZIP in memory
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	
	files := map[string]string{
		"index.html":        "<h1>Index</h1>",
		"css/style.css":     "body { color: red; }",
		"js/app.js":         "console.log('hello');",
	}
	
	for name, content := range files {
		w, _ := zw.Create(name)
		w.Write([]byte(content))
	}
	zw.Close()

	m := &mockStorage{
		versions: []storage.VersionInfo{
			{Name: "v1", Time: time.Now()},
		},
		zipData: buf.Bytes(),
	}
	
	srv, _ := NewServer(nil, m)

	tests := []struct {
		url          string
		wantStatus   int
		wantContent  string
		wantType     string
	}{
		{"/book1/v1/", http.StatusOK, "<h1>Index</h1>", "text/html"},
		{"/book1/v1/index.html", http.StatusOK, "<h1>Index</h1>", "text/html"},
		{"/book1/v1/css/style.css", http.StatusOK, "body { color: red; }", "text/css"},
		{"/book1/v1/js/app.js", http.StatusOK, "console.log('hello');", "text/javascript"},
		{"/book1/v1/nonexistent", http.StatusNotFound, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.url, nil)
			rr := httptest.NewRecorder()
			srv.HandleIndex(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("URL %s: expected status %d, got %d", tt.url, tt.wantStatus, rr.Code)
			}
			if tt.wantStatus == http.StatusOK {
				if !strings.Contains(rr.Body.String(), tt.wantContent) {
					t.Errorf("URL %s: expected content %q, got %q", tt.url, tt.wantContent, rr.Body.String())
				}
				gotType := rr.Header().Get("Content-Type")
				if !strings.HasPrefix(gotType, tt.wantType) {
					t.Errorf("URL %s: expected type %q, got %q", tt.url, tt.wantType, gotType)
				}
			}
		})
	}
}
