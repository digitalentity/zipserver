package auth

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gorilla/sessions"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"zipserver/internal/config"
)

const sessionName = "zipserver-session-v2"

type Authenticator struct {
	config       *oauth2.Config
	store        *sessions.CookieStore
	allowedUsers []string
	secure       bool
}

func NewAuthenticator(cfg *config.AuthConfig) (*Authenticator, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	if cfg.SessionKey == "" {
		return nil, fmt.Errorf("auth.session_key is required when auth is enabled")
	}

	// Default to secure if redirect_url is https
	secure := strings.HasPrefix(cfg.RedirectURL, "https://")
	if cfg.CookieSecure != nil {
		secure = *cfg.CookieSecure
	}

	store := sessions.NewCookieStore([]byte(cfg.SessionKey))
	store.Options = &sessions.Options{
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
		MaxAge:   86400 * 30, // 30 days
	}
	oauthConfig := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		Endpoint: google.Endpoint,
	}

	slog.Info("authenticator initialized", "secure_cookies", secure, "redirect_url", cfg.RedirectURL)

	return &Authenticator{
		config:       oauthConfig,
		store:        store,
		allowedUsers: cfg.AllowedUsers,
		secure:       secure,
	}, nil
}

// isSecure returns true if the cookie should be marked as Secure.
func (a *Authenticator) isSecure(r *http.Request) bool {
	if !a.secure {
		return false
	}
	if r.Header.Get("X-Forwarded-Proto") == "https" || r.TLS != nil {
		return true
	}
	return a.secure
}

