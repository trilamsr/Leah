// Package launcher shells out to macOS open(1) for a curated set of named
// targets — streaming services, social sites, etc. Native app via URL scheme
// when registered; otherwise web URL fallback. Static intent table only — no
// config file in MVP.
package launcher

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrUnknownTarget = errors.New("launcher: unknown target")

// ExecRunner mirrors the (ctx, name, args, stdin) shape every other adapter
// uses so cmd/leah can pass the same nativeExec it already wires elsewhere.
type ExecRunner interface {
	Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error)
}

// Intent is the launch recipe for one named target. URLScheme is preferred
// over WebURL when present so the native app gets a chance before the browser.
type Intent struct {
	URLScheme string
	WebURL    string
}

type Launcher struct {
	exec    ExecRunner
	intents map[string]Intent
}

func New(exec ExecRunner, intents map[string]Intent) (*Launcher, error) {
	if exec == nil {
		return nil, errors.New("launcher: ExecRunner required")
	}
	if intents == nil {
		return nil, errors.New("launcher: intents required")
	}
	return &Launcher{exec: exec, intents: intents}, nil
}

// Open looks up the named target and shells out to `open <scheme-or-url>`.
// Name is normalized case-insensitively with whitespace trimmed so shell
// quoting differences don't surface as "unknown target".
func (l *Launcher) Open(ctx context.Context, name string) error {
	key := strings.ToLower(strings.TrimSpace(name))
	intent, ok := l.intents[key]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownTarget, name)
	}
	target := intent.WebURL
	if intent.URLScheme != "" {
		target = intent.URLScheme
	}
	if _, err := l.exec.Run(ctx, "open", []string{target}, nil); err != nil {
		return fmt.Errorf("launcher: open %q: %w", name, err)
	}
	return nil
}

// defaultIntents is the v1 launch surface. Repeats are spelled out (no
// builder pattern) — three similar lines beat a premature abstraction.
func defaultIntents() map[string]Intent {
	netflix := Intent{WebURL: "https://netflix.com"}
	hulu := Intent{WebURL: "https://hulu.com"}
	max := Intent{WebURL: "https://play.max.com"}
	disney := Intent{WebURL: "https://disneyplus.com"}
	prime := Intent{WebURL: "https://primevideo.com"}
	youtube := Intent{WebURL: "https://youtube.com"}
	appletv := Intent{URLScheme: "videos://", WebURL: "https://tv.apple.com"}
	spotify := Intent{URLScheme: "spotify:", WebURL: "https://open.spotify.com"}

	linkedin := Intent{WebURL: "https://linkedin.com"}
	facebook := Intent{WebURL: "https://facebook.com"}
	instagram := Intent{WebURL: "https://instagram.com"}
	x := Intent{WebURL: "https://x.com"}
	tiktok := Intent{WebURL: "https://tiktok.com"}
	reddit := Intent{WebURL: "https://reddit.com"}
	github := Intent{WebURL: "https://github.com"}

	return map[string]Intent{
		"netflix":     netflix,
		"hulu":        hulu,
		"hbomax":      max,
		"max":         max,
		"disney":      disney,
		"disneyplus":  disney,
		"prime":       prime,
		"primevideo":  prime,
		"youtube":     youtube,
		"appletv":     appletv,
		"spotify":     spotify,
		"linkedin":    linkedin,
		"facebook":    facebook,
		"fb":          facebook,
		"instagram":   instagram,
		"ig":          instagram,
		"x":           x,
		"twitter":     x,
		"tiktok":      tiktok,
		"reddit":      reddit,
		"github":      github,
		"gh":          github,
	}
}

// DefaultIntents exposes the static table so the CLI wires through New().
func DefaultIntents() map[string]Intent { return defaultIntents() }
