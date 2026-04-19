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
}

type UploadConfig struct {
	Enabled bool   `yaml:"enabled"`
	Token   string `yaml:"token"`
}

type GCSConfig struct {
	Bucket          string `yaml:"bucket"`
	CredentialsFile string `yaml:"credentials_file"`
}

type DriveConfig struct {
	FolderID        string `yaml:"folder_id"`
	CredentialsFile string `yaml:"credentials_file"`
}

type CacheConfig struct {
	Dir string `yaml:"dir"`
	TTL string `yaml:"ttl"`
}

type Config struct {
	Port        string       `yaml:"port"`
	StorageType string       `yaml:"storage_type"` // "local", "gcs", "drive"
	ZipDir      string       `yaml:"zip_dir"`      // for local
	Cache       CacheConfig  `yaml:"cache"`        // for cloud caching
	GCS         GCSConfig    `yaml:"gcs"`
	Drive       DriveConfig  `yaml:"drive"`
	Auth        AuthConfig   `yaml:"auth"`
	Upload      UploadConfig `yaml:"upload"`
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
	if cfg.Cache.Dir == "" {
		cfg.Cache.Dir = "./cache"
	}
	if cfg.Cache.TTL == "" {
		cfg.Cache.TTL = "1m"
	}

	if os.Getenv("UPLOAD_TOKEN") != "" {
		cfg.Upload.Token = os.Getenv("UPLOAD_TOKEN")
	}

	return &cfg, nil
}
