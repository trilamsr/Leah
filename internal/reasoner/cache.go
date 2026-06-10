package reasoner

import "github.com/anthropics/anthropic-sdk-go"

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
