package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"zipserver/internal/auth"
	"zipserver/internal/comments"
	"zipserver/internal/config"
)

func TestCommentsAPIHandlers(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "zipserver-handlers-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := comments.NewJSONFileCommentStore(tempDir)
	if err != nil {
		t.Fatalf("failed to create comment store: %v", err)
	}

	// Server initialized with nil Authenticator triggers dev/mock auth mode
	srv, err := NewServer(nil, nil, store, nil)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ctx := context.Background()

	// 1. Get comments on empty page
	req := httptest.NewRequest("GET", "/_/api/v1/comments?page=/book1/v1.0/intro.html", nil)
	w := httptest.NewRecorder()
	srv.HandleGetComments(w, req.WithContext(ctx))

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", w.Code)
	}
	var annos []comments.Annotation
	if err := json.Unmarshal(w.Body.Bytes(), &annos); err != nil {
		t.Fatalf("failed to unmarshal comments: %v", err)
	}
	if len(annos) != 0 {
		t.Errorf("expected 0 comments, got %d", len(annos))
	}

	// 2. Create comment
	payload := `{
		"body": {
			"value": "Test comment content"
		},
		"target": {
			"source": "/book1/v1.0/intro.html",
			"selector": [
				{
					"type": "TextQuoteSelector",
					"exact": "hello"
				}
			]
		}
	}`
	req = httptest.NewRequest("POST", "/_/api/v1/comments", strings.NewReader(payload))
	w = httptest.NewRecorder()
	srv.HandleCreateComment(w, req.WithContext(ctx))

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201 Created, got %d", w.Code)
	}

	var created comments.Annotation
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to unmarshal created comment: %v", err)
	}

	if created.ID == "" || created.Creator.ID != "usr_dev_123" {
		t.Errorf("unexpected created annotation: %+v", created)
	}

	// 3. Create reply
	replyPayload := `{
		"body": {
			"value": "Test reply"
		},
		"target": "` + created.ID + `",
		"motivation": "replying"
	}`
	req = httptest.NewRequest("POST", "/_/api/v1/comments", strings.NewReader(replyPayload))
	w = httptest.NewRecorder()
	srv.HandleCreateComment(w, req.WithContext(ctx))

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201 Created for reply, got %d", w.Code)
	}

	var replyCreated comments.Annotation
	if err := json.Unmarshal(w.Body.Bytes(), &replyCreated); err != nil {
		t.Fatalf("failed to unmarshal reply comment: %v", err)
	}

	if replyCreated.Target.String != created.ID || replyCreated.Motivation != "replying" {
		t.Errorf("unexpected reply comment fields: %+v", replyCreated)
	}

	// 4. Get comments again
	req = httptest.NewRequest("GET", "/_/api/v1/comments?page=/book1/v1.0/intro.html", nil)
	w = httptest.NewRecorder()
	srv.HandleGetComments(w, req.WithContext(ctx))

	if err := json.Unmarshal(w.Body.Bytes(), &annos); err != nil {
		t.Fatalf("failed to unmarshal comments: %v", err)
	}
	if len(annos) != 2 {
		t.Errorf("expected 2 comments (1 parent, 1 reply), got %d", len(annos))
	}

	// 5. Update comment (resolve state)
	resolvePayload := `{"resolved": true}`
	req = httptest.NewRequest("PATCH", "/_/api/v1/comments/"+created.ID, strings.NewReader(resolvePayload))
	// Set wildcard path value since we handle it in handler or router
	req.SetPathValue("id", created.ID)
	w = httptest.NewRecorder()
	srv.HandleUpdateComment(w, req.WithContext(ctx))

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 OK for patch resolve, got %d. Body: %s", w.Code, w.Body.String())
	}

	var updated comments.Annotation
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatalf("failed to unmarshal updated annotation: %v", err)
	}

	if updated.Resolved == nil || !*updated.Resolved {
		t.Error("expected annotation to be resolved")
	}

	// 6. Delete comment
	req = httptest.NewRequest("DELETE", "/_/api/v1/comments/"+created.ID, nil)
	req.SetPathValue("id", created.ID)
	w = httptest.NewRecorder()
	srv.HandleDeleteComment(w, req.WithContext(ctx))

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204 No Content for delete, got %d", w.Code)
	}

	// Get comments should be empty now
	req = httptest.NewRequest("GET", "/_/api/v1/comments?page=/book1/v1.0/intro.html", nil)
	w = httptest.NewRecorder()
	srv.HandleGetComments(w, req.WithContext(ctx))

	if err := json.Unmarshal(w.Body.Bytes(), &annos); err != nil {
		t.Fatalf("failed to unmarshal comments after delete: %v", err)
	}
	if len(annos) != 0 {
		t.Errorf("expected 0 comments after cascading delete, got %d", len(annos))
	}
}

func TestCommentsAPIAuthentication(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "zipserver-auth-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := comments.NewJSONFileCommentStore(tempDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	authCfg := &config.AuthConfig{
		Enabled:    true,
		SessionKey: "dummy-key-dummy-key-dummy-key-dummy-key",
	}
	authenticator, err := auth.NewAuthenticator(authCfg)
	if err != nil {
		t.Fatalf("failed to create authenticator: %v", err)
	}

	srv, err := NewServer(nil, nil, store, authenticator)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ctx := context.Background()

	// 1. GET Comments without authentication -> should be 401
	req := httptest.NewRequest("GET", "/_/api/v1/comments?page=/book1/v1.0/intro.html", nil)
	w := httptest.NewRecorder()
	srv.HandleGetComments(w, req.WithContext(ctx))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for GET comments, got %d", w.Code)
	}
	if strings.Contains(w.Header().Get("Location"), "/_/login") {
		t.Error("GET comments should return error directly, not redirect to login flow")
	}

	// 2. POST Comments without authentication -> should be 401
	payload := `{"body": {"value": "test"}, "target": {"source": "/book1/v1.0/intro.html"}}`
	req = httptest.NewRequest("POST", "/_/api/v1/comments", strings.NewReader(payload))
	w = httptest.NewRecorder()
	srv.HandleCreateComment(w, req.WithContext(ctx))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for POST comment, got %d", w.Code)
	}
	if strings.Contains(w.Header().Get("Location"), "/_/login") {
		t.Error("POST comments should return error directly, not redirect to login flow")
	}
}
