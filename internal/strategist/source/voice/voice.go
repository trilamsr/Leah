// Package voice implements source.Source over a directory of operator
// text dumps (~/.config/leah/strategist/voice-dumps/). Each .md/.txt
// file becomes one Fact; the operator drafts these by hand or via a
// dictation tool, so leah never owns the format.
package voice

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/trilam/leah/internal/strategist/source"
)

// Per-file cap keeps a stray 100MB transcript from blowing the process.
// 1 MiB still admits a multi-hour dictation in plain text.
const maxFileBytes = 1 << 20

type Config struct {
	Dir string
}

type Source struct{ cfg Config }

func New(cfg Config) *Source { return &Source{cfg: cfg} }

func (*Source) Name() string { return "voice" }

// Candidates scans Dir for .md/.txt files modified at-or-after `since`.
// Non-text extensions and dotfiles are skipped silently; an unreadable
// or oversize file logs nothing and is skipped (one bad file must not
// blank the whole source — that's the per-source failure-isolation
// contract from the strategist spec).
func (s *Source) Candidates(_ context.Context, since time.Time) ([]source.Fact, error) {
	entries, err := os.ReadDir(s.cfg.Dir)
	if err != nil {
		return nil, fmt.Errorf("voice source: %w", err)
	}
	var facts []source.Fact
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".md" && ext != ".txt" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if !since.IsZero() && info.ModTime().Before(since) {
			continue
		}
		if info.Size() > maxFileBytes {
			continue
		}
		path := filepath.Join(s.cfg.Dir, name)
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		facts = append(facts, source.Fact{
			Slot:    "voice",
			Summary: strings.TrimSpace(string(body)),
			Origin:  path,
		})
	}
	return facts, nil
}
