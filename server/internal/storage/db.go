package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	DB *sql.DB
}

func Open(driver, dsn string) (*Store, error) {
	driver = strings.ToLower(strings.TrimSpace(driver))
	switch driver {
	case "sqlite", "sqlite3":
		if err := os.MkdirAll(filepath.Dir(dsn), 0o755); err != nil {
			return nil, fmt.Errorf("create sqlite directory: %w", err)
		}
		db, err := sql.Open("sqlite", dsn)
		if err != nil {
			return nil, err
		}
		db.SetMaxOpenConns(1)
		db.SetConnMaxLifetime(5 * time.Minute)
		if err := db.Ping(); err != nil {
			_ = db.Close()
			return nil, err
		}
		return &Store{DB: db}, nil
	case "postgres", "postgresql":
		return nil, errors.New("postgres driver not enabled in MVP build; configure DB_DRIVER=sqlite")
	default:
		return nil, fmt.Errorf("unsupported DB_DRIVER %q", driver)
	}
}

func (s *Store) Close() error {
	if s == nil || s.DB == nil {
		return nil
	}
	return s.DB.Close()
}

func (s *Store) ExecMigrations(ctx context.Context, migrationsDir string) error {
	files, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	for _, entry := range files {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		path := filepath.Join(migrationsDir, entry.Name())
		sqlBytes, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		if _, err := s.DB.ExecContext(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("exec migration %s: %w", entry.Name(), err)
		}
	}

	return nil
}

func NewID(prefix string) string {
	buf := make([]byte, 12)
	_, _ = rand.Read(buf)
	if prefix == "" {
		return hex.EncodeToString(buf)
	}
	return prefix + "_" + hex.EncodeToString(buf)
}

func NowUTC() time.Time {
	return time.Now().UTC()
}

