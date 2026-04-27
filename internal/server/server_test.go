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

func (m *mockStorage) UploadZip(ctx context.Context, book, version string, r io.Reader) error {
	if book == "fail" {
		return io.ErrUnexpectedEOF
	}
	return nil
}

type mockZipContent struct {
	io.ReaderAt
	size int64
}

func (m *mockZipContent) Size() int64 { return m.size }
func (m *mockZipContent) Close() error { return nil }

func TestHandleUpload(t *testing.T) {
	srv, _ := NewServer(nil, &mockStorage{})

	tests := []struct {
		name        string
		method      string
		query       string
		wantStatus  int
		wantBody    string
	}{
		{
			"Success",
			"POST",
			"?book=testbook&version=v1",
			http.StatusCreated,
			`{"outcome":"success","uri":"/testbook/v1/"}`,
		},
		{
			"Method Not Allowed",
			"GET",
			"?book=testbook&version=v1",
			http.StatusMethodNotAllowed,
			`{"error":"Method not allowed"}`,
		},
		{
			"Missing Parameters",
			"POST",
			"?book=testbook",
			http.StatusBadRequest,
			`{"error":"Missing book or version parameter"}`,
		},
		{
			"Upload Failure",
			"POST",
			"?book=fail&version=v1",
			http.StatusInternalServerError,
			`{"error":"Upload failed: unexpected EOF"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/_/upload"+tt.query, strings.NewReader("zipdata"))
			rr := httptest.NewRecorder()
			srv.HandleUpload(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}

			gotBody := strings.TrimSpace(rr.Body.String())
			if gotBody != tt.wantBody {
				t.Errorf("expected body %q, got %q", tt.wantBody, gotBody)
			}

			if gotType := rr.Header().Get("Content-Type"); gotType != "application/json" {
				t.Errorf("expected Content-Type application/json, got %q", gotType)
			}
		})
	}
}


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
