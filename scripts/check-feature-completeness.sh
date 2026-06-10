#!/usr/bin/env bash
# V6/W91 placeholder-detection gate: catch `panic("not implemented")` /
# TODO-in-public-func smell in NEW feat/ branches. Honest "explicitly deferred"
# panics with a named sentinel error (see ErrUplinkNotShipped in
# internal/voice/listener/openai_realtime_decoder.go) pass.
#
# Coverage (reviewer-driven, PR#160):
#   - panic("not implemented" | "not yet implemented" | "not_implemented" |
#     "unimplemented" | "TODO" | "todo") incl. case-insensitive variants.
#   - panic(errors.New("...")) and panic(fmt.Errorf("...")) wrappers.
#   - Method-receiver funcs `func (r *R) PublicMethod() { panic(...) }`.
#   - Multi-line public func signatures where panic(placeholder) appears in
#     the body window (next 20 lines after the signature opens).
#   - return errors.New("TODO" | "FIXME") in non-test files.
set -euo pipefail

branch=$(git rev-parse --abbrev-ref HEAD)
case "$branch" in
  feat/*) ;;
  *) exit 0 ;;
esac

# Single placeholder-keyword alternation used by every regex below.
# `not[ _]?yet[ _]?implemented` covers "not implemented", "not yet implemented",
# "not_implemented", "notimplemented" with optional separators.
placeholder_re='([Nn]ot[ _]?([Yy]et[ _]?)?[Ii]mplemented|[Uu]nimplemented|TODO|todo|FIXME)'

failed=0
while IFS= read -r f; do
  [ -z "$f" ] && continue
  [ -f "$f" ] || continue

  # Rule 1: panic with a placeholder string literal or wrapped placeholder error.
  # Three forms:
  #   panic("...placeholder...")
  #   panic(errors.New("...placeholder..."))
  #   panic(fmt.Errorf("...placeholder..."))
  if grep -nE "panic\\((errors\\.New\\(|fmt\\.Errorf\\()?\"[^\"]*${placeholder_re}[^\"]*\"" "$f"; then
    echo "ERROR: $f contains panic with placeholder string" >&2
    echo "Fix: replace with named sentinel error (see ErrUplinkNotShipped pattern in internal/voice/listener/openai_realtime_decoder.go)" >&2
    failed=1
  fi

  # Rule 2: return errors.New("...placeholder...") / fmt.Errorf("...placeholder...")
  # in non-test code is a placeholder ship.
  if grep -nE "return (errors\\.New|fmt\\.Errorf)\\(\"[^\"]*${placeholder_re}[^\"]*\"" "$f"; then
    echo "ERROR: $f returns a placeholder-string error" >&2
    echo "Fix: define a package-level sentinel (var ErrXxx = errors.New(...)) and return it" >&2
    failed=1
  fi

  # Rule 3: public func declaration with TODO/FIXME/XXX in the signature line.
  # Matches BOTH `func PublicName(` AND `func (r *R) PublicName(` — the canonical
  # PR#146 case (`func (d *Decoder) SendAudio(...)`) is a method receiver.
  # Multi-line signatures are caught by Rule 4 below.
  if grep -nE '^func( \([^)]+\))? [A-Z][[:alnum:]_]+' "$f" | grep -qE 'TODO|FIXME|XXX'; then
    echo "ERROR: $f public function declaration has TODO/FIXME/XXX placeholder" >&2
    failed=1
  fi

  # Rule 4: multi-line public func signature followed by a placeholder panic
  # within the next 20 lines. Catches:
  #   func (r *R) Multi(
  #       a int,
  #   ) error {
  #       panic("not implemented")
  #   }
  # awk window walks the file once; no per-line subshell.
  if awk -v re="$placeholder_re" '
    /^func( \([^)]+\))? [A-Z][[:alnum:]_]+/ { sig_line=NR; in_window=20 }
    in_window > 0 {
      if (NR > sig_line && match($0, "panic\\((errors\\.New\\(|fmt\\.Errorf\\()?\"[^\"]*" re "[^\"]*\"")) {
        print FILENAME ":" NR ": " $0
        found=1
        in_window=0
      }
      in_window--
    }
    END { exit found ? 0 : 1 }
  ' "$f"; then
    echo "ERROR: $f public function body contains placeholder panic" >&2
    echo "Fix: replace with named sentinel error (see ErrUplinkNotShipped pattern in internal/voice/listener/openai_realtime_decoder.go)" >&2
    failed=1
  fi
done < <(git diff --name-only origin/main...HEAD -- '*.go' | grep -vE '_test\.go$' || true)

exit $failed
