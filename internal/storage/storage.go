package storage

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

var validName = regexp.MustCompile(`^[a-zA-Z0-9._\-:+~]+$`)

func validateName(s string) error {
	if !validName.MatchString(s) || strings.Contains(s, "..") {
		return fmt.Errorf("invalid name %q: must match [a-zA-Z0-9._-]+ and must not contain ..", s)
	}
	return nil
}

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
	OpenZipStream(ctx context.Context, book, version string) (io.ReadCloser, error)
	UploadZip(ctx context.Context, book, version string, r io.Reader) error
	Close() error
}
