package dispatcher

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/oklog/ulid/v2"
	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/ghclient"
)

type GHClient interface {
	CreateIssue(ctx context.Context, args ghclient.CreateIssueArgs) (string, error)
}

type Ship struct {
	Reasoner Reasoner
	GH       GHClient
	Audit    *audit.Logger
	Budget   *budget.Budget
	Out      io.Writer
	Repo     string
	Title    string
	TmpDir   string
}

func (s *Ship) Run(ctx context.Context, intent string) error {
	ulidVal, err := ulid.New(ulid.Now(), ulid.Monotonic(rand.Reader, 0))
	if err != nil {
		return fmt.Errorf("ulid: %w", err)
	}
	actionID := ulidVal.String()

	bodyDraft, err := s.Reasoner.Ask(ctx,
		"Intent:\n"+intent+"\n\nDraft the regatta issue body per the template you were given.")
	if err != nil {
		s.auditFail("draft body: "+err.Error(), intent)
		return err
	}

	cleanDraft := strings.ReplaceAll(bodyDraft, "-->", "-- >")
	body := cleanDraft + "\n\n<!-- leah-dispatched: " + actionID + " -->\n<!-- intent-source: terminal -->\n"

	bodyFile := filepath.Join(s.TmpDir, "leah-ship-"+actionID+".md")
	if err := os.WriteFile(bodyFile, []byte(body), 0o600); err != nil {
		s.auditFail("write body file: "+err.Error(), intent)
		return err
	}

	title := s.Title
	if title == "" {
		title = deriveTitle(intent)
	}

	url, err := s.GH.CreateIssue(ctx, ghclient.CreateIssueArgs{
		Repo:     s.Repo,
		Title:    title,
		BodyFile: bodyFile,
		Labels:   []string{"ready-for-agent"},
	})
	if err != nil {
		s.auditFail("gh issue create: "+err.Error(), intent)
		return err
	}

	if err := s.Audit.Append(audit.Entry{
		Kind:        "ship",
		ArgsHash:    argsHash(intent),
		BlastRadius: 3,
		Outcome:     "pending",
		CostDollars: s.Budget.Spent(),
		Detail:      url,
	}); err != nil {
		fmt.Fprintf(s.Out, "warning: audit append failed: %v\n", err)
	}

	fmt.Fprintln(s.Out, url)
	return nil
}

func (s *Ship) auditFail(detail, intent string) {
	_ = s.Audit.Append(audit.Entry{
		Kind:        "ship",
		ArgsHash:    argsHash(intent),
		BlastRadius: 3,
		Outcome:     "failed",
		CostDollars: s.Budget.Spent(),
		Detail:      detail,
	})
}

func deriveTitle(intent string) string {
	t := strings.TrimSpace(intent)
	// Pass-through if operator already prefixed
	for _, p := range []string{"[FEAT]", "[FIX]", "[DOCS]", "[CHORE]", "[REFACTOR]", "[CI]"} {
		if strings.HasPrefix(t, p) {
			if len(t) > 60 {
				t = t[:57] + "..."
			}
			return t
		}
	}
	prefix := classifyIntentPrefix(t)
	if len(t) > 60-len(prefix)-1 {
		t = t[:57-len(prefix)-1] + "..."
	}
	return prefix + " " + t
}

func classifyIntentPrefix(s string) string {
	low := strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.HasPrefix(low, "fix"), strings.HasPrefix(low, "patch"), strings.HasPrefix(low, "repair"):
		return "[FIX]"
	case strings.HasPrefix(low, "doc"), strings.HasPrefix(low, "document"):
		return "[DOCS]"
	case strings.HasPrefix(low, "refactor"):
		return "[REFACTOR]"
	case strings.HasPrefix(low, "chore"):
		return "[CHORE]"
	default:
		return "[FEAT]"
	}
}
