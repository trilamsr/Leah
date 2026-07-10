package main

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/trilam/leah/internal/actions/connect"
	strategistpersona "github.com/trilam/leah/internal/thinking/strategist/persona"
	"github.com/trilam/leah/internal/thinking/strategist/store"
)

// LLM integration deferred to next PR; this MVP wires the store + UX surface
// only. `post` writes a placeholder body into inbox/ so the operator-facing
// pipeline is exercisable end-to-end while real generation lands behind it.

// clipboardCopier is the seam the post path uses to hand the body to the OS
// clipboard. Tests substitute a capturing fake — production uses pbcopyClip
// which feeds bytes via stdin (never argv), so a topic containing shell
// metacharacters cannot influence execution.
type clipboardCopier interface {
	Copy(ctx context.Context, body string) error
}

// pathLooker abstracts exec.LookPath so doctor tests don't depend on the host
// machine's installed binaries.
type pathLooker interface {
	Look(name string) (string, error)
}

// strategistDeps bundles the test seams. Zero value uses production defaults
// (real pbcopy, real exec.LookPath, time.Now).
type strategistDeps struct {
	clip clipboardCopier
	look pathLooker
	now  func() time.Time
}

func (d strategistDeps) clipboard() clipboardCopier {
	if d.clip != nil {
		return d.clip
	}
	return pbcopyClip{}
}

func (d strategistDeps) lookPath() pathLooker {
	if d.look != nil {
		return d.look
	}
	return realLookPath{}
}

func (d strategistDeps) clock() func() time.Time {
	if d.now != nil {
		return d.now
	}
	return time.Now
}

// runStrategist dispatches `leah strategist <sub>`. Subcommand routing lives
// here, not in main.go, to keep the dispatch table in main.go a one-liner.
func runStrategist(ctx context.Context, args []string, w io.Writer) int {
	if shouldShowHelp(args) {
		printStrategistHelp(w)
		return 0
	}
	if len(args) == 0 {
		printStrategistHelp(os.Stderr)
		return 2
	}
	deps := strategistDeps{}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "post":
		_ = ctx // LLM call deferred — post is a pure store write today
		return runStrategistPost(deps, rest, w)
	case "next":
		return runStrategistNext(deps, rest, w)
	case "inbox":
		return runStrategistInbox(deps, rest, w)
	case "queue":
		return runStrategistQueue(deps, rest, w)
	case "doctor":
		return runStrategistDoctor(deps, w)
	default:
		_, _ = fmt.Fprintf(os.Stderr, "leah strategist: unknown subcommand %q\n", sub)
		printStrategistHelp(os.Stderr)
		return 2
	}
}

// runStrategistPost generates one post for a topic and copies the body to the
// clipboard. MVP stub — real LLM generation lands in the next PR. The body
// shape matches what the real generator will emit so downstream callers (the
// inbox listing, the kill flow) don't change when generation lands.
func runStrategistPost(deps strategistDeps, args []string, w io.Writer) int {
	if len(args) < 1 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah strategist post <topic>")
		return 2
	}
	topic := strings.Join(args, " ")

	root, err := strategistRoot()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah strategist post: %v\n", err)
		return 1
	}
	m, err := store.Open(root)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah strategist post: %v\n", err)
		return 1
	}

	// Persona load is best-effort here — the LLM call is stubbed, so the
	// persona body is unused. We still call Load so a missing persona.md
	// surfaces an early WARN (operator hint) rather than silent omission
	// when LLM wiring lands.
	configDir, _ := strategistConfigDir()
	if _, err := strategistpersona.Load(configDir); err != nil && errors.Is(err, os.ErrNotExist) {
		_, _ = fmt.Fprintln(os.Stderr, "warn: persona.md absent — using fallback voice")
	}

	now := deps.clock()()
	id := newULID(now)
	body := fmt.Sprintf("# %s\n\n[generated content placeholder]\n", topic)

	it := store.Item{
		Schema:  store.SchemaCurrent,
		ID:      id,
		Channel: "linkedin", // default; --channel flag lands when LLM wiring lands
		Slot:    "voice",    // topic-driven posts route through the voice slot
		Text:    body,
		Origin:  "topic:" + topic,
		Created: now,
	}
	// MVP writes to inbox/ — operator reviews before promoting to queue/. The
	// generator PR will route healthy items to queue/ and soft-fails to inbox/.
	if err := m.Inbox(it); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah strategist post: %v\n", err)
		return 1
	}
	if err := deps.clipboard().Copy(context.Background(), body); err != nil {
		// Clipboard failure is non-fatal — the item is on disk; operator can
		// still cat it. WARN-and-continue keeps the post recoverable.
		_, _ = fmt.Fprintf(os.Stderr, "warn: clipboard copy failed: %v\n", err)
	}
	_, _ = fmt.Fprintf(w, "%s inbox/%s.md (clipboard updated)\n", id, id)
	return 0
}

