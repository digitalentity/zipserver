package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type DriveStorage struct {
	service  *drive.Service
	folderID string
}

func NewDriveStorage(ctx context.Context, folderID string, credentialsFile string) (*DriveStorage, error) {
	opts := []option.ClientOption{
		option.WithScopes(drive.DriveScope),
	}
	if credentialsFile != "" {
		opts = append(opts, option.WithCredentialsFile(credentialsFile))
	}

	srv, err := drive.NewService(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &DriveStorage{
		service:  srv,
		folderID: folderID,
	}, nil
}

func (d *DriveStorage) ListBooks(ctx context.Context) ([]BookInfo, error) {
	query := fmt.Sprintf("'%s' in parents and mimeType = 'application/vnd.google-apps.folder' and trashed = false", d.folderID)
	res, err := d.service.Files.List().Q(query).Fields("files(id, name)").Context(ctx).Do()
	if err != nil {
		return nil, err
	}

	var books []BookInfo
	for _, f := range res.Files {
		books = append(books, BookInfo{Name: f.Name})
	}
	return books, nil
}

func (d *DriveStorage) findFolder(ctx context.Context, name string) (string, error) {
	query := fmt.Sprintf("'%s' in parents and name = '%s' and mimeType = 'application/vnd.google-apps.folder' and trashed = false", d.folderID, name)
	res, err := d.service.Files.List().Q(query).Fields("files(id)").Context(ctx).Do()
	if err != nil {
		return "", err
	}
	if len(res.Files) == 0 {
		return "", fmt.Errorf("folder not found: %s", name)
	}
	return res.Files[0].Id, nil
}

func (d *DriveStorage) ListVersions(ctx context.Context, book string) ([]VersionInfo, error) {
	bookID, err := d.findFolder(ctx, book)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf("'%s' in parents and mimeType = 'application/zip' and trashed = false", bookID)
	res, err := d.service.Files.List().Q(query).Fields("files(id, name, modifiedTime, size)").Context(ctx).Do()
	if err != nil {
		return nil, err
	}

	var versions []VersionInfo
	for _, f := range res.Files {
		t, _ := time.Parse(time.RFC3339, f.ModifiedTime)
		versions = append(versions, VersionInfo{
			Name: strings.TrimSuffix(f.Name, ".zip"),
			Time: t,
			Path: f.Id,
			size: f.Size,
		})
	}
	return versions, nil
}

type driveFile struct {
	ctx     context.Context
	service *drive.Service
	fileID  string
	size    int64
}

func (df *driveFile) ReadAt(p []byte, off int64) (n int, err error) {
	rangeHeader := fmt.Sprintf("bytes=%d-%d", off, off+int64(len(p))-1)
	call := df.service.Files.Get(df.fileID)
	call.Header().Set("Range", rangeHeader)
	resp, err := call.Context(df.ctx).Download()
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return io.ReadFull(resp.Body, p)
}

func (df *driveFile) Size() int64 { return df.size }
func (df *driveFile) Close() error { return nil }

func (d *DriveStorage) OpenZip(ctx context.Context, book, version string) (ZipFileContent, error) {
	bookID, err := d.findFolder(ctx, book)
	if err != nil {
		return nil, err
	}

	if !strings.HasSuffix(version, ".zip") {
		version += ".zip"
	}

	query := fmt.Sprintf("'%s' in parents and name = '%s' and trashed = false", bookID, version)
	res, err := d.service.Files.List().Q(query).Fields("files(id, size)").Context(ctx).Do()
	if err != nil || len(res.Files) == 0 {
		return nil, fmt.Errorf("version not found: %s/%s", book, version)
	}
	f := res.Files[0]

	return &driveFile{
		ctx:     ctx,
		service: d.service,
		fileID:  f.Id,
		size:    f.Size,
	}, nil
}

func (d *DriveStorage) UploadZip(ctx context.Context, book, version string, r io.Reader) error {
	bookID, err := d.findFolder(ctx, book)
	if err != nil {
		// Create folder if not found
		f := &drive.File{
			Name:     book,
			MimeType: "application/vnd.google-apps.folder",
			Parents:  []string{d.folderID},
		}
		res, err := d.service.Files.Create(f).Context(ctx).Do()
		if err != nil {
			return err
		}
		bookID = res.Id
	}

	if !strings.HasSuffix(version, ".zip") {
		version += ".zip"
	}

	f := &drive.File{
		Name:    version,
		Parents: []string{bookID},
	}
	_, err = d.service.Files.Create(f).Media(r).Context(ctx).Do()
	return err
}
