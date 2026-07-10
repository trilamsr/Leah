package eval

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	"github.com/trilam/leah/internal/platform/audit"
)

// Reader streams audit entries in append order. A tiny iterator (rather than
// passing []audit.Entry) so the harness can walk a multi-GB audit.jsonl
// without slurping it; tests feed a JSONL string via NewJSONLReader.
type Reader interface {
	Next() (audit.Entry, bool)
}

// chainOrder is the closed-loop sequence asserted by Validate. The order
// matters — a `self-build.outcome` row arriving before its `self-build`
// dispatch is a cross-loop bleed-over, not a closed loop.
var chainOrder = [...]string{
	"self-build",
	"ship",
	"daemon.transition",
	"self-build.outcome",
}

// ClosedLoop is the M2-V validator. Stateless; safe to reuse across streams.
type ClosedLoop struct{}

// Validate returns true when every chainOrder kind appears in r in order.
// Unrelated rows are ignored. Repeats are tolerated (first match advances).
func (ClosedLoop) Validate(r Reader) bool {
	idx := 0
	for {
		e, ok := r.Next()
		if !ok {
			break
		}
		if idx < len(chainOrder) && e.Kind == chainOrder[idx] {
			idx++
		}
	}
	return idx == len(chainOrder)
}

// jsonlReader walks audit-shaped JSONL one line at a time. Malformed lines
// are skipped — a partially-flushed final row in audit.jsonl is normal and
// must not poison the rest of the validation.
type jsonlReader struct {
	sc *bufio.Scanner
}

// NewJSONLReader adapts an io.Reader of audit JSONL to the Reader interface.
func NewJSONLReader(r io.Reader) Reader {
	return &jsonlReader{sc: bufio.NewScanner(r)}
}

func (j *jsonlReader) Next() (audit.Entry, bool) {
	for j.sc.Scan() {
		line := strings.TrimSpace(j.sc.Text())
		if line == "" {
			continue
		}
		var e audit.Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		return e, true
	}
	return audit.Entry{}, false
}
