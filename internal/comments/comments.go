package comments

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// W3C Web Annotation Structs

type Creator struct {
	ID     string `json:"id"`
	Type   string `json:"type"` // "Person"
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

type TextualBody struct {
	Type   string `json:"type"` // "TextualBody"
	Value  string `json:"value"`
	Format string `json:"format"` // "text/markdown"
}

type Selector struct {
	Type   string `json:"type"` // "TextQuoteSelector", "TextPositionSelector", "FragmentSelector"
	Exact  string `json:"exact,omitempty"`
	Prefix string `json:"prefix,omitempty"`
	Suffix string `json:"suffix,omitempty"`
	Start  int    `json:"start,omitempty"`
	End    int    `json:"end,omitempty"`
	Value  string `json:"value,omitempty"`
}

type TargetObject struct {
	Source   string     `json:"source"`
	Selector []Selector `json:"selector,omitempty"`
}

type Target struct {
	String string
	Object *TargetObject
}

func (t *Target) MarshalJSON() ([]byte, error) {
	if t.Object != nil {
		return json.Marshal(t.Object)
	}
	return json.Marshal(t.String)
}

func (t *Target) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		t.String = s
		t.Object = nil
		return nil
	}
	var obj TargetObject
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	t.Object = &obj
	t.String = ""
	return nil
}

type Annotation struct {
	Context    string       `json:"@context"`
	ID         string       `json:"id"`
	Type       string       `json:"type"` // "Annotation"
	Created    string       `json:"created"`
	Modified   string       `json:"modified"`
	Creator    Creator      `json:"creator"`
	Body       TextualBody  `json:"body"`
	Target     Target       `json:"target"`
	Motivation string       `json:"motivation,omitempty"` // "replying" for replies
	Resolved   *bool        `json:"resolved,omitempty"`

	// Internal metadata
	Book    string `json:"book,omitempty"`
	Page    string `json:"page,omitempty"`
	Version string `json:"version_created,omitempty"`
}

var validName = regexp.MustCompile(`^[a-zA-Z0-9._\-:+~]+$`)

func validateName(s string) error {
	if !validName.MatchString(s) || strings.Contains(s, "..") {
		return fmt.Errorf("invalid name %q: must match [a-zA-Z0-9._-]+ and must not contain ..", s)
	}
	return nil
}

// ParsePageParam parses a full URL or relative path into book, version, and page path.
func ParsePageParam(pageParam string) (book, version, innerPath string, err error) {
	if pageParam == "" {
		return "", "", "", fmt.Errorf("empty page parameter")
	}
	if strings.Contains(pageParam, "..") {
		return "", "", "", fmt.Errorf("page parameter must not contain ..")
	}

	p := pageParam
	if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		u, err := url.Parse(p)
		if err != nil {
			return "", "", "", fmt.Errorf("invalid page URL: %w", err)
		}
		p = u.Path
	}

	p = path.Clean(p)
	p = strings.Trim(p, "/")

	parts := strings.Split(p, "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", "", "", fmt.Errorf("invalid page path structure")
	}

	// Filter system paths
	if parts[0] == "_" || parts[0] == "api" {
		return "", "", "", fmt.Errorf("system path not commentable")
	}

	book = parts[0]
	if err := validateName(book); err != nil {
		return "", "", "", fmt.Errorf("invalid book name: %w", err)
	}
	if len(parts) == 1 {
		// Book level index (e.g. /mybook/) -> default to latest/index.html
		version = "latest"
		innerPath = "index.html"
		return book, version, innerPath, nil
	}

	version = parts[1]
	if err := validateName(version); err != nil {
		return "", "", "", fmt.Errorf("invalid version name: %w", err)
	}
	if len(parts) == 2 {
		innerPath = "index.html"
	} else {
		innerPath = strings.Join(parts[2:], "/")
	}

	return book, version, innerPath, nil
}

// HashPagePath returns a safe hex-encoded SHA-256 hash of the page path.
func HashPagePath(pagePath string) string {
	cleaned := path.Clean(strings.ToLower(strings.Trim(pagePath, "/")))
	hash := sha256.Sum256([]byte(cleaned))
	return hex.EncodeToString(hash[:])
}

// CommentStore defines the interface for comment storage
type CommentStore interface {
	GetComments(ctx context.Context, book, version, pagePath string) ([]Annotation, error)
	CreateComment(ctx context.Context, book, version, pagePath string, anno *Annotation) (*Annotation, error)
	UpdateComment(ctx context.Context, book, id string, updateFn func(*Annotation) error) (*Annotation, error)
	DeleteComment(ctx context.Context, book, id string, creatorID string) error
	GetPageForComment(id string) (book, version, pagePath string, found bool)
}

// JSONFileCommentStore implements CommentStore using local JSON files
type JSONFileCommentStore struct {
	rootDir string
	mu      sync.RWMutex

	// Index map from comment ID to its page context details
	commentIndex map[string]pageInfo
}

type pageInfo struct {
	book     string
	version  string
	pagePath string
}

func NewJSONFileCommentStore(rootDir string) (*JSONFileCommentStore, error) {
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create comments directory: %w", err)
	}

	store := &JSONFileCommentStore{
		rootDir:      rootDir,
		commentIndex: make(map[string]pageInfo),
	}

	if err := store.rebuildIndex(); err != nil {
		slog.Error("failed to build comment index", "error", err)
	}

	return store, nil
}

