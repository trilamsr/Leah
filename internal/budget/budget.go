// Package budget owns two distinct meters. The original Budget/Charge enforces
// a per-process dollar ceiling on Reasoner + reviewer API spend (Phase 1–3).
// Runtime (spec §8) adds per-bucket privacy meters — bytes, seconds, tokens —
// that gate every cloud call site, persist samples to SQLite, and degrade per
// the §8.4 ladder when caps are approached.
package budget

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"
)

// DefaultCeiling is the fallback per-process $ ceiling when
// LEAH_BUDGET_DOLLARS is unset or invalid.
const DefaultCeiling = 5.0

// Budget tracks dollars spent against a fixed Ceiling. Zero value is NOT
// usable — callers MUST construct via New so Ceiling is populated from env.
type Budget struct {
	Ceiling float64
	spent   float64
	mu      sync.Mutex
}

// Spent returns the dollars charged so far, taking the mutex so the read
// observes a consistent value across racing Charge calls.
func (b *Budget) Spent() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.spent
}

// New builds a Budget with Ceiling sourced from LEAH_BUDGET_DOLLARS, falling
// back to DefaultCeiling on missing / unparseable / non-positive input.
func New() *Budget {
	c := DefaultCeiling
	if v := os.Getenv("LEAH_BUDGET_DOLLARS"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil && parsed > 0 {
			c = parsed
		}
	}
	return &Budget{Ceiling: c}
}

// ExceededError is returned by Charge when the new charge would push
// total spend past Ceiling. Carries all three numbers so the operator can
// see exactly which call tripped the gate.
type ExceededError struct {
	Spent, Attempted, Ceiling float64
}

// Error formats the three dollar amounts at 4-decimal precision so cent-level
// SDK costs are visible.
func (e *ExceededError) Error() string {
	return fmt.Sprintf("budget exceeded: spent=$%.4f attempted=$%.4f ceiling=$%.4f",
		e.Spent, e.Attempted, e.Ceiling)
}

// Charge adds dollars to spend, returning *ExceededError when the new total
// would exceed Ceiling. Mutates spend only on success — failed charges leave
// the budget unchanged so the caller can choose to retry with a smaller ask.
func (b *Budget) Charge(dollars float64) error {
	b.mu.Lock()
	if b.spent+dollars > b.Ceiling {
		spent, ceiling := b.spent, b.Ceiling
		b.mu.Unlock()
		slog.Error("budget.exceeded",
			"package", "budget",
			"spent_dollars", spent,
			"attempted_dollars", dollars,
			"ceiling_dollars", ceiling,
		)
		return &ExceededError{Spent: spent, Attempted: dollars, Ceiling: ceiling}
	}
	b.spent += dollars
	spent := b.spent
	b.mu.Unlock()
	slog.Info("budget.charge",
		"package", "budget",
		"charged_dollars", dollars,
		"spent_dollars", spent,
		"ceiling_dollars", b.Ceiling,
	)
	return nil
}

// ErrOverBudget is returned by Runtime.Charge when the increment would push
// a bucket past its cap. Callers use errors.Is to branch onto the §8.4
// degrade ladder.
var ErrOverBudget = errors.New("budget: over cap")

// Runtime is the §8 privacy-budget interface. Implementations persist samples
// to SQLite (budget_bucket / budget_sample per §8.5) and emit Events to
// Subscribers on every Charge.
type Runtime interface {
	Charge(ctx context.Context, b Bucket, n int64) error
	Peek(ctx context.Context, b Bucket) (Balance, error)
	Set(ctx context.Context, b Bucket, cap int64, win Window) error
	Reset(ctx context.Context, b Bucket) error
	Subscribe() <-chan Event
}

// budgetSchema bootstraps the §8.5 tables. Owned canonically by T05's
// migration runner; we run it here too so Runtime is usable standalone in
// tests and in pre-T05 daemon builds. IF NOT EXISTS makes the second runner
// a no-op.
const budgetSchema = `
CREATE TABLE IF NOT EXISTS budget_bucket (
    name    TEXT PRIMARY KEY,
    cap     INTEGER NOT NULL,
    window  TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE IF NOT EXISTS budget_sample (
    bucket TEXT NOT NULL,
    at     INTEGER NOT NULL,
    spent  INTEGER NOT NULL,
    PRIMARY KEY(bucket, at)
);
CREATE INDEX IF NOT EXISTS idx_budget_sample_bucket_at ON budget_sample(bucket, at DESC);
`

type runtime struct {
	mu     sync.Mutex
	db     *sql.DB
	subs   []chan Event
	caps   map[Bucket]int64  // cap snapshot — re-read on Set
	wins   map[Bucket]Window // window snapshot
	cache  map[bucketAt]int64
	nowFn  func() time.Time
	writes chan sampleWrite // async DB flush — §8.7 optimistic charge
	done   chan struct{}
	wg     sync.WaitGroup
}

type bucketAt struct {
	b  Bucket
	at int64
}