// runStrategistNext pops the oldest queued item, prints body + asset paths,
// then renames queue/<id>.md → sent/<id>.md. --peek prints without consuming.
func runStrategistNext(_ strategistDeps, args []string, w io.Writer) int {
	fs := flag.NewFlagSet("next", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	peek := fs.Bool("peek", false, "print the next item without consuming it")
	if err := fs.Parse(args); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah strategist next [--peek]")
		return 2
	}

	root, err := strategistRoot()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah strategist next: %v\n", err)
		return 1
	}
	m, err := store.Open(root)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah strategist next: %v\n", err)
		return 1
	}
	it, err := m.Next()
	if err != nil {
		if errors.Is(err, store.ErrEmptyQueue) {
			_, _ = fmt.Fprintln(os.Stderr, "queue empty")
			return 3
		}
		_, _ = fmt.Fprintf(os.Stderr, "leah strategist next: %v\n", err)
		return 1
	}
	renderItem(w, it)
	if *peek {
		return 0
	}
	if err := m.Sent(it.ID); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah strategist next: %v\n", err)
		return 1
	}
	return 0
}

// runStrategistInbox lists candidate items (filename + 1-line title) so the
// operator can decide what to promote with `inbox approve` (next PR).
func runStrategistInbox(_ strategistDeps, args []string, w io.Writer) int {
	if shouldShowHelp(args) {
		_, _ = fmt.Fprintln(w, "usage: leah strategist inbox")
		return 0
	}
	root, err := strategistRoot()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah strategist inbox: %v\n", err)
		return 1
	}
	m, err := store.Open(root)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah strategist inbox: %v\n", err)
		return 1
	}
	items, err := m.ListInbox()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah strategist inbox: %v\n", err)
		return 1
	}
	renderItemTable(w, items)
	return 0
}

// runStrategistQueue lists queued items, optionally capped by --peek N. The
// peek flag is a soft bound — the queue's full size is independent.
func runStrategistQueue(_ strategistDeps, args []string, w io.Writer) int {
	fs := flag.NewFlagSet("queue", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	peek := fs.Int("peek", 0, "show at most N items (0 = no cap)")
	if err := fs.Parse(args); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah strategist queue [--peek N]")
		return 2
	}
	root, err := strategistRoot()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah strategist queue: %v\n", err)
		return 1
	}
	m, err := store.Open(root)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah strategist queue: %v\n", err)
		return 1
	}
	items, err := m.ListQueue()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah strategist queue: %v\n", err)
		return 1
	}
	if *peek > 0 && len(items) > *peek {
		items = items[:*peek]
	}
	renderItemTable(w, items)
	return 0
}

// runStrategistDoctor probes the five readiness signals (persona, ffmpeg,
// magick, higgsfield token, openai token) and prints one line per probe.
// Non-zero exit when ANY probe fails — the operator can grep "MISSING" to
// know what to install.
func runStrategistDoctor(deps strategistDeps, w io.Writer) int {
	configDir, _ := strategistConfigDir()
	personaPath := filepath.Join(configDir, "strategist", "persona.md")
	personaOK := fileExists(personaPath)

	look := deps.lookPath()
	_, ffmpegErr := look.Look("ffmpeg")
	_, magickErr := look.Look("magick")
	ffmpegOK := ffmpegErr == nil
	magickOK := magickErr == nil

	hfOK := fileExists(connect.DefaultTokenPath("higgsfield"))
	oaOK := fileExists(connect.DefaultTokenPath("openai"))

	probes := []struct {
		label string
		ok    bool
		hint  string
	}{
		{"persona", personaOK, "create " + personaPath},
		{"ffmpeg", ffmpegOK, "brew install ffmpeg"},
		{"magick", magickOK, "brew install imagemagick"},
		{"higgsfield", hfOK, "leah connect higgsfield"},
		{"openai", oaOK, "leah connect openai"},
	}
	allOK := true
	for _, p := range probes {
		if p.ok {
			_, _ = fmt.Fprintf(w, "  OK   %-12s\n", p.label)
			continue
		}
		allOK = false
		_, _ = fmt.Fprintf(w, "  MISS %-12s — %s\n", p.label, p.hint)
	}
	if !allOK {
		return 1
	}
	return 0
}

