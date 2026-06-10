#!/usr/bin/env bash
# check-no-bare-sleep.sh - reject `time.Sleep` lexically nested inside a `for`
# block in any `*_test.go` file. Forces migration to state-driven settle
# (testutil.Eventually), which fails fast on timeout, short-circuits on
# success, and never burns a fixed wall-clock budget.
#
# Why: bare polling Sleeps are the dominant flake source — slow CI runners
# overshoot the settle window, fast runners pad it. They also hide the
# observable predicate from the failing-test record.
#
# Excludes:
#   - filenames ending `_bench_test.go` (benchmarks measure wall-clock by design)
#   - lines carrying `// allow-sleep: <reason>` directive
#
# Annotation contract for `// allow-sleep: <reason>`:
#   - MUST appear as a LINE-TRAILING comment on the SAME LINE as the
#     `time.Sleep(...)` call. Preceding-line comments do NOT count. Block
#     comments (`/* allow-sleep: ... */`) do NOT count — only `//` form.
#   - Reason text is free-form but should be one short clause naming the
#     non-polling property (wall-clock observation, mtime resolution,
#     goroutine-leak settle, poll-interval inside Eventually helper, etc).
#
#   GOOD:  time.Sleep(20 * time.Millisecond) // allow-sleep: mtime resolution on darwin
#   BAD:   // allow-sleep: mtime resolution
#          time.Sleep(20 * time.Millisecond)
#   BAD:   /* allow-sleep: mtime resolution */ time.Sleep(20 * time.Millisecond)
#
# Scope: every `*_test.go` file under TESTS_ROOT (defaults to repo root).
# Exit: 0 clean, 1 on first violation (lists every hit before exit).

set -uo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd -- "$SCRIPT_DIR/.." && pwd)

: "${TESTS_ROOT:=$REPO_ROOT}"

if [ ! -d "$TESTS_ROOT" ]; then
  echo "check-no-bare-sleep: TESTS_ROOT not found: $TESTS_ROOT (skipping)"
  exit 0
fi

scan_file() {
  perl - "$1" <<'PERL'
use strict;
use warnings;

my $path = shift @ARGV;
open(my $fh, '<', $path) or die "open $path: $!";

my $depth = 0;
my @for_stack = ();
my $lineno = 0;
my $in_block_comment = 0;

while (my $raw = <$fh>) {
    $lineno++;
    chomp(my $line = $raw);

    my $scan = $line;

    $scan =~ s{/\*.*?\*/}{}g;

    if ($in_block_comment) {
        if ($scan =~ s{^.*?\*/}{}) {
            $in_block_comment = 0;
        } else {
            next;
        }
    }
    if ($scan =~ s{/\*.*$}{}) {
        $in_block_comment = 1;
    }

    # Capture directive BEFORE stripping line comments.
    my $allow = ($line =~ m{//\s*allow-sleep:});

    $scan =~ s{//.*$}{};

    $scan =~ s{"(?:\\.|[^"\\])*"}{""}g;
    $scan =~ s{`[^`]*`}{``}g;

    my $for_opens_here = 0;
    if ($scan =~ /\bfor\b/) {
        if ($scan =~ /\bfor\b[^{}]*\{/) {
            $for_opens_here = 1;
        }
    }

    my $opens  = () = $scan =~ /\{/g;
    my $closes = () = $scan =~ /\}/g;

    $depth += $opens;

    if ($for_opens_here) {
        push @for_stack, $depth;
    }

    if (@for_stack && $scan =~ /\btime\.Sleep\s*\(/) {
        unless ($allow) {
            print "$path:$lineno: time.Sleep inside for-loop — migrate to testutil.Eventually, or annotate `// allow-sleep: <reason>` if legitimately non-polling\n";
        }
    }

    $depth -= $closes;
    if ($depth < 0) { $depth = 0; }

    while (@for_stack && $for_stack[-1] > $depth) {
        pop @for_stack;
    }
}

close($fh);
PERL
}

scanned=0
violations=0
violation_lines=""

while IFS= read -r -d '' f; do
  base=$(basename -- "$f")
  case "$base" in
    *_bench_test.go) continue ;;
  esac
  scanned=$((scanned + 1))
  hits=$(scan_file "$f")
  if [ -n "$hits" ]; then
    violation_lines="${violation_lines}${hits}"$'\n'
    n=$(printf '%s\n' "$hits" | grep -c .)
    violations=$((violations + n))
  fi
# Exclusions use the `-prune` form so they skip the matching directory
# wherever it appears beneath TESTS_ROOT, regardless of whether TESTS_ROOT
# itself sits inside one of the excluded paths. The bare `-not -path '*/X/*'`
# form filters by absolute-path substring, which silently skips the ENTIRE
# scan when TESTS_ROOT itself contains an excluded segment (e.g. invoking
# this script from `.claude/worktrees/<name>/` — the worktree root path
# matches `*/.claude/worktrees/*` so every descendant is rejected, scan
# count is 0, and the gate returns a false PASS).
done < <(find "$TESTS_ROOT" \
  \( -type d \( \
       -name vendor \
       -o -path '*/.git' \
       -o -path '*/.claude/worktrees' \
       -o -path '*/scripts/testdata' \
     \) -prune \) \
  -o \( -type f -name '*_test.go' -print0 \))

if [ "$violations" -gt 0 ]; then
  echo "check-no-bare-sleep: $violations bare-Sleep-in-for-loop violation(s) across $scanned test file(s):"
  printf '%s' "$violation_lines" | sed 's/^/  - /'
  echo
  echo "Polling sleeps are flaky on CI (slow runners overshoot, fast runners pad)."
  echo "Migrate to state-driven settle via internal/testutil.Eventually."
  echo "If the Sleep is legitimately non-polling (wall-clock observation,"
  echo "mtime resolution, goroutine-leak settle), annotate with a reason:"
  echo "  time.Sleep(d) // allow-sleep: <one-line reason>"
  exit 1
fi

echo "check-no-bare-sleep: $scanned test file(s) scanned; no bare Sleep-in-for violations"
exit 0
