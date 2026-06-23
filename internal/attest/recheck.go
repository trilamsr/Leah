package attest

import "time"

const (
	selfRecheck           = 24 * time.Hour
	artifactRecheck       = 7 * 24 * time.Hour
	pluginRecheck         = 60 * time.Minute
	revocationStale       = 7 * 24 * time.Hour
	revocationHTTPTimeout = 10 * time.Second
	subscriberBuffer      = 8
)
