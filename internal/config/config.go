package config

import (
	"os"

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

type Config struct {
	Port     string         `yaml:"port"`
	ZipDir   string         `yaml:"zip_dir"`
	Auth     AuthConfig     `yaml:"auth"`
	Upload   UploadConfig   `yaml:"upload"`
	Comments CommentsConfig `yaml:"comments"`
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

	return &cfg, nil
}
