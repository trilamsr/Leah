package selfbuildstatus

import (
	"testing"

	"github.com/trilam/leah/internal/audit"
)

func cp(loop Loop, name string) Checkpoint {
	for _, c := range loop.Checkpoints {
		if c.Name == name {
			return c
		}
	}
	return Checkpoint{}
}

func TestClassify_FullLoopClosed(t *testing.T) {
	entries := []audit.Entry{
		{Kind: "self-build", ArgsHash: "h1", Outcome: "dispatched", Detail: "url=https://github.com/trilamsr/Leah/issues/1", Timestamp: "2026-06-20T10:00:00Z"},
		{Kind: "ship", ArgsHash: "h1", Outcome: "pending", Timestamp: "2026-06-20T10:01:00Z"},
		{Kind: "daemon.transition", Outcome: "observed", ArgsHash: "h1", Detail: "agent a1: running → merged (PR #1)", Timestamp: "2026-06-20T11:00:00Z"},
		{Kind: "self-build.outcome", ArgsHash: "h1", Outcome: "MERGED", Detail: "state=MERGED", Timestamp: "2026-06-20T11:01:00Z"},
	}
	loops := Classify(entries)
	if len(loops) != 1 {
		t.Fatalf("want 1 loop, got %d", len(loops))
	}
	if !loops[0].Closed {
		t.Fatalf("want Closed=true, got %+v", loops[0])
	}
	for _, name := range []string{"dispatched", "shipped", "pr_opened", "merged", "outcome"} {
		if !cp(loops[0], name).Done {
			t.Errorf("checkpoint %q not Done", name)
		}
	}
}

func TestClassify_PartialPending(t *testing.T) {
	entries := []audit.Entry{
		{Kind: "self-build", ArgsHash: "h1", Outcome: "dispatched", Detail: "url=https://github.com/trilamsr/Leah/issues/1", Timestamp: "2026-06-20T10:00:00Z"},
		{Kind: "ship", ArgsHash: "h1", Outcome: "pending", Timestamp: "2026-06-20T10:01:00Z"},
	}
	loops := Classify(entries)
	if len(loops) != 1 {
		t.Fatalf("want 1 loop, got %d", len(loops))
	}
	if loops[0].Closed {
		t.Fatalf("want Closed=false for in-flight loop, got %+v", loops[0])
	}
	if !cp(loops[0], "dispatched").Done || !cp(loops[0], "shipped").Done {
		t.Errorf("dispatched+shipped must be Done")
	}
	if cp(loops[0], "merged").Done || cp(loops[0], "outcome").Done {
		t.Errorf("merged+outcome must be pending")
	}
}

func TestClassify_TwoHashesNoCrossTalk(t *testing.T) {
	entries := []audit.Entry{
		{Kind: "self-build", ArgsHash: "h1", Outcome: "dispatched", Timestamp: "2026-06-20T10:00:00Z"},
		{Kind: "self-build", ArgsHash: "h2", Outcome: "dispatched", Timestamp: "2026-06-20T10:05:00Z"},
		{Kind: "ship", ArgsHash: "h2", Outcome: "pending", Timestamp: "2026-06-20T10:06:00Z"},
	}
	loops := Classify(entries)
	if len(loops) != 2 {
		t.Fatalf("want 2 loops, got %d", len(loops))
	}
	byHash := map[string]Loop{}
	for _, l := range loops {
		byHash[l.ArgsHash] = l
	}
	if cp(byHash["h1"], "shipped").Done {
		t.Errorf("h1 must not borrow h2's ship")
	}
	if !cp(byHash["h2"], "shipped").Done {
		t.Errorf("h2 ship missing")
	}
}

func TestClassify_NonMergedTransitionDoesNotSatisfyMerged(t *testing.T) {
	entries := []audit.Entry{
		{Kind: "self-build", ArgsHash: "h1", Outcome: "dispatched", Timestamp: "2026-06-20T10:00:00Z"},
		{Kind: "daemon.transition", ArgsHash: "h1", Outcome: "observed", Detail: "agent a1: running → escalated (PR #1)", Timestamp: "2026-06-20T11:00:00Z"},
	}
	loops := Classify(entries)
	if len(loops) != 1 {
		t.Fatalf("want 1 loop, got %d", len(loops))
	}
	if cp(loops[0], "merged").Done {
		t.Errorf("escalated transition must NOT satisfy merged checkpoint")
	}
}

func TestClassify_TimestampRecorded(t *testing.T) {
	entries := []audit.Entry{
		{Kind: "self-build", ArgsHash: "h1", Outcome: "dispatched", Timestamp: "2026-06-20T10:00:00Z"},
	}
	loops := Classify(entries)
	if got := cp(loops[0], "dispatched").TS; got != "2026-06-20T10:00:00Z" {
		t.Errorf("dispatched TS: got %q", got)
	}
	if got := cp(loops[0], "shipped").TS; got != "" {
		t.Errorf("pending checkpoint TS must be empty, got %q", got)
	}
}
