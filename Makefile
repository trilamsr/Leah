.PHONY: dev dev-stop verify-pr baseline check lint ensure-lint smoke phase2-smoke phase2-smoke-stop install upgrade install-janitor uninstall-janitor eval eval-all verify-checksums verify-attestation handoff-test check-amend-guard help

# Pinned to match .github/workflows/check.yml. Bump both together.
GOLANGCI_LINT_VERSION := v2.12.2

# Boot full Phase 2 dev environment: build daemon, launch it in background,
# wait for socket, launch the Swift app, and tail unified logs.
# Companion: make dev-stop tears everything down.
dev:
	@mkdir -p ~/.leah-state-dev/ ~/Library/Caches/Leah
	@cd app/Leah && swift build 2>&1 | tail -5; test $${PIPESTATUS[0]} -eq 0 || { echo "FAIL: swift build"; exit 1; }
	@LEAH_STATE_DIR=~/.leah-state-dev/ go run ./cmd/leah-daemon > /tmp/leah-dev.log 2>&1 & \
	  echo $$! > /tmp/leah-dev-daemon.pid
	@echo "waiting for socket at ~/Library/Caches/Leah/leah.sock ..."; \
	  for i in $$(seq 1 100); do \
	    [ -S "$$HOME/Library/Caches/Leah/leah.sock" ] && break; \
	    sleep 0.1; \
	  done; \
	  [ -S "$$HOME/Library/Caches/Leah/leah.sock" ] || \
	    { echo "FAIL: socket did not appear"; tail -10 /tmp/leah-dev.log; exit 1; }
	@APP="app/Leah/.build/debug/Leah.app"; \
	  if [ -e "$$APP" ]; then open "$$APP"; \
	  else echo "skip app launch — bundle not at $$APP (swift build produces a binary, not .app, in dev)"; fi
	@log stream --predicate 'subsystem == "com.maydow.leah" OR processImagePath ENDSWITH "leah-daemon"' \
	  --style syslog >> /tmp/leah-dev.log 2>&1 &
	@echo "READY — pid file at /tmp/leah-dev-daemon.pid, log at /tmp/leah-dev.log"

# Tear down the dev environment started by make dev.
dev-stop:
	@if [ -f /tmp/leah-dev-daemon.pid ]; then \
	  kill "$$(cat /tmp/leah-dev-daemon.pid)" 2>/dev/null || true; \
	  rm -f /tmp/leah-dev-daemon.pid; \
	fi
	@osascript -e 'tell application "Leah" to quit' 2>/dev/null || true
	@pkill -f 'leah-daemon' 2>/dev/null || true
	@echo "dev environment stopped"

# Re-run check.sh against a specific PR head SHA locally.
# Use: make verify-pr PR=<num>
verify-pr:
	@test -n "$(PR)" || (echo "set PR=<num>" && exit 1)
	@HEAD=$$(gh api repos/trilamsr/Leah/pulls/$(PR) --jq '.head.sha'); \
	BRANCH=$$(gh api repos/trilamsr/Leah/pulls/$(PR) --jq '.head.ref'); \
	echo "verifying PR #$(PR) at $$HEAD ($$BRANCH)"; \
	git fetch origin $$BRANCH && \
	git stash --include-untracked --quiet 2>/dev/null; \
	git checkout $$HEAD -- . 2>&1 | head -5; \
	./scripts/check.sh 2>&1 | tee /tmp/verify-pr-$(PR).log | grep -E "^(FAIL|ok|---|Error|error:|PASS|==>)" | tail -40; \
	echo "exit=$$?"; \
	git checkout main -- .; \
	git stash pop --quiet 2>/dev/null || true

# Snapshot test + bench baseline. Append to ~/.leah-state/baseline-history.jsonl
baseline:
	@./scripts/baseline.sh

# Block merging a PR whose REVIEWER APPROVE pre-dates the current head commit (post-amend self-approve).
# Use: make check-amend-guard PR=<num>
check-amend-guard:
	@test -n "$(PR)" || (echo "set PR=<num>" && exit 1)
	@./scripts/check-amend-after-approve.sh $(PR)

check: ensure-lint
	@./scripts/check.sh

# Mirrors the CI lint step. Standalone shortcut; `make check` runs the
# same binary in parallel inside scripts/check.sh.
lint: ensure-lint
	@golangci-lint run --timeout=5m ./...

