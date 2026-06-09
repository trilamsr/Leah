package rules

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/trilam/leah/internal/audit"
)

// AttestationGate scans audit.jsonl for dispatched self-build PRs that
// merged WITHOUT an `Attestation:` comment from the operator. The
// SelfBuild dispatcher appends an "operator merge attestation" footer to
// every issue body (selfbuild.go::attestationBlock) but, prior to this
// gate, nothing verified the comment ever arrived — the rule was
// honor-system only (Wave2-5 retro H3).
//
// The gate is read-only: it reports violations; the operator decides
// whether to roll back, file a follow-up, or update the prompt. Retro
// renders violations under "## Attestation gate violations".
//
// Self-build audit rows carry `url=<issue URL>` in Detail (per
// SelfBuild.appendAuditSuccess); the gate hops issue → closing PR via
// the IssueToPR probe before checking the PR's state + comments.
type AttestationGate struct {
	// IssueToPR resolves a self-build issue number to its closing PR
	// number (gh issue view --json closedByPullRequestsReferences).
	// Returns 0 when no PR has been merged/linked yet. Tests stub this;
	// production wiring shells to gh.
	IssueToPR func(ctx context.Context, repo string, issueNum int) (int, error)
	// PR lets tests stub the gh probe. Defaults to shelling out to gh.
	PR func(ctx context.Context, repo string, prNum int) (PRView, error)
}

// PRView is the gh response subset the gate needs.
type PRView struct {
	State    string      `json:"state"`
	Comments []PRComment `json:"comments"`
}

// PRComment is one PR comment body. We deliberately do NOT track the
// author here — the attestation footer publishes the required login and
// the operator-habituation defense is the existence of the sentinel
// comment + operator review of the violation list, not a per-comment
// authorship check.
type PRComment struct {
	Body string `json:"body"`
}

// Violation is one merged-without-attestation finding.
type Violation struct {
	Repo     string
	PRNumber int
	URL      string
}

// attestationIssueURL matches `https://github.com/<owner>/<repo>/issues/<N>`
// or `pull/<N>` inside an audit row Detail. SelfBuild appends
// `url=<issue URL>` to its success row Detail; older rows (pre-H3) may
// not have it — the gate silently skips entries without a parseable URL.
var attestationIssueURL = regexp.MustCompile(`https?://github\.com/([^/\s]+/[^/\s]+)/(?:issues|pull)/(\d+)`)

// Scan walks auditPath, probes each dispatched self-build PR, and
// returns violations for merged PRs lacking an Attestation: comment.
// Probe errors are surfaced (single-tenant audit volume is small enough
// that one transient gh failure should not be papered over).
func (g AttestationGate) Scan(ctx context.Context, auditPath string) ([]Violation, error) {
	probe := g.PR
	if probe == nil {
		probe = defaultPRView
	}
	issueToPR := g.IssueToPR
	if issueToPR == nil {
		issueToPR = defaultIssueToPR
	}

	entries, err := readAttestationAudit(auditPath)
	if err != nil {
		return nil, err
	}

	seen := map[string]struct{}{} // "<repo>#<n>" — dedupe re-dispatches.
	var out []Violation
	for _, e := range entries {
		if e.Kind != "self-build" || e.Outcome != "dispatched" {
			continue
		}
		// Match URL inside Detail; URL is appended after prompt_sha by
		// SelfBuild.appendAuditSuccess (`url=<full URL>`). The path segment
		// is `/issues/N` (selfbuild dispatches issues) but we accept
		// `/pull/N` defensively for tests / future shapes.
		m := attestationIssueURL.FindStringSubmatch(e.Detail)
		if m == nil {
			continue
		}
		repo := m[1]
		num, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		// If the URL is an issue URL, hop to the closing PR. A PR URL is
		// treated as the PR directly (tests + back-compat).
		prNum := num
		if strings.Contains(e.Detail, "/issues/") {
			pr, err := issueToPR(ctx, repo, num)
			if err != nil {
				return nil, fmt.Errorf("issue-to-pr %s#%d: %w", repo, num, err)
			}
			if pr == 0 {
				// Issue exists but no closing PR linked — likely still
				// in-flight; skip (we only flag MERGED PRs without attestation).
				continue
			}
			prNum = pr
		}
		key := fmt.Sprintf("%s#%d", repo, prNum)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		view, err := probe(ctx, repo, prNum)
		if err != nil {
			return nil, fmt.Errorf("probe %s: %w", key, err)
		}
		if view.State != "MERGED" {
			continue
		}
		if hasAttestation(view.Comments) {
			continue
		}
		out = append(out, Violation{
			Repo:     repo,
			PRNumber: prNum,
			URL:      fmt.Sprintf("https://github.com/%s/pull/%d", repo, prNum),
		})
	}
	return out, nil
}

// hasAttestation returns true if any comment starts (after optional
// leading whitespace) with the exact case-sensitive "Attestation:"
// sentinel. Case strictness is the operator-habituation defense —
// auto-text that lowercases the prefix must not satisfy the gate.
func hasAttestation(cs []PRComment) bool {
	for _, c := range cs {
		if startsWithAttestation(c.Body) {
			return true
		}
	}
	return false
}

// startsWithAttestation checks the first non-empty line of body against
// the sentinel. Operators sometimes paste a heading line before the
// attestation; scanning only the FIRST line forces the comment-shape
// discipline named in attestationBlock ("a PR comment whose first line
// starts with `Attestation:`").
func startsWithAttestation(body string) bool {
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		line := strings.TrimLeft(sc.Text(), " \t")
		if line == "" {
			continue
		}
		return strings.HasPrefix(line, "Attestation:")
	}
	return false
}

// defaultPRView shells out to gh. Returns a typed empty view when the PR
// is missing — selfbuild dispatch may have failed AFTER the audit row was
// appended (rare; surfaces as "skipped" in the gate).
func defaultPRView(ctx context.Context, repo string, prNum int) (PRView, error) {
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", strconv.Itoa(prNum), "--repo", repo, "--json", "state,comments")
	out, err := cmd.Output()
	if err != nil {
		return PRView{}, err
	}
	var v PRView
	if err := json.Unmarshal(out, &v); err != nil {
		return PRView{}, err
	}
	return v, nil
}

// defaultIssueToPR queries gh issue view for the closing PR. Returns 0
// when no PR has linked the issue. The first element of
// closedByPullRequestsReferences is the latest linked PR.
func defaultIssueToPR(ctx context.Context, repo string, issueNum int) (int, error) {
	cmd := exec.CommandContext(ctx, "gh", "issue", "view", strconv.Itoa(issueNum), "--repo", repo, "--json", "closedByPullRequestsReferences")
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	var v struct {
		Refs []struct {
			Number int `json:"number"`
		} `json:"closedByPullRequestsReferences"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return 0, err
	}
	if len(v.Refs) == 0 {
		return 0, nil
	}
	return v.Refs[0].Number, nil
}

// readAttestationAudit is a local copy of selflearn.readAudit — kept
// here to avoid an import cycle (selflearn/rules already imports
// selflearn for Outcome). audit.jsonl shape is stable; the duplication
// is 12 LoC and the cycle cost is higher.
func readAttestationAudit(path string) ([]audit.Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open audit: %w", err)
	}
	defer func() { _ = f.Close() }()
	var entries []audit.Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e audit.Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan audit: %w", err)
	}
	return entries, nil
}
