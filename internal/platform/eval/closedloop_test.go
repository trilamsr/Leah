package eval

import (
	"strings"
	"testing"

	"github.com/trilam/leah/internal/platform/audit"
)

// jsonlReader is a test fixture audit.Entry stream backed by an inline JSONL
// string — keeps the test self-contained (no tmp-file ceremony) while still
// exercising the same decode path a real audit.jsonl walker uses.
func fixtureReader(t *testing.T, jsonl string) Reader {
	t.Helper()
	return NewJSONLReader(strings.NewReader(strings.TrimSpace(jsonl)))
}

func TestClosedLoop_Validate(t *testing.T) {
	const (
		dispatch = `{"ts":"2026-06-21T10:00:00Z","kind":"self-build","args_hash":"abc"}`
		shipRow  = `{"ts":"2026-06-21T10:05:00Z","kind":"ship","args_hash":"abc"}`
		trans    = `{"ts":"2026-06-21T10:10:00Z","kind":"daemon.transition","args_hash":"abc","detail":"to=merged"}`
		outcome  = `{"ts":"2026-06-21T10:15:00Z","kind":"self-build.outcome","args_hash":"abc","outcome":"ok"}`
		noise    = `{"ts":"2026-06-21T09:00:00Z","kind":"plan","args_hash":"abc"}`
	)

	cases := []struct {
		name  string
		jsonl string
		want  bool
	}{
		{
			name:  "all four kinds in chain order closes the loop",
			jsonl: dispatch + "\n" + shipRow + "\n" + trans + "\n" + outcome,
			want:  true,
		},
		{
			name:  "all four with unrelated noise rows still closes",
			jsonl: noise + "\n" + dispatch + "\n" + shipRow + "\n" + trans + "\n" + outcome,
			want:  true,
		},
		{
			name:  "missing self-build dispatch is open",
			jsonl: shipRow + "\n" + trans + "\n" + outcome,
			want:  false,
		},
		{
			name:  "missing ship is open",
			jsonl: dispatch + "\n" + trans + "\n" + outcome,
			want:  false,
		},
		{
			name:  "missing daemon.transition is open",
			jsonl: dispatch + "\n" + shipRow + "\n" + outcome,
			want:  false,
		},
		{
			name:  "missing self-build.outcome is open",
			jsonl: dispatch + "\n" + shipRow + "\n" + trans,
			want:  false,
		},
		{
			name:  "out-of-order (outcome before dispatch) is open",
			jsonl: outcome + "\n" + dispatch + "\n" + shipRow + "\n" + trans,
			want:  false,
		},
	}

	cl := ClosedLoop{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cl.Validate(fixtureReader(t, tc.jsonl))
			if got != tc.want {
				t.Fatalf("Validate = %v, want %v", got, tc.want)
			}
		})
	}
}

// Compile-time canary: if audit.Entry.Kind is renamed upstream, the harness
// stops compiling here instead of silently never matching a chain kind.
var _ = audit.Entry{}.Kind
