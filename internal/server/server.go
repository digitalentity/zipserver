package server

import (
	"archive/zip"
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"zipserver/internal/config"
	"zipserver/internal/storage"
)

//go:embed templates/*.tpl
var templateFS embed.FS

type ZipFile struct {
	Name string
	Time string
	Path string
}

type VersionPageData struct {
	BookName string
	Versions []ZipFile
}

type Server struct {
	storage     storage.Storage
	bookTmpl    *template.Template
	versionTmpl *template.Template
}

func renderTemplate(w http.ResponseWriter, tmpl *template.Template, data any) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		slog.Error("template execution error", "template", tmpl.Name(), "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

func NewServer(cfg *config.Config, s storage.Storage) (*Server, error) {
	bookTmpl, err := template.ParseFS(templateFS, "templates/books.html.tpl")
	if err != nil {
		return nil, err
	}
	versionTmpl, err := template.ParseFS(templateFS, "templates/versions.html.tpl")
	if err != nil {
		return nil, err
	}

	return &Server{
		storage:     s,
		bookTmpl:    bookTmpl,
		versionTmpl: versionTmpl,
	}, nil
}

func (s *Server) HandleIndex(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(r.URL.Path, "/")
	if path == "" {
		s.renderBookList(w, r)
		return
	}

	parts := strings.Split(path, "/")
	book := parts[0]

	if len(parts) == 1 {
		s.renderVersionList(w, r, book)
		return
	}

	version := parts[1]
	if version == "latest" {
		resolved, err := s.getLatestVersion(r.Context(), book)
		if err != nil || resolved == "" {
			http.NotFound(w, r)
			return
		}
		version = resolved
	}

	innerPath := ""
	if len(parts) > 2 {
		innerPath = strings.Join(parts[2:], "/")
	}
	if innerPath == "" {
		innerPath = "index.html"
	}

	s.serveFromZip(w, r, book, version, innerPath)
}

func (s *Server) getLatestVersion(ctx context.Context, book string) (string, error) {
	versions, err := s.storage.ListVersions(ctx, book)
	if err != nil {
		return "", err
	}
	if len(versions) == 0 {
		return "", nil
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Time.After(versions[j].Time)
	})

	return versions[0].Name, nil
}

func (s *Server) HandleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	book := r.URL.Query().Get("book")
	version := r.URL.Query().Get("version")

	if book == "" || version == "" {
		http.Error(w, "Missing book or version parameter", http.StatusBadRequest)
		return
	}

	defer r.Body.Close()
	if err := s.storage.UploadZip(r.Context(), book, version, r.Body); err != nil {
		slog.Error("upload failed", "error", err, "book", book, "version", version)
		http.Error(w, "Upload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	if _, err := w.Write([]byte("Upload successful")); err != nil {
		slog.Error("failed to write upload response", "error", err)
	}
}

func (s *Server) renderBookList(w http.ResponseWriter, r *http.Request) {
	books, err := s.storage.ListBooks(r.Context())
	if err != nil {
		slog.Error("unable to list books", "error", err)
		http.Error(w, "Unable to list books: "+err.Error(), http.StatusInternalServerError)
		return
	}

	sort.Slice(books, func(i, j int) bool {
		return books[i].Name < books[j].Name
	})

	renderTemplate(w, s.bookTmpl, books)
}

func (s *Server) renderVersionList(w http.ResponseWriter, r *http.Request, book string) {
	versionsInfo, err := s.storage.ListVersions(r.Context(), book)
	if err != nil {
		slog.Error("unable to list versions", "error", err, "book", book)
		http.NotFound(w, r)
		return
	}

	sort.Slice(versionsInfo, func(i, j int) bool {
		return versionsInfo[i].Time.After(versionsInfo[j].Time)
	})

	var versions []ZipFile
	for _, f := range versionsInfo {
		versions = append(versions, ZipFile{
			Name: f.Name,
			Time: f.Time.Format("2006-01-02 15:04:05"),
			Path: f.Path,
		})
	}

	data := VersionPageData{
		BookName: book,
		Versions: versions,
	}

	renderTemplate(w, s.versionTmpl, data)
}

func (s *Server) serveFromZip(w http.ResponseWriter, r *http.Request, book, version, innerPath string) {
	content, err := s.storage.OpenZip(r.Context(), book, version)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer content.Close()

	reader, err := zip.NewReader(content, content.Size())
	if err != nil {
		slog.Error("unable to read zip", "error", err, "book", book, "version", version)
		http.Error(w, "Unable to read zip", http.StatusInternalServerError)
		return
	}

	slog.Info("searching for file in zip", "book", book, "version", version, "innerPath", innerPath)

	var targetFile *zip.File
	for _, f := range reader.File {
		if f.Name == innerPath {
			targetFile = f
			break
		}
	}

	// If not found and it's a directory-like request, try adding index.html
	if targetFile == nil && !strings.HasSuffix(innerPath, ".html") {
		altPath := strings.TrimSuffix(innerPath, "/") + "/index.html"
		if strings.HasPrefix(altPath, "/") {
			altPath = altPath[1:]
		}
		for _, f := range reader.File {
			if f.Name == altPath {
				targetFile = f
				break
			}
		}
	}

	if targetFile == nil {
		var names []string
		for i, f := range reader.File {
			if i > 10 {
				names = append(names, "...")
				break
			}
			names = append(names, f.Name)
		}
		slog.Warn("file not found in zip", "book", book, "version", version, "innerPath", innerPath, "zip_contents_sample", names)
		http.NotFound(w, r)
		return
	}

	const maxUncompressedSize = 256 << 20 // 256 MB
	if targetFile.UncompressedSize64 > maxUncompressedSize {
		slog.Error("file too large in zip", "size", targetFile.UncompressedSize64, "limit", maxUncompressedSize, "book", book, "version", version, "path", innerPath)
		http.Error(w, "File too large", http.StatusRequestEntityTooLarge)
		return
	}

	ext := filepath.Ext(targetFile.Name)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", targetFile.UncompressedSize64))

	lastModified := targetFile.Modified.UTC()
	etag := fmt.Sprintf("\"%x-%x\"", targetFile.CRC32, targetFile.UncompressedSize64)

	w.Header().Set("Last-Modified", lastModified.Format(http.TimeFormat))
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=31536000") // ZIP versions are immutable

	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	if ifModSince := r.Header.Get("If-Modified-Since"); ifModSince != "" {
		if t, err := http.ParseTime(ifModSince); err == nil {
			if !lastModified.After(t) {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
	}

	rc, err := targetFile.Open()
	if err != nil {
		slog.Error("error opening file in zip", "error", err, "book", book, "version", version, "innerPath", innerPath)
		http.Error(w, "Error opening file in zip", http.StatusInternalServerError)
		return
	}
	defer rc.Close()

	if _, err := io.Copy(w, rc); err != nil {
		slog.Error("error streaming file from zip", "error", err, "book", book, "version", version, "path", innerPath)
	}
}
