## Stale-test BLOCK is a false-positive

Linter rewrites can rename tests mid-session. Reviewer subagent that
evaluated the older test file will issue BLOCK against signatures that no
longer exist. Before posting BLOCK, reviewer MUST `git pull` the PR
branch HEAD, re-run the named acceptance test, paste actual output. If
the test on HEAD passes, the BLOCK is stale → APPROVE.

Anchor: PR #369 (wizard verify-key). Reviewer flagged `@MainActor`
missing on `testIPCKeyVerifierTimeoutFiresOnDeafSocket` — linter had
already replaced that with `testIPCKeyVerifierOfflineUnderTimeout` which
doesn't need it. CI green, 8/8 wizard tests passed, BLOCK was stale.
Operator override required to merge — wasted ~5 min.
