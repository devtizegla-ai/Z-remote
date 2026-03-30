package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"remoteaccess/server/internal/auth"
	"remoteaccess/server/internal/config"
	"remoteaccess/server/internal/devices"
	"remoteaccess/server/internal/files"
	apphttp "remoteaccess/server/internal/http"
	"remoteaccess/server/internal/sessions"
	"remoteaccess/server/internal/storage"
	"remoteaccess/server/internal/ws"
)

func main() {
	if err := config.LoadDotEnv(".env"); err != nil {
		log.Fatalf("failed loading .env: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	store, err := storage.Open(cfg.DatabaseDriver, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db open error: %v", err)
	}
	defer store.Close()

	if err := os.MkdirAll(cfg.FileStorageDir, 0o755); err != nil {
		log.Fatalf("failed creating file storage dir: %v", err)
	}

	migrationsDir := filepath.Join("migrations")
	migrationCtx, migrationCancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer migrationCancel()
	if err := store.ExecMigrations(migrationCtx, migrationsDir); err != nil {
		log.Fatalf("migration error: %v", err)
	}

	tokenManager := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTIssuer, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	authService := auth.NewService(store, tokenManager)
	devicesService := devices.NewService(store)
	sessionsService := sessions.NewService(store, devicesService, cfg.SessionTokenTTL)
	filesService := files.NewService(store, sessionsService, devicesService, cfg.FileMaxBytes, cfg.FileStorageDir)
	if cfg.SeedDevData {
		if err := ensureDevSeed(context.Background(), store, devicesService); err != nil {
			log.Printf("seed error: %v", err)
		}
	}

	hub := ws.NewHub(tokenManager, devicesService, sessionsService, cfg.CORSAllowedOrigins)
	sessionsService.SetNotifier(hub)
	filesService.SetNotifier(hub)

	authHandler := auth.NewHandler(authService)
	devicesHandler := devices.NewHandler(devicesService)
	sessionsHandler := sessions.NewHandler(sessionsService)
	filesHandler := files.NewHandler(filesService)
	wsHandler := ws.NewHandler(hub)

	loginLimiter := auth.NewRateLimiter(cfg.LoginRateLimit, cfg.LoginRateWindow)
	authMW := auth.Middleware(tokenManager, devicesService)
	router := apphttp.NewRouter(cfg, apphttp.RouteHandlers{
		Health: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apphttp.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
		}),
		AuthRegister:    http.HandlerFunc(authHandler.Register),
		AuthLogin:       loginLimiter.Middleware(http.HandlerFunc(authHandler.Login)),
		Me:              authMW(http.HandlerFunc(authHandler.Me)),
		DevicesRegister: authMW(http.HandlerFunc(devicesHandler.Register)),
		DevicesList:     authMW(http.HandlerFunc(devicesHandler.List)),
		SessionsRequest: authMW(http.HandlerFunc(sessionsHandler.Request)),
		SessionsRespond: authMW(http.HandlerFunc(sessionsHandler.Respond)),
		SessionsStart:   authMW(http.HandlerFunc(sessionsHandler.Start)),
		SessionsList:    authMW(http.HandlerFunc(sessionsHandler.List)),
		FilesUpload:     authMW(http.HandlerFunc(filesHandler.Upload)),
		FilesDownload:   authMW(http.HandlerFunc(filesHandler.Download)),
		WS:              http.HandlerFunc(wsHandler.ServeWS),
	})

	srv := &http.Server{
		Addr:              cfg.ServerAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("server listening on %s", cfg.ServerAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown error: %v", err)
	}
}

func ensureDevSeed(ctx context.Context, store *storage.Store, devicesService *devices.Service) error {
	const seedEmail = "dev@example.com"
	const seedPassword = "dev123456"

	var userID string
	err := store.DB.QueryRowContext(ctx, `SELECT id FROM users WHERE email = ?`, seedEmail).Scan(&userID)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	if err == sql.ErrNoRows {
		userID = storage.NewID("usr")
		hash, hashErr := auth.HashPassword(seedPassword)
		if hashErr != nil {
			return hashErr
		}
		now := storage.NowUTC()
		if _, execErr := store.DB.ExecContext(ctx, `
			INSERT INTO users (id, name, email, password_hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			userID,
			"Dev User",
			seedEmail,
			hash,
			now,
			now,
		); execErr != nil {
			return execErr
		}
	}

	_, _ = devicesService.Register(ctx, devices.RegisterInput{
		DeviceID:   "dev_seed_1",
		UserID:     userID,
		DeviceName: "Seed Laptop",
		Platform:   "windows",
		AppVersion: "0.1.0",
	})
	_, _ = devicesService.Register(ctx, devices.RegisterInput{
		DeviceID:   "dev_seed_2",
		UserID:     userID,
		DeviceName: "Seed VM",
		Platform:   "linux",
		AppVersion: "0.1.0",
	})
	return nil
}
