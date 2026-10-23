package database

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

type MigrationDiagnostic struct {
	Version int64  `json:"version"`
	State   string `json:"state"`
}

// InspectMigrations never repairs or applies migrations. It compares the ledger
// to the exact SQL embedded in this executable, including missing/unknown versions.
func InspectMigrations(ctx context.Context, tx pgx.Tx) ([]MigrationDiagnostic, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}
	expected := map[int64]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version, err := strconv.ParseInt(strings.SplitN(entry.Name(), "_", 2)[0], 10, 64)
		if err != nil {
			return nil, err
		}
		data, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, err
		}
		expected[version] = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	rows, err := tx.Query(ctx, `SELECT version,checksum FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	applied := map[int64]string{}
	for rows.Next() {
		var version int64
		var checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			return nil, err
		}
		applied[version] = checksum
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []MigrationDiagnostic{}
	for version, checksum := range expected {
		state := "pending"
		if actual, ok := applied[version]; ok {
			state = "applied"
			if actual != checksum {
				state = "mismatch"
			}
		}
		out = append(out, MigrationDiagnostic{version, state})
	}
	for version := range applied {
		if _, ok := expected[version]; !ok {
			out = append(out, MigrationDiagnostic{version, "unknown"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}
