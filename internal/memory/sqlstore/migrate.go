package sqlstore

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrationFiles is the frozen registry — append-only, order is causal
// (006 references tables from 004/005's referenced subjects via attest_record,
// 007 references its own consent table which itself FK-chains to a2a_peer).
// Frozen-enum-file: single-owner per dispatch.
var migrationFiles = []string{
	"2026-06-22-001-voice.sql",
	"2026-06-22-002-vision.sql",
	"2026-06-22-003-learn.sql",
	"2026-06-22-004-budget.sql",
	"2026-06-22-005-attest.sql",
	"2026-06-22-006-sync.sql",
	"2026-06-22-007-a2a.sql",
	"2026-06-22-008-plugin.sql",
	"2026-06-22-009-supervisor.sql",
}

const migrationLedgerSQL = `CREATE TABLE IF NOT EXISTS schema_migration (
  name       TEXT PRIMARY KEY,
  applied_at INTEGER NOT NULL
);`

// MigrateUp applies every registered migration not yet recorded in
// schema_migration. Each migration runs in a single transaction; the ledger
// row commits with the DDL so a partial apply is impossible.
func MigrateUp(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, migrationLedgerSQL); err != nil {
		return fmt.Errorf("bootstrap schema_migration: %w", err)
	}
	for _, name := range migrationFiles {
		var applied int
		if err := db.QueryRowContext(ctx,
			`SELECT count(*) FROM schema_migration WHERE name=?`, name).Scan(&applied); err != nil {
			return fmt.Errorf("check %s: %w", name, err)
		}
		if applied > 0 {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migration(name, applied_at) VALUES(?, strftime('%s','now'))`, name); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("ledger %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit %s: %w", name, err)
		}
	}
	return nil
}
