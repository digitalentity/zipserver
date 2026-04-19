package storage

import (
	"context"
	"io"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

type GCSStorage struct {
	client *storage.Client
	bucket string
}

func NewGCSStorage(ctx context.Context, bucket string) (*GCSStorage, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	return &GCSStorage{
		client: client,
		bucket: bucket,
	}, nil
}

func (g *GCSStorage) ListBooks(ctx context.Context) ([]BookInfo, error) {
	it := g.client.Bucket(g.bucket).Objects(ctx, &storage.Query{Delimiter: "/"})
	var books []BookInfo
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		if attrs.Prefix != "" {
			books = append(books, BookInfo{Name: strings.TrimSuffix(attrs.Prefix, "/")})
		}
	}
	return books, nil
}

func (g *GCSStorage) ListVersions(ctx context.Context, book string) ([]VersionInfo, error) {
	prefix := book + "/"
	it := g.client.Bucket(g.bucket).Objects(ctx, &storage.Query{Prefix: prefix})
	var versions []VersionInfo
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		if !attrs.Deleted.IsZero() || !strings.HasSuffix(attrs.Name, ".zip") {
			continue
		}
		versions = append(versions, VersionInfo{
			Name: strings.TrimSuffix(strings.TrimPrefix(attrs.Name, prefix), ".zip"),
			Time: attrs.Updated,
			Path: attrs.Name,
			size: attrs.Size,
		})
	}
	return versions, nil
}

type gcsFile struct {
	ctx  context.Context
	obj  *storage.ObjectHandle
	name string
	size int64
}

func (gf *gcsFile) ReadAt(p []byte, off int64) (n int, err error) {
	rc, err := gf.obj.NewRangeReader(gf.ctx, off, int64(len(p)))
	if err != nil {
		return 0, err
	}
	defer rc.Close()
	return io.ReadFull(rc, p)
}

func (gf *gcsFile) Size() int64 {
	return gf.size
}

func (gf *gcsFile) Close() error {
	return nil
}

func (g *GCSStorage) OpenZip(ctx context.Context, book, version string) (ZipFileContent, error) {
	if !strings.HasSuffix(version, ".zip") {
		version += ".zip"
	}
	name := book + "/" + version
	obj := g.client.Bucket(g.bucket).Object(name)
	attrs, err := obj.Attrs(ctx)
	if err != nil {
		return nil, err
	}

	return &gcsFile{
		ctx:  ctx,
		obj:  obj,
		name: name,
		size: attrs.Size,
	}, nil
}
