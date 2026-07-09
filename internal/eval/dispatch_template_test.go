package eval

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestDispatchTemplates_ParseAndReferences asserts that every dispatch
// template parses and that each repo-relative path it links to actually
// exists. The harness exists because dropped template rules (notably the
// errcheck rule) leaked into Wave-1 PRs when prompts re-authored ad-hoc
// instead of citing the template (see feedback_dispatch_template_reference).
func TestDispatchTemplates_ParseAndReferences(t *testing.T) {
	repoRoot := findRepoRoot(t)
	tplDir := filepath.Join(repoRoot, "docs", "engineer", "dispatch-templates")

	required := []string{"implementer.md", "implementer-adapter.md", "reviewer.md", "designer.md", "triage.md"}
	for _, name := range required {
		path := filepath.Join(tplDir, name)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if len(body) < 200 {
			t.Errorf("%s suspiciously short (%d bytes)", name, len(body))
		}
		refs := extractRepoPaths(string(body))
		if len(refs) == 0 {
			t.Errorf("%s references zero repo-relative paths — template is decorative", name)
		}
		for _, ref := range refs {
			resolved := resolveRef(repoRoot, tplDir, ref)
			if _, err := os.Stat(resolved); err != nil {
				t.Errorf("%s cites missing path %q (resolved %q): %v", name, ref, resolved, err)
			}
		}
	}
}

// extractRepoPaths pulls likely repo-relative file references out of a
// markdown template body. Under-matches deliberately — only known repo
// subtrees with code/doc/script extensions — so prose noise never trips
// the gate. Placeholder paths (containing `foo` or `bar` segments) are
// skipped: templates use them as universal example syntax, not real cites.
func extractRepoPaths(body string) []string {
	re := regexp.MustCompile(`(?:\.\./)*(?:docs|internal|scripts|cmd)/[A-Za-z0-9_./-]+\.(?:md|go|sh)`)
	matches := re.FindAllString(body, -1)
	seen := map[string]struct{}{}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		ref := strings.TrimSpace(m)
		if isPlaceholderRef(ref) {
			continue
		}
		if _, dup := seen[ref]; dup {
			continue
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}
	return out
}

// isPlaceholderRef detects example/placeholder paths the templates use as
// inline syntax demonstrations rather than real cross-references. Anything
// with `/foo.` or `/bar.` or a `foo/` or `bar/` segment is treated as a
// placeholder; ditto explicitly-removed CI scripts that templates still
// reference for historical context.
func isPlaceholderRef(ref string) bool {
	placeholders := []string{"/foo.", "/bar.", "/foo/", "/bar/"}
	for _, p := range placeholders {
		if strings.Contains(ref, p) {
			return true
		}
	}
	// check-tdd-evidence.sh is documented in reviewer.md as removed in
	// PR #287; the reference is historical, not a live cite.
	return strings.HasSuffix(ref, "scripts/check-tdd-evidence.sh")
}

// resolveRef turns a markdown link target (which may be repo-relative or
// template-dir-relative via leading "../") into an absolute path.
func resolveRef(repoRoot, tplDir, ref string) string {
	if strings.HasPrefix(ref, "../") {
		return filepath.Clean(filepath.Join(tplDir, ref))
	}
	return filepath.Join(repoRoot, ref)
}

// findRepoRoot walks up from cwd looking for go.mod so the test stays
// robust against future repo layout changes.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := cwd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("repo root not found from %s", cwd)
	return ""
}
