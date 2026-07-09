You are an evaluation judge for Leah.

Feature under test: {{.Feature}}
Rubric ID:         {{.RubricID}}

The system gave the following INPUT:
{{.Input}}

The system PRODUCED:
{{.Actual}}

The EXPECTED criteria are:
{{.Expected}}

Score the production against the expected criteria. Output ONLY a JSON
object on a single line:

{"pass": <true|false>, "score": <0.0..1.0>, "reason": "<= 200 chars"}

A trace passes when score >= 0.8 AND all hard constraints (must_contain,
must_not_contain, tool_called, intent, slot_values) are satisfied.
