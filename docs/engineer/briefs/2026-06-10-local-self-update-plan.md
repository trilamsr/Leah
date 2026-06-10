# Local Self-Update — W81–W84 Plan

**Spec:** `docs/engineer/specs/2026-06-10-local-self-update.md`
**Date:** 2026-06-10
**Total scope:** four waves, ≤1200 LOC across all four, no PR exceeds 400 LOC.

Each wave is a single PR. Reviewer pass-through is mandatory per CLAUDE.md.

---

## W81 — Makefile targets + binary layout + lockfile

**Goal:** Establish `~/.leah-state/bin/` layout, `make install`, `make upgrade`, and the `flock(2)` primitive. No daemon restart yet; no attestation yet; no CLI subcommand yet.

**Files touched:**
- `Makefile` — add `install`, `upgrade`, `upgrade-dry-run` targets.
- `scripts/self-upgrade.sh` — single-purpose shell that does build + symlink swap (callable from Makefile and later from `leah self-upgrade`).
- `scripts/flock-helper.sh` — wrapper around `flock(2)` (macOS lacks `flock` CLI by default — use `/usr/bin/lockf` or a small Go helper at `cmd/leah-lock/main.go`).
- `cmd/leah-lock/main.go` — ≤30 LOC; wraps `syscall.Flock` so we don't depend on a Homebrew-installed `flock`.
- `docs/engineer/specs/2026-06-10-local-self-update.md` — link from `README.md` "Development" section.

**Risk:** macOS `mv` on a symlink across the same directory is atomic; verified via `rename(2)` semantics. Validate on APFS (default) and document the assumption.

**Size:** ~250 LOC (mostly shell + one tiny Go helper).

**Test plan:**
- `TestLockfile_SerializesUpgrades` — two `cmd/leah-lock` invocations against same path; assert second exits non-zero immediately.
- `scripts/self-upgrade.sh --dry-run` exits 0 and prints intended actions without mutating `~/.leah-state/`.
- `make upgrade-dry-run` in a fresh clone succeeds.

**Unblocks:** W82 (version embedding has somewhere to land), W83 (CLI subcommand reuses `scripts/self-upgrade.sh`).

---

## W82 — Version embedding + `--version` flag

**Goal:** `leah --version` and `leah-daemon --version` print `leah <semver> (commit: <sha>; built: <date>)`. Daemon `/health` payload gains `commit_sha` and `built_at`.

**Files touched:**
- `cmd/leah/version.go` — declare `var commit, builtAt string`; print on `--version`.
- `cmd/leah-daemon/version.go` — same.
- `cmd/leah-daemon/health.go` — extend `/health` JSON with new fields.
- `Makefile` — already builds with `-ldflags`; verify `make build` wires `$SHA` and `$DATE`.
- `cmd/leah/version_test.go`, `cmd/leah-daemon/health_test.go`.

**Risk:** None — pure additive. `version` package boundary is well-isolated.

**Size:** ~120 LOC.

**Test plan:**
- `TestVersion_PrintsEmbeddedCommit` — build with `-X main.commit=DEADBEEF`; exec the binary with `--version`; assert output contains `DEADBEEF`.
- `TestHealth_IncludesCommitSha` — start daemon; GET `/health`; assert JSON has non-empty `commit_sha`.

**Unblocks:** W83 (CLI needs to read current SHA for audit row), HUD ambient SHA badge (separate wave, downstream).

---

## W83 — `leah self-upgrade` CLI + attestation + audit row + restart hook

**Goal:** Operator-attested `leah self-upgrade [--rollback|--recover|--status|--dry-run]` callable from any cwd. Daemon graceful restart wired via `signal.NotifyContext`.

**Files touched:**
- `cmd/leah/cmd/selfupgrade.go` — Cobra subcommand; calls `scripts/self-upgrade.sh` via `os/exec`.
- `internal/selfupgrade/upgrader.go` — `Upgrader` struct; `Apply`, `Rollback`, `Recover`, `Status` methods.
- `internal/selfupgrade/restart.go` — daemon PID read, SIGTERM, 10s grace, SIGKILL escalation, relaunch via `nohup`-equivalent (`syscall.Setpgid` + detach).
- `internal/selfupgrade/audit.go` — emit audit row to the existing audit sink (per audit spec).
- `internal/attest/scopes.go` — register `self_upgrade:apply`, `self_upgrade:rollback`, `self_upgrade:recover`.
- `internal/selfupgrade/upgrader_test.go` — covers attestation-denied, lockfile-contention, force-kill, partial-swap recovery.

**Risk:** Restarting the daemon from a child of the daemon itself is tricky. The CLI must NOT be invoked by the daemon; it shells out via `os/exec.Command`, the new process becomes the parent, sends SIGTERM, then `Setsid` + `Setpgid` to detach the relaunched daemon. Document the process-tree dance in `restart.go` godoc (1 line).

**Size:** ~350 LOC.

**Test plan:** All eight tests from spec §11 land in this wave.

**Unblocks:** W84 (brew tap docs reference `leah self-upgrade` as the supported in-process update path).

---

## W84 — Brew tap repo + formula + rollback documentation

**Goal:** Document and seed `trilamsr/homebrew-leah`. Add operator-facing rollback documentation. No code changes in the main repo beyond docs.

**Files touched:**
- `docs/operator/upgrade.md` — operator-facing guide: `make upgrade` (canonical), `leah self-upgrade` (CLI), `brew upgrade --fetch-HEAD leah` (optional), `leah self-upgrade --rollback` (escape hatch).
- `docs/operator/brew-tap.md` — exact `Formula/leah.rb` to paste into the (separately created) `trilamsr/homebrew-leah` repo, with one-line setup instructions.
- `README.md` — "Upgrading" section with three-line summary linking to the above.
- `SECURITY.md` — trust-boundary statement: operator's clone + their `go` toolchain; no remote-fetched binaries.

**Risk:** The tap repo lives outside this monorepo. Doc must make clear that the formula is sourced from `docs/operator/brew-tap.md` and the tap is a separate human-managed artifact. Drift risk between the two is real but acceptable for friends-and-family scale.

**Size:** ~200 LOC of markdown.

**Test plan:**
- Manual: paste formula into a scratch tap, run `brew install --HEAD trilamsr/leah/leah` on a clean machine; assert `leah --version` works.
- No automated tests — pure documentation wave.

**Unblocks:** Traction-phase waves (Sparkle, notarization, GH Releases) — each layers on without disturbing W81–W83.

---

## Wave dependency graph

```
W81 (layout + lockfile)
   └── W82 (version embedding)
          └── W83 (CLI + attestation + restart)
                 └── W84 (brew tap + docs)
```

Linear chain. No parallelization opportunity — W83 reuses W81's shell script and W82's embedded SHA; W84 documents what W83 builds.

## Cumulative deletions

- After W83 lands: the operator's muscle-memory `pkill leah-daemon && go build && ./leah-daemon &` ritual is gone. Replaced by `make upgrade`.
- After W84 lands: README "manual rebuild" section is deleted; `docs/operator/upgrade.md` is the only path.
- Future cost-bearing tiers (Sparkle, notarization) layer on **without** requiring deletion of the local path — the local path remains canonical fallback.
