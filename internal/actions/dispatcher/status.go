package dispatcher

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/trilam/leah/internal/platform/audit"
)

// Status renders the tail of audit.jsonl as either a fixed-width table or
// a JSON array (--json mode). Limit caps the row count; 0 or > total = all.
type Status struct {
	AuditPath string
	Out       io.Writer
	Limit     int
	JSON      bool
}

// Run streams the audit file, parses each line as audit.Entry (skipping
// malformed rows), and emits the tail in table or JSON form. Missing audit
// file is treated as "no activity" — first-run UX, not an error.
func (s *Status) Run() error {
	f, err := os.Open(s.AuditPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if s.JSON {
				_, _ = fmt.Fprintln(s.Out, "[]")
				return nil
			}
			_, _ = fmt.Fprintln(s.Out, "no activity")
			return nil
		}
		return fmt.Errorf("open audit: %w", err)
	}
	defer func() { _ = f.Close() }()

	var entries []audit.Entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var e audit.Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue // skip malformed
		}
		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan audit: %w", err)
	}

	if len(entries) == 0 {
		if s.JSON {
			_, _ = fmt.Fprintln(s.Out, "[]")
			return nil
		}
		_, _ = fmt.Fprintln(s.Out, "no activity")
		return nil
	}

	limit := s.Limit
	if limit <= 0 || limit > len(entries) {
		limit = len(entries)
	}
	start := len(entries) - limit

	if s.JSON {
		enc := json.NewEncoder(s.Out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(entries[start:]); err != nil {
			return fmt.Errorf("encode json: %w", err)
		}
		return nil
	}

	_, _ = fmt.Fprintf(s.Out, "%-20s  %-7s  %-4s  %-7s  %s\n",
		"TIMESTAMP", "KIND", "BR", "OUTCOME", "DETAIL")
	for _, e := range entries[start:] {
		detail := e.Detail
		if len(detail) > 80 {
			detail = detail[:77] + "..."
		}
		_, _ = fmt.Fprintf(s.Out, "%s  %-7s  BR=%d  %-7s  %s\n",
			e.Timestamp, e.Kind, e.BlastRadius, e.Outcome, detail)
	}
	return nil
}
