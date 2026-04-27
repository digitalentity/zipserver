package main

import (
	"context"
	"crypto/subtle"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"zipserver/internal/auth"
	"zipserver/internal/config"
	"zipserver/internal/server"
	"zipserver/internal/storage"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err, "path", *configPath)
		os.Exit(1)
	}

	cacheTTL, err := time.ParseDuration(cfg.Cache.TTL)
	if err != nil {
		slog.Error("invalid cache.ttl", "error", err, "value", cfg.Cache.TTL)
		os.Exit(1)
	}

	var storageBackend storage.Storage
	switch cfg.StorageType {
	case "gcs":
		gcs, err := storage.NewGCSStorage(context.Background(), cfg.GCS.Bucket, cfg.GCS.CredentialsFile)
		if err != nil {
			slog.Error("failed to initialize GCS storage", "error", err, "bucket", cfg.GCS.Bucket)
			os.Exit(1)
		}
		storageBackend, err = storage.NewCachingStorage(gcs, cfg.Cache.Dir, cacheTTL)
		if err != nil {
			slog.Error("failed to initialize GCS cache", "error", err, "dir", cfg.Cache.Dir)
			os.Exit(1)
		}
	case "drive":
		drive, err := storage.NewDriveStorage(context.Background(), cfg.Drive.FolderID, cfg.Drive.CredentialsFile)
		if err != nil {
			slog.Error("failed to initialize Google Drive storage", "error", err, "folder_id", cfg.Drive.FolderID)
			os.Exit(1)
		}
		storageBackend, err = storage.NewCachingStorage(drive, cfg.Cache.Dir, cacheTTL)
		if err != nil {
			slog.Error("failed to initialize Google Drive cache", "error", err, "dir", cfg.Cache.Dir)
			os.Exit(1)
		}

	default: // "local"
		if _, err := os.Stat(cfg.ZipDir); os.IsNotExist(err) {
			slog.Error("zip directory does not exist", "path", cfg.ZipDir)
			os.Exit(1)
		}
		storageBackend = storage.NewLocalStorage(cfg.ZipDir)
	}

	authenticator, err := auth.NewAuthenticator(&cfg.Auth)
	if err != nil {
		slog.Error("failed to initialize authenticator", "error", err)
		os.Exit(1)
	}

	srv, err := server.NewServer(cfg, storageBackend)
	if err != nil {
		slog.Error("failed to initialize server", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()

	if authenticator != nil {
		mux.HandleFunc("/_/login", authenticator.HandleLogin)
		mux.HandleFunc("/_/callback", authenticator.HandleCallback)
		mux.Handle("/", authenticator.AuthMiddleware(http.HandlerFunc(srv.HandleIndex)))
	} else {
		mux.HandleFunc("/", srv.HandleIndex)
	}

	if cfg.Upload.Enabled {
		if cfg.Upload.Token == "" {
			slog.Error("upload.token must not be empty when upload is enabled")
			os.Exit(1)
		}
		mux.Handle("/_/upload", uploadMiddleware(cfg.Upload.Token, http.HandlerFunc(srv.HandleUpload)))
	}

	handler := server.Logger(mux)

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20, // 1 MB
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("starting server", "port", cfg.Port, "zip_dir", cfg.ZipDir)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("shutdown failed", "error", err)
	}
	if err := storageBackend.Close(); err != nil {
		slog.Error("storage close failed", "error", err)
	}
}

func uploadMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Unauthorized: Missing or invalid Authorization header", http.StatusUnauthorized)
			return
		}

		providedToken := strings.TrimPrefix(authHeader, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(providedToken), []byte(token)) != 1 {
			http.Error(w, "Unauthorized: Invalid token", http.StatusUnauthorized)
			return
		}

		const maxUploadSize = 512 << 20 // 512 MB
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
		next.ServeHTTP(w, r)
	})
}
