# Eval fixtures

This directory is the in-repo home for the eval-fixture schema and authoring
notes. At runtime the daemon reads from `<state-dir>/eval-fixtures/` by
default (override with `LEAH_EVAL_FIXTURES_DIR`); point that env var at this
directory during development to score the in-repo set.

Scheduler is off by default; the daemon enables it when `LEAH_EVAL=1`.
`LEAH_EVAL_INTERVAL_SECONDS` overrides the default 3600s cadence.

Each `*.yaml` file is one fixture. Schema:

```yaml
id: short-stable-handle
question: "what gets sent to Asker.Ask"
expected_contains:
  - "substring 1"
  - "substring 2"
```

Scoring is substring-match against every entry in `expected_contains`. A
fixture passes when all substrings appear in the answer; the first missing
substring is recorded as the failure reason. Unknown YAML fields fail the
load, as does an empty `expected_contains` (would auto-pass).

Results land in `eval.db` (eval_runs + eval_results tables) under the
daemon state dir.

No fixtures ship in-repo; this is intentional opt-in.
