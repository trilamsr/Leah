package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunWatch_AddListRemove(t *testing.T) {
	t.Setenv("LEAH_STATE_DIR", t.TempDir())

	var out, errOut bytes.Buffer
	if code := runWatch([]string{"aapl"}, &out, &errOut); code != 0 {
		t.Fatalf("add aapl: code=%d err=%s", code, errOut.String())
	}
	if code := runWatch([]string{"MSFT"}, &out, &errOut); code != 0 {
		t.Fatalf("add MSFT: code=%d", code)
	}

	out.Reset()
	if code := runWatch(nil, &out, &errOut); code != 0 {
		t.Fatalf("list: code=%d", code)
	}
	lines := strings.Fields(out.String())
	if len(lines) != 2 || lines[0] != "AAPL" || lines[1] != "MSFT" {
		t.Fatalf("list got %q", out.String())
	}

	out.Reset()
	if code := runWatch([]string{"--rm", "aapl"}, &out, &errOut); code != 0 {
		t.Fatalf("rm: code=%d err=%s", code, errOut.String())
	}
	out.Reset()
	_ = runWatch(nil, &out, &errOut)
	if strings.TrimSpace(out.String()) != "MSFT" {
		t.Fatalf("after rm got %q", out.String())
	}
}

func TestRunWatch_RejectInvalid(t *testing.T) {
	t.Setenv("LEAH_STATE_DIR", t.TempDir())
	var out, errOut bytes.Buffer
	if code := runWatch([]string{"ab1"}, &out, &errOut); code != 2 {
		t.Fatalf("expected exit 2 for invalid, got %d", code)
	}
}

func TestRunWatch_RmRequiresArg(t *testing.T) {
	t.Setenv("LEAH_STATE_DIR", t.TempDir())
	var out, errOut bytes.Buffer
	if code := runWatch([]string{"--rm"}, &out, &errOut); code != 2 {
		t.Fatalf("expected exit 2, got %d", code)
	}
}
