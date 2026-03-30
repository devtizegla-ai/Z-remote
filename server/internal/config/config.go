package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env                string
	ServerAddr         string
	JWTSecret          string
	JWTIssuer          string
	AccessTokenTTL     time.Duration
	RefreshTokenTTL    time.Duration
	DatabaseDriver     string
	DatabaseURL        string
	CORSAllowedOrigins []string
	LoginRateLimit     int
	LoginRateWindow    time.Duration
	SessionTokenTTL    time.Duration
	FileMaxBytes       int64
	FileStorageDir     string
	SeedDevData        bool
}

func Load() (Config, error) {
	cfg := Config{
		Env:                getEnv("APP_ENV", "development"),
		ServerAddr:         getEnv("SERVER_ADDR", ":8080"),
		JWTSecret:          getEnv("JWT_SECRET", "change-me-in-production"),
		JWTIssuer:          getEnv("JWT_ISSUER", "remote-access-mvp"),
		DatabaseDriver:     getEnv("DB_DRIVER", "sqlite"),
		DatabaseURL:        getEnv("DB_URL", "./data/remote_access.db"),
		CORSAllowedOrigins: splitCSV(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:1420,http://127.0.0.1:1420")),
		FileStorageDir:     getEnv("FILE_STORAGE_DIR", "./data/uploads"),
	}

	accessTTL, err := parseDurationEnv("ACCESS_TOKEN_TTL", "15m")
	if err != nil {
		return cfg, err
	}
	cfg.AccessTokenTTL = accessTTL

	refreshTTL, err := parseDurationEnv("REFRESH_TOKEN_TTL", "168h")
	if err != nil {
		return cfg, err
	}
	cfg.RefreshTokenTTL = refreshTTL

	rateWindow, err := parseDurationEnv("LOGIN_RATE_WINDOW", "1m")
	if err != nil {
		return cfg, err
	}
	cfg.LoginRateWindow = rateWindow

	sessionTokenTTL, err := parseDurationEnv("SESSION_TOKEN_TTL", "2h")
	if err != nil {
		return cfg, err
	}
	cfg.SessionTokenTTL = sessionTokenTTL

	cfg.LoginRateLimit, err = parseIntEnv("LOGIN_RATE_LIMIT", 10)
	if err != nil {
		return cfg, err
	}

	cfg.FileMaxBytes, err = parseInt64Env("FILE_MAX_BYTES", 10485760)
	if err != nil {
		return cfg, err
	}

	cfg.SeedDevData, err = parseBoolEnv("SEED_DEV_DATA", false)
	if err != nil {
		return cfg, err
	}

	if cfg.JWTSecret == "" {
		return cfg, fmt.Errorf("JWT_SECRET cannot be empty")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func parseDurationEnv(key, fallback string) (time.Duration, error) {
	raw := getEnv(key, fallback)
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid duration for %s: %w", key, err)
	}
	return value, nil
}

func parseIntEnv(key string, fallback int) (int, error) {
	raw := getEnv(key, strconv.Itoa(fallback))
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid int for %s: %w", key, err)
	}
	return value, nil
}

func parseInt64Env(key string, fallback int64) (int64, error) {
	raw := getEnv(key, strconv.FormatInt(fallback, 10))
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid int64 for %s: %w", key, err)
	}
	return value, nil
}

func parseBoolEnv(key string, fallback bool) (bool, error) {
	raw := getEnv(key, strconv.FormatBool(fallback))
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("invalid bool for %s: %w", key, err)
	}
	return value, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