# Auto-install the pinned version so `make check` cannot pass locally
# while CI fails on lint — the silent-skip path shipped #278.
ensure-lint:
	@command -v golangci-lint >/dev/null 2>&1 || { \
	    echo "installing golangci-lint $(GOLANGCI_LINT_VERSION)"; \
	    go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	  }

# Run all per-adapter smoke tests (mock mode by default; LEAH_SMOKE_LIVE=1 for live)
smoke:
	@./scripts/smoke-all.sh

# Phase 2 end-to-end smoke (macOS only — SKIPs cleanly elsewhere). Eight
# invariants: widget.mount stream, pin persistence, 2-pin cap, toast frame
# shape, dark/light toggle, fsnotify debounce, BGE backend wiring, spec parity.
phase2-smoke:
	@./scripts/smoke/phase2-e2e.sh

# Force-tear down any state left by an aborted phase2-smoke run.
phase2-smoke-stop:
	@./scripts/smoke/phase2-e2e.sh --cleanup-only

# Build leah+leah-daemon, symlink ~/bin/leah → ~/.leah-state/bin/leah-current.
# Idempotent. See docs/engineer/specs/2026-06-10-local-self-update.md §3.
install:
	@./scripts/upgrade.sh install

# Build new SHA, atomic-swap symlinks, restart daemon. DRY_RUN=1 skips mutation.
upgrade:
	@LEAH_UPGRADE_DRY_RUN=$(DRY_RUN) ./scripts/upgrade.sh upgrade

# Install launchd manifest that sweeps merged agent-* worktrees every 5 min.
install-janitor:
	@mkdir -p ~/Library/LaunchAgents ~/.leah-state
	@sed -e "s|__LEAH_ROOT__|$$(pwd)|g" -e "s|__LEAH_STATE__|$$HOME/.leah-state|g" \
	    scripts/leah-worktree-janitor.plist > ~/Library/LaunchAgents/com.leah.worktree-janitor.plist
	@launchctl bootout gui/$$(id -u)/com.leah.worktree-janitor 2>/dev/null || true
	@launchctl bootstrap gui/$$(id -u) ~/Library/LaunchAgents/com.leah.worktree-janitor.plist
	@echo "janitor installed; logs at ~/.leah-state/janitor.log"

uninstall-janitor:
	@launchctl bootout gui/$$(id -u)/com.leah.worktree-janitor 2>/dev/null || true
	@rm -f ~/Library/LaunchAgents/com.leah.worktree-janitor.plist
	@echo "janitor uninstalled"

# Guards against the audit-session Phase 0 regression — a session that
# never opens the prior handoff before doing new work.
handoff-test:
	@./scripts/check-handoff-continuity_test.sh

# Run feature eval. FEATURE=<name> picks one evals/<name>.jsonl file.
# BASE=<ref> sets the comparison ref (default origin/main; phase-1 stub).
# JSON=1 emits machine-readable output (phase-1: human table only).
# LEAH_EVAL_BUDGET_DOLLARS caps judge spend per run (spec §8.1, default $3).
eval:
	@test -n "$(FEATURE)" || (echo "set FEATURE=<name>" && exit 1)
	@LEAH_EVAL_BUDGET_DOLLARS=$${LEAH_EVAL_BUDGET_DOLLARS:-3} \
	  go run ./cmd/leah-eval --feature=$(FEATURE) --base=$${BASE:-origin/main} --json=$${JSON:-0}

# Run every evals/*.jsonl file in one invocation.
eval-all:
	@LEAH_EVAL_BUDGET_DOLLARS=$${LEAH_EVAL_BUDGET_DOLLARS:-3} \
	  go run ./cmd/leah-eval --base=$${BASE:-origin/main} --json=$${JSON:-0}

