package recommend

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/sqlstore"
)

// sqliteSchemaVersion is the embedded DDL version. Parsed-int compare per PR #58
// avoids the lex "10" < "9" bug; on-disk newer than embedded refuses to migrate.
const sqliteSchemaVersion = "3"

// sqliteDDL provisions the two tables used by SQLiteEngine. Kept inline (no
// embed) because the schema is small and the recommend package owns it end to end.
const sqliteDDL = `
CREATE TABLE IF NOT EXISTS schema_meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS recommendations (
  id          TEXT PRIMARY KEY,
  pattern     TEXT NOT NULL,
  tier        TEXT NOT NULL,
  source      TEXT NOT NULL,
  confidence  REAL NOT NULL,
  created_at  INTEGER NOT NULL,
  accepted_at INTEGER NOT NULL DEFAULT 0,
  applied_at  INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS feedback (
  id      TEXT PRIMARY KEY,
  rec_id  TEXT NOT NULL,
  pattern TEXT NOT NULL DEFAULT '',
  kind    TEXT NOT NULL,
  signal  REAL NOT NULL,
  ts      INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS feedback_pattern_idx ON feedback(pattern);
`

// SQLiteEngine is the persistent twin of MemoryEngine — same Engine surface
// (Propose / Accept / Reject / Apply) but pending and accepted state survive
// process restart. Action closures still live in memory: the spec deliberately
// stores only the row data; per-pattern Actions get reattached at boot via the
// registry (W18+).
type SQLiteEngine struct {
	audit *audit.Logger
	db    *sql.DB

	mu          sync.Mutex
	actions     map[string]Action    // id → in-memory Action; lost on restart
	matchers    []SignalMatcher      // V8: registered for OnSignal fan-out
	lastFiredAt map[string]time.Time // V8: pattern → last OnSignal fire
	now         func() time.Time     // test seam; default time.Now().UTC()

	banditFloor banditFloorCache // W101: lazy ≥20×≥5 gate
}

// NewSQLiteEngine opens (creating if needed) the SQLite file at path with
// 0600 perms and runs schema migration. Idempotent; safe to call against the
// same path from a new process.
func NewSQLiteEngine(path string, logger *audit.Logger) (*SQLiteEngine, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("mkdir state dir: %w", err)
	}
	// Touch with 0600 before sqlite opens — modernc respects existing mode.
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		f, ferr := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
		if ferr != nil {
			return nil, fmt.Errorf("create db file: %w", ferr)
		}
		_ = f.Close()
	}
	// Re-chmod in case file pre-existed with looser perms.
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("chmod db file: %w", err)
	}

	db, err := sqlstore.OpenWAL(path)
	if err != nil {
		return nil, err
	}
	e := &SQLiteEngine{
		audit:       logger,
		db:          db,
		actions:     make(map[string]Action),
		lastFiredAt: make(map[string]time.Time),
		now:         func() time.Time { return time.Now().UTC() },
	}
	if err := e.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return e, nil
}

// Close releases the DB handle. Test harnesses should `defer e.Close()` to
// avoid file-handle leaks on tmpdir cleanup (meta-learn rule #4).
func (e *SQLiteEngine) Close() error { return e.db.Close() }

