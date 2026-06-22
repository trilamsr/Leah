// Package git implements source.Source over `git log`. The strategist
// riffs off the operator's own commits; filtering by author keeps the
// fact stream first-person even on a multi-contributor repo.
package git

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/trilam/leah/internal/strategist/source"
)

// Exec is the shell-out seam. Mirrors contracts.OSExec so a fake can
// inject canned `git log` output in tests without exercising git.
type Exec interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)
}

type Config struct {
	Exec   Exec
	Author string // matched against `git log --author=` (substring, per git's default)
	Dir    string // repo working directory; passed via `-C`
}

type Source struct{ cfg Config }

func New(cfg Config) *Source { return &Source{cfg: cfg} }

func (*Source) Name() string { return "git" }

// Candidates shells out to `git log` with a structured format. Unit-sep
// (\x1f) separates fields and record-sep (\x1e) terminates each commit
// so subjects/bodies containing newlines parse unambiguously.
func (s *Source) Candidates(ctx context.Context, since time.Time) ([]source.Fact, error) {
	args := []string{
		"-C", s.cfg.Dir,
		"log",
		"--since=" + since.UTC().Format(time.RFC3339),
		"--author=" + s.cfg.Author,
		"--format=%H%x1f%s%x1f%b%x1e",
	}
	stdout, _, err := s.cfg.Exec.Run(ctx, "git", args...)
	if err != nil {
		return nil, fmt.Errorf("git source: %w", err)
	}
	out := strings.TrimRight(string(stdout), "\x1e\n")
	if out == "" {
		return nil, nil
	}
	var facts []source.Fact
	for _, rec := range strings.Split(out, "\x1e") {
		rec = strings.TrimLeft(rec, "\n")
		if rec == "" {
			continue
		}
		fields := strings.SplitN(rec, "\x1f", 3)
		if len(fields) < 2 {
			continue
		}
		sha, subj := fields[0], fields[1]
		body := ""
		if len(fields) == 3 {
			body = strings.TrimSpace(fields[2])
		}
		summary := subj
		if body != "" {
			summary = subj + "\n\n" + body
		}
		facts = append(facts, source.Fact{Slot: "commit", Summary: summary, Origin: sha})
	}
	return facts, nil
}
