.PHONY: dev verify-pr baseline check smoke install upgrade install-janitor uninstall-janitor help

# Run leah-daemon against ~/.leah-state-dev/ sandbox.
# Opens browser to dashboard. Tails audit log in foreground.
dev:
	@mkdir -p ~/.leah-state-dev/
	@LEAH_STATE_DIR=~/.leah-state-dev/ go run ./cmd/leah-daemon &
	@sleep 2
	@command -v open >/dev/null && open http://127.0.0.1:8080/dashboard || echo "open dashboard at http://127.0.0.1:8080/dashboard"
	@tail -f ~/.leah-state-dev/audit.jsonl 2>/dev/null || echo "audit log not yet created"

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

check:
	@./scripts/check.sh

# Run all per-adapter smoke tests (mock mode by default; LEAH_SMOKE_LIVE=1 for live)
smoke:
	@./scripts/smoke-all.sh

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

help:
	@echo "Targets: dev, verify-pr PR=<n>, baseline, check, smoke, install, upgrade [DRY_RUN=1], install-janitor, uninstall-janitor"
