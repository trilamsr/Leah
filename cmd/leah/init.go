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
	"strings"

	"github.com/trilam/leah/internal/connect"
)

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
		fmt.Fprintln(w, "usage: leah init [--force]")
		fmt.Fprintln(w, "first-launch wizard: installs the macos-integration daemon plist + walks adapter setup")
		return 0
	}
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	force := fs.Bool("force", false, "re-run even if marker file exists")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(w, "usage: leah init [--force]")
		return 2
	}

	marker := filepath.Join(stateDir(), "init.done")
	if !*force {
		if _, err := os.Stat(marker); err == nil {
			fmt.Fprintln(w, "leah init: already initialized (--force to re-run)")
			return 0
		}
	}

	fmt.Fprintln(w, "Welcome to Leah — personal AI chief-of-staff.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Step 1: macos-integration daemon")

	if err := installPlist(w); err != nil {
		fmt.Fprintf(w, "  plist install failed: %v\n", err)
		return 1
	}
	if os.Getenv("LEAH_INIT_SKIP_LAUNCHCTL") != "1" {
		if err := loadLaunchAgent(ctx, w); err != nil {
			fmt.Fprintf(w, "  launchctl load failed (non-fatal): %v\n", err)
		}
	}

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Step 2: integrations")
	promptAdapters(ctx, w, in)

	if err := os.WriteFile(marker, []byte("ok\n"), 0o600); err != nil {
		fmt.Fprintf(w, "  marker write failed: %v\n", err)
		return 1
	}
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Done. Run `leah connect <adapter>` anytime to add more.")
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
		fmt.Fprintf(w, "  plist already present at %s\n", path)
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
	fmt.Fprintf(w, "  wrote %s\n", path)
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
	fmt.Fprintln(w, "  launchctl: loaded com.leah.daemon")
	return nil
}

func promptAdapters(ctx context.Context, w io.Writer, in io.Reader) {
	reg := connect.DefaultRegistry()
	auto := os.Getenv("LEAH_INIT_AUTO_ACCEPT") == "1"
	r := bufio.NewReader(in)
	for _, s := range reg.List() {
		state := "not authorized"
		if s.Authorized {
			state = "authorized"
		}
		fmt.Fprintf(w, "  - %s (%s)\n", s.Name, state)
		if s.Authorized {
			continue
		}
		if auto {
			continue
		}
		fmt.Fprintf(w, "    connect %s now? [y/N] ", s.Name)
		line, _ := r.ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			code := runConnect(ctx, []string{s.Name}, w)
			if code != 0 {
				fmt.Fprintf(w, "    (skipped — re-run later with `leah connect %s`)\n", s.Name)
			}
		}
	}
}
