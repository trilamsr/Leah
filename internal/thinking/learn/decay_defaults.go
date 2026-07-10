package learn

import (
	"context"
	"database/sql"
	"time"
)

// RegisterDecayDefaults seeds §3.5; idempotent per-kind across restarts.
func RegisterDecayDefaults(ctx context.Context, db *sql.DB) error {
	defaults := []DecaySchedule{
		{"integration-connect", 14 * 24 * time.Hour, 60 * 24 * time.Hour},
		{"pin-widget", 7 * 24 * time.Hour, 21 * 24 * time.Hour},
		{"voice-on", 30 * 24 * time.Hour, 180 * 24 * time.Hour},
		{"wake-on", 90 * 24 * time.Hour, 0},
		{"memory-purge", 24 * time.Hour, 7 * 24 * time.Hour},
		{"plugin-install", 30 * 24 * time.Hour, 90 * 24 * time.Hour},
		{"multi-device-pair", 30 * 24 * time.Hour, 90 * 24 * time.Hour},
	}
	for _, d := range defaults {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO learn_decay(kind, half_life_s, hard_expire_s)
			 SELECT ?,?,? WHERE NOT EXISTS(SELECT 1 FROM learn_decay WHERE kind=?)`,
			string(d.Kind), int(d.HalfLife/time.Second), int(d.HardExpire/time.Second),
			string(d.Kind)); err != nil {
			return err
		}
	}
	return nil
}
