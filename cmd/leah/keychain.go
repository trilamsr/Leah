package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/trilam/leah/internal/keychain"
)

// runKeychain is `leah keychain <set|get|delete> <service>`. Reads the
// secret from stdin (not argv) to avoid shell-history leak.
func runKeychain(_ context.Context, args []string, stdin io.Reader, stdout io.Writer) int {
	if shouldShowHelp(args) || len(args) < 2 {
		printKeychainUsage(stdout)
		return 0
	}
	op, name := args[0], args[1]
	service, account, err := resolveKeychainSlot(name, args[2:])
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah keychain: %v\n", err)
		return 2
	}
	switch op {
	case "set":
		secret, err := readSecret(stdin)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah keychain set: %v\n", err)
			return 1
		}
		if err := keychain.Save(service, account, secret); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah keychain set: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "ok: saved %s/%s\n", service, account)
		return 0
	case "get":
		v, err := keychain.Load(service, account)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah keychain get: %v\n", err)
			return 1
		}
		if v == "" {
			_, _ = fmt.Fprintf(os.Stderr, "not found: %s/%s\n", service, account)
			return 1
		}
		_, _ = fmt.Fprintln(stdout, v)
		return 0
	case "delete":
		if err := keychain.Delete(service, account); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah keychain delete: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "ok: deleted %s/%s\n", service, account)
		return 0
	default:
		printKeychainUsage(os.Stderr)
		return 2
	}
}

// resolveKeychainSlot maps a shorthand name (anthropic, elevenlabs, voyage,
// openai, pushover-user, pushover-token) OR a raw service string to
// (service, account). Extra args allow overriding the account when using
// a raw service string.
func resolveKeychainSlot(name string, rest []string) (string, string, error) {
	switch name {
	case "anthropic":
		return keychain.AnthropicService, keychain.DefaultAccount, nil
	case "elevenlabs":
		return keychain.ElevenLabsService, keychain.DefaultAccount, nil
	case "voyage":
		return keychain.VoyageService, keychain.DefaultAccount, nil
	case "openai":
		return keychain.OpenAIService, keychain.DefaultAccount, nil
	case "pushover-user":
		return keychain.PushoverService, keychain.PushoverUserAccount, nil
	case "pushover-token":
		return keychain.PushoverService, keychain.PushoverTokenAccount, nil
	}
	// Raw service form: `leah keychain set <service> [account]`
	if strings.Contains(name, ".") {
		account := keychain.DefaultAccount
		if len(rest) > 0 && rest[0] != "" {
			account = rest[0]
		}
		return name, account, nil
	}
	return "", "", fmt.Errorf("unknown service %q (try anthropic|elevenlabs|voyage|openai|pushover-user|pushover-token)", name)
}

// readSecret pulls the secret from stdin. Trims trailing newline only —
// leading/embedded whitespace is preserved (some tokens contain them).
func readSecret(r io.Reader) (string, error) {
	// Prompt only when the input is an interactive tty; scripts pipe raw.
	if f, ok := r.(*os.File); ok {
		if fi, err := f.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) != 0 {
			_, _ = fmt.Fprint(os.Stderr, "secret: ")
		}
	}
	br := bufio.NewReader(r)
	line, err := br.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return "", fmt.Errorf("empty secret")
	}
	return line, nil
}

func printKeychainUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, `usage:
  leah keychain set <service>         # secret read from stdin
  leah keychain get <service>
  leah keychain delete <service>

services: anthropic | elevenlabs | voyage | openai | pushover-user | pushover-token
Or pass a raw slot name (dot-separated) with an optional account:
  leah keychain set com.example.foo bar-account`)
}
