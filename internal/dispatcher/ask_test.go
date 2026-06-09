package dispatcher

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget"
)

type fakeReasoner struct {
	got string
	out string
}

func (f *fakeReasoner) Ask(ctx context.Context, user string) (string, error) {
	f.got = user
	return f.out, nil
}

func TestAskWritesToWriterAndAuditsAndReturnsCleanly(t *testing.T) {
	dir := t.TempDir()
	auditPath := dir + "/audit.jsonl"
	a := &audit.Logger{Path: auditPath}
	b := &budget.Budget{Ceiling: 5.0}
	r := &fakeReasoner{out: "the answer is 42"}
	out := &bytes.Buffer{}

	ask := &Ask{Reasoner: r, Audit: a, Budget: b, Out: out}
	if err := ask.Run(context.Background(), "what is the meaning of life"); err != nil {
		t.Fatalf("run: %v", err)
	}

	if r.got != "what is the meaning of life" {
		t.Errorf("reasoner got %q", r.got)
	}
	if !strings.Contains(out.String(), "the answer is 42") {
		t.Errorf("output: %q", out.String())
	}

	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if !strings.Contains(string(data), `"kind":"ask"`) {
		t.Errorf("audit missing kind=ask: %q", data)
	}
	if !strings.Contains(string(data), `"blast_radius":0`) {
		t.Errorf("audit missing blast_radius=0: %q", data)
	}
}
