// Package markets is the Alpha Vantage GLOBAL_QUOTE widget adapter — wave 1
// of the Phase 2 widget-adapter rollout (see docs/superpowers/specs/
// 2026-06-21-leah-macos-native-ui-design.md §10.6). The 5 req/min default
// matches Alpha Vantage's free-tier ceiling; lift only when the operator has
// paid for a higher key.
package markets

// Quote is one rendered row in the markets widget. All strings — formatting
// happens at the source so the widget never re-rounds upstream prices.
type Quote struct {
	Symbol    string `json:"symbol"`
	Price     string `json:"price"`
	Change    string `json:"change"`
	ChangePct string `json:"change_pct"`
	AsOf      string `json:"as_of"`
}

// Options configures the adapter. APIKey is required; Endpoint defaults to
// Alpha Vantage prod; RateLimit defaults to 5/min (free-tier ceiling).
type Options struct {
	Endpoint  string
	APIKey    string
	RateLimit int
}
