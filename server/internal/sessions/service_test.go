package sessions

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"remoteaccess/server/internal/devices"
	"remoteaccess/server/internal/storage"
)

func TestRequestAllowsCrossAccountTarget(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	devicesService := devices.NewService(store)
	service := NewService(store, devicesService, time.Hour)

	now := time.Now().UTC()
	_, err := store.DB.ExecContext(ctx, `
		INSERT INTO users (id, name, email, password_hash, created_at, updated_at) VALUES
		('usr_a', 'User A', 'a@example.com', 'x', ?, ?),
		('usr_b', 'User B', 'b@example.com', 'x', ?, ?)`,
		now, now, now, now,
	)
	if err != nil {
		t.Fatalf("insert users: %v", err)
	}

	_, err = store.DB.ExecContext(ctx, `
		INSERT INTO devices (id, user_id, device_name, machine_name, mac_address, platform, app_version, status, device_key_hash, last_seen_at, created_at, updated_at) VALUES
		('dev_a', 'usr_a', 'A', 'HOST-A', '', 'windows', '0.1.0', 'online', 'k', ?, ?, ?),
		('dev_b', 'usr_b', 'B', 'HOST-B', '', 'windows', '0.1.0', 'online', 'k', ?, ?, ?)`,
		now, now, now, now, now, now,
	)
	if err != nil {
		t.Fatalf("insert devices: %v", err)
	}

	request, err := service.Request(ctx, "usr_a", "dev_a", "dev_b")
	if err != nil {
		t.Fatalf("request should succeed across accounts: %v", err)
	}
	if request.TargetDeviceID != "dev_b" {
		t.Fatalf("unexpected target device: %s", request.TargetDeviceID)
	}
}

func newTestStore(t *testing.T) (*storage.Store, func()) {
	t.Helper()

	tempDir := t.TempDir()
	store, err := storage.Open("sqlite", filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	_, err = store.DB.Exec(`
		CREATE TABLE users (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);
		CREATE TABLE devices (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			device_name TEXT NOT NULL,
			machine_name TEXT NOT NULL DEFAULT '',
			mac_address TEXT NOT NULL DEFAULT '',
			platform TEXT NOT NULL,
			app_version TEXT NOT NULL,
			status TEXT NOT NULL,
			device_key_hash TEXT NOT NULL DEFAULT '',
			last_seen_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);
		CREATE TABLE session_requests (
			id TEXT PRIMARY KEY,
			requester_device_id TEXT NOT NULL,
			target_device_id TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			responded_at DATETIME
		);
		CREATE TABLE audit_logs (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			device_id TEXT,
			action TEXT NOT NULL,
			metadata_json TEXT NOT NULL,
			created_at DATETIME NOT NULL
		);`)
	if err != nil {
		_ = store.Close()
		t.Fatalf("create test tables: %v", err)
	}

	return store, func() { _ = store.Close() }
}