func renderItem(w io.Writer, it store.Item) {
	_, _ = fmt.Fprintf(w, "id:      %s\n", it.ID)
	_, _ = fmt.Fprintf(w, "channel: %s\n", it.Channel)
	_, _ = fmt.Fprintf(w, "slot:    %s\n", it.Slot)
	_, _ = fmt.Fprintf(w, "origin:  %s\n", it.Origin)
	if it.ImageRef != "" {
		_, _ = fmt.Fprintf(w, "image:   %s\n", it.ImageRef)
	}
	if it.ClipRef != "" {
		_, _ = fmt.Fprintf(w, "clip:    %s\n", it.ClipRef)
	}
	_, _ = fmt.Fprintln(w, "---")
	_, _ = fmt.Fprintln(w, strings.TrimRight(it.Text, "\n"))
}

func renderItemTable(w io.Writer, items []store.Item) {
	if len(items) == 0 {
		_, _ = fmt.Fprintln(w, "(none)")
		return
	}
	for _, it := range items {
		_, _ = fmt.Fprintf(w, "%s  %-10s %-7s %s\n", it.ID, it.Channel, it.Slot, firstLine(it.Text))
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// strategistRoot resolves <configDir>/strategist where queue/inbox/sent/killed
// live. configDir is XDG-ish — env override first, then ~/.config/leah/.
func strategistRoot() (string, error) {
	cfg, err := strategistConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, "strategist"), nil
}

func strategistConfigDir() (string, error) {
	if d := os.Getenv("LEAH_CONFIG_DIR"); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".config", "leah"), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// newULID returns a lexicographically sortable ID. The strategist package
// docs reference ULIDs; we use a simplified 26-char Crockford-base32 encoding
// of timestamp + random tail. The store package only requires non-empty +
// sort-stable, so re-pulling the oklog/ulid dependency just for the CLI is
// unwarranted churn.
func newULID(t time.Time) string {
	const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	ms := uint64(t.UnixMilli())
	var b [26]byte
	// 10 chars of timestamp (high bits first).
	for i := 9; i >= 0; i-- {
		b[i] = alphabet[ms&0x1f]
		ms >>= 5
	}
	// 16 chars of randomness. crypto/rand is overkill for a sort key but
	// keeps collisions vanishingly unlikely under multi-second invocations.
	max := big.NewInt(32)
	for i := 10; i < 26; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			// Random source must never fail in practice; if it does, the
			// caller's whole environment is broken and we surface a fixed
			// fallback rather than crashing.
			b[i] = alphabet[i%32]
			continue
		}
		b[i] = alphabet[n.Int64()]
	}
	return string(b[:])
}

// pbcopyClip pipes the body to `pbcopy` via stdin. The body is NEVER
// interpolated into the command string — exec.Command takes argv as a slice,
// so shell metacharacters in the topic cannot influence execution.
type pbcopyClip struct{}

func (pbcopyClip) Copy(ctx context.Context, body string) error {
	cmd := exec.CommandContext(ctx, "pbcopy")
	cmd.Stdin = strings.NewReader(body)
	return cmd.Run()
}

// realLookPath is the production pathLooker — wraps exec.LookPath verbatim.
type realLookPath struct{}

func (realLookPath) Look(name string) (string, error) { return exec.LookPath(name) }

func printStrategistHelp(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: leah strategist <post|next|inbox|queue|doctor> [args...]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "  post <topic>          generate one post (MVP stub) → inbox/, copy body to clipboard")
	_, _ = fmt.Fprintln(w, "  next [--peek]         pop oldest queued, print + mark sent (or --peek to view only)")
	_, _ = fmt.Fprintln(w, "  inbox                 list pending candidate items")
	_, _ = fmt.Fprintln(w, "  queue [--peek N]      list queued items (up to N)")
	_, _ = fmt.Fprintln(w, "  doctor                check persona + ffmpeg + magick + higgsfield/openai tokens")
}