func (a *Authenticator) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, err := a.store.Get(r, sessionName)
		if err != nil {
			slog.Warn("failed to get session in middleware", "error", err)
		}

		auth, ok := session.Values["authenticated"].(bool)
		userID, _ := session.Values["user_id"].(string)
		if !ok || !auth || userID == "" {
			// Save the target URL path and query to redirect back after login
			session.Values = make(map[any]any)
			session.Values["next"] = r.RequestURI
			
			session.Options.Secure = a.isSecure(r)
			session.Options.MaxAge = -1 // expire immediately to force re-login
			if err := session.Save(r, w); err != nil {
				slog.Error("failed to save session in middleware", "error", err)
			}
			http.Redirect(w, r, "/_/login", http.StatusFound)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (a *Authenticator) HandleLogin(w http.ResponseWriter, r *http.Request) {
	session, err := a.store.Get(r, sessionName)
	if err != nil {
		slog.Warn("failed to get session in login handler", "error", err)
	}

	redirect := r.URL.Query().Get("redirect")
	if redirect != "" {
		session.Values["next"] = redirect
		session.Options.Secure = a.isSecure(r)
		if err := session.Save(r, w); err != nil {
			slog.Error("failed to save session in login handler", "error", err)
		}
	}

	state, err := a.generateState(w, r)
	if err != nil {
		slog.Error("failed to generate oauth state", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	url := a.config.AuthCodeURL(state)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func (a *Authenticator) generateState(w http.ResponseWriter, r *http.Request) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	state := base64.URLEncoding.EncodeToString(b)
	cookie := &http.Cookie{
		Name:     "oauthstate",
		Value:    state,
		MaxAge:   300,
		HttpOnly: true,
		Secure:   a.isSecure(r),
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	}
	http.SetCookie(w, cookie)
	return state, nil
}

func (a *Authenticator) HandleCallback(w http.ResponseWriter, r *http.Request) {
	oauthState, _ := r.Cookie("oauthstate")
	if oauthState == nil || r.FormValue("state") != oauthState.Value {
		slog.Warn("oauth state mismatch or missing cookie")
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	token, err := a.config.Exchange(context.Background(), r.FormValue("code"))
	if err != nil {
		slog.Error("oauth exchange failed", "error", err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	client := a.config.Client(r.Context(), token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		slog.Error("failed to get user info", "error", err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}
	defer resp.Body.Close()

	var googleUser struct {
		ID      string `json:"id"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&googleUser); err != nil {
		slog.Error("failed to decode user info", "error", err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	if !a.IsUserAllowed(googleUser.Email) {
		slog.Warn("forbidden: user not in allowed list", "email", googleUser.Email)
		http.Error(w, "Forbidden: User "+googleUser.Email+" is not in the allowed list", http.StatusForbidden)
		return
	}

	session, err := a.store.Get(r, sessionName)
	if err != nil {
		slog.Warn("failed to get session in callback", "error", err)
	}
	session.Values["authenticated"] = true
	session.Values["email"] = googleUser.Email
	session.Values["user_id"] = "usr_" + googleUser.ID
	session.Values["user_name"] = googleUser.Name
	session.Values["avatar_url"] = googleUser.Picture

	slog.Info("user authenticated", "email", googleUser.Email, "name", googleUser.Name)

	nextURL := "/"
	if val, ok := session.Values["next"].(string); ok && val != "" {
		nextURL = val
		delete(session.Values, "next")
	}

	session.Options.Secure = a.isSecure(r)
	if err := session.Save(r, w); err != nil {
		slog.Error("failed to save session in callback", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	
	slog.Info("redirecting after login", "url", nextURL)
	http.Redirect(w, r, nextURL, http.StatusFound)
}

func (a *Authenticator) IsUserAllowed(email string) bool {
	for _, pattern := range a.allowedUsers {
		if pattern == "*" {
			return true
		}
		if strings.HasPrefix(pattern, "*@") {
			domain := strings.TrimPrefix(pattern, "*@")
			if strings.HasSuffix(email, "@"+domain) {
				return true
			}
		}
		if pattern == email {
			return true
		}
	}
	return false
}

func (a *Authenticator) HandleLogout(w http.ResponseWriter, r *http.Request) {
	session, err := a.store.Get(r, sessionName)
	if err != nil {
		slog.Warn("failed to get session in logout handler", "error", err)
	}

	session.Values = make(map[any]any)
	session.Options.MaxAge = -1 // expire immediately
	session.Options.Secure = a.isSecure(r)
	if err := session.Save(r, w); err != nil {
		slog.Error("failed to save session in logout handler", "error", err)
	}

	redirect := r.URL.Query().Get("redirect")
	if redirect == "" {
		redirect = "/"
	}
	http.Redirect(w, r, redirect, http.StatusFound)
}

type SessionUser struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl"`
}

func (a *Authenticator) GetSessionUser(r *http.Request) (*SessionUser, bool) {
	if a == nil {
		return nil, false
	}
	session, err := a.store.Get(r, sessionName)
	if err != nil {
		return nil, false
	}
	auth, ok := session.Values["authenticated"].(bool)
	if !ok || !auth {
		return nil, false
	}
	id, _ := session.Values["user_id"].(string)
	name, _ := session.Values["user_name"].(string)
	avatar, _ := session.Values["avatar_url"].(string)
	email, _ := session.Values["email"].(string)

	if id == "" && email != "" {
		id = email
	}
	if name == "" && email != "" {
		parts := strings.Split(email, "@")
		name = parts[0]
	}
	if avatar == "" && email != "" {
		emailMD5 := md5.Sum([]byte(strings.ToLower(strings.TrimSpace(email))))
		avatar = fmt.Sprintf("https://www.gravatar.com/avatar/%x?d=mp", emailMD5)
	}

	return &SessionUser{
		ID:        id,
		Name:      name,
		AvatarURL: avatar,
	}, true
}

func (a *Authenticator) HandleAuthMe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, authenticated := a.GetSessionUser(r)
	if !authenticated {
		json.NewEncoder(w).Encode(map[string]any{
			"authenticated": false,
			"user":          nil,
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{
		"authenticated": true,
		"user":          user,
	})
}