type sampleWrite struct {
	bucket Bucket
	at     int64
	n      int64
}

// NewRuntime opens (or attaches to) the privacy-budget meter against db. The
// schema is created if absent and Defaults() are seeded for any bucket not
// already present — operator-edited caps survive restart by virtue of the
// "INSERT … ON CONFLICT DO NOTHING" seed.
func NewRuntime(db *sql.DB) (Runtime, error) {
	r := &runtime{
		db:     db,
		caps:   map[Bucket]int64{},
		wins:   map[Bucket]Window{},
		cache:  map[bucketAt]int64{},
		nowFn:  time.Now,
		writes: make(chan sampleWrite, 256),
		done:   make(chan struct{}),
	}
	if db != nil {
		if _, err := db.Exec(budgetSchema); err != nil {
			return nil, fmt.Errorf("budget: schema: %w", err)
		}
		if err := r.seedDefaults(); err != nil {
			return nil, fmt.Errorf("budget: seed: %w", err)
		}
		if err := r.loadCaps(); err != nil {
			return nil, fmt.Errorf("budget: load caps: %w", err)
		}
		r.wg.Add(1)
		go r.flushLoop()
	} else {
		// In-memory-only mode for tests that don't want a DB. Caps come from
		// Defaults() directly.
		for _, d := range Defaults() {
			r.caps[d.Bucket] = d.Cap
			r.wins[d.Bucket] = d.Window
		}
	}
	return r, nil
}

func (r *runtime) seedDefaults() error {
	for _, d := range Defaults() {
		if _, err := r.db.Exec(
			`INSERT INTO budget_bucket(name, cap, window, enabled) VALUES(?,?,?,1)
			 ON CONFLICT(name) DO NOTHING`,
			string(d.Bucket), d.Cap, string(d.Window),
		); err != nil {
			return err
		}
	}
	return nil
}

func (r *runtime) loadCaps() error {
	rows, err := r.db.Query(`SELECT name, cap, window FROM budget_bucket WHERE enabled=1`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name, win string
		var cap int64
		if err := rows.Scan(&name, &cap, &win); err != nil {
			return err
		}
		r.caps[Bucket(name)] = cap
		r.wins[Bucket(name)] = Window(win)
	}
	return rows.Err()
}

// windowStart truncates now to the rollover boundary for win. Used as the
// budget_sample.at primary-key component so concurrent charges in the same
// window collide on the same row.
func windowStart(win Window, now time.Time) time.Time {
	switch win {
	case WindowHour:
		return now.Truncate(time.Hour)
	case WindowWeek:
		// ISO-ish week — start of Sunday-anchored week, midnight.
		d := now.AddDate(0, 0, -int(now.Weekday()))
		return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, now.Location())
	case WindowMonth:
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	default: // WindowDay (the §8.2 default for every bucket)
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	}
}

// Charge increments the running spend for b by n. Returns ErrOverBudget when
// the new total would exceed cap; spend is NOT mutated on rejection. A zero
// cap means "unlimited" (BYOK ledger only). The hot path is mutex + map
// lookups; the DB write is best-effort behind the lock so a transient SQLite
// hiccup logs at WARN but does not fail the call (§8.7: charge proceeds
// optimistically, reconciles on next sample tick).
func (r *runtime) Charge(ctx context.Context, b Bucket, n int64) error {
	if n <= 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cap, ok := r.caps[b]
	if !ok {
		return fmt.Errorf("budget: unknown bucket %q", b)
	}
	win, ok := r.wins[b]
	if !ok {
		win = WindowDay
	}
	ws := windowStart(win, r.nowFn()).Unix()
	key := bucketAt{b: b, at: ws}
	spent := r.cache[key]
	if cap > 0 && spent+n > cap {
		r.emitLocked(Event{Bucket: b, Spent: spent, Cap: cap, Action: ActionBlock})
		return ErrOverBudget
	}
	r.cache[key] = spent + n
	if r.db != nil {
		// Non-blocking send — §8.7 optimistic charge: hot path stays sub-ms,
		// flushLoop persists. Channel full means the flusher fell behind and
		// the cache has the truth; the next Peek FlushPending closes the gap.
		select {
		case r.writes <- sampleWrite{bucket: b, at: ws, n: n}:
		default:
			slog.Warn("budget.write_queue_full", "package", "budget", "bucket", string(b))
		}
	}
	var action DegradeAction
	if cap > 0 {
		action = DegradePath(b, float64(spent+n)/float64(cap))
	}
	r.emitLocked(Event{Bucket: b, Spent: spent + n, Cap: cap, Action: action})
	return nil
}

