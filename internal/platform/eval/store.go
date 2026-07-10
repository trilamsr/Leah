package eval

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite"

	"github.com/trilam/leah/internal/memory/sqlstore"
)

const evalSchemaVersion = "1"

const evalDDL = `
CREATE TABLE IF NOT EXISTS eval_runs (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  started_at  INTEGER NOT NULL,
  finished_at INTEGER NOT NULL DEFAULT 0,
  source      TEXT NOT NULL DEFAULT '',
  total       INTEGER NOT NULL DEFAULT 0,
  passed      INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS eval_results (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id     INTEGER NOT NULL,
  fixture_id TEXT NOT NULL,
  pass       INTEGER NOT NULL,
  score      REAL NOT NULL,
  actual     TEXT NOT NULL DEFAULT '',
  reason     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS eval_results_run_idx ON eval_results(run_id);
`

type Store struct {
	db *sql.DB
}

type RunRecord struct {
	StartedAt time.Time
	Source    string
	Results   []ResultRecord
}

type ResultRecord struct {
	FixtureID string
	Pass      bool
	Score     float64
	Actual    string
	Reason    string
}

func OpenStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("mkdir state dir: %w", err)
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		f, ferr := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
		if ferr != nil {
			return nil, fmt.Errorf("create db file: %w", ferr)
		}
		_ = f.Close()
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("chmod db file: %w", err)
	}
	db, err := sqlstore.OpenWAL(path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	embedded, _ := strconv.Atoi(evalSchemaVersion)
	if err := sqlstore.EnsureSchemaVersion(s.db, "eval.db", embedded); err != nil {
		return err
	}
	if _, err := s.db.Exec(evalDDL); err != nil {
		return fmt.Errorf("exec ddl: %w", err)
	}
	if _, err := s.db.Exec(`INSERT OR REPLACE INTO schema_meta(key, value) VALUES('version', ?)`, evalSchemaVersion); err != nil {
		return fmt.Errorf("stamp version: %w", err)
	}
	return nil
}

func (s *Store) RecordRun(ctx context.Context, r RunRecord) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	passed := 0
	for _, res := range r.Results {
		if res.Pass {
			passed++
		}
	}
	finished := time.Now().UTC().Unix()
	if r.StartedAt.IsZero() {
		r.StartedAt = time.Now().UTC()
	}
	resInsert, err := tx.ExecContext(ctx,
		`INSERT INTO eval_runs(started_at, finished_at, source, total, passed) VALUES(?,?,?,?,?)`,
		r.StartedAt.Unix(), finished, r.Source, len(r.Results), passed,
	)
	if err != nil {
		return 0, fmt.Errorf("insert run: %w", err)
	}
	runID, err := resInsert.LastInsertId()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO eval_results(run_id, fixture_id, pass, score, actual, reason) VALUES(?,?,?,?,?,?)`)
	if err != nil {
		return 0, err
	}
	defer func() { _ = stmt.Close() }()
	for _, res := range r.Results {
		passInt := 0
		if res.Pass {
			passInt = 1
		}
		if _, err := stmt.ExecContext(ctx, runID, res.FixtureID, passInt, res.Score, res.Actual, res.Reason); err != nil {
			return 0, fmt.Errorf("insert result %s: %w", res.FixtureID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return runID, nil
}

func (s *Store) ListResults(ctx context.Context, runID int64) ([]ResultRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT fixture_id, pass, score, actual, reason FROM eval_results WHERE run_id = ? ORDER BY id`, runID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []ResultRecord
	for rows.Next() {
		var r ResultRecord
		var pass int
		if err := rows.Scan(&r.FixtureID, &pass, &r.Score, &r.Actual, &r.Reason); err != nil {
			return nil, err
		}
		r.Pass = pass != 0
		out = append(out, r)
	}
	return out, rows.Err()
}
