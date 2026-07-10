// Package gmail is Leah's adapter for the Gmail v1 API. It exposes a narrow
// Client surface (ListUnread, MarkRead, Send) and routes every secret-touching
// RPC through the operator-attestation gate (Attestor) before loading the
// OAuth token. Daemon integration (morning-brief enrichment, send-on-demand)
// composes this adapter via internal/dispatcher; the constructor performs no
// I/O so wiring stays deterministic.
package gmail
