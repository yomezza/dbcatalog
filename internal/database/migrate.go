package database

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yomezza/test/internal/config"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate applies all pending migrations atomically. A database-wide transaction
// advisory lock serializes initialization, including concurrent first launches.
// Each target schema owns its own immutable version/checksum ledger.
func Migrate(ctx context.Context, pool *pgxpool.Pool, schema string) error {
	c := config.Connection{Host: "local", Port: 5432, Database: "catalog", Username: "catalog", Schema: schema, SSLMode: "require"}
	if err := c.Validate(); err != nil {
		return err
	}
	return migrateFS(ctx, pool, schema, migrationFiles)
}
func migrateFS(ctx context.Context, pool *pgxpool.Pool, schema string, files fs.FS) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()
	// Transaction locks release automatically on error, cancellation and disconnect.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(724194301112)`); err != nil {
		return err
	}
	quoted := pgx.Identifier{schema}.Sanitize()
	var schemaExists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM pg_catalog.pg_namespace WHERE nspname=$1)`, schema).Scan(&schemaExists); err != nil {
		return err
	}
	if !schemaExists {
		if _, err = tx.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+quoted); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `SELECT set_config('search_path',$1,true)`, quoted+", pg_catalog, pg_temp"); err != nil {
		return err
	}
	var ledgerExists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relname='schema_migrations')`, schema).Scan(&ledgerExists); err != nil {
		return err
	}
	if !ledgerExists {
		if _, err = tx.Exec(ctx, `CREATE TABLE schema_migrations (version bigint PRIMARY KEY, name text NOT NULL, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
			return err
		}
	}
	entries, err := fs.ReadDir(files, "migrations")
	if err != nil {
		return err
	}
	type migration struct {
		version  int64
		name     string
		sql      []byte
		checksum string
	}
	var list []migration
	known := map[int64]bool{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		v, err := strconv.ParseInt(strings.SplitN(entry.Name(), "_", 2)[0], 10, 64)
		if err != nil || v <= 0 || known[v] {
			return errors.New("invalid migration version")
		}
		known[v] = true
		data, err := fs.ReadFile(files, "migrations/"+entry.Name())
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		list = append(list, migration{v, entry.Name(), data, hex.EncodeToString(sum[:])})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].version < list[j].version })
	rows, err := tx.Query(ctx, `SELECT version,checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		return err
	}
	applied := map[int64]string{}
	var maxVersion int64
	for rows.Next() {
		var v int64
		var checksum string
		if err = rows.Scan(&v, &checksum); err != nil {
			rows.Close()
			return err
		}
		applied[v] = checksum
		maxVersion = v
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for v := range applied {
		if !known[v] {
			return &MigrationStateError{Version: v, Reason: "newer schema"}
		}
	}
	for _, m := range list {
		if sum, ok := applied[m.version]; ok {
			if sum != m.checksum {
				return &MigrationStateError{Version: m.version, Reason: "checksum mismatch"}
			}
			continue
		}
		if m.version < maxVersion {
			return errors.New("out-of-order migration is not allowed")
		}
		if _, err = tx.Exec(ctx, string(m.sql)); err != nil {
			return fmt.Errorf("migration %d: %w", m.version, err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO schema_migrations(version,name,checksum) VALUES($1,$2,$3)`, m.version, m.name, m.checksum); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
