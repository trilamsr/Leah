package main

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ParseSince consumes `--since <RFC3339>` / `--since=<RFC3339>` from args.
// Returns the parsed cursor (zero time = absent), remaining args, and any
// parse error. Repeats and bare `--since` reject with a usage-shaped error
// so recall/brief/news share one cursor surface — same exit-2 reason for the
// operator across surfaces.
func ParseSince(args []string) (time.Time, []string, error) {
	rest := make([]string, 0, len(args))
	seen := false
	var raw string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--since":
			if seen {
				return time.Time{}, nil, errors.New("--since may only be specified once")
			}
			if i+1 >= len(args) {
				return time.Time{}, nil, errors.New("--since requires a value")
			}
			raw = args[i+1]
			seen = true
			i++
		case strings.HasPrefix(a, "--since="):
			if seen {
				return time.Time{}, nil, errors.New("--since may only be specified once")
			}
			raw = strings.TrimPrefix(a, "--since=")
			if raw == "" {
				return time.Time{}, nil, errors.New("--since requires a value")
			}
			seen = true
		default:
			rest = append(rest, a)
		}
	}
	if !seen {
		return time.Time{}, rest, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("--since: invalid RFC3339 %q: %w", raw, err)
	}
	return t, rest, nil
}
