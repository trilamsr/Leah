# reviewer-verdict/path-classifier.sh — flag PR as load-bearing when any
# changed path hits leah's load-bearing surface list. Sets
# LOAD_BEARING_BY_PATH (0/1) and upgrades LOAD_BEARING when matched.
#
# Originated in regatta; ported to leah 2026-06-10. The load-bearing
# surfaces are the leah equivalents: every internal/<pkg>/ that ships
# user-visible behavior, every cmd/leah* entrypoint, all engineer prose
# surfaces (specs/briefs/dispatch-templates/autonomous-session-prompt),
# CLAUDE.md, every scripts/check-*.sh gate, and every CI workflow.

rv_classify_paths() {
  LOAD_BEARING_BY_PATH=0
  if [ -z "$PATHS_FILE" ]; then
    return 0
  fi
  if [ ! -f "$PATHS_FILE" ]; then
    echo "check-reviewer-verdict: --changed-paths-file $PATHS_FILE not found" >&2
    exit 3
  fi
  while IFS= read -r changed_path; do
    [ -z "$changed_path" ] && continue
    case "$changed_path" in
      internal/adapters/*|internal/audit/*|internal/backup/*|internal/brief/*|internal/budget/*|internal/costview/*|internal/ctxmgr/*|internal/daemonloop/*|internal/dispatcher/*|internal/embed/*|internal/ghclient/*|internal/intent/*|internal/memory/*|internal/notify/*|internal/obs/*|internal/operatormodel/*|internal/patterns/*|internal/persona/*|internal/reasoner/*|internal/regattaclient/*|internal/reviewer/*|internal/selflearn/*|internal/testutil/*|internal/voice/*|internal/watchdog/*|internal/web/*)
        LOAD_BEARING_BY_PATH=1
        break
        ;;
      cmd/leah/*|cmd/leah-daemon/*|cmd/leah-hud/*)
        LOAD_BEARING_BY_PATH=1
        break
        ;;
      docs/engineer/specs/*.md|docs/engineer/dispatch-templates/*.md|docs/engineer/briefs/*.md|docs/engineer/autonomous-session-prompt.md|CLAUDE.md)
        LOAD_BEARING_BY_PATH=1
        break
        ;;
      scripts/check-*.sh|.github/workflows/*)
        LOAD_BEARING_BY_PATH=1
        break
        ;;
    esac
  done < "$PATHS_FILE"
  if [ "$LOAD_BEARING_BY_PATH" -eq 1 ]; then
    LOAD_BEARING=1
  fi
}
