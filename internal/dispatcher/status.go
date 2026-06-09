package dispatcher

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/trilam/leah/internal/audit"
)

type Status struct {
	AuditPath string
	Out       io.Writer
	Limit     int
}

func (s *Status) Run() error {
	f, err := os.Open(s.AuditPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
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
		_, _ = fmt.Fprintln(s.Out, "no activity")
		return nil
	}

	limit := s.Limit
	if limit <= 0 || limit > len(entries) {
		limit = len(entries)
	}
	start := len(entries) - limit
	for _, e := range entries[start:] {
		_, _ = fmt.Fprintf(s.Out, "%s  %-7s  BR=%d  %s  %s\n",
			e.Timestamp, e.Kind, e.BlastRadius, e.Outcome, e.Detail)
	}
	return nil
}
