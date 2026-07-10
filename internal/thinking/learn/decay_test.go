package learn

import (
	"context"
	"testing"
	"time"
)

func TestDecay_HalfLifeHalvesScore(t *testing.T) {
	s := DecaySchedule{Kind: "pin-widget", HalfLife: 7 * 24 * time.Hour}
	got := Decay(1.0, 7*24*time.Hour, s)
	if got > 0.55 || got < 0.45 {
		t.Fatalf("score at one half-life: want ~0.5, got %v", got)
	}
}

func TestDecay_HardExpireZeros(t *testing.T) {
	s := DecaySchedule{Kind: "memory-purge", HalfLife: 24 * time.Hour, HardExpire: 7 * 24 * time.Hour}
	got := Decay(1.0, 8*24*time.Hour, s)
	if got != 0 {
		t.Fatalf("post-hard-expire: want 0, got %v", got)
	}
}

func TestDecay_ZeroHalfLifePassesThrough(t *testing.T) {
	s := DecaySchedule{Kind: "wake-on", HalfLife: 0, HardExpire: 0}
	got := Decay(0.7, 365*24*time.Hour, s)
	if got != 0.7 {
		t.Fatalf("zero half-life should pass score through unchanged, got %v", got)
	}
}

func TestRegisterDecayDefaults_Idempotent(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	// schema seeds one row; clear to test full default-load
	if _, err := db.Exec(`DELETE FROM learn_decay`); err != nil {
		t.Fatal(err)
	}
	if err := RegisterDecayDefaults(context.Background(), db); err != nil {
		t.Fatalf("first load: %v", err)
	}
	if err := RegisterDecayDefaults(context.Background(), db); err != nil {
		t.Fatalf("second load: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM learn_decay`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 7 {
		t.Fatalf("want 7 decay rows after idempotent re-load, got %d", n)
	}
}
