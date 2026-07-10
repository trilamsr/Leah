package learn

import (
	"context"
	"database/sql"
	"time"
)

type AntiList interface {
	Add(ctx context.Context, kind RecommendKind, reason string, src AntiSource) error
	Remove(ctx context.Context, kind RecommendKind, src AntiSource) error
	List(ctx context.Context) ([]AntiRule, error)
	IsBlocked(ctx context.Context, kind RecommendKind) (bool, error)
	AutoSweep(ctx context.Context) error
}

type antiList struct {
	db  *sql.DB
	now func() time.Time
}

func newAntiList(db *sql.DB, now func() time.Time) *antiList {
	return &antiList{db: db, now: now}
}

func (a *antiList) Add(ctx context.Context, kind RecommendKind, reason string, src AntiSource) error {
	_, err := a.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO anti_recommend(kind, reason, added_at, source) VALUES(?,?,?,?)`,
		string(kind), reason, a.now().Unix(), string(src))
	return err
}

// Remove rejects cross-source deletions so an operator can't drop the spec-pinned
// wake-word rule (§3.9 — adversary reading SQLite can't bypass via UI either).
func (a *antiList) Remove(ctx context.Context, kind RecommendKind, src AntiSource) error {
	if src != AntiSpec {
		var exists int
		err := a.db.QueryRowContext(ctx,
			`SELECT 1 FROM anti_recommend WHERE kind=? AND source='spec'`,
			string(kind)).Scan(&exists)
		if err == nil {
			return ErrSpecLocked
		} else if err != sql.ErrNoRows {
			return err
		}
	}
	_, err := a.db.ExecContext(ctx,
		`DELETE FROM anti_recommend WHERE kind=? AND source=?`,
		string(kind), string(src))
	return err
}

func (a *antiList) List(ctx context.Context) ([]AntiRule, error) {
	rows, err := a.db.QueryContext(ctx,
		`SELECT kind, reason, added_at, source FROM anti_recommend ORDER BY added_at DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []AntiRule
	for rows.Next() {
		var r AntiRule
		var kind, reason, src string
		var addedAt int64
		if err := rows.Scan(&kind, &reason, &addedAt, &src); err != nil {
			return nil, err
		}
		r.Kind = RecommendKind(kind)
		r.Reason = reason
		r.Source = AntiSource(src)
		r.AddedAt = time.Unix(addedAt, 0)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (a *antiList) IsBlocked(ctx context.Context, kind RecommendKind) (bool, error) {
	var n int
	err := a.db.QueryRowContext(ctx,
		`SELECT count(*) FROM anti_recommend WHERE kind=?`, string(kind)).Scan(&n)
	return n > 0, err
}

// AutoSweep promotes kinds with ≥ AutoDismissThreshold Dismissed states inside
// AutoDismissWindow into auto-source anti-rules (§3.6).
func (a *antiList) AutoSweep(ctx context.Context) error {
	since := a.now().Add(-AutoDismissWindow).Unix()
	rows, err := a.db.QueryContext(ctx,
		`SELECT kind, count(*) FROM learn_recommendation
		 WHERE state='dismissed' AND surfaced_at >= ?
		 GROUP BY kind HAVING count(*) >= ?`,
		since, AutoDismissThreshold)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	var promote []RecommendKind
	for rows.Next() {
		var kind string
		var n int
		if err := rows.Scan(&kind, &n); err != nil {
			return err
		}
		promote = append(promote, RecommendKind(kind))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, k := range promote {
		if err := a.Add(ctx, k, "auto: 3 consecutive Dismissed within 30 d", AntiAuto); err != nil {
			return err
		}
	}
	return nil
}
