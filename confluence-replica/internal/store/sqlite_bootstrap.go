package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"sync"
)

//go:embed schema.sql
var schemaFS embed.FS

var sqliteAutoOnce sync.Once

func sqliteDSN(path string) string {
	return fmt.Sprintf("file:%s?_busy_timeout=5000&_foreign_keys=on", path)
}

func configureSQLite(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `PRAGMA journal_mode=WAL;`); err != nil {
		return fmt.Errorf("enable wal: %w", err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys=ON;`); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}
	return nil
}

func bootstrapSchema(ctx context.Context, db *sql.DB) error {
	schemaSQL, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return fmt.Errorf("read embedded schema: %w", err)
	}
	if _, err := db.ExecContext(ctx, string(schemaSQL)); err != nil {
		return fmt.Errorf("apply sqlite schema: %w", err)
	}
	return nil
}

func ensureVectorTable(ctx context.Context, db *sql.DB, profile IndexProfile) error {
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM sqlite_master WHERE type = 'table' AND name = 'chunk_vectors'`).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}
	ddl := fmt.Sprintf(`CREATE VIRTUAL TABLE chunk_vectors USING vec0(embedding float[%d] distance_metric=cosine);`, profile.EmbeddingDimension)
	if _, err := db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("create vec0 table: %w", err)
	}
	return nil
}

func ensureIndexProfile(ctx context.Context, db *sql.DB, profile IndexProfile) error {
	current, err := readIndexProfile(ctx, db)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		_, err := db.ExecContext(ctx, `
			INSERT INTO replica_meta(
				singleton, schema_version, embedding_provider, embedding_model, embedding_dimension, chunking_version, embedding_normalization, created_at, updated_at
			) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?)
		`, profile.SchemaVersion, profile.EmbeddingProvider, profile.EmbeddingModel, profile.EmbeddingDimension, profile.ChunkingVersion, profile.EmbeddingNormalization, currentTimestamp(), currentTimestamp())
		return err
	}
	if current != profile {
		return fmt.Errorf("reindex required: index profile mismatch (have %+v want %+v)", current, profile)
	}
	return nil
}

func readIndexProfile(ctx context.Context, db *sql.DB) (IndexProfile, error) {
	var profile IndexProfile
	err := db.QueryRowContext(ctx, `
		SELECT schema_version, embedding_provider, embedding_model, embedding_dimension, chunking_version, embedding_normalization
		FROM replica_meta
		WHERE singleton = 1
	`).Scan(&profile.SchemaVersion, &profile.EmbeddingProvider, &profile.EmbeddingModel, &profile.EmbeddingDimension, &profile.ChunkingVersion, &profile.EmbeddingNormalization)
	if err != nil {
		return IndexProfile{}, err
	}
	return profile, nil
}
