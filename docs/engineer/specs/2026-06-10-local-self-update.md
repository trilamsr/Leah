# Local Self-Update — Zero-Cost Operator Build/Swap

**Status:** Draft
**Date:** 2026-06-10
**Scope:** MVP. macOS only. Local-built binaries only. Zero recurring cost.
**Explicitly NOT:** cloud-distribution, code-signing/notarization, Sparkle, GitHub Releases hosting, DMG packaging, auto-update server, appcast XML. All cost-bearing items deferred until traction emerges.

## 1. Goal

Operator can pull latest Leah source + swap the running daemon binary in **< 30 seconds** without re-running `leah connect` or losing in-flight subagent state.

Trust boundary = operator's own clone + their `go` toolchain. No remote-fetched binaries. No third-party signing chain.

## 2. Mental Model — Three Tiers

All three tiers converge on the same primitive: **atomic symlink swap + graceful daemon restart**.

| Tier | Trigger | Audience |
| --- | --- | --- |
| **dev** | `make upgrade` from a cloned repo | the operator, fastest iteration |
| **brew** (optional) | `brew upgrade --fetch-HEAD leah` via private tap | friends-and-family; brew builds from source locally |
| **CLI** | `leah self-upgrade` | operator-attested, callable from any cwd while daemon runs |

Each tier produces the same end state: `~/bin/leah` resolves (via symlinks) to the newly built artifact; the daemon process is the new binary; the audit log records the SHA transition.

## 3. Binary Layout

```
~/bin/leah                              → ~/.leah-state/bin/leah-current  (operator-owned PATH entry)
~/.leah-state/bin/leah-current          → ~/.leah-state/bin/leah-<sha>
~/.leah-state/bin/leah-previous         → ~/.leah-state/bin/leah-<prev-sha>  (instant rollback)
~/.leah-state/bin/leah-<sha>            (immutable artifact, SHA-suffixed)
~/.leah-state/bin/leah-daemon-<sha>     (daemon counterpart)
~/.leah-state/daemon.pid                (PID of running daemon)
~/.leah-state/daemon.lock               (flock(2) target — serializes upgrades)
```

SHA-suffixed artifacts are never modified in place. Rollback is a symlink swap, never a file copy.

## 4. Upgrade Sequence (`make upgrade` / `leah self-upgrade`)