// Peek returns the current Balance for b. ResetsAt is the start of the NEXT
// window; Trend is up to 24 most-recent samples from budget_sample
// (oldest→newest) so callers can chart without re-querying.
func (r *runtime) Peek(ctx context.Context, b Bucket) (Balance, error) {
	r.mu.Lock()
	cap := r.caps[b]
	win, ok := r.wins[b]
	if !ok {
		win = WindowDay
	}
	now := r.nowFn()
	ws := windowStart(win, now)
	spent := r.cache[bucketAt{b: b, at: ws.Unix()}]
	r.mu.Unlock()
	bal := Balance{
		Bucket:   b,
		Spent:    spent,
		Cap:      cap,
		Window:   win,
		ResetsAt: nextWindow(win, ws),
	}
	if r.db != nil {
		rows, err := r.db.QueryContext(ctx,
			`SELECT spent FROM budget_sample WHERE bucket=? ORDER BY at DESC LIMIT 24`,
			string(b))
		if err != nil {
			return bal, err
		}
		defer func() { _ = rows.Close() }()
		var trend []int64
		for rows.Next() {
			var v int64
			if err := rows.Scan(&v); err != nil {
				return bal, err
			}
			trend = append([]int64{v}, trend...) // oldest→newest
		}
		if err := rows.Err(); err != nil {
			return bal, err
		}
		bal.Trend = trend
	}
	return bal, nil
}

func nextWindow(win Window, ws time.Time) time.Time {
	switch win {
	case WindowHour:
		return ws.Add(time.Hour)
	case WindowWeek:
		return ws.AddDate(0, 0, 7)
	case WindowMonth:
		return ws.AddDate(0, 1, 0)
	default:
		return ws.AddDate(0, 0, 1)
	}
}

// Set updates the cap + window for b, persisting to budget_bucket. Existing
// in-window spend is preserved — if the new cap is below current spent, the
// next Charge will trip ErrOverBudget per §8.7 (no retroactive refund).
func (r *runtime) Set(ctx context.Context, b Bucket, cap int64, win Window) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.db != nil {
		_, err := r.db.ExecContext(ctx,
			`INSERT INTO budget_bucket(name, cap, window, enabled) VALUES(?,?,?,1)
			 ON CONFLICT(name) DO UPDATE SET cap=excluded.cap, window=excluded.window, enabled=1`,
			string(b), cap, string(win))
		if err != nil {
			return err
		}
	}
	r.caps[b] = cap
	r.wins[b] = win
	return nil
}

// Reset zeroes the current-window sample for b. Used by operator "reset now"
// in Settings and by the sample-tick aggregator on rollover.
func (r *runtime) Reset(ctx context.Context, b Bucket) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	win, ok := r.wins[b]
	if !ok {
		win = WindowDay
	}
	ws := windowStart(win, r.nowFn()).Unix()
	delete(r.cache, bucketAt{b: b, at: ws})
	if r.db != nil {
		if _, err := r.db.ExecContext(ctx,
			`DELETE FROM budget_sample WHERE bucket=? AND at=?`,
			string(b), ws); err != nil {
			return err
		}
	}
	return nil
}

// Subscribe returns a channel that receives an Event for every Charge. The
// channel is buffered (32) and non-blocking — a slow subscriber drops events
// rather than stalling the charge path (§8.7).
func (r *runtime) Subscribe() <-chan Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch := make(chan Event, 32)
	r.subs = append(r.subs, ch)
	return ch
}

// emitLocked publishes ev to every Subscriber. Caller MUST hold r.mu so the
// subs slice and the spent counter advance atomically — subscribers see
// monotonically increasing Spent values.
func (r *runtime) emitLocked(ev Event) {
	for _, s := range r.subs {
		select {
		case s <- ev:
		default:
		}
	}
}

// flushLoop drains the writes channel and persists samples in batches. One
// goroutine per Runtime — sequential UPSERTs against SQLite are cheaper than
// per-call goroutines paying the connection-acquire cost on the hot path.
func (r *runtime) flushLoop() {
	defer r.wg.Done()
	for {
		select {
		case <-r.done:
			r.drain()
			return
		case w := <-r.writes:
			r.persist(w)
		}
	}
}

func (r *runtime) drain() {
	for {
		select {
		case w := <-r.writes:
			r.persist(w)
		default:
			return
		}
	}
}

func (r *runtime) persist(w sampleWrite) {
	if _, err := r.db.Exec(
		`INSERT INTO budget_sample(bucket, at, spent) VALUES(?,?,?)
		 ON CONFLICT(bucket, at) DO UPDATE SET spent = budget_sample.spent + excluded.spent`,
		string(w.bucket), w.at, w.n,
	); err != nil {
		slog.Warn("budget.sample_write_failed",
			"package", "budget", "bucket", string(w.bucket), "err", err.Error())
	}
}

// Close stops the flush goroutine and persists any queued writes. Idempotent.
func (r *runtime) Close() error {
	r.mu.Lock()
	select {
	case <-r.done:
		r.mu.Unlock()
		return nil
	default:
		close(r.done)
	}
	r.mu.Unlock()
	r.wg.Wait()
	return nil
}

// Closer is implemented by runtime so the daemon can shut down cleanly. The
// Runtime interface deliberately omits Close — most callers don't own the
// runtime lifecycle; the daemon wiring does.
type Closer interface {
	Close() error
}