func (s *JSONFileCommentStore) rebuildIndex() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	clear(s.commentIndex)

	err := filepath.Walk(s.rootDir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".json") {
			return nil
		}

		rel, err := filepath.Rel(s.rootDir, filePath)
		if err != nil {
			return nil
		}
		relParts := strings.Split(filepath.ToSlash(rel), "/")
		if len(relParts) < 3 {
			return nil
		}
		book := relParts[0]
		version := relParts[1]

		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", filePath, err)
		}

		var annos []Annotation
		if err := json.Unmarshal(data, &annos); err != nil {
			slog.Warn("skipping corrupted comment file", "file", filePath, "error", err)
			return nil
		}

		for _, anno := range annos {
			if anno.ID != "" {
				s.commentIndex[anno.ID] = pageInfo{
					book:     book,
					version:  version,
					pagePath: anno.Page,
				}
			}
		}

		return nil
	})

	slog.Info("comment index built", "total_comments", len(s.commentIndex))
	return err
}

func (s *JSONFileCommentStore) GetComments(ctx context.Context, book, version, pagePath string) ([]Annotation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	hash := HashPagePath(pagePath)
	filePath := filepath.Join(s.rootDir, book, version, hash+".json")

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []Annotation{}, nil
		}
		return nil, err
	}

	var annos []Annotation
	if err := json.Unmarshal(data, &annos); err != nil {
		return nil, fmt.Errorf("failed to unmarshal comments: %w", err)
	}

	return annos, nil
}

func generateID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("anno_%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("anno_%s", hex.EncodeToString(b))
}

func (s *JSONFileCommentStore) CreateComment(ctx context.Context, book, version, pagePath string, anno *Annotation) (*Annotation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	anno.ID = generateID()
	anno.Context = "http://www.w3.org/ns/anno.jsonld"
	anno.Type = "Annotation"
	now := time.Now().UTC().Format(time.RFC3339)
	anno.Created = now
	anno.Modified = now
	anno.Book = book
	anno.Page = pagePath
	anno.Version = version

	hash := HashPagePath(pagePath)
	versionDir := filepath.Join(s.rootDir, book, version)
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		return nil, err
	}

	filePath := filepath.Join(versionDir, hash+".json")
	var annos []Annotation

	data, err := os.ReadFile(filePath)
	if err == nil {
		if err := json.Unmarshal(data, &annos); err != nil {
			slog.Warn("corrupted comment file, resetting", "file", filePath, "error", err)
		}
	}

	annos = append(annos, *anno)

	updatedData, err := json.MarshalIndent(annos, "", "  ")
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(filePath, updatedData, 0644); err != nil {
		return nil, err
	}

	s.commentIndex[anno.ID] = pageInfo{
		book:     book,
		version:  version,
		pagePath: pagePath,
	}

	return anno, nil
}

func (s *JSONFileCommentStore) UpdateComment(ctx context.Context, book, id string, updateFn func(*Annotation) error) (*Annotation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	info, exists := s.commentIndex[id]
	if !exists || info.book != book {
		return nil, os.ErrNotExist
	}

	hash := HashPagePath(info.pagePath)
	filePath := filepath.Join(s.rootDir, book, info.version, hash+".json")

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var annos []Annotation
	if err := json.Unmarshal(data, &annos); err != nil {
		return nil, err
	}

	var updatedAnno *Annotation
	found := false
	for i := range annos {
		if annos[i].ID == id {
			if err := updateFn(&annos[i]); err != nil {
				return nil, err
			}
			annos[i].Modified = time.Now().UTC().Format(time.RFC3339)
			updatedAnno = &annos[i]
			found = true
			break
		}
	}

	if !found {
		return nil, os.ErrNotExist
	}

	updatedData, err := json.MarshalIndent(annos, "", "  ")
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(filePath, updatedData, 0644); err != nil {
		return nil, err
	}

	return updatedAnno, nil
}

func (s *JSONFileCommentStore) DeleteComment(ctx context.Context, book, id string, creatorID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	info, exists := s.commentIndex[id]
	if !exists || info.book != book {
		return os.ErrNotExist
	}

	hash := HashPagePath(info.pagePath)
	filePath := filepath.Join(s.rootDir, book, info.version, hash+".json")

	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	var annos []Annotation
	if err := json.Unmarshal(data, &annos); err != nil {
		return err
	}

	var targetIdx = -1
	for i := range annos {
		if annos[i].ID == id {
			if annos[i].Creator.ID != creatorID {
				return fmt.Errorf("unauthorized: creator mismatch")
			}
			targetIdx = i
			break
		}
	}

	if targetIdx == -1 {
		return os.ErrNotExist
	}

	var filteredAnnos []Annotation
	for i := range annos {
		if i == targetIdx {
			delete(s.commentIndex, annos[i].ID)
			continue
		}
		if annos[i].Motivation == "replying" && annos[i].Target.String == id {
			delete(s.commentIndex, annos[i].ID)
			continue
		}
		filteredAnnos = append(filteredAnnos, annos[i])
	}

	if len(filteredAnnos) == 0 {
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			return err
		}
	} else {
		updatedData, err := json.MarshalIndent(filteredAnnos, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filePath, updatedData, 0644); err != nil {
			return err
		}
	}

	return nil
}

func (s *JSONFileCommentStore) GetPageForComment(id string) (book, version, pagePath string, found bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	info, exists := s.commentIndex[id]
	if !exists {
		return "", "", "", false
	}
	return info.book, info.version, info.pagePath, true
}
