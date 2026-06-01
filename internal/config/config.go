package config

import (
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type AuthConfig struct {
	Enabled      bool     `yaml:"enabled"`
	ClientID     string   `yaml:"client_id"`
	ClientSecret string   `yaml:"client_secret"`
	RedirectURL  string   `yaml:"redirect_url"`
	AllowedUsers []string `yaml:"allowed_users"`
	SessionKey   string   `yaml:"session_key"`
	CookieSecure *bool    `yaml:"cookie_secure"`
}

type UploadConfig struct {
	Enabled bool   `yaml:"enabled"`
	Token   string `yaml:"token"`
}

type CommentsConfig struct {
	Dir   string `yaml:"dir"`
	Scope string `yaml:"scope"`
}

type SMTPConfig struct {
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	Username   string `yaml:"username"`
	Password   string `yaml:"password"`
	From       string `yaml:"from"`
	TLS        string `yaml:"tls"`         // "none", "starttls", "ssl", or empty (auto)
	SkipVerify bool   `yaml:"skip_verify"` // true to skip TLS certificate verification
	Hello      string `yaml:"hello"`       // HELO/EHLO hostname
}

type NotificationsConfig struct {
	Enabled  bool       `yaml:"enabled"`
	SMTP     SMTPConfig `yaml:"smtp"`
	BaseURL  string     `yaml:"base_url"`
	Watchers []string   `yaml:"watchers"`
}

type Config struct {
	Port          string              `yaml:"port"`
	ZipDir        string              `yaml:"zip_dir"`
	Auth          AuthConfig          `yaml:"auth"`
	Upload        UploadConfig        `yaml:"upload"`
	Comments      CommentsConfig      `yaml:"comments"`
	Notifications NotificationsConfig `yaml:"notifications"`
}

func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cfg Config
	decoder := yaml.NewDecoder(f)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}

	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	if cfg.ZipDir == "" {
		cfg.ZipDir = "./publish"
	}
	if cfg.Comments.Dir == "" {
		cfg.Comments.Dir = "./comments"
	}
	if cfg.Comments.Scope == "" {
		cfg.Comments.Scope = "version"
	}

	if os.Getenv("PORT") != "" {
		cfg.Port = os.Getenv("PORT")
	}
	if os.Getenv("ZIP_DIR") != "" {
		cfg.ZipDir = os.Getenv("ZIP_DIR")
	}
	if os.Getenv("COMMENTS_DIR") != "" {
		cfg.Comments.Dir = os.Getenv("COMMENTS_DIR")
	}
	if os.Getenv("COMMENTS_SCOPE") != "" {
		cfg.Comments.Scope = os.Getenv("COMMENTS_SCOPE")
	}
	if os.Getenv("AUTH_ENABLED") != "" {
		cfg.Auth.Enabled = os.Getenv("AUTH_ENABLED") == "true"
	}
	if os.Getenv("SESSION_KEY") != "" {
		cfg.Auth.SessionKey = os.Getenv("SESSION_KEY")
	}
	if os.Getenv("COOKIE_SECURE") != "" {
		val := os.Getenv("COOKIE_SECURE") == "true"
		cfg.Auth.CookieSecure = &val
	}
	if os.Getenv("UPLOAD_TOKEN") != "" {
		cfg.Upload.Token = os.Getenv("UPLOAD_TOKEN")
	}
	if os.Getenv("SMTP_HOST") != "" {
		cfg.Notifications.SMTP.Host = os.Getenv("SMTP_HOST")
	}
	if os.Getenv("SMTP_PORT") != "" {
		port, err := strconv.Atoi(os.Getenv("SMTP_PORT"))
		if err == nil {
			cfg.Notifications.SMTP.Port = port
		}
	}
	if os.Getenv("SMTP_USERNAME") != "" {
		cfg.Notifications.SMTP.Username = os.Getenv("SMTP_USERNAME")
	}
	if os.Getenv("SMTP_PASSWORD") != "" {
		cfg.Notifications.SMTP.Password = os.Getenv("SMTP_PASSWORD")
	}
	if os.Getenv("SMTP_FROM") != "" {
		cfg.Notifications.SMTP.From = os.Getenv("SMTP_FROM")
	}
	if os.Getenv("SMTP_TLS") != "" {
		cfg.Notifications.SMTP.TLS = os.Getenv("SMTP_TLS")
	}
	if os.Getenv("SMTP_SKIP_VERIFY") != "" {
		cfg.Notifications.SMTP.SkipVerify = os.Getenv("SMTP_SKIP_VERIFY") == "true"
	}
	if os.Getenv("SMTP_HELLO") != "" {
		cfg.Notifications.SMTP.Hello = os.Getenv("SMTP_HELLO")
	}
	if os.Getenv("BASE_URL") != "" {
		cfg.Notifications.BaseURL = os.Getenv("BASE_URL")
	}
	if os.Getenv("NOTIFICATIONS_ENABLED") != "" {
		cfg.Notifications.Enabled = os.Getenv("NOTIFICATIONS_ENABLED") == "true"
	}
	if os.Getenv("SMTP_WATCHERS") != "" {
		var cleaned []string
		for _, w := range strings.Split(os.Getenv("SMTP_WATCHERS"), ",") {
			w = strings.TrimSpace(w)
			if w != "" {
				cleaned = append(cleaned, w)
			}
		}
		cfg.Notifications.Watchers = cleaned
	}

	return &cfg, nil
}
