package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"remoteaccess/server/internal/auth"
	"remoteaccess/server/internal/config"
	"remoteaccess/server/internal/storage"
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

	ctx := context.Background()
	email := "dev@example.com"
	password := "dev123456"

	var existingID string
	err = store.DB.QueryRowContext(ctx, `SELECT id FROM users WHERE email = ?`, email).Scan(&existingID)
	if err == nil {
		fmt.Printf("Seed already exists. user_id=%s email=%s\n", existingID, email)
		return
	}
	if err != nil && err != sql.ErrNoRows {
		log.Fatalf("failed checking user: %v", err)
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		log.Fatalf("failed hashing password: %v", err)
	}

	now := storage.NowUTC()
	userID := storage.NewID("usr")
	device1 := storage.NewID("dev")
	device2 := storage.NewID("dev")

	_, err = store.DB.ExecContext(ctx, `
		INSERT INTO users (id, name, email, password_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		userID,
		"Dev User",
		email,
		hash,
		now,
		now,
	)
	if err != nil {
		log.Fatalf("failed inserting user: %v", err)
	}

	_, err = store.DB.ExecContext(ctx, `
		INSERT INTO devices (id, user_id, device_name, platform, app_version, status, last_seen_at, created_at, updated_at)
		VALUES
		  (?, ?, 'Dev Laptop', 'windows', '0.1.0', 'online', ?, ?, ?),
		  (?, ?, 'Dev VM', 'linux', '0.1.0', 'online', ?, ?, ?)`,
		device1, userID, now, now, now,
		device2, userID, now, now, now,
	)
	if err != nil {
		log.Fatalf("failed inserting devices: %v", err)
	}

	fmt.Printf("Seed created successfully\nEmail: %s\nPassword: %s\nDevice1: %s\nDevice2: %s\n", email, password, device1, device2)
}
