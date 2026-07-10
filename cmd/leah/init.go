package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/trilam/leah/internal/actions/connect"
	"github.com/trilam/leah/internal/platform/keychain"
)

// anthropicKeyRE mirrors the phase-5 design regex
// (docs/superpowers/designs/2026-06-23-leah-phase5-design.md:2391). Enforced
// here so a typo answer to the wizard prompt (e.g. "y") can't get persisted
// as the API key and silently break later `leah ask` invocations with a 401.
var anthropicKeyRE = regexp.MustCompile(`^sk-ant-[A-Za-z0-9\-_]{40,}$`)

// plistTemplate mirrors scripts/leah.plist but with the canonical
// com.leah.daemon label this wizard installs. Kept inline (vs embed) so the
// init package stays self-contained — operator only needs the binary.
const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.leah.daemon</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{LEAH_BIN}}</string>
        <string>listen</string>
    </array>
    <key>KeepAlive</key>
    <true/>
    <key>RunAtLoad</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/leah.stdout.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/leah.stderr.log</string>
</dict>
</plist>
`

func runInit(ctx context.Context, args []string, w io.Writer, in io.Reader) int {
	if shouldShowHelp(args) {
		_, _ = fmt.Fprintln(w, "usage: leah init [--force]")
		_, _ = fmt.Fprintln(w, "first-launch wizard: installs the macos-integration daemon plist + walks adapter setup")
		return 0
	}
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	force := fs.Bool("force", false, "re-run even if marker file exists")
	if err := fs.Parse(args); err != nil {
		_, _ = fmt.Fprintln(w, "usage: leah init [--force]")
		return 2
	}

	marker := filepath.Join(stateDir(), "init.done")
	if !*force {
		if _, err := os.Stat(marker); err == nil {
			_, _ = fmt.Fprintln(w, "leah init: already initialized (--force to re-run)")
			return 0
		}
	}

	// One shared bufio.Reader across steps — a per-step reader would eat
	// read-ahead bytes from the underlying io.Reader that later steps then
	// never see (interactive test feeds one "y\n" and expects step 3's
	// adapter prompt to observe it).
	r := bufio.NewReader(in)

	_, _ = fmt.Fprintln(w, "Welcome to Leah — personal chief-of-staff.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Step 1: API key")
	promptAnthropicKey(w, r)

	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Step 2: macos-integration daemon")

	if err := installPlist(w); err != nil {
		_, _ = fmt.Fprintf(w, "  plist install failed — ensure ~/Library/LaunchAgents is writable (%v)\n", err)
		return 1
	}
	if os.Getenv("LEAH_INIT_SKIP_LAUNCHCTL") != "1" {
		if err := loadLaunchAgent(ctx, w); err != nil {
			_, _ = fmt.Fprintf(w, "  launchctl load failed (non-fatal): %v\n", err)
		}
	}

	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Step 3: integrations")
	promptAdapters(ctx, w, r)

	if err := os.WriteFile(marker, []byte("ok\n"), 0o600); err != nil {
		_, _ = fmt.Fprintf(w, "  marker write failed: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Done. To add integrations (Gmail, Slack, etc), run `leah connect --list`")
	return 0
}

func installPlist(w io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "com.leah.daemon.plist")
	if _, err := os.Stat(path); err == nil {
		_, _ = fmt.Fprintf(w, "  plist already present at %s\n", path)
		return nil
	}
	bin, err := os.Executable()
	if err != nil || bin == "" {
		bin = "/usr/local/bin/leah"
	}
	body := strings.ReplaceAll(plistTemplate, "{{LEAH_BIN}}", bin)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "  wrote %s\n", path)
	return nil
}

// loadLaunchAgent is best-effort — many envs (CI, headless) lack launchctl
// privileges and we already printed the plist location so the operator can
// finish by hand.
func loadLaunchAgent(ctx context.Context, w io.Writer) error {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, "Library", "LaunchAgents", "com.leah.daemon.plist")
	cmd := exec.CommandContext(ctx, "launchctl", "load", "-w", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	_, _ = fmt.Fprintln(w, "  launchctl: loaded com.leah.daemon")
	return nil
}

// promptAnthropicKey stores ANTHROPIC_API_KEY into the macOS Keychain slot
// (keychain.AnthropicService / DefaultAccount) that the rest of the codebase
// reads via keychain.LoadAnthropicKey(). Env-var-set + Keychain-hit both mean
// "operator already has a key" and short-circuit. LEAH_INIT_AUTO_ACCEPT=1
// skips the prompt for headless runs.
func promptAnthropicKey(w io.Writer, r *bufio.Reader) {
	if strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")) != "" {
		_, _ = fmt.Fprintln(w, "  ANTHROPIC_API_KEY already set in environment — skipping.")
		return
	}
	if k, err := keychain.Load(keychain.AnthropicService, keychain.DefaultAccount); err == nil && k != "" {
		_, _ = fmt.Fprintln(w, "  Anthropic key already in Keychain — skipping.")
		return
	}
	if os.Getenv("LEAH_INIT_AUTO_ACCEPT") == "1" {
		_, _ = fmt.Fprintln(w, "  (auto-accept: skipping key prompt)")
		return
	}
	_, _ = fmt.Fprint(w, "  Paste ANTHROPIC_API_KEY (leave blank to skip): ")
	line, err := r.ReadString('\n')
	if err == io.EOF && line == "" {
		_, _ = fmt.Fprintln(w, "  (stdin closed — skipping key step)")
		return
	}
	if err != nil && err != io.EOF {
		_, _ = fmt.Fprintf(w, "  (input error: %v — skipping key step)\n", err)
		return
	}
	key := strings.TrimSpace(line)
	if key == "" {
		_, _ = fmt.Fprintln(w, "  (skipped — set ANTHROPIC_API_KEY later or re-run `leah init --force`)")
		return
	}
	if !anthropicKeyRE.MatchString(key) {
		_, _ = fmt.Fprintln(w, "  (input does not look like an API key `sk-ant-…` — skipping)")
		return
	}
	if err := keychain.Save(keychain.AnthropicService, keychain.DefaultAccount, key); err != nil {
		_, _ = fmt.Fprintf(w, "  Keychain save failed (%v) — set ANTHROPIC_API_KEY in your shell rc instead\n", err)
		return
	}
	_, _ = fmt.Fprintln(w, "  stored in Keychain.")
}

func promptAdapters(ctx context.Context, w io.Writer, r *bufio.Reader) {
	reg := connect.DefaultRegistry()
	auto := os.Getenv("LEAH_INIT_AUTO_ACCEPT") == "1"
	for _, s := range reg.List() {
		state := "not authorized"
		if s.Authorized {
			state = "authorized"
		}
		_, _ = fmt.Fprintf(w, "  - %s (%s)\n", s.Name, state)
		if s.Authorized {
			continue
		}
		if auto {
			continue
		}
		_, _ = fmt.Fprintf(w, "    connect %s now? [y/N] ", s.Name)
		line, err := r.ReadString('\n')
		// EOF = closed stdin (piped / headless install). Treat as "skip rest" —
		// re-prompting on a dead reader would spin. Other read errors are
		// per-adapter soft-fails so a transient stdin glitch can't strand the
		// wizard mid-loop.
		if err == io.EOF {
			_, _ = fmt.Fprintln(w, "    (stdin closed — skipping remaining prompts)")
			return
		}
		if err != nil {
			_, _ = fmt.Fprintf(w, "    (input error: %v — skipping %s)\n", err, s.Name)
			continue
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			if code := runConnect(ctx, []string{s.Name}, w); code != 0 {
				_, _ = fmt.Fprintf(w, "    (skipped — re-run later with `leah connect %s`)\n", s.Name)
			}
		}
	}
}
