package auth

import (
	"context"
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
		Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email"},
		Endpoint:     google.Endpoint,
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

		if auth, ok := session.Values["authenticated"].(bool); !ok || !auth {
			// Save the target URL path and query to redirect back after login
			session.Values["next"] = r.RequestURI
			
			session.Options.Secure = a.isSecure(r)
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

	var user struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		slog.Error("failed to decode user info", "error", err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	if !a.IsUserAllowed(user.Email) {
		slog.Warn("forbidden: user not in allowed list", "email", user.Email)
		http.Error(w, "Forbidden: User "+user.Email+" is not in the allowed list", http.StatusForbidden)
		return
	}

	session, err := a.store.Get(r, sessionName)
	if err != nil {
		slog.Warn("failed to get session in callback", "error", err)
	}
	session.Values["authenticated"] = true
	session.Values["email"] = user.Email

	slog.Info("user authenticated", "email", user.Email)

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
