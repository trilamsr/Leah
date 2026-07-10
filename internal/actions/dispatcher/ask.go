// Package dispatcher is Layer 4 of the closed loop — the CLI orchestrations
// (Ship, SelfBuild, Status) that take operator intent, drive Reasoner +
// gh + regatta, and write the audit row. Every operator action lands here.
// The streaming `leah ask` path lives in cmd/leah (runAskWith) and consumes
// the LLMDimReporter + Reasoner contracts defined here.
package dispatcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/trilam/leah/internal/thinking/reasoner"
)

// LLMDimReporter is the optional rich-info side of Reasoner. Implementations
// (notably *reasoner.Reasoner) expose the LLM-dim slice of the most recent
// call so the caller can stamp it on the audit row. Wrappers that do not
// surface LLM-dim data (prebakedReasoner) simply do not implement this.
type LLMDimReporter interface {
	LastCallInfo() reasoner.CallInfo
}

// Reasoner is the LLM surface dispatcher uses. Defined here so Ship/SelfBuild
// can swap in a prebakedReasoner wrapper (selfbuild.go) without dragging in
// the reasoner package.
type Reasoner interface {
	Ask(ctx context.Context, user string) (string, error)
}

func argsHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}
