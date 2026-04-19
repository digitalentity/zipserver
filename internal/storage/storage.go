package storage

import (
	"context"
	"io"
	"time"
)

type BookInfo struct {
	Name string
}

type VersionInfo struct {
	Name string
	Time time.Time
	Path string
	size int64
}

func (v *VersionInfo) Size() int64 {
	return v.size
}

type ZipFileContent interface {
	io.ReaderAt
	Size() int64
	io.Closer
}

type Storage interface {
	ListBooks(ctx context.Context) ([]BookInfo, error)
	ListVersions(ctx context.Context, book string) ([]VersionInfo, error)
	OpenZip(ctx context.Context, book, version string) (ZipFileContent, error)
	UploadZip(ctx context.Context, book, version string, r io.Reader) error
}
