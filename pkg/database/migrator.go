package database

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Migrator struct {
	pool    *pgxpool.Pool
	dirPath string
	fsys    fs.FS
}

func NewMigrator(pool *pgxpool.Pool, migrationsDir string) (*Migrator, error) {
	absPath, err := filepath.Abs(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("invalid migrations path: %w", err)
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("migrations directory not found: %s", absPath)
	}

	return &Migrator{
		pool:    pool,
		dirPath: absPath,
	}, nil
}

func NewFSMigrator(pool *pgxpool.Pool, fsys fs.FS) *Migrator {
	return &Migrator{
		pool: pool,
		fsys: fsys,
	}
}

func (m *Migrator) DropAll(ctx context.Context) error {
	query := `
	DROP SCHEMA IF EXISTS public CASCADE;
	CREATE SCHEMA public;
	GRANT ALL ON SCHEMA public TO postgres;
	GRANT ALL ON SCHEMA public TO public;
	`
	_, err := m.pool.Exec(ctx, query)
	return err
}

func (m *Migrator) EnsureSchemaTable(ctx context.Context) error {
	query := `
	DO $$
	BEGIN
		IF EXISTS (
			SELECT 1 FROM information_schema.columns 
			WHERE table_schema = CURRENT_SCHEMA 
			  AND table_name = 'schema_migrations' 
			  AND column_name = 'version' 
			  AND data_type = 'bigint'
		) THEN
			DROP TABLE schema_migrations;
		END IF;
	END $$;

	CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR(255) PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);`
	_, err := m.pool.Exec(ctx, query)
	return err
}

func (m *Migrator) GetAppliedMigrations(ctx context.Context) (map[string]bool, error) {
	if err := m.EnsureSchemaTable(ctx); err != nil {
		return nil, fmt.Errorf("failed to ensure schema_migrations table: %w", err)
	}

	rows, err := m.pool.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}
	return applied, rows.Err()
}

func (m *Migrator) Run(ctx context.Context) error {
	applied, err := m.GetAppliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch applied migrations: %w", err)
	}

	files, err := m.loadMigrationFiles()
	if err != nil {
		return err
	}

	appliedCount := 0
	for _, f := range files {
		if applied[f.Name] {
			continue
		}

		slog.Info("Applying migration", slog.String("file", f.Name))

		err := m.applyMigration(ctx, f.Name, f.Content)
		if err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", f.Name, err)
		}

		appliedCount++
		slog.Info("Migration applied successfully", slog.String("file", f.Name))
	}

	if appliedCount == 0 {
		slog.Info("Database schema is up to date (no pending migrations)")
	}
	return nil
}

func (m *Migrator) applyMigration(ctx context.Context, version string, sqlContent string) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, sqlContent); err != nil {
		return fmt.Errorf("execution error: %w", err)
	}

	if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
		return fmt.Errorf("failed to record version: %w", err)
	}

	return tx.Commit(ctx)
}

type migrationFile struct {
	Name    string
	Content string
}

func (m *Migrator) loadMigrationFiles() ([]migrationFile, error) {
	var files []migrationFile

	if m.fsys != nil {
		entries, err := fs.ReadDir(m.fsys, ".")
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
				content, err := fs.ReadFile(m.fsys, entry.Name())
				if err != nil {
					return nil, err
				}
				files = append(files, migrationFile{Name: entry.Name(), Content: string(content)})
			}
		}
	} else {
		entries, err := os.ReadDir(m.dirPath)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
				content, err := os.ReadFile(filepath.Join(m.dirPath, entry.Name()))
				if err != nil {
					return nil, err
				}
				files = append(files, migrationFile{Name: entry.Name(), Content: string(content)})
			}
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})

	return files, nil
}
