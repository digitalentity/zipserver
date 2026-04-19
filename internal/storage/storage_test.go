package storage

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func createDummyZip(t *testing.T, path string) {
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	w, err := zw.Create("test.txt")
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("hello world"))
}

func TestLocalStorage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "zipserver-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create structure: tmpDir/book1/v1.zip
	bookDir := filepath.Join(tmpDir, "book1")
	if err := os.Mkdir(bookDir, 0755); err != nil {
		t.Fatal(err)
	}
	createDummyZip(t, filepath.Join(bookDir, "v1.zip"))

	s := NewLocalStorage(tmpDir)
	ctx := context.Background()

	// Test ListBooks
	books, err := s.ListBooks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].Name != "book1" {
		t.Errorf("expected 1 book 'book1', got %v", books)
	}

	// Test ListVersions
	versions, err := s.ListVersions(ctx, "book1")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].Name != "v1" {
		t.Errorf("expected 1 version 'v1', got %v", versions)
	}

	// Test OpenZip
	content, err := s.OpenZip(ctx, "book1", "v1")
	if err != nil {
		t.Fatal(err)
	}
	defer content.Close()

	zr, err := zip.NewReader(content, content.Size())
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != 1 || zr.File[0].Name != "test.txt" {
		t.Errorf("unexpected zip content")
	}
}
