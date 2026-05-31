package comments

import (
	"context"
	"os"
	"testing"
)

func TestParsePageParam(t *testing.T) {
	tests := []struct {
		input     string
		wantBook  string
		wantVer   string
		wantPath  string
		expectErr bool
	}{
		{
			input:     "/mybook/v1.0/system/architecture.html",
			wantBook:  "mybook",
			wantVer:   "v1.0",
			wantPath:  "system/architecture.html",
			expectErr: false,
		},
		{
			input:     "/mybook/latest/index.html",
			wantBook:  "mybook",
			wantVer:   "latest",
			wantPath:  "index.html",
			expectErr: false,
		},
		{
			input:     "http://localhost:8888/mybook/v2.0/page.html",
			wantBook:  "mybook",
			wantVer:   "v2.0",
			wantPath:  "page.html",
			expectErr: false,
		},
		{
			input:     "/mybook/v1.0/",
			wantBook:  "mybook",
			wantVer:   "v1.0",
			wantPath:  "index.html",
			expectErr: false,
		},
		{
			input:     "/mybook/v1.0",
			wantBook:  "mybook",
			wantVer:   "v1.0",
			wantPath:  "index.html",
			expectErr: false,
		},
		{
			input:     "/_/login",
			expectErr: true,
		},
		{
			input:     "/api/v1/comments",
			expectErr: true,
		},
		{
			input:     "",
			expectErr: true,
		},
		{
			input:     "../../etc/passwd",
			expectErr: true,
		},
		{
			input:     "/mybook/../../etc/passwd",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		book, ver, innerPath, err := ParsePageParam(tt.input)
		if tt.expectErr {
			if err == nil {
				t.Errorf("ParsePageParam(%q) expected error, got nil", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParsePageParam(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if book != tt.wantBook || ver != tt.wantVer || innerPath != tt.wantPath {
			t.Errorf("ParsePageParam(%q) = (%q, %q, %q), want (%q, %q, %q)",
				tt.input, book, ver, innerPath, tt.wantBook, tt.wantVer, tt.wantPath)
		}
	}
}

func TestHashPagePath(t *testing.T) {
	h1 := HashPagePath("system/architecture.html")
	h2 := HashPagePath("/System/Architecture.html/")
	if h1 != h2 {
		t.Errorf("HashPagePath should be case and slash-insensitive, got %s and %s", h1, h2)
	}
}

func TestJSONFileCommentStore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "zipserver-comments-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := NewJSONFileCommentStore(tempDir, "version")
	if err != nil {
		t.Fatalf("failed to create JSONFileCommentStore: %v", err)
	}

	ctx := context.Background()
	book := "testbook"
	pagePath := "index.html"

	// 1. Initially empty on v1.0
	annos, err := store.GetComments(ctx, book, "v1.0", pagePath)
	if err != nil {
		t.Fatalf("GetComments failed: %v", err)
	}
	if len(annos) != 0 {
		t.Errorf("expected 0 comments, got %d", len(annos))
	}

	// 2. Create parent comment on v1.0
	parentAnno := &Annotation{
		Creator: Creator{ID: "usr_alex", Name: "Alex Mercer"},
		Body:    TextualBody{Type: "TextualBody", Value: "Parent comment text"},
		Target:  Target{Object: &TargetObject{Source: "/testbook/v1.0/index.html"}},
	}
	createdParent, err := store.CreateComment(ctx, book, "v1.0", pagePath, parentAnno)
	if err != nil {
		t.Fatalf("CreateComment failed: %v", err)
	}
	if createdParent.ID == "" {
		t.Error("expected generated ID, got empty")
	}

	// 3. Verify in index
	b, v, p, found := store.GetPageForComment(createdParent.ID)
	if !found || b != book || v != "v1.0" || p != pagePath {
		t.Errorf("GetPageForComment failed: found=%t, book=%q, version=%q, pagePath=%q", found, b, v, p)
	}

	// 4. Create reply on v1.0
	replyAnno := &Annotation{
		Creator:    Creator{ID: "usr_elena", Name: "Elena"},
		Body:       TextualBody{Type: "TextualBody", Value: "Reply text"},
		Target:     Target{String: createdParent.ID},
		Motivation: "replying",
	}
	createdReply, err := store.CreateComment(ctx, book, "v1.0", pagePath, replyAnno)
	if err != nil {
		t.Fatalf("CreateComment reply failed: %v", err)
	}

	// 5. Retrieve comments on v1.0
	annos, err = store.GetComments(ctx, book, "v1.0", pagePath)
	if err != nil {
		t.Fatalf("GetComments failed: %v", err)
	}
	if len(annos) != 2 {
		t.Errorf("expected 2 comments, got %d", len(annos))
	}

	// 6. Verify version isolation: comments on v2.0 should be empty
	annosV2, err := store.GetComments(ctx, book, "v2.0", pagePath)
	if err != nil {
		t.Fatalf("GetComments v2.0 failed: %v", err)
	}
	if len(annosV2) != 0 {
		t.Errorf("expected 0 comments on v2.0, got %d", len(annosV2))
	}

	// Create a comment on v2.0
	v2Anno := &Annotation{
		Creator: Creator{ID: "usr_alex", Name: "Alex Mercer"},
		Body:    TextualBody{Type: "TextualBody", Value: "V2 comment"},
		Target:  Target{Object: &TargetObject{Source: "/testbook/v2.0/index.html"}},
	}
	_, err = store.CreateComment(ctx, book, "v2.0", pagePath, v2Anno)
	if err != nil {
		t.Fatalf("CreateComment v2.0 failed: %v", err)
	}

	// Retrieve comments on v2.0 again
	annosV2, err = store.GetComments(ctx, book, "v2.0", pagePath)
	if err != nil {
		t.Fatalf("GetComments v2.0 failed: %v", err)
	}
	if len(annosV2) != 1 {
		t.Errorf("expected 1 comment on v2.0, got %d", len(annosV2))
	}

	// 7. Update comment on v1.0 (resolve/reopen or edit body)
	updated, err := store.UpdateComment(ctx, book, createdParent.ID, func(anno *Annotation) error {
		resolved := true
		anno.Resolved = &resolved
		return nil
	})
	if err != nil {
		t.Fatalf("UpdateComment failed: %v", err)
	}
	if updated.Resolved == nil || !*updated.Resolved {
		t.Error("expected comment to be resolved")
	}

	// 8. Cascading delete on v1.0
	err = store.DeleteComment(ctx, book, createdParent.ID, "usr_alex")
	if err != nil {
		t.Fatalf("DeleteComment failed: %v", err)
	}

	// Retrieve again, both parent and reply should be deleted on v1.0
	annos, err = store.GetComments(ctx, book, "v1.0", pagePath)
	if err != nil {
		t.Fatalf("GetComments after delete failed: %v", err)
	}
	if len(annos) != 0 {
		t.Errorf("expected 0 comments after cascading delete, got %d", len(annos))
	}

	// Verify ID is removed from index
	_, _, _, found = store.GetPageForComment(createdParent.ID)
	if found {
		t.Error("parent comment ID still in index after delete")
	}
	_, _, _, found = store.GetPageForComment(createdReply.ID)
	if found {
		t.Error("reply comment ID still in index after delete")
	}
}

func TestJSONFileCommentStoreBookScope(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "zipserver-comments-book-scope-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := NewJSONFileCommentStore(tempDir, "book")
	if err != nil {
		t.Fatalf("failed to create JSONFileCommentStore: %v", err)
	}

	ctx := context.Background()
	book := "testbook"
	pagePath := "index.html"

	// Create comment under v1.0
	anno := &Annotation{
		Creator: Creator{ID: "usr_alex", Name: "Alex"},
		Body:    TextualBody{Value: "Hello under v1.0"},
	}
	_, err = store.CreateComment(ctx, book, "v1.0", pagePath, anno)
	if err != nil {
		t.Fatalf("CreateComment failed: %v", err)
	}

	// Retrieve comments on v2.0 - should find the comment!
	annos, err := store.GetComments(ctx, book, "v2.0", pagePath)
	if err != nil {
		t.Fatalf("GetComments failed: %v", err)
	}
	if len(annos) != 1 {
		t.Errorf("expected 1 comment under v2.0 (book scope), got %d", len(annos))
	} else if annos[0].Body.Value != "Hello under v1.0" {
		t.Errorf("expected comment body 'Hello under v1.0', got %q", annos[0].Body.Value)
	}
}

