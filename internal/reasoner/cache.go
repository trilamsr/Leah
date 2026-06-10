package reasoner

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/anthropics/anthropic-sdk-go"
)

// PromptSHA returns the first 16 hex chars of SHA256(text). Stable
// across processes; used as the audit `prompt_sha` label so an audit
// replay can resolve back to the exact system prompt bytes.
func PromptSHA(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:8])
}

const (
	CacheableThresholdTokens = 1024 // Sonnet drops cache_control below this; 25% write surcharge sunk.
	charsPerToken            = 4    // Anthropic rule-of-thumb; one-sided so under-count is safe.
)

func MeasurePromptTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return (len(text) + charsPerToken - 1) / charsPerToken
}

func ShouldCachePrompt(text string) bool {
	return MeasurePromptTokens(text) >= CacheableThresholdTokens
}

func buildSystemBlock(text string) anthropic.TextBlockParam {
	blk := anthropic.TextBlockParam{Text: text}
	if ShouldCachePrompt(text) {
		blk.CacheControl = anthropic.NewCacheControlEphemeralParam()
	}
	return blk
}
