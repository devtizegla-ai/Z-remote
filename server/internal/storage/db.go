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
	"sort"
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

	if _, err := s.DB.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			applied_at DATETIME NOT NULL
		)`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	migrationNames := make([]string, 0, len(files))
	for _, entry := range files {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		migrationNames = append(migrationNames, entry.Name())
	}
	sort.Strings(migrationNames)

	for _, migrationName := range migrationNames {
		var appliedCount int
		if err := s.DB.QueryRowContext(
			ctx,
			`SELECT COUNT(1) FROM schema_migrations WHERE name = ?`,
			migrationName,
		).Scan(&appliedCount); err != nil {
			return fmt.Errorf("check migration %s: %w", migrationName, err)
		}
		if appliedCount > 0 {
			continue
		}

		path := filepath.Join(migrationsDir, migrationName)
		sqlBytes, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", migrationName, err)
		}

		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", migrationName, err)
		}

		if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("exec migration %s: %w", migrationName, err)
		}

		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`,
			migrationName,
			NowUTC(),
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", migrationName, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", migrationName, err)
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
