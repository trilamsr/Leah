// Package selfbuildstatus folds the audit log into a per-self-build closed-loop receipt.
package selfbuildstatus

import (
	"strings"

	"github.com/trilam/leah/internal/platform/audit"
)

// Checkpoint is one observable step of the closed loop.
type Checkpoint struct {
	Name string
	Done bool
	TS   string
}

// Loop is the per-self-build receipt keyed by the dispatch ArgsHash.
type Loop struct {
	ArgsHash    string
	Checkpoints []Checkpoint
	Closed      bool
}

var checkpointOrder = []string{"dispatched", "shipped", "pr_opened", "merged", "outcome"}

// Classify folds audit entries into one Loop per dispatch ArgsHash, pure over the input.
func Classify(entries []audit.Entry) []Loop {
	done := map[string]map[string]string{}

	mark := func(hash, name, ts string) {
		if hash == "" {
			return
		}
		if done[hash] == nil {
			done[hash] = map[string]string{}
		}
		if _, seen := done[hash][name]; !seen {
			done[hash][name] = ts
		}
	}

	var order []string
	seen := map[string]bool{}
	track := func(hash string) {
		if hash != "" && !seen[hash] {
			seen[hash] = true
			order = append(order, hash)
		}
	}

	for _, e := range entries {
		switch e.Kind {
		case "self-build":
			track(e.ArgsHash)
			mark(e.ArgsHash, "dispatched", e.Timestamp)
			if strings.Contains(e.Detail, "url=") {
				mark(e.ArgsHash, "pr_opened", e.Timestamp)
			}
		case "ship":
			track(e.ArgsHash)
			mark(e.ArgsHash, "shipped", e.Timestamp)
		case "self-build.outcome":
			track(e.ArgsHash)
			mark(e.ArgsHash, "outcome", e.Timestamp)
		case "daemon.transition":
			if strings.Contains(e.Detail, "→ merged") {
				mark(e.ArgsHash, "merged", e.Timestamp)
			}
		}
	}

	loops := make([]Loop, 0, len(order))
	for _, hash := range order {
		l := Loop{ArgsHash: hash, Closed: true}
		for _, name := range checkpointOrder {
			ts, ok := done[hash][name]
			l.Checkpoints = append(l.Checkpoints, Checkpoint{Name: name, Done: ok, TS: ts})
			if !ok {
				l.Closed = false
			}
		}
		loops = append(loops, l)
	}
	return loops
}
