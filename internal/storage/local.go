package storage

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type LocalStorage struct {
	dir string
}

func NewLocalStorage(dir string) *LocalStorage {
	return &LocalStorage{dir: dir}
}

func (l *LocalStorage) Close() error { return nil }

func (l *LocalStorage) ListBooks(ctx context.Context) ([]BookInfo, error) {
	files, err := os.ReadDir(l.dir)
	if err != nil {
		return nil, err
	}

	var books []BookInfo
	for _, f := range files {
		if f.IsDir() {
			books = append(books, BookInfo{Name: f.Name()})
		}
	}
	return books, nil
}

func (l *LocalStorage) ListVersions(ctx context.Context, book string) ([]VersionInfo, error) {
	if err := ValidateName(book); err != nil {
		return nil, err
	}
	bookDir := filepath.Join(l.dir, book)
	files, err := os.ReadDir(bookDir)
	if err != nil {
		return nil, err
	}

	var versions []VersionInfo
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".zip") {
			info, err := f.Info()
			if err != nil {
				return nil, err
			}
			versions = append(versions, VersionInfo{
				Name: strings.TrimSuffix(f.Name(), ".zip"),
				Time: info.ModTime(),
				Path: f.Name(),
				size: info.Size(),
			})
		}
	}
	return versions, nil
}

type localFile struct {
	*os.File
	size int64
}

func (lf *localFile) Size() int64 {
	return lf.size
}
func (l *LocalStorage) resolvePath(book, version string) (string, error) {
	if err := ValidateName(book); err != nil {
		return "", err
	}
	if err := ValidateName(version); err != nil {
		return "", err
	}
	return filepath.Join(l.dir, book, EnsureZipSuffix(version)), nil
}

func (l *LocalStorage) OpenZip(ctx context.Context, book, version string) (ZipFileContent, error) {
	path, err := l.resolvePath(book, version)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	return &localFile{File: f, size: info.Size()}, nil
}

func (l *LocalStorage) OpenZipStream(ctx context.Context, book, version string) (io.ReadCloser, error) {
	path, err := l.resolvePath(book, version)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

func (l *LocalStorage) UploadZip(ctx context.Context, book, version string, r io.Reader) error {
	path, err := l.resolvePath(book, version)
	if err != nil {
		return err
	}
	bookDir := filepath.Dir(path)
	if err := os.MkdirAll(bookDir, 0755); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, r)
	return err
}