# Reproducibility gate (S10 M6 + S12 §8). Rebuild from source with the release
# flags, then diff against the published SHA256SUMS.unsigned. Pre-signing —
# signed+stapled tarballs (SHA256SUMS) can never hash-match a local rebuild
# because codesign rewrites Mach-O bytes; the .unsigned file fixes the
# checkpoint at the pre-signing tarball so the rebuild can match exactly.
# Use: make verify-checksums TAG=vX.Y.Z
verify-checksums:
	@test -n "$(TAG)" || (echo "set TAG=vX.Y.Z" && exit 1)
	@os="$$(uname -s | tr '[:upper:]' '[:lower:]')"; arch="$$(uname -m)"; \
	  case "$$arch" in arm64|aarch64) arch=arm64;; x86_64|amd64) arch=amd64;; esac; \
	  test "$$os" = darwin || (echo "verify-checksums: macOS only (got $$os)" && exit 1); \
	  rm -rf dist && mkdir -p dist; \
	  CGO_ENABLED=0 GOFLAGS='-trimpath -mod=readonly' bash -c '\
	    for cmd in leah leah-daemon leah-hud; do \
	      go build -ldflags="-s -w -buildid=" -o dist/$$cmd ./cmd/$$cmd; \
	    done'; \
	  cd dist && tar czf "leah-$(TAG)-$$os-$$arch-unsigned.tar.gz" leah leah-daemon leah-hud; \
	  shasum -a 256 ./*-unsigned.tar.gz > SHA256SUMS.unsigned.local; \
	  curl -fsSL "https://github.com/trilamsr/Leah/releases/download/$(TAG)/SHA256SUMS.unsigned" -o SHA256SUMS.unsigned.upstream; \
	  diff -u SHA256SUMS.unsigned.upstream SHA256SUMS.unsigned.local && echo "checksums match"

# W135/S10/M6: verify SLSA L2 provenance + cosign keyless signature over
# SHA256SUMS (signed+stapled tarball checksums published by W141 release.yml).
# Use: make verify-attestation TAG=vX.Y.Z
# Requires cosign + slsa-verifier installed locally. End-to-end attestation chain:
#   SHA256SUMS.sig -> Sigstore Rekor entry -> GHA OIDC identity -> repo ref.
verify-attestation:
	@test -n "$(TAG)" || (echo "set TAG=vX.Y.Z" && exit 1)
	@command -v cosign >/dev/null 2>&1 || (echo "install cosign: brew install cosign" && exit 1)
	@command -v slsa-verifier >/dev/null 2>&1 || (echo "install slsa-verifier: brew install slsa-verifier" && exit 1)
	@DEST="dist-verify/$(TAG)"; \
	    mkdir -p "$$DEST"; \
	    gh release download "$(TAG)" --repo trilamsr/Leah --dir "$$DEST" --clobber; \
	    echo "verifying cosign keyless signature on SHA256SUMS"; \
	    cosign verify-blob \
	        --certificate "$$DEST/SHA256SUMS.pem" \
	        --signature "$$DEST/SHA256SUMS.sig" \
	        --certificate-identity-regexp "^https://github.com/trilamsr/Leah/" \
	        --certificate-oidc-issuer https://token.actions.githubusercontent.com \
	        "$$DEST/SHA256SUMS"; \
	    echo "verifying SLSA L2 provenance"; \
	    slsa-verifier verify-artifact \
	        --provenance-path "$$DEST/leah.intoto.jsonl" \
	        --source-uri github.com/trilamsr/Leah \
	        --source-tag "$(TAG)" \
	        "$$DEST"/leah-*-* "$$DEST"/leah-daemon-*-* "$$DEST"/leah-hud-*-*

.PHONY: app-build app-test app-run sign-and-notarize

# Swift Package build of the Leah.app shell.
app-build:
	@cd app/Leah && swift build -c release

app-test:
	@cd app/Leah && swift test

# Run the app from the SwiftPM build artifact (dev loop; production app is
# built via xcodebuild + sign-and-notarize.sh — see scripts/sign-and-notarize.sh).
app-run: app-build
	@app/Leah/.build/release/Leah

# Sign + notarize + staple Leah.app for distribution.
# ARGS controls the operation: --build-only | --sign | --notarize | --staple | --all
# See scripts/sign-and-notarize.sh and docs/engineer/runbooks/signing-and-notarization.md.
sign-and-notarize:
	@bash scripts/sign-and-notarize.sh $(ARGS)

help:
	@echo "Targets: dev, dev-stop, verify-pr PR=<n>, baseline, check, lint, smoke, phase2-smoke, phase2-smoke-stop, install, upgrade [DRY_RUN=1], install-janitor, uninstall-janitor, eval FEATURE=<name>, eval-all, verify-checksums TAG=<vX.Y.Z>, verify-attestation TAG=<vX.Y.Z>, app-build, app-test, app-run, sign-and-notarize [ARGS=--all]"
