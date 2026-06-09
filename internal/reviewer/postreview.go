package reviewer

import (
	"fmt"
	"regexp"
)

// Canonical agent-id shapes accepted by regatta's check-reviewer-verdict.sh
// allowlist (see /Users/treedesk/Desktop/Projects/regatta/scripts/lib/reviewer-verdict/verdict.sh).
//
// Regatta's gate accepts `^(a[0-9a-f]{16}|(cavecrew|designer|triage|implementer|reviewer)-[a-z0-9-]+)$`.
// Leah's client-side gate is intentionally tighter: it requires `cavecrew-reviewer-<slug>` (not bare
// `cavecrew-<anything>`) and a non-empty slug with no trailing hyphen, matching the dispatch-prompt
// convention. Tighter-than-regatta is fail-safe — anything Leah accepts will also pass regatta's gate.
var canonicalIDRegex = regexp.MustCompile(
	`^(a[0-9a-f]{16}|(cavecrew-reviewer|designer|triage|implementer|reviewer)-[a-z0-9]+(-[a-z0-9]+)*)$`)

func ValidateAgentID(id string) error {
	if id == "" {
		return fmt.Errorf("reviewer agent-id is empty")
	}
	if !canonicalIDRegex.MatchString(id) {
		return fmt.Errorf("reviewer agent-id %q does not match canonical allowlist "+
			"(^a[0-9a-f]{16}$ OR ^(cavecrew|designer|triage|implementer|reviewer)-[a-z0-9-]+$)", id)
	}
	return nil
}

func ValidateAgentIDAgainstAuthor(reviewerID, prAuthor string) error {
	if err := ValidateAgentID(reviewerID); err != nil {
		return err
	}
	if reviewerID == prAuthor {
		return fmt.Errorf("reviewer agent-id %q matches PR author — self-tag rejected", reviewerID)
	}
	return nil
}
