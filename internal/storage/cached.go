package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type CachingStorage struct {
	base     Storage
	cacheDir string
	cacheTTL time.Duration
	mu       sync.Mutex // for file downloads

	cacheMu           sync.RWMutex
	booksCache        []BookInfo
	booksFetchedAt    time.Time
	versionsCache     map[string][]VersionInfo
	versionsFetchedAt map[string]time.Time
}

func NewCachingStorage(base Storage, cacheDir string, cacheTTL time.Duration) (*CachingStorage, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, err
	}
	return &CachingStorage{
		base:              base,
		cacheDir:          cacheDir,
		cacheTTL:          cacheTTL,
		versionsCache:     make(map[string][]VersionInfo),
		versionsFetchedAt: make(map[string]time.Time),
	}, nil
}

func (c *CachingStorage) ListBooks(ctx context.Context) ([]BookInfo, error) {
	c.cacheMu.RLock()
	if time.Since(c.booksFetchedAt) < c.cacheTTL {
		defer c.cacheMu.RUnlock()
		return c.booksCache, nil
	}
	c.cacheMu.RUnlock()

	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	// Re-check after acquiring write lock
	if time.Since(c.booksFetchedAt) < c.cacheTTL {
		return c.booksCache, nil
	}

	books, err := c.base.ListBooks(ctx)
	if err != nil {
		return nil, err
	}

	c.booksCache = books
	c.booksFetchedAt = time.Now()
	return books, nil
}

func (c *CachingStorage) ListVersions(ctx context.Context, book string) ([]VersionInfo, error) {
	c.cacheMu.RLock()
	if time.Since(c.versionsFetchedAt[book]) < c.cacheTTL {
		defer c.cacheMu.RUnlock()
		return c.versionsCache[book], nil
	}
	c.cacheMu.RUnlock()

	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	// Re-check after acquiring write lock
	if time.Since(c.versionsFetchedAt[book]) < c.cacheTTL {
		return c.versionsCache[book], nil
	}

	versions, err := c.base.ListVersions(ctx, book)
	if err != nil {
		return nil, err
	}

	c.versionsCache[book] = versions
	c.versionsFetchedAt[book] = time.Now()
	return versions, nil
}

func (c *CachingStorage) OpenZip(ctx context.Context, book, version string) (ZipFileContent, error) {
	// 1. Get remote info to check if cache is up to date
	// We call our own ListVersions which now uses caching
	versions, err := c.ListVersions(ctx, book)
	if err != nil {
		return nil, err
	}

	var remoteInfo *VersionInfo
	for _, v := range versions {
		if v.Name == version {
			remoteInfo = &v
			break
		}
	}

	if remoteInfo == nil {
		return nil, fmt.Errorf("version %s not found for book %s", version, book)
	}

	cachePath := filepath.Join(c.cacheDir, book, version+".zip")
	
	c.mu.Lock()
	defer c.mu.Unlock()

	// 2. Check cache
	if s, err := os.Stat(cachePath); err == nil {
		// If size matches and cache isn't older than remote, use it
		if s.Size() == remoteInfo.Size() && !remoteInfo.Time.After(s.ModTime()) {
			f, err := os.Open(cachePath)
			if err == nil {
				return &localFile{File: f, size: s.Size()}, nil
			}
		}
	}

	// 3. Download to cache
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return nil, err
	}

	remoteContent, err := c.base.OpenZip(ctx, book, version)
	if err != nil {
		return nil, err
	}
	defer remoteContent.Close()

	tmpFile, err := os.CreateTemp(c.cacheDir, "download-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Download by reading from the ReaderAt (which will be a GCS/Drive stream)
	// We wrap it in a Reader
	limitReader := io.NewSectionReader(remoteContent, 0, remoteContent.Size())
	if _, err := io.Copy(tmpFile, limitReader); err != nil {
		return nil, err
	}

	if err := tmpFile.Close(); err != nil {
		return nil, err
	}

	if err := os.Rename(tmpFile.Name(), cachePath); err != nil {
		return nil, err
	}

	// 4. Open the newly cached file
	f, err := os.Open(cachePath)
	if err != nil {
		return nil, err
	}
	s, _ := f.Stat()
	return &localFile{File: f, size: s.Size()}, nil
}

func (c *CachingStorage) UploadZip(ctx context.Context, book, version string, r io.Reader) error {
	err := c.base.UploadZip(ctx, book, version, r)
	if err != nil {
		return err
	}

	// Invalidate cache on upload
	cachePath := filepath.Join(c.cacheDir, book, version+".zip")
	_ = os.Remove(cachePath)

	c.cacheMu.Lock()
	c.booksFetchedAt = time.Time{}           // force refresh books list
	delete(c.versionsFetchedAt, book)        // force refresh versions for this book
	c.cacheMu.Unlock()

	return nil
}
