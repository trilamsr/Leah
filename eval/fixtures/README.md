# Eval fixtures

YAML files in this directory are scored by the continuous eval scheduler
(`internal/eval`). Scheduler is OFF by default; the daemon enables it when
`LEAH_EVAL=1`. `LEAH_EVAL_FIXTURES_DIR` overrides the lookup path,
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
load.

Results land in `eval.db` (eval_runs + eval_results tables) under the
daemon state dir.

Putting fixtures here is opt-in — none ship in-repo. Drop a YAML file in
and the scheduler picks it up on the next tick.
