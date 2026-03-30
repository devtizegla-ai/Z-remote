package devices

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"remoteaccess/server/internal/storage"
)

func TestRegisterRejectsCrossUserDeviceTakeover(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	ctx := context.Background()
	_, err := svc.Register(ctx, RegisterInput{
		DeviceID:    "dev_fixed_1",
		UserID:      "usr_a",
		DeviceName:  "Laptop A",
		MachineName: "HOST-A",
		Platform:    "windows",
		AppVersion:  "0.1.0",
		DeviceKey:   "key-A",
	})
	if err != nil {
		t.Fatalf("first register failed: %v", err)
	}

	_, err = svc.Register(ctx, RegisterInput{
		DeviceID:    "dev_fixed_1",
		UserID:      "usr_b",
		DeviceName:  "Laptop B",
		MachineName: "HOST-B",
		Platform:    "windows",
		AppVersion:  "0.1.0",
		DeviceKey:   "key-B",
	})
	if !errors.Is(err, ErrDeviceOwnershipConflict) {
		t.Fatalf("expected ErrDeviceOwnershipConflict, got: %v", err)
	}
}

func TestRegisterRejectsWrongDeviceKeyForSameUser(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	ctx := context.Background()
	_, err := svc.Register(ctx, RegisterInput{
		DeviceID:    "dev_fixed_2",
		UserID:      "usr_a",
		DeviceName:  "Laptop A",
		MachineName: "HOST-A",
		Platform:    "windows",
		AppVersion:  "0.1.0",
		DeviceKey:   "key-A",
	})
	if err != nil {
		t.Fatalf("first register failed: %v", err)
	}

	_, err = svc.Register(ctx, RegisterInput{
		DeviceID:    "dev_fixed_2",
		UserID:      "usr_a",
		DeviceName:  "Laptop A",
		MachineName: "HOST-A",
		Platform:    "windows",
		AppVersion:  "0.1.0",
		DeviceKey:   "key-other",
	})
	if !errors.Is(err, ErrDeviceAuthFailed) {
		t.Fatalf("expected ErrDeviceAuthFailed, got: %v", err)
	}
}

func newTestService(t *testing.T) (*Service, func()) {
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
			email TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);`)
	if err != nil {
		_ = store.Close()
		t.Fatalf("create users table: %v", err)
	}

	_, err = store.DB.Exec(`
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
		);`)
	if err != nil {
		_ = store.Close()
		t.Fatalf("create table: %v", err)
	}

	return NewService(store), func() {
		_ = store.Close()
	}
}
