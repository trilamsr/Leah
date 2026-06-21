// Package papers persists the operator's read-later queue as JSON-lines so a
// future brief consumer can stream-parse without loading the whole array.
// Dedup keys on the canonical arXiv ID (sans version suffix) so saving
// `2406.12345` and `2406.12345v2` collapse to one row.
package papers

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Status is the read-later lifecycle of a saved paper.
type Status string

const (
	StatusUnread  Status = "unread"
	StatusReading Status = "reading"
	StatusRead    Status = "read"
)

// validStatus returns true for the three enum values the CLI exposes. A
// surface-level guard at SetStatus stops a typo silently corrupting the
// status field in the on-disk JSONL.
func validStatus(s Status) bool {
	switch s {
	case StatusUnread, StatusReading, StatusRead:
		return true
	}
	return false
}

// Paper is one row of the read-later queue. JSON tags are the on-disk
// contract — consumers (brief, dashboard) stream-parse lines.
type Paper struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Abstract string    `json:"abstract"`
	Authors  []string  `json:"authors"`
	Link     string    `json:"link"`
	SavedTS  time.Time `json:"saved_ts"`
	Status   Status    `json:"status"`
}

// Store is the JSONL-backed read-later queue. Lifecycle is caller-owned —
// every mutation rewrites the file atomically; concurrent writers are not
// supported (CLI is single-process per invocation).
type Store struct {
	Path string
}

// Save appends a paper to the queue, deduping on canonical ID. Re-saving an
// existing ID is a no-op (original SavedTS + Status preserved) so the operator
// can re-run `leah paper save` after a transient arXiv hiccup without losing
// the previously captured "when did I want to read this" cursor.
func (s *Store) Save(p Paper) error {
	all, err := s.load()
	if err != nil {
		return err
	}
	for _, existing := range all {
		if existing.ID == p.ID {
			return nil
		}
	}
	all = append(all, p)
	return s.write(all)
}

// List returns the queue filtered by status. Empty filter returns all rows.
func (s *Store) List(filter Status) ([]Paper, error) {
	all, err := s.load()
	if err != nil {
		return nil, err
	}
	if filter == "" {
		return all, nil
	}
	out := all[:0]
	for _, p := range all {
		if p.Status == filter {
			out = append(out, p)
		}
	}
	return out, nil
}

// SetStatus updates one paper's status. Unknown ID returns an error so the
// CLI can surface a precise exit code rather than silently no-op.
func (s *Store) SetStatus(id string, status Status) error {
	if !validStatus(status) {
		return fmt.Errorf("invalid status %q (want unread|reading|read)", status)
	}
	all, err := s.load()
	if err != nil {
		return err
	}
	found := false
	for i := range all {
		if all[i].ID == id {
			all[i].Status = status
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("paper %q not found", id)
	}
	return s.write(all)
}

// load reads every line as a Paper. Missing file = empty queue (operator
// hasn't saved yet — not an error). Blank lines are skipped so a future
// hand-edit of the JSONL doesn't crash the loader.
func (s *Store) load() ([]Paper, error) {
	f, err := os.Open(s.Path) // #nosec G304 — path under operator's state dir
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var out []Paper
	sc := bufio.NewScanner(f)
	// arXiv abstracts can be long; raise the per-line cap so a verbose
	// summary doesn't bail with bufio.ErrTooLong.
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var p Paper
		if err := json.Unmarshal(line, &p); err != nil {
			return nil, fmt.Errorf("papers: corrupt line: %w", err)
		}
		out = append(out, p)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// write serializes one paper per line via tmp+rename so a crash mid-write
// can't leave a partial JSONL the consumer would fail to parse.
func (s *Store) write(papers []Paper) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.Path), ".papers-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	w := bufio.NewWriter(tmp)
	for _, p := range papers {
		b, err := json.Marshal(p)
		if err != nil {
			_ = tmp.Close()
			cleanup()
			return err
		}
		if _, err := w.Write(b); err != nil {
			_ = tmp.Close()
			cleanup()
			return err
		}
		if err := w.WriteByte('\n'); err != nil {
			_ = tmp.Close()
			cleanup()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, s.Path); err != nil {
		cleanup()
		return err
	}
	return nil
}
