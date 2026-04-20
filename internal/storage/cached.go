package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

type CachingStorage struct {
	base     Storage
	cacheDir string
	cacheTTL time.Duration
	sf       singleflight.Group // deduplicates concurrent downloads per (book, version)

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
		result := copyVersions(c.versionsCache[book])
		c.cacheMu.RUnlock()
		return result, nil
	}
	c.cacheMu.RUnlock()

	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	// Re-check after acquiring write lock
	if time.Since(c.versionsFetchedAt[book]) < c.cacheTTL {
		return copyVersions(c.versionsCache[book]), nil
	}

	versions, err := c.base.ListVersions(ctx, book)
	if err != nil {
		return nil, err
	}

	c.versionsCache[book] = versions
	c.versionsFetchedAt[book] = time.Now()
	return copyVersions(versions), nil
}

func copyVersions(src []VersionInfo) []VersionInfo {
	dst := make([]VersionInfo, len(src))
	copy(dst, src)
	return dst
}

func (c *CachingStorage) OpenZip(ctx context.Context, book, version string) (ZipFileContent, error) {
	v := version
	if !strings.HasSuffix(v, ".zip") {
		v += ".zip"
	}
	cachePath := filepath.Join(c.cacheDir, book, v)

	// Deduplicate concurrent downloads of the same (book, version). Different
	// pairs proceed in parallel; duplicates share a single download.
	_, err, _ := c.sf.Do(book+"/"+v, func() (any, error) {
		return nil, c.ensureCached(ctx, book, version, cachePath)
	})
	if err != nil {
		return nil, err
	}

	// Each caller gets its own file descriptor into the cached file.
	f, err := os.Open(cachePath)
	if err != nil {
		return nil, err
	}
	s, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &localFile{File: f, size: s.Size()}, nil
}

func (c *CachingStorage) OpenZipStream(ctx context.Context, book, version string) (io.ReadCloser, error) {
	v := version
	if !strings.HasSuffix(v, ".zip") {
		v += ".zip"
	}
	cachePath := filepath.Join(c.cacheDir, book, v)

	_, err, _ := c.sf.Do(book+"/"+v, func() (any, error) {
		return nil, c.ensureCached(ctx, book, version, cachePath)
	})
	if err != nil {
		return nil, err
	}

	return os.Open(cachePath)
}

// ensureCached downloads (book, version) to cachePath if the local copy is
// absent or stale. It is always called from within a singleflight.Do, so at
// most one goroutine runs it for any given (book, version) at a time.
func (c *CachingStorage) ensureCached(ctx context.Context, book, version, cachePath string) error {
	// Fetch metadata inside this serialised call so an interleaved UploadZip
	// (which invalidates the versions cache) cannot make us serve a stale file.
	versions, err := c.ListVersions(ctx, book)
	if err != nil {
		return err
	}

	vName := strings.TrimSuffix(version, ".zip")
	var remoteInfo *VersionInfo
	for i := range versions {
		if versions[i].Name == vName {
			remoteInfo = &versions[i]
			break
		}
	}
	if remoteInfo == nil {
		return fmt.Errorf("version %s not found for book %s", version, book)
	}

	if s, err := os.Stat(cachePath); err == nil {
		if s.Size() == remoteInfo.Size() && !remoteInfo.Time.After(s.ModTime()) {
			return nil // cache is valid
		}
	}

	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return err
	}

	remoteStream, err := c.base.OpenZipStream(ctx, book, version)
	if err != nil {
		return err
	}
	defer remoteStream.Close()

	tmpFile, err := os.CreateTemp(c.cacheDir, "download-*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	renamed := false
	defer func() {
		if !renamed {
			os.Remove(tmpName)
		}
	}()

	if _, err := io.Copy(tmpFile, remoteStream); err != nil {
		tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, cachePath); err != nil {
		return err
	}
	renamed = true
	return nil
}

func (c *CachingStorage) Close() error { return c.base.Close() }

func (c *CachingStorage) UploadZip(ctx context.Context, book, version string, r io.Reader) error {
	err := c.base.UploadZip(ctx, book, version, r)
	if err != nil {
		return err
	}

	// Invalidate cache on upload
	v := version
	if !strings.HasSuffix(v, ".zip") {
		v += ".zip"
	}
	cachePath := filepath.Join(c.cacheDir, book, v)
	_ = os.Remove(cachePath)

	c.cacheMu.Lock()
	c.booksFetchedAt = time.Time{}           // force refresh books list
	delete(c.versionsFetchedAt, book)        // force refresh versions for this book
	c.cacheMu.Unlock()

	return nil
}
