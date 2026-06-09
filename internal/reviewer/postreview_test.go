package reviewer

import (
	"strings"
	"testing"
)

func TestPostReviewAcceptsCanonicalHexID(t *testing.T) {
	id := "a1234567890abcdef"
	if err := ValidateAgentID(id); err != nil {
		t.Errorf("canonical hex id rejected: %v", err)
	}
}

func TestPostReviewAcceptsCanonicalNamedID(t *testing.T) {
	for _, id := range []string{
		"cavecrew-reviewer-leah-pr-1234",
		"cavecrew-reviewer-tier1-2026-06-09",
	} {
		if err := ValidateAgentID(id); err != nil {
			t.Errorf("canonical named id %q rejected: %v", id, err)
		}
	}
}

func TestPostReviewRejectsWrongShape(t *testing.T) {
	for _, id := range []string{
		"",
		"leah",
		"leah-reviewer-foo",
		"main-thread-adversarial-self",
		"self-tagged-defer",
		"a1234567890abcde",     // 16 chars (need 17 total including 'a')
		"a1234567890abcdefg",   // 18 chars
		"cavecrew-reviewer-",   // empty slug
		"cavecrew-something-x", // wrong canonical prefix
	} {
		if err := ValidateAgentID(id); err == nil {
			t.Errorf("wrong-shape id %q accepted", id)
		}
	}
}

func TestPostReviewRejectsAuthorMatch(t *testing.T) {
	if err := ValidateAgentIDAgainstAuthor("a1234567890abcdef", "a1234567890abcdef"); err == nil {
		t.Error("self-matched id accepted")
	}
	if err := ValidateAgentIDAgainstAuthor("cavecrew-reviewer-x", "cavecrew-reviewer-x"); err == nil {
		t.Error("self-matched named id accepted")
	}
}

func TestRejectAuthorMatchErrorMessage(t *testing.T) {
	err := ValidateAgentIDAgainstAuthor("a1234567890abcdef", "a1234567890abcdef")
	if err == nil || !strings.Contains(err.Error(), "author") {
		t.Errorf("unhelpful error: %v", err)
	}
}
