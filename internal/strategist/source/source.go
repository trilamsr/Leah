// Package source defines the Source interface implemented by the
// strategist's three fact-gathering inputs (git, voice, news). The
// implementations land in follow-up PRs; this file is interface-only
// so the generator package can compile against the seam.
package source

import (
	"context"
	"time"
)

// Source produces candidate facts the generator riffs on. Three
// implementers (git, voice, news) justify the interface — the CLI
// dispatches across them as a set.
type Source interface {
	// Name returns the slot label ("git" | "voice" | "news"). The
	// generator routes per slot, so the string is load-bearing.
	Name() string
	// Candidates returns facts created since `since`. One bad source
	// must NOT block the others — the CLI logs and skips on error.
	Candidates(ctx context.Context, since time.Time) ([]Fact, error)
}

// Fact is the unit a Source emits — short summary plus the original
// reference. Origin is persisted in queue/<id>.md front-matter so an
// operator scanning sent posts can trace each one back to its input.
type Fact struct {
	Slot    string // commit | voice | news
	Summary string
	Origin  string // git sha, voice clip path, or feed URL
}
