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

// TestPostReview_HexBoundary nails the 17-char (1+16) length boundary:
// the regex must reject 16- or 18-char hex variants AND any non-hex char
// inside the 16-char tail.
func TestPostReview_HexBoundary(t *testing.T) {
	bad := []string{
		"a0123456789abcde",   // 16 chars total — short
		"a0123456789abcdef0", // 18 chars total — long
		"b1234567890abcdef",  // wrong leading byte
		"a1234567890abcdeg",  // 'g' not hex
		"a1234567890ABCDEF",  // uppercase hex — case-sensitive
	}
	for _, id := range bad {
		if err := ValidateAgentID(id); err == nil {
			t.Errorf("hex-boundary id %q wrongly accepted", id)
		}
	}
}

// TestPostReview_NamedTrailingHyphenRejected pins the spec rule that a
// canonical-named id cannot end in a hyphen or contain double hyphens —
// keeps the slug a stable downstream key.
func TestPostReview_NamedTrailingHyphenRejected(t *testing.T) {
	bad := []string{
		"cavecrew-reviewer-foo-",  // trailing hyphen
		"cavecrew-reviewer--foo",  // double hyphen
		"cavecrew-reviewer-Foo",   // uppercase slug
		"cavecrew-reviewer-foo!",  // illegal char
		"cavecrew-reviewer-foo ",  // trailing space
		" cavecrew-reviewer-foo",  // leading space
		"cavecrew-Reviewer-foo",   // uppercase prefix
	}
	for _, id := range bad {
		if err := ValidateAgentID(id); err == nil {
			t.Errorf("named-boundary id %q wrongly accepted", id)
		}
	}
}

// TestPostReview_NamedMultiSegmentSlug asserts the regex accepts dashed
// multi-segment slugs (cavecrew-reviewer-foo-bar-baz) — the dispatch
// convention uses dotted PR refs like cavecrew-reviewer-leah-pr-1234.
func TestPostReview_NamedMultiSegmentSlug(t *testing.T) {
	good := []string{
		"cavecrew-reviewer-a",
		"cavecrew-reviewer-leah-pr-1234",
		"cavecrew-reviewer-2026-06-09",
		"implementer-w4-5-detector",
		"designer-1",
		"triage-pr-100",
		"reviewer-batch-7",
	}
	for _, id := range good {
		if err := ValidateAgentID(id); err != nil {
			t.Errorf("multi-segment id %q rejected: %v", id, err)
		}
	}
}

// TestPostReview_AuthorMatchStillValidates asserts the author-match check
// runs after the canonical-shape check so a malformed id never falsely
// passes via author == author.
func TestPostReview_AuthorMatchStillValidates(t *testing.T) {
	err := ValidateAgentIDAgainstAuthor("not-canonical", "not-canonical")
	if err == nil {
		t.Fatal("malformed self-matched id wrongly accepted")
	}
	if strings.Contains(err.Error(), "author") {
		t.Errorf("expected canonical-shape error before author check, got author-match: %v", err)
	}
}

func TestRejectAuthorMatchErrorMessage(t *testing.T) {
	err := ValidateAgentIDAgainstAuthor("a1234567890abcdef", "a1234567890abcdef")
	if err == nil || !strings.Contains(err.Error(), "author") {
		t.Errorf("unhelpful error: %v", err)
	}
}
