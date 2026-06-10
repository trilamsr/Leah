// Package slack is Leah's adapter for the Slack Web API. It exposes a narrow
// surface (ListChannels, PostMessage, GetThread, Search) and routes every
// secret-touching RPC through the operator-attestation gate before loading
// the bearer token. Daemon integration (morning-brief mentions, dispatcher
// channel posts) composes this adapter via internal/dispatcher; the
// constructor performs no I/O so wiring stays deterministic.
package slack