1. Acquire `flock(2)` on `~/.leah-state/daemon.lock`. If held, exit `ErrUpgradeInProgress` immediately — no waiting.
2. `SHA=$(git rev-parse HEAD)`; verify working tree is clean (no dirty edits in upgrade scope).
3. `go build -ldflags "-X main.commit=$SHA -X main.builtAt=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o ~/.leah-state/bin/leah-$SHA ./cmd/leah`
4. `go build -ldflags "-X main.commit=$SHA -X main.builtAt=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o ~/.leah-state/bin/leah-daemon-$SHA ./cmd/leah-daemon`
5. Operator-attest **before any symlink mutates** (CLI tier only — `make upgrade` is implicitly trusted because it lives on the operator's box and they invoked it): `Attestor.Attest(ctx, "self_upgrade:apply")`. Denied → abort, no symlink change, no audit row beyond `denied`.
6. `mv -f ~/.leah-state/bin/leah-current ~/.leah-state/bin/leah-previous` (atomic rename on same FS).
7. `ln -s ~/.leah-state/bin/leah-$SHA ~/.leah-state/bin/leah-current.new && mv ~/.leah-state/bin/leah-current.new ~/.leah-state/bin/leah-current` (atomic via rename).
8. Repeat 6–7 for the daemon binary.
9. Send `SIGTERM` to PID at `~/.leah-state/daemon.pid`. Daemon drains in-flight subagents using `signal.NotifyContext` (PR #55 plumbing) and exits cleanly.
10. Wait for daemon exit (poll PID up to 10s; if still alive escalate `SIGKILL` and record `force_killed: true` in audit).
11. Relaunch: `nohup ~/.leah-state/bin/leah-current daemon > ~/.leah-state/daemon.log 2>&1 &`; write new PID to `~/.leah-state/daemon.pid`.
12. Append audit row: `Kind: "self_upgrade", old_sha, new_sha, success, force_killed, duration_ms`.
13. Release `flock`.

Steps 6–8 are individually atomic. Steps 6+7 together are *not* atomic: a crash between them leaves `leah-previous` pointing at what was current and no `leah-current`. Recovery: `leah self-upgrade --recover` re-creates `leah-current` from `leah-previous`.

## 5. Rollback

```
leah self-upgrade --rollback
```

1. Acquire flock.
2. Operator-attest `self_upgrade:rollback`.
3. Swap `leah-current` ↔ `leah-previous` symlinks (two-step rename via a temp link).
4. Restart daemon (steps 9–11 above).
5. Audit row records the reversal.

Operator cannot roll back further than N-1. That is a deliberate UX choice: SHA-suffixed artifacts accumulate, but the *blessed* rollback target is always exactly one step back. Operator can `ls ~/.leah-state/bin/` and manually re-point `leah-current` for older SHAs; that path is unattested and unrecorded.

## 6. Version Reporting

`leah --version` (and `leah-daemon --version`) output:

```
leah 0.x.y (commit: 5448ac2; built: 2026-06-10T14:22:01Z)
```

- Embedded via `-ldflags "-X main.commit=$SHA -X main.builtAt=$DATE"`.
- Daemon `/health` endpoint (per observability spec) returns `commit_sha`, `built_at`, `pid` in JSON payload.
- HUD ambient panel surfaces the truncated 7-char SHA so the operator knows which build is live.
- `leah self-upgrade --status` prints current SHA, previous SHA, last upgrade timestamp, last upgrade actor (audit-attested or `make`).

## 7. Operator-Attestation Surface

| Scope | Required by | Denial effect |
| --- | --- | --- |
| `self_upgrade:apply` | `leah self-upgrade` | Build artifacts remain on disk; no symlink mutation; daemon untouched. |
| `self_upgrade:rollback` | `leah self-upgrade --rollback` | No symlink mutation; daemon untouched. |
| `self_upgrade:recover` | `leah self-upgrade --recover` | No symlink reconstruction; daemon untouched. |

`make upgrade` does not attest: it is an out-of-band operator action on their own box. Audit row records `actor: "make"` for traceability.

## 8. Brew Tap (Optional, Still Zero-Cost)

Separate public repo `trilamsr/homebrew-leah` (operator creates; documented path — empty for now). Formula:

```ruby
class Leah < Formula
  desc "Operator-controlled personal AI assistant"
  homepage "https://github.com/trilamsr/Leah"
  head "https://github.com/trilamsr/Leah.git", branch: "main"
  depends_on "go" => :build

  def install
    sha = Utils.git_short_head
    ldflags = "-X main.commit=#{sha} -X main.builtAt=#{Time.now.utc.iso8601}"
    system "go", "build", *std_go_args(ldflags: ldflags, output: bin/"leah"), "./cmd/leah"
    system "go", "build", *std_go_args(ldflags: ldflags, output: bin/"leah-daemon"), "./cmd/leah-daemon"
  end

  test do
    assert_match "leah", shell_output("#{bin}/leah --version")
  end
end
```

Usage:

```
brew tap trilamsr/leah
brew install --HEAD trilamsr/leah/leah
brew upgrade --fetch-HEAD leah
```

No notarization required — brew compiles locally on the friend's machine. GitHub hosts the formula repo for free.

## 9. CI Verification

Add a CI job that exercises the build pipeline (not the swap/restart) on every PR:

- Re-uses `./scripts/check.sh` infrastructure.
- Runs `make upgrade DRY_RUN=1` against a fresh checkout: builds both binaries with embedded SHA, asserts `--version` prints the SHA, skips symlink mutation and daemon restart.
- Fails the PR if the build pipeline breaks.

This is the only piece that touches CI; everything else is local.

## 10. Threat Model

| Threat | Mitigation |
| --- | --- |
| Binary tampering at build time | Trust boundary is operator's clone + their `go` toolchain. No remote binary fetch. Documented in `SECURITY.md`. |
| PID-file race (two daemons writing same PID file) | `flock(2)` on `~/.leah-state/daemon.lock` serializes all upgrade/rollback/recover paths. |
| Symlink hijack on `~/bin/leah` | Operator owns `~/bin/`; if compromised, all bets off. Boundary documented; not in scope to defend. |
| Rollback to a malicious previous artifact | `leah-previous` symlink is only ever written by Leah itself; never operator-supplied path. SHA-suffix in audit row pins which artifact is which. |
| Concurrent upgrades | `flock` returns `ErrUpgradeInProgress` immediately on the loser; no queueing, no silent waiting. |
| Daemon hangs in drain | 10s SIGTERM grace, then SIGKILL. Audit row flags `force_killed: true`. |
| Partial swap (crash between `leah-current` rename and `leah-previous` rename) | `leah self-upgrade --recover` reconstructs `leah-current` from `leah-previous`. Idempotent. |

## 11. Test Plan

- `TestBuild_VersionStringEmbedded` — build with `-X main.commit=X` and `-X main.builtAt=Y`; assert `leah --version` output contains both.
- `TestSwap_AtomicReplace` — seed `leah-current` symlink + new artifact; run swap; assert `leah-current` now resolves to new artifact and `leah-previous` to the old one.
- `TestRollback_PreviousRestored` — after an upgrade, rollback returns `leah-current` to N-1.
- `TestRecover_PartialSwapReconstructed` — delete `leah-current` to simulate a mid-swap crash; `--recover` re-creates it from `leah-previous`.
- `TestRestart_GracefulShutdown` — start daemon, SIGTERM it, assert exit code 0 within grace window; mock in-flight subagents drain.
- `TestRestart_ForceKilledOnHang` — daemon ignores SIGTERM; assert SIGKILL escalates after 10s and audit row has `force_killed: true`.
- `TestAttestationDenied_NoSwap` — Attestor stub returns deny; assert symlinks unchanged, daemon PID unchanged, audit row records `denied`.
- `TestLockfile_SerializesUpgrades` — spawn two upgrade goroutines against a shared lock dir; assert exactly one acquires the lock and the other returns `ErrUpgradeInProgress` immediately (no blocking).
- `TestAuditRow_RecordsTransition` — successful upgrade produces a row with `old_sha`, `new_sha`, `actor`, `duration_ms`, `force_killed=false`.

All tests live in `cmd/leah/selfupgrade/` package, no integration with the actual daemon required (mock the restart hook).

## 12. Out of Scope (MVP)

- Self-update from internet (HTTP fetch + signature verify) — deferred to traction phase.
- Cross-platform (Windows/Linux) — macOS-only per recent operator direction.
- Update-available notification — operator runs `make upgrade` manually; no polling, no nag.
- A/B build channels — single linear progression.
- Beta vs stable channels — single channel.

## 13. Future Wiring When Traction Arrives

Each cost-bearing tier is additive; none breaks the local path.

| Tier | Trigger | Path |
| --- | --- | --- |
| Code-signing + notarization | Apple Developer ID acquired ($99/yr) | `make notarize` target; runs `codesign` + `notarytool` against existing artifacts. |
| GitHub Releases | First public release-worthy build | GHA workflow on `v*` tags publishes signed binaries; `leah self-upgrade --from-release` becomes available. |
| Sparkle appcast | Public binaries flowing | Publish `appcast.xml` alongside releases; in-app updater consumes; `leah-current` swap mechanics unchanged. |
| Auto-update notification | After Sparkle wiring | Daemon polls appcast on a low cadence; HUD surfaces availability; operator still attests apply. |

The local `make upgrade` path remains the canonical fallback at every tier.
