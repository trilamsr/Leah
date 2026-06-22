// Package persona loads the operator-edited strategist voice file from
// <configDir>/strategist/persona.md. Distinct from internal/persona/ —
// that one is a SQLite workspace store; this one is a hand-edited
// markdown file the operator owns with vim+git, never written by leah.
package persona

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultVoice is the fallback persona body the CLI splices into the
// LLM system prompt when persona.md is absent. Kept terse — operators
// who want voice control are expected to author persona.md.
const DefaultVoice = `You are a pragmatic first-person engineer.
Write short, concrete posts in your own voice. No hashtags, no emoji,
no marketing tone. Ship the receipt.`

// Load returns the full markdown body of <configDir>/strategist/persona.md.
// The body is spliced verbatim into the LLM system prompt — Load makes
// no parsing decisions and no normalization. A missing file returns an
// error wrapping os.ErrNotExist so the CLI can fall back to DefaultVoice.
func Load(configDir string) (string, error) {
	path := filepath.Join(configDir, "strategist", "persona.md")
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("strategist persona load %s: %w", path, err)
	}
	return string(b), nil
}