func (e *SQLiteEngine) migrate() error {
	embedded, err := parseRecommendSchemaVersion(sqliteSchemaVersion)
	if err != nil {
		return fmt.Errorf("parse embedded schema version %q: %w", sqliteSchemaVersion, err)
	}
	if err := sqlstore.EnsureSchemaVersion(e.db, "recommend.db", embedded); err != nil {
		return err
	}
	if _, err := e.db.Exec(sqliteDDL); err != nil {
		return fmt.Errorf("exec schema: %w", err)
	}
	// v1→v2: feedback.pattern landed in W100 for propose-time blending.
	// PRAGMA-based probe avoids re-running ALTER on already-migrated dbs.
	hasPattern, err := columnExists(e.db, "feedback", "pattern")
	if err != nil {
		return fmt.Errorf("probe feedback.pattern: %w", err)
	}
	if !hasPattern {
		if _, err := e.db.Exec(`ALTER TABLE feedback ADD COLUMN pattern TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add feedback.pattern: %w", err)
		}
		if _, err := e.db.Exec(`UPDATE feedback SET pattern = COALESCE((SELECT pattern FROM recommendations WHERE id = feedback.rec_id), '') WHERE pattern = ''`); err != nil {
			return fmt.Errorf("backfill feedback.pattern: %w", err)
		}
	}
	// v2→v3 (W101): per-pattern Beta posterior table for Thompson sampling.
	if err := applyBanditMigration(e.db); err != nil {
		return err
	}
	if _, err := e.db.Exec(`INSERT OR REPLACE INTO schema_meta(key, value) VALUES('version', ?)`, sqliteSchemaVersion); err != nil {
		return fmt.Errorf("stamp schema version: %w", err)
	}
	return nil
}

// parseRecommendSchemaVersion mirrors memory.parseSchemaVersion (PR #58) —
// strconv.Atoi guards against lex "10" < "9" mis-ordering.
func parseRecommendSchemaVersion(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}

func columnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// Seed inserts a Recommendation into recommendations as pending (accepted_at=0).
// The Action closure is held in memory only — restarts drop it and surfaces
// re-bind via the pattern registry.
func (e *SQLiteEngine) Seed(rec Recommendation) error {
	created := rec.CreatedAt.UnixNano()
	if rec.CreatedAt.IsZero() {
		created = time.Now().UTC().UnixNano()
	}
	_, err := e.db.Exec(`
		INSERT INTO recommendations (id, pattern, tier, source, confidence, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET pattern=excluded.pattern, tier=excluded.tier,
		    source=excluded.source, confidence=excluded.confidence`,
		rec.ID, rec.Pattern, string(rec.Tier), rec.Source, rec.Confidence, created,
	)
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	if rec.Action != nil {
		e.mu.Lock()
		e.actions[rec.ID] = rec.Action
		e.mu.Unlock()
	}
	return nil
}

// Propose returns pending (not-yet-accepted) recommendations with persisted
// per-pattern feedback blended into Confidence (W100). When bandit is on AND
// the cold-start floor has cleared (≥20 rows × ≥5 patterns), top-K is taken
// over Thompson-sampled posteriors instead of greedy point-estimate (W101).
func (e *SQLiteEngine) Propose(ctx context.Context) ([]Recommendation, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT id, pattern, tier, source, confidence, created_at
		FROM recommendations WHERE accepted_at = 0`)
	if err != nil {
		return nil, fmt.Errorf("propose query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Recommendation
	patterns := make(map[string]struct{})
	for rows.Next() {
		var rec Recommendation
		var tier string
		var created int64
		if err := rows.Scan(&rec.ID, &rec.Pattern, &tier, &rec.Source, &rec.Confidence, &created); err != nil {
			return nil, fmt.Errorf("propose scan: %w", err)
		}
		rec.Tier = Tier(tier)
		rec.CreatedAt = time.Unix(0, created).UTC()
		e.mu.Lock()
		rec.Action = e.actions[rec.ID]
		e.mu.Unlock()
		patterns[rec.Pattern] = struct{}{}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	deltas, err := e.feedbackDeltas(ctx, patterns)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Confidence += deltas[out[i].Pattern]
	}

	if !banditOn() {
		return greedyRank(out), nil
	}
	cleared, err := e.banditFloor.check(ctx, e.db)
	if err != nil {
		return nil, err
	}
	if !cleared || len(out) == 0 {
		return greedyRank(out), nil
	}

	patternList := make([]string, 0, len(patterns))
	for p := range patterns {
		patternList = append(patternList, p)
	}
	posts, err := loadPosteriors(ctx, e.db, patternList)
	if err != nil {
		return nil, err
	}
	seed := banditSeed()
	rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed)+1))
	draws := rankBandit(out, posts, rng)
	ranked := make([]Recommendation, len(draws))
	for i, d := range draws {
		ranked[i] = d.rec
	}
	logBanditAudit(e.audit, draws, seed)
	return ranked, nil
}

// feedbackDeltas reads denormalized feedback.pattern so the dampener on a
// pattern survives a Reject of the originating rec — pattern, not rec ID,
// is the durable identity per spec §9.
func (e *SQLiteEngine) feedbackDeltas(ctx context.Context, patterns map[string]struct{}) (map[string]float64, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	placeholders := make([]string, 0, len(patterns))
	args := make([]any, 0, len(patterns))
	for p := range patterns {
		placeholders = append(placeholders, "?")
		args = append(args, p)
	}
	q := `SELECT pattern, kind, signal, ts FROM feedback
	      WHERE pattern IN (` + strings.Join(placeholders, ",") + `)`
	rows, err := e.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("feedback blend: %w", err)
	}
	defer func() { _ = rows.Close() }()
	now := time.Now().UTC()
	deltas := make(map[string]float64, len(patterns))
	for rows.Next() {
		var pattern, kind string
		var signal float64
		var ts int64
		if err := rows.Scan(&pattern, &kind, &signal, &ts); err != nil {
			return nil, fmt.Errorf("feedback scan: %w", err)
		}
		hl := feedbackHalflifeDays
		if FeedbackKind(kind) == FeedbackIgnore {
			hl = behavioralHalflifeDays
		}
		deltas[pattern] += decayedMagnitude(signal, time.Unix(0, ts).UTC(), now, hl)
	}
	return deltas, rows.Err()
}

