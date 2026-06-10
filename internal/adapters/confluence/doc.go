// Package confluence is Leah's adapter for Atlassian Confluence Cloud. It
// exposes a narrow Client surface (ListRecentPages, GetPage, SearchCQL) and
// routes every secret-touching RPC through the operator-attestation gate
// before loading the bearer. Daemon integration (morning-brief enrichment)
// composes this adapter via internal/dispatcher; the constructor performs no
// I/O so wiring stays deterministic.
package confluence
