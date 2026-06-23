package learn

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"math/rand"
)

type ABKernel interface {
	Assign(ctx context.Context, kind RecommendKind) (arm string, expID int64, err error)
	Record(ctx context.Context, expID int64, arm string, won bool) error
	Lock(ctx context.Context, expID int64) error
}

// ErrABNotReady — Lock called before both arms cleared ABLockThreshold or with a tied Wilson-LB.
var ErrABNotReady = errors.New("learn: experiment not ready to lock (insufficient impressions or tied)")

type abKernel struct {
	db   *sql.DB
	coin func() float64
}

func newABKernel(db *sql.DB, coin func() float64) *abKernel {
	if coin == nil {
		coin = rand.Float64
	}
	return &abKernel{db: db, coin: coin}
}

func (k *abKernel) Assign(ctx context.Context, kind RecommendKind) (string, int64, error) {
	var id int64
	var armA, armB string
	var locked int
	var lockedArm sql.NullString
	err := k.db.QueryRowContext(ctx,
		`SELECT id, arm_a, arm_b, locked, locked_arm
		 FROM learn_experiment WHERE kind=? ORDER BY id LIMIT 1`,
		string(kind)).Scan(&id, &armA, &armB, &locked, &lockedArm)
	if err != nil {
		return "", 0, err
	}
	if locked == 1 && lockedArm.Valid {
		return lockedArm.String, id, nil
	}
	arm := armA
	if k.coin() >= 0.5 {
		arm = armB
	}
	col := "impressions_a"
	if arm == armB {
		col = "impressions_b"
	}
	if _, err := k.db.ExecContext(ctx,
		`UPDATE learn_experiment SET `+col+`=`+col+`+1 WHERE id=?`, id); err != nil {
		return "", 0, err
	}
	return arm, id, nil
}

func (k *abKernel) Record(ctx context.Context, expID int64, arm string, won bool) error {
	var armA, armB string
	if err := k.db.QueryRowContext(ctx,
		`SELECT arm_a, arm_b FROM learn_experiment WHERE id=?`, expID).Scan(&armA, &armB); err != nil {
		return err
	}
	impCol, winCol := "impressions_a", "wins_a"
	if arm == armB {
		impCol, winCol = "impressions_b", "wins_b"
	} else if arm != armA {
		return errors.New("learn: arm does not match experiment")
	}
	q := `UPDATE learn_experiment SET ` + impCol + `=` + impCol + `+1`
	if won {
		q += `, ` + winCol + `=` + winCol + `+1`
	}
	q += ` WHERE id=?`
	_, err := k.db.ExecContext(ctx, q, expID)
	return err
}

// Lock seals the experiment to the Wilson-LB winner once both arms hit threshold.
// Ties stay 50/50 (§3.7) — winner is decided by the 95% lower bound, not raw rate.
func (k *abKernel) Lock(ctx context.Context, expID int64) error {
	var armA, armB string
	var iA, iB, wA, wB int
	if err := k.db.QueryRowContext(ctx,
		`SELECT arm_a, arm_b, impressions_a, impressions_b, wins_a, wins_b
		 FROM learn_experiment WHERE id=?`, expID).Scan(&armA, &armB, &iA, &iB, &wA, &wB); err != nil {
		return err
	}
	if iA < ABLockThreshold || iB < ABLockThreshold {
		return ErrABNotReady
	}
	lbA := wilsonLowerBound(wA, iA)
	lbB := wilsonLowerBound(wB, iB)
	// Epsilon tie — float == is brittle; two arms with imperceptibly close
	// Wilson LBs should stay 50/50 per §3.7, not silently lock to one side.
	if math.Abs(lbA-lbB) < 1e-9 {
		return ErrABNotReady
	}
	winner := armA
	if lbB > lbA {
		winner = armB
	}
	_, err := k.db.ExecContext(ctx,
		`UPDATE learn_experiment SET locked=1, locked_arm=? WHERE id=?`, winner, expID)
	return err
}

// wilsonLowerBound — 95% one-sided lower confidence bound for a binomial.
// Used so a 40/60 winner (raw 0.667) is ranked against a 20/55 (raw 0.364)
// by the conservative floor, not the noisy point estimate.
func wilsonLowerBound(wins, impressions int) float64 {
	if impressions == 0 {
		return 0
	}
	const z = 1.96
	n := float64(impressions)
	p := float64(wins) / n
	denom := 1 + z*z/n
	centre := p + z*z/(2*n)
	margin := z * math.Sqrt((p*(1-p)+z*z/(4*n))/n)
	return (centre - margin) / denom
}
