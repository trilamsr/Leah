package crdt

import (
	"context"
	"testing"
	"time"
)

// GCTombstones removes only rows whose deleted_at < cutoff; younger tombstones stay.
func TestGCTombstones_RetainsYoung(t *testing.T) {
	l, db := newLog(t, "self")
	ctx := context.Background()
	now := time.Now().UTC().Unix()
	oldTs := now - int64(120*24*time.Hour/time.Second) // 120 days old
	youngTs := now - int64(30*24*time.Hour/time.Second)

	for _, row := range []struct {
		rid       int64
		deletedAt int64
	}{
		{1, oldTs},
		{2, youngTs},
	} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO sync_tombstone(table_name,row_id,deleted_at,deleted_by) VALUES('memory',?,?,?)`,
			row.rid, row.deletedAt, "node-a"); err != nil {
			t.Fatalf("seed %d: %v", row.rid, err)
		}
	}

	cutoff := time.Unix(now-int64(90*24*time.Hour/time.Second), 0)
	n, err := l.GCTombstones(ctx, cutoff)
	if err != nil {
		t.Fatalf("gc: %v", err)
	}
	if n != 1 {
		t.Fatalf("deleted %d, want 1 (the 120d-old row)", n)
	}
	var remaining int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sync_tombstone`).Scan(&remaining); err != nil {
		t.Fatalf("count: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("remaining = %d, want 1 (the 30d-old row)", remaining)
	}
}
