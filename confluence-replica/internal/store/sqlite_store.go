package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

type SQLiteStore struct {
	db      *sql.DB
	path    string
	profile IndexProfile
}

func NewSQLiteStore(ctx context.Context, path string, profile IndexProfile) (*SQLiteStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("database path is required")
	}
	if profile.SchemaVersion <= 0 {
		return nil, errors.New("schema version must be positive")
	}
	if profile.EmbeddingDimension <= 0 {
		return nil, errors.New("embedding dimension must be positive")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	sqliteAutoOnce.Do(sqlite_vec.Auto)

	db, err := sql.Open("sqlite3", sqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := configureSQLite(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := bootstrapSchema(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureVectorTable(ctx, db, profile); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureIndexProfile(ctx, db, profile); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &SQLiteStore{
		db:      db,
		path:    path,
		profile: profile,
	}, nil
}

func (s *SQLiteStore) Close() {
	if s == nil || s.db == nil {
		return
	}
	_ = s.db.Close()
}

func (s *SQLiteStore) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *SQLiteStore) IndexProfile(ctx context.Context) (IndexProfile, error) {
	return readIndexProfile(ctx, s.db)
}