// Get returns the stored row for id plus whether it has been Accept-ed.
// Exposed for tests that assert persistence across instances.
func (e *SQLiteEngine) Get(id string) (Recommendation, bool, error) {
	var rec Recommendation
	var tier string
	var created, accepted int64
	err := e.db.QueryRow(`
		SELECT id, pattern, tier, source, confidence, created_at, accepted_at
		FROM recommendations WHERE id = ?`, id).
		Scan(&rec.ID, &rec.Pattern, &tier, &rec.Source, &rec.Confidence, &created, &accepted)
	if errors.Is(err, sql.ErrNoRows) {
		return Recommendation{}, false, ErrNotFound
	}
	if err != nil {
		return Recommendation{}, false, fmt.Errorf("get: %w", err)
	}
	rec.Tier = Tier(tier)
	rec.CreatedAt = time.Unix(0, created).UTC()
	e.mu.Lock()
	rec.Action = e.actions[rec.ID]
	e.mu.Unlock()
	return rec, accepted != 0, nil
}

// Accept marks the rec accepted and audits the decision.
func (e *SQLiteEngine) Accept(ctx context.Context, id string) error {
	res, err := e.db.ExecContext(ctx, `
		UPDATE recommendations SET accepted_at = ? WHERE id = ? AND accepted_at = 0`,
		time.Now().UTC().UnixNano(), id,
	)
	if err != nil {
		return fmt.Errorf("accept update: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("accept rows: %w", err)
	}
	if n == 0 {
		// Either unknown id or already accepted; mirror MemoryEngine's
		// ErrNotFound for the unknown case, swallow the already-accepted
		// case so retries are idempotent.
		if _, alreadyAccepted, gerr := e.Get(id); gerr != nil {
			return gerr
		} else if !alreadyAccepted {
			return ErrNotFound
		}
		return nil
	}
	rec, _, err := e.Get(id)
	if err != nil {
		return err
	}
	return e.logDecision("recommendation_accept", rec, "")
}

// Reject deletes the rec and audits the decision.
func (e *SQLiteEngine) Reject(ctx context.Context, id string) error {
	rec, _, err := e.Get(id)
	if err != nil {
		return err
	}
	if _, err := e.db.ExecContext(ctx, `DELETE FROM recommendations WHERE id = ?`, id); err != nil {
		return fmt.Errorf("reject delete: %w", err)
	}
	e.mu.Lock()
	delete(e.actions, id)
	e.mu.Unlock()
	return e.logDecision("recommendation_reject", rec, "")
}

// Apply enforces the same tier rubric as MemoryEngine.Apply (spec §6).
// SQLiteEngine reads the accepted-state from the row, not an in-memory map,
// so an Accept from a prior process survives.
func (e *SQLiteEngine) Apply(ctx context.Context, rec Recommendation) error {
	switch rec.Tier {
	case TierBlocked:
		_ = e.logDecision("recommendation_apply", rec, "blocked")
		return ErrBlocked
	case TierSilent:
		return e.logDecision("recommendation_apply", rec, "silent")
	case TierConfirm:
		_, accepted, err := e.Get(rec.ID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if !accepted {
			return ErrConfirmRequired
		}
	case TierAuto:
		// fallthrough to invoke Action below.
	}
	action := rec.Action
	if action == nil {
		e.mu.Lock()
		action = e.actions[rec.ID]
		e.mu.Unlock()
	}
	if action != nil {
		if err := action(ctx); err != nil {
			_ = e.logDecision("recommendation_apply", rec, "failed")
			return err
		}
	}
	if _, err := e.db.ExecContext(ctx,
		`UPDATE recommendations SET applied_at = ? WHERE id = ?`,
		time.Now().UTC().UnixNano(), rec.ID,
	); err != nil {
		return fmt.Errorf("apply stamp: %w", err)
	}
	return e.logDecision("recommendation_apply", rec, "success")
}

// RegisterMatcher attaches m to the engine's OnSignal fan-out (Wave-9 V8).
func (e *SQLiteEngine) RegisterMatcher(m SignalMatcher) {
	e.mu.Lock()
	e.matchers = append(e.matchers, m)
	e.mu.Unlock()
}

// OnSignal fans sig through every registered matcher and persists the
// matched Recommendations via Seed. Per-pattern debounce drops any rec
// whose Pattern fired within SignalDebounceWindow — see MemoryEngine.OnSignal.
func (e *SQLiteEngine) OnSignal(ctx context.Context, sig Signal) ([]Recommendation, error) {
	e.mu.Lock()
	matchers := make([]SignalMatcher, len(e.matchers))
	copy(matchers, e.matchers)
	e.mu.Unlock()

	var out []Recommendation
	now := e.now()
	for _, m := range matchers {
		recs, err := m.Match(ctx, sig)
		if err != nil {
			return nil, err
		}
		for _, r := range recs {
			e.mu.Lock()
			last, seen := e.lastFiredAt[r.Pattern]
			if seen && now.Sub(last) < SignalDebounceWindow {
				e.mu.Unlock()
				continue
			}
			e.lastFiredAt[r.Pattern] = now
			e.mu.Unlock()
			if err := e.Seed(r); err != nil {
				return nil, err
			}
			out = append(out, r)
		}
	}
	return out, nil
}

// RecordFeedback persists one feedback row keyed by recommendation id; the
// signal/kind shape mirrors spec §8 (Accept=+1.0, Reject=-0.5, Ignore=-0.1,
// Apply.success=+0.2, Apply.failed=-0.3). Pattern is denormalized onto the row
// so the W100 propose-time blender survives a later Reject of the parent rec.
func (e *SQLiteEngine) RecordFeedback(ctx context.Context, recID, kind string, signal float64) error {
	id, err := newFeedbackID()
	if err != nil {
		return err
	}
	var pattern string
	if err := e.db.QueryRowContext(ctx, `SELECT pattern FROM recommendations WHERE id = ?`, recID).Scan(&pattern); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("lookup pattern: %w", err)
	}
	now := time.Now().UTC()
	_, err = e.db.ExecContext(ctx,
		`INSERT INTO feedback (id, rec_id, pattern, kind, signal, ts) VALUES (?, ?, ?, ?, ?, ?)`,
		id, recID, pattern, kind, signal, now.UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("record feedback: %w", err)
	}
	// W101: roll the per-pattern (α, β) posterior forward in lockstep.
	return updateBetaPosterior(ctx, e.db, pattern, FeedbackKind(kind), now)
}

func (e *SQLiteEngine) logDecision(kind string, rec Recommendation, outcome string) error {
	if e.audit == nil {
		return nil
	}
	return e.audit.Append(audit.Entry{
		Kind:     kind,
		ArgsHash: rec.ID,
		Outcome:  outcome,
		Detail:   rec.Pattern,
	})
}

// newFeedbackID returns a sortable random id. ulid would pull in another dep
// for one column — crypto/rand + timestamp is dense enough for hermetic tests.
func newFeedbackID() (string, error) {
	// Nanosecond hex is monotonic enough for FTS ordering and avoids a dep.
	return fmt.Sprintf("fb-%x", time.Now().UTC().UnixNano()), nil
}
