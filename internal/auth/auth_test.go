package auth

import (
	"net/http/httptest"
	"strings"
	"testing"

	"zipserver/internal/config"
)

func TestIsUserAllowed(t *testing.T) {
	tests := []struct {
		name         string
		allowedUsers []string
		email        string
		want         bool
	}{
		{
			name:         "Exact match",
			allowedUsers: []string{"user@example.com"},
			email:        "user@example.com",
			want:         true,
		},
		{
			name:         "Domain wildcard",
			allowedUsers: []string{"*@company.com"},
			email:        "dev@company.com",
			want:         true,
		},
		{
			name:         "Domain wildcard mismatch",
			allowedUsers: []string{"*@company.com"},
			email:        "dev@other.com",
			want:         false,
		},
		{
			name:         "All users",
			allowedUsers: []string{"*"},
			email:        "any@thing.com",
			want:         true,
		},
		{
			name:         "No match",
			allowedUsers: []string{"admin@example.com"},
			email:        "user@example.com",
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Authenticator{allowedUsers: tt.allowedUsers}
			if got := a.IsUserAllowed(tt.email); got != tt.want {
				t.Errorf("IsUserAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetSessionUserFallback(t *testing.T) {
	a, err := NewAuthenticator(&config.AuthConfig{
		Enabled:    true,
		SessionKey: "supersecretkeysupersecretkey1234",
	})
	if err != nil {
		t.Fatalf("failed to create authenticator: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	session, _ := a.store.Get(req, sessionName)
	session.Values["authenticated"] = true
	session.Values["email"] = "alex@example.com"
	if err := session.Save(req, w); err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	// Create a new request with the cookie from the response recorder
	req2 := httptest.NewRequest("GET", "/", nil)
	for _, cookie := range w.Result().Cookies() {
		req2.AddCookie(cookie)
	}

	user, ok := a.GetSessionUser(req2)
	if !ok {
		t.Fatalf("expected GetSessionUser to return ok=true")
	}

	if user.ID != "alex@example.com" {
		t.Errorf("expected generated user ID 'alex@example.com', got %q", user.ID)
	}
	if user.Name != "alex" {
		t.Errorf("expected generated name 'alex', got %q", user.Name)
	}
	if !strings.Contains(user.AvatarURL, "gravatar.com") {
		t.Errorf("expected Gravatar URL, got %q", user.AvatarURL)
	}
}
