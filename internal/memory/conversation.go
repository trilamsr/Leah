package memory

import (
	"database/sql"
	"fmt"
	"time"
)

// Turn is one recorded exchange: a user prompt and assistant reply.
type Turn struct {
	ID            string
	UserText      string
	AssistantText string
	CreatedAt     time.Time
}

// RecordTurn inserts (or replaces) a conversation_turn row. turnID is caller-assigned;
// INSERT OR REPLACE means re-recording the same turn ID is idempotent.
// RFC3339Nano (not RFC3339) so two turns recorded within the same wall-clock
// second still sort deterministically by created_at DESC.
func RecordTurn(db *sql.DB, turnID, userText, assistantText string) error {
	_, err := db.Exec(
		`INSERT OR REPLACE INTO conversation_turn(id, user_text, assistant_text, created_at) VALUES(?,?,?,?)`,
		turnID, userText, assistantText, time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("record turn: %w", err)
	}
	return nil
}

// RecentTurns returns the most recent limit turns, newest first.
func RecentTurns(db *sql.DB, limit int) ([]Turn, error) {
	rows, err := db.Query(
		`SELECT id, user_text, assistant_text, created_at
		   FROM conversation_turn
		  ORDER BY created_at DESC
		  LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("recent turns: %w", err)
	}
	defer rows.Close()
	var out []Turn
	for rows.Next() {
		var t Turn
		var created string
		if err := rows.Scan(&t.ID, &t.UserText, &t.AssistantText, &created); err != nil {
			return nil, fmt.Errorf("scan turn: %w", err)
		}
		// RFC3339Nano parses RFC3339 too (Nano is a superset — fractional seconds optional).
		parsed, perr := time.Parse(time.RFC3339Nano, created)
		if perr != nil {
			return nil, fmt.Errorf("parse created_at %q: %w", created, perr)
		}
		t.CreatedAt = parsed
		out = append(out, t)
	}
	return out, rows.Err()
}
