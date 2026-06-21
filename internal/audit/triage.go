package audit

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"strings"
	"time"
)

// IsAlarming is the operator-facing definition of an on-call signal: an
// Outcome that named a non-success terminal state, OR a Kind whose suffix
// pattern marks the row as a failure-shaped emission (breaker trips, error
// channels, *_failure / *.error kinds, daemon transitions whose detail
// carries an "error" substring).
//
// Pinned by TestIsAlarming_Cases — change this list, change the test.
func IsAlarming(e Entry) bool {
	switch e.Outcome {
	case "failed", "interrupted", "rejected", "block", "peer_mismatch":
		return true
	}
	k := e.Kind
	if strings.HasSuffix(k, ".tripped") ||
		strings.HasSuffix(k, ".error") ||
		strings.HasSuffix(k, "_error") ||
		strings.HasSuffix(k, "_failed") ||
		strings.HasSuffix(k, "_fail") ||
		strings.HasSuffix(k, "_failure") ||
		strings.HasSuffix(k, ".emit_failure") {
		return true
	}
	if k == "daemon.transition" && strings.Contains(strings.ToLower(e.Detail), "error") {
		return true
	}
	return false
}

// Triage streams audit.jsonl at path and returns rows that (a) timestamp at
// or after since, (b) satisfy IsAlarming, and (c) — when kindFilter is
// non-empty — contain kindFilter as a substring of Kind. Missing file is
// not an error so a fresh install returns an empty slice. Malformed rows
// are skipped.
func Triage(path string, since time.Time, kindFilter string) ([]Entry, error) {
	var out []Entry
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return out, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var e Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		ts, err := time.Parse(time.RFC3339, e.Timestamp)
		if err != nil {
			continue
		}
		if ts.Before(since) {
			continue
		}
		if !IsAlarming(e) {
			continue
		}
		if kindFilter != "" && !strings.Contains(e.Kind, kindFilter) {
			continue
		}
		out = append(out, e)
	}
	return out, sc.Err()
}
