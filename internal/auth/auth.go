package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/sessions"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"zipserver/internal/config"
)

type Authenticator struct {
	config      *oauth2.Config
	store       *sessions.CookieStore
	allowedUsers []string
}

func NewAuthenticator(cfg *config.AuthConfig) (*Authenticator, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	if cfg.SessionKey == "" {
		return nil, fmt.Errorf("auth.session_key is required when auth is enabled")
	}

	store := sessions.NewCookieStore([]byte(cfg.SessionKey))
	oauthConfig := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email"},
		Endpoint:     google.Endpoint,
	}

	return &Authenticator{
		config:      oauthConfig,
		store:       store,
		allowedUsers: cfg.AllowedUsers,
	}, nil
}

func (a *Authenticator) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" || r.URL.Path == "/callback" {
			next.ServeHTTP(w, r)
			return
		}

		session, _ := a.store.Get(r, "docserver-session")
		if auth, ok := session.Values["authenticated"].(bool); !ok || !auth {
			// Save the target URL to redirect back after login
			session.Values["next"] = r.URL.String()
			session.Save(r, w)
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (a *Authenticator) HandleLogin(w http.ResponseWriter, r *http.Request) {
	state := a.generateState(w)
	url := a.config.AuthCodeURL(state)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func (a *Authenticator) generateState(w http.ResponseWriter) string {
	b := make([]byte, 16)
	rand.Read(b)
	state := base64.URLEncoding.EncodeToString(b)
	cookie := &http.Cookie{
		Name:     "oauthstate",
		Value:    state,
		MaxAge:   300,
		HttpOnly: true,
	}
	http.SetCookie(w, cookie)
	return state
}

func (a *Authenticator) HandleCallback(w http.ResponseWriter, r *http.Request) {
	oauthState, _ := r.Cookie("oauthstate")
	if oauthState == nil || r.FormValue("state") != oauthState.Value {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	token, err := a.config.Exchange(context.Background(), r.FormValue("code"))
	if err != nil {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	resp, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + token.AccessToken)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}
	defer resp.Body.Close()

	var user struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	if !a.IsUserAllowed(user.Email) {
		http.Error(w, "Forbidden: User "+user.Email+" is not in the allowed list", http.StatusForbidden)
		return
	}

	session, _ := a.store.Get(r, "docserver-session")
	session.Values["authenticated"] = true
	session.Values["email"] = user.Email

	nextURL := "/"
	if val, ok := session.Values["next"].(string); ok && val != "" {
		nextURL = val
		delete(session.Values, "next")
	}

	session.Save(r, w)
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
