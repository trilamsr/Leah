package commsout

import (
	"context"
	"strings"
	"testing"
)

type fakeOsa struct {
	lastScript string
}

func (f *fakeOsa) Run(ctx context.Context, args []string) (string, error) {
	for i, a := range args {
		if a == "-e" && i+1 < len(args) {
			f.lastScript = args[i+1]
		}
	}
	return "", nil
}

func TestDesktopNotifyBuildsCorrectScript(t *testing.T) {
	fo := &fakeOsa{}
	d := &Desktop{Exec: fo}

	if err := d.Notify(context.Background(), "Leah", "PR #1112 merged"); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if !strings.Contains(fo.lastScript, `display notification "PR #1112 merged" with title "Leah"`) {
		t.Errorf("script wrong: %q", fo.lastScript)
	}
}

func TestDesktopNotifyEscapesQuotes(t *testing.T) {
	fo := &fakeOsa{}
	d := &Desktop{Exec: fo}

	if err := d.Notify(context.Background(), `My "title"`, `body with "quotes"`); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if strings.Contains(fo.lastScript, `body with "quotes"`) {
		t.Errorf("quotes not escaped: %q", fo.lastScript)
	}
}
